package main

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/fiatjaf/khatru"
	"github.com/nbd-wtf/go-nostr"

	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/config"
	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/membership"
)

// An event refused only for "auth-required" must not cost a database lookup.
// Before 2026-10-29 the tombstone check (and five NIP-86 management queries)
// ran ahead of auth, so the ~31,000 unauthenticated writes a day each paid six
// Postgres round trips just to be refused; production logs showed those
// refusals taking up to 2.6s. Once the client authenticates, the tombstone
// check must still run.
func TestWriteGates_AuthBeforeDatabaseLookups(t *testing.T) {
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	cfg.AuthPolicy = "auth-write"

	var lookups atomic.Int32
	tombstone := func(ctx context.Context, id string) (bool, error) {
		lookups.Add(1)
		return false, nil
	}

	// Configure the relay completely before it serves anything: its fields are
	// read from connection goroutines.
	relay := khatru.NewRelay()
	relay.StoreEvent = append(relay.StoreEvent, func(context.Context, *nostr.Event) error { return nil })
	registerWriteGates(relay, cfg, parseAuthConfig(cfg), tombstone, nil, nil)
	srv := httptest.NewUnstartedServer(relay)
	relay.ServiceURL = "ws://" + srv.Listener.Addr().String()
	srv.Start()
	defer srv.Close()
	url := relay.ServiceURL

	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))

	sk := nostr.GeneratePrivateKey()
	pk, _ := nostr.GetPublicKey(sk)
	signed := func(kind int, tags nostr.Tags, content string) nostr.Event {
		evt := nostr.Event{PubKey: pk, CreatedAt: nostr.Now(), Kind: kind, Tags: tags, Content: content}
		if err := evt.Sign(sk); err != nil {
			t.Fatal(err)
		}
		return evt
	}
	send := func(v ...any) {
		if err := conn.WriteJSON(v); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	// next returns the next OK, remembering any AUTH challenge seen on the way
	// (khatru sends the challenge just before the auth-required OK).
	var challenge string
	nextOK := func() []json.RawMessage {
		for {
			var msg []json.RawMessage
			if err := conn.ReadJSON(&msg); err != nil {
				t.Fatalf("waiting for OK: %v", err)
			}
			var typ string
			_ = json.Unmarshal(msg[0], &typ)
			switch typ {
			case "OK":
				return msg
			case "AUTH":
				_ = json.Unmarshal(msg[1], &challenge)
			}
		}
	}
	okResult := func() (bool, string) {
		msg := nextOK()
		var ok bool
		var reason string
		_ = json.Unmarshal(msg[2], &ok)
		_ = json.Unmarshal(msg[3], &reason)
		return ok, reason
	}

	send("EVENT", signed(1, nostr.Tags{}, "before auth"))
	if ok, reason := okResult(); ok || !strings.HasPrefix(reason, "auth-required") {
		t.Fatalf("unauthenticated publish: ok=%v reason=%q, want auth-required", ok, reason)
	}
	if n := lookups.Load(); n != 0 {
		t.Fatalf("an event refused for auth-required made %d tombstone lookup(s); want 0 -- auth must run before database-backed checks", n)
	}

	if challenge == "" {
		t.Fatal("relay refused for auth-required without sending an AUTH challenge")
	}
	send("AUTH", signed(22242, nostr.Tags{{"relay", url}, {"challenge", challenge}}, ""))
	if ok, reason := okResult(); !ok {
		t.Fatalf("auth refused: %q", reason)
	}

	send("EVENT", signed(1, nostr.Tags{}, "after auth"))
	if ok, reason := okResult(); !ok {
		t.Fatalf("authenticated publish refused: %q", reason)
	}
	if n := lookups.Load(); n != 1 {
		t.Fatalf("authenticated publish made %d tombstone lookup(s); want 1 -- the deletion check must still run", n)
	}
}

type hungMembers struct{ release chan struct{} }

func (h hungMembers) GetMember(context.Context, string) (*membership.Member, error) {
	<-h.release // a dead lib/pq connection: ignores its context
	return nil, nil
}

// The paid-tier check runs inside the WoT gate on the event path. A lookup on a
// hung connection must answer "not paid" at the deadline, not hold the event.
func TestPaidMemberChecker_HungLookupReturnsWithinDeadline(t *testing.T) {
	old := paidLookupTimeout
	paidLookupTimeout = 100 * time.Millisecond
	t.Cleanup(func() { paidLookupTimeout = old })

	release := make(chan struct{})
	defer close(release)
	isPaid := newPaidMemberChecker(hungMembers{release})

	done := make(chan bool, 1)
	go func() { done <- isPaid("c0ffee") }()
	select {
	case paid := <-done:
		if paid {
			t.Error("a timed-out lookup must not grant paid status")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("paid-member check still blocked after 2s on a hung lookup")
	}
}
