package arbiterclaim

import (
	"context"
	"strings"
	"testing"

	"github.com/fiatjaf/khatru"
	"github.com/nbd-wtf/go-nostr"
)

func claimHandler(t *testing.T, dsn string) func(context.Context, *nostr.Event) (bool, string) {
	t.Helper()
	relay := khatru.NewRelay()
	before := len(relay.RejectEvent)
	RegisterHandlers(relay, storeWith(t, dsn))
	if len(relay.RejectEvent) != before+1 {
		t.Fatalf("RegisterHandlers must append exactly one RejectEvent handler; went from %d to %d", before, len(relay.RejectEvent))
	}
	return relay.RejectEvent[len(relay.RejectEvent)-1]
}

// THE TEST THAT PROTECTS STASH.
//
// Kind 30078 is shared: WoT settings, HAVEN settings, stash file and folder
// metadata, cloistr docs/sheets/slides and third-party clients all publish it.
// This handler sits in the write path of every one of them. It must reach its
// early return WITHOUT touching the database -- otherwise a claim feature adds a
// Postgres round trip to every settings write on the relay, and a bug in claim
// code fails stash rather than failing claims.
func TestHandler_NonClaimTrafficIsAcceptedWithoutTouchingTheDatabase(t *testing.T) {
	h := claimHandler(t, "mode=err") // any DB contact would ERROR, so a leak is loud
	resetDBCalls()

	traffic := []struct {
		name string
		evt  *nostr.Event
	}{
		{"WoT settings", ev(30078, nostr.Tags{{"d", "cloistr-wot-settings"}})},
		{"HAVEN settings", ev(30078, nostr.Tags{{"d", "cloistr-haven-settings"}})},
		{"stash folder metadata", ev(30078, nostr.Tags{{"d", "folder-1787602143334-385c99eb"}})},
		{"stash file metadata (30079)", ev(30079, nostr.Tags{{"d", "file-1787602143334-385c99eb"}})},
		{"cloistr doc", ev(30078, nostr.Tags{{"d", "doc-1787602143334-385c99eb"}, {"t", "doc"}})},
		{"cloistr sheet", ev(30078, nostr.Tags{{"d", "sheet-1787676215483-dedae12b"}, {"t", "sheet"}})},
		{"third-party Amethyst", ev(30078, nostr.Tags{{"d", "AmethystSettings"}})},
		{"root-key", ev(30078, nostr.Tags{{"d", "root-key"}, {"key", "x"}})},
		{"a plain note", ev(1, nostr.Tags{})},
		{"NIP-46 remote signing", ev(24133, nostr.Tags{})},
	}
	for _, tc := range traffic {
		reject, msg := h(context.Background(), tc.evt)
		if reject {
			t.Errorf("%s: handler REJECTED non-claim traffic (%q)", tc.name, msg)
		}
	}
	if n := dbCallCount(); n != 0 {
		t.Errorf("non-claim traffic made %d database call(s); it must make ZERO. queries: %v", n, dbCalls.queries)
	}
}

func TestHandler_UncontestedClaimIsAccepted(t *testing.T) {
	h := claimHandler(t, "mode=win")
	e := BuildClaimEvent("task-1", 100)
	e.ID = "aaa"
	if reject, msg := h(context.Background(), e); reject {
		t.Errorf("an uncontested claim must be accepted, got reject with %q", msg)
	}
}

// THE LOAD-BEARING REJECTION TEST. A handler that accepts everything passes every
// test that only checks the happy path.
func TestHandler_SecondClaimForSameTaskIsRejected(t *testing.T) {
	h := claimHandler(t, "mode=lose&holder=aaa")
	e := BuildClaimEvent("task-1", 100)
	e.ID = "zzz"
	reject, msg := h(context.Background(), e)
	if !reject {
		t.Fatal("a second claim for an already-held task MUST be rejected")
	}
	if !strings.HasPrefix(msg, "conflict:") {
		t.Errorf("rejection must be marked as a conflict so a client can tell it from a network failure; got %q", msg)
	}
	if !strings.Contains(msg, "aaa") {
		t.Errorf("rejection must carry the winning event id (the fencing token); got %q", msg)
	}
}

// Losing and being unable to reach the store must not look the same.
func TestHandler_StoreOutageIsDistinguishableFromLosing(t *testing.T) {
	lost, lostMsg := claimHandler(t, "mode=lose&holder=aaa")(context.Background(), claimEvt("task-1", "zzz"))
	down, downMsg := claimHandler(t, "mode=err")(context.Background(), claimEvt("task-1", "zzz"))
	if !lost || !down {
		t.Fatal("both a lost race and a store outage must reject")
	}
	if lostMsg == downMsg {
		t.Fatal("a lost race and a store outage must not produce identical messages")
	}
	if !strings.HasPrefix(downMsg, "error:") {
		t.Errorf("store outage should be marked as an error, got %q", downMsg)
	}
}

// A malformed claim must be rejected, never waved through as non-claim traffic --
// otherwise enforcement is bypassable by omitting the marker tag.
func TestHandler_MalformedClaimIsRejectedNotBypassed(t *testing.T) {
	h := claimHandler(t, "mode=win")
	noMarker := ev(30078, nostr.Tags{{"d", DTagPrefix + "task-1"}})
	reject, msg := h(context.Background(), noMarker)
	if !reject {
		t.Fatal("a claim missing the marker tag must be REJECTED, not accepted as ordinary traffic")
	}
	if !strings.HasPrefix(msg, "invalid:") {
		t.Errorf("malformed claim should be marked invalid, got %q", msg)
	}
}

func claimEvt(task, id string) *nostr.Event {
	e := BuildClaimEvent(task, 100)
	e.ID = id
	return e
}
