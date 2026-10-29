package groups

import (
	"context"
	"testing"
	"time"

	"github.com/fiatjaf/eventstore/slicestore"
	"github.com/fiatjaf/khatru"
	"github.com/fiatjaf/relay29"
	"github.com/nbd-wtf/go-nostr"
)

// These tests attach the real relay29 library to a real khatru relay, the way
// cmd/relay/main.go does, and check the shared relay is unchanged for everything
// that is not NIP-29 traffic. relay29 is written for a groups-only relay; wired
// unscoped it refuses every event without an h tag and every plain REQ.

type authKey struct{}

func authedAs(pubkey string) context.Context {
	return context.WithValue(context.Background(), authKey{}, pubkey)
}

func testGetAuthed(ctx context.Context) string {
	pk, _ := ctx.Value(authKey{}).(string)
	return pk
}

type adder struct{ *khatru.Relay }

func (a adder) BroadcastEvent(evt *nostr.Event) { a.Relay.BroadcastEvent(evt) }

type user struct{ sk, pk string }

func newUser(t *testing.T) user {
	t.Helper()
	sk := nostr.GeneratePrivateKey()
	pk, err := nostr.GetPublicKey(sk)
	if err != nil {
		t.Fatal(err)
	}
	return user{sk, pk}
}

func (u user) sign(t *testing.T, kind int, tags nostr.Tags, content string) *nostr.Event {
	t.Helper()
	evt := &nostr.Event{Kind: kind, Tags: tags, Content: content, CreatedAt: nostr.Now()}
	if err := evt.Sign(u.sk); err != nil {
		t.Fatal(err)
	}
	return evt
}

type harness struct {
	r     *khatru.Relay
	st    *relay29.State
	store *slicestore.SliceStore
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	store := &slicestore.SliceStore{}
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}

	r := khatru.NewRelay()
	r.StoreEvent = append(r.StoreEvent, store.SaveEvent)
	r.QueryEvents = append(r.QueryEvents, store.QueryEvents)
	r.CountEvents = append(r.CountEvents, store.CountEvents)
	r.DeleteEvent = append(r.DeleteEvent, store.DeleteEvent)
	r.ReplaceEvent = append(r.ReplaceEvent, store.ReplaceEvent)

	st := relay29.New(relay29.Options{
		Domain:                  "groups.test",
		DB:                      store,
		SecretKey:               nostr.GeneratePrivateKey(),
		DefaultRoles:            Roles,
		GroupCreatorDefaultRole: AdminRole,
	})
	st.AllowPrivateGroups = true
	st.AllowAction = AllowAction
	st.Relay = adder{r}
	st.GetAuthed = testGetAuthed

	Wire(r, st, WireConfig{AllowPublicCreation: true})
	return &harness{r: r, st: st, store: store}
}

func (h *harness) createGroup(t *testing.T, id string, creator user, private bool) {
	t.Helper()
	if err := h.st.CreateGroup(context.Background(), id, creator.pk, relay29.EditMetadata{PrivateValue: &private}); err != nil {
		t.Fatalf("create group %s: %v", id, err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		if g, _ := h.st.Groups.Load(id); g != nil && g.Private == private {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("group %s never appeared in relay29 state", id)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// publish runs the event through the relay's write pipeline (RejectEvent,
// store, OnEventSaved), recovering a panic so a crashing hook fails the test
// instead of the run.
func (h *harness) publish(ctx context.Context, evt *nostr.Event) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = panicError{p}
		}
	}()
	_, err = h.r.AddEvent(ctx, evt)
	return err
}

type panicError struct{ v any }

func (p panicError) Error() string { return "panic in write pipeline" }

func (h *harness) rejected(ctx context.Context, f nostr.Filter) (bool, string) {
	for _, reject := range h.r.RejectFilter {
		if blocked, msg := reject(ctx, f); blocked {
			return true, msg
		}
	}
	return false, ""
}

// query runs every QueryEvents handler, as khatru does for a REQ, and returns
// how many times each event id came back.
func (h *harness) query(t *testing.T, ctx context.Context, f nostr.Filter) map[string]int {
	t.Helper()
	seen := map[string]int{}
	for _, q := range h.r.QueryEvents {
		ch, err := q(ctx, f)
		if err != nil {
			t.Fatalf("query handler error: %v", err)
		}
		if ch == nil {
			continue
		}
		for evt := range ch {
			seen[evt.ID]++
		}
	}
	return seen
}

func (h *harness) count(t *testing.T, ctx context.Context, f nostr.Filter) int64 {
	t.Helper()
	var n int64
	for _, c := range h.r.CountEvents {
		got, err := c(ctx, f)
		if err != nil {
			t.Fatalf("count handler error: %v", err)
		}
		n += got
	}
	return n
}

func ordinaryEvents(t *testing.T, u, other user) []*nostr.Event {
	return []*nostr.Event{
		u.sign(t, 0, nil, `{"name":"alice"}`),
		u.sign(t, 1, nostr.Tags{{"p", other.pk}}, "hello"),
		u.sign(t, 3, nostr.Tags{{"p", other.pk}}, ""),
		u.sign(t, 30078, nostr.Tags{{"d", "stash:root"}}, "ciphertext"),
		u.sign(t, 24133, nostr.Tags{{"p", other.pk}}, "nip46"),
		// A gift-wrap thread bucket: h-tagged, but not a NIP-29 group.
		u.sign(t, 1059, nostr.Tags{{"h", "bucket-7f"}}, "wrapped"),
		// Plain chat (kind 9) with no group.
		u.sign(t, 9, nil, "ungrouped chat"),
	}
}

func TestWire_OrdinaryEventsAccepted(t *testing.T) {
	h := newHarness(t)
	alice, bob := newUser(t), newUser(t)
	for _, evt := range ordinaryEvents(t, alice, bob) {
		if err := h.publish(authedAs(alice.pk), evt); err != nil {
			t.Errorf("kind %d (tags %v) refused with groups on: %v", evt.Kind, evt.Tags, err)
		}
	}
}

func TestWire_OrdinaryREQsUntouched(t *testing.T) {
	h := newHarness(t)
	alice, bob := newUser(t), newUser(t)
	evts := ordinaryEvents(t, alice, bob)
	for _, evt := range evts {
		if err := h.store.SaveEvent(context.Background(), evt); err != nil {
			t.Fatal(err)
		}
	}

	filters := map[string]nostr.Filter{
		"authors+kinds": {Authors: []string{alice.pk}, Kinds: []int{0, 1, 3}},
		"#p":            {Tags: nostr.TagMap{"p": []string{bob.pk}}},
		"#e":            {Tags: nostr.TagMap{"e": []string{evts[1].ID}}},
		"ids":           {IDs: []string{evts[1].ID, evts[3].ID}},
		"#d (stash)":    {Kinds: []int{30078}, Tags: nostr.TagMap{"d": []string{"stash:root"}}},
		"bucket #h":     {Kinds: []int{1059}, Tags: nostr.TagMap{"h": []string{"bucket-7f"}}},
		"kind 9":        {Kinds: []int{9}},
	}

	for name, f := range filters {
		for _, ctx := range []context.Context{context.Background(), authedAs(bob.pk)} {
			if blocked, msg := h.rejected(ctx, f); blocked {
				t.Errorf("%s: REQ refused with groups on: %s", name, msg)
				continue
			}
			want := map[string]int{}
			ch, _ := h.store.QueryEvents(context.Background(), f)
			for evt := range ch {
				want[evt.ID] = 1
			}
			got := h.query(t, ctx, f)
			for id, n := range got {
				if n != 1 {
					t.Errorf("%s: event %s returned %d times, want once", name, id[:8], n)
				}
			}
			if len(got) != len(want) {
				t.Errorf("%s: got %d events, want %d (same as groups off)", name, len(got), len(want))
			}
			if c := h.count(t, ctx, f); c != int64(len(want)) {
				t.Errorf("%s: COUNT %d, want %d", name, c, len(want))
			}
		}
	}
}

func TestWire_GroupRulesStillApply(t *testing.T) {
	h := newHarness(t)
	alice, bob := newUser(t), newUser(t)
	h.createGroup(t, "g-closed", alice, false)

	cases := map[string]*nostr.Event{
		"group kind without h tag":     bob.sign(t, 9021, nil, "join?"),
		"moderation in unknown group":  bob.sign(t, 9000, nostr.Tags{{"h", "no-such-group"}, {"p", bob.pk}}, ""),
		"non-member moderates a group": bob.sign(t, 9000, nostr.Tags{{"h", "g-closed"}, {"p", bob.pk}}, ""),
		"client-signed group metadata": bob.sign(t, 39000, nostr.Tags{{"d", "g-closed"}}, ""),
	}
	for name, evt := range cases {
		if err := h.publish(authedAs(bob.pk), evt); err == nil {
			t.Errorf("%s: accepted, want refused by relay29", name)
		}
	}

	if err := h.publish(authedAs(alice.pk), alice.sign(t, 9, nostr.Tags{{"h", "g-closed"}}, "hi group")); err != nil {
		t.Errorf("member message to own group refused: %v", err)
	}
}

func TestWire_GroupMetadataServedOnceFromRelay(t *testing.T) {
	h := newHarness(t)
	alice := newUser(t)
	h.createGroup(t, "g-pub", alice, false)

	f := nostr.Filter{Kinds: []int{39000}, Tags: nostr.TagMap{"d": []string{"g-pub"}}}
	if blocked, msg := h.rejected(context.Background(), f); blocked {
		t.Fatalf("metadata REQ refused: %s", msg)
	}
	got := h.query(t, context.Background(), f)
	if len(got) != 1 {
		t.Fatalf("got %d metadata events, want exactly 1 (relay-generated)", len(got))
	}
}

func TestWire_PrivateGroupDoesNotLeak(t *testing.T) {
	h := newHarness(t)
	alice, bob := newUser(t), newUser(t)
	h.createGroup(t, "g-priv", alice, true)

	msg := alice.sign(t, 9, nostr.Tags{{"h", "g-priv"}}, "secret")
	if err := h.publish(authedAs(alice.pk), msg); err != nil {
		t.Fatalf("member message refused: %v", err)
	}

	anon, outsider, member := context.Background(), authedAs(bob.pk), authedAs(alice.pk)

	if blocked, _ := h.rejected(anon, nostr.Filter{Tags: nostr.TagMap{"h": []string{"g-priv"}}}); !blocked {
		t.Error("anonymous #h REQ on a private group was not refused")
	}
	if blocked, _ := h.rejected(outsider, nostr.Filter{Tags: nostr.TagMap{"h": []string{"g-priv"}}}); !blocked {
		t.Error("non-member #h REQ on a private group was not refused")
	}

	plain := map[string]nostr.Filter{
		"kinds":   {Kinds: []int{9}},
		"ids":     {IDs: []string{msg.ID}},
		"authors": {Authors: []string{alice.pk}},
	}
	for name, f := range plain {
		for who, ctx := range map[string]context.Context{"anonymous": anon, "non-member": outsider} {
			if got := h.query(t, ctx, f); got[msg.ID] != 0 {
				t.Errorf("%s REQ by %s returned the private message", name, who)
			}
			if c := h.count(t, ctx, f); c != 0 {
				t.Errorf("%s COUNT by %s = %d, want 0", name, who, c)
			}
		}
		got := h.query(t, member, f)
		if got[msg.ID] != 1 {
			t.Errorf("%s REQ by the member returned the message %d times, want 1", name, got[msg.ID])
		}
		// The member's COUNT matches what the member's REQ returns (the group's
		// creation event is also authored by the member, so this can exceed 1).
		if c := h.count(t, member, f); c != int64(len(got)) {
			t.Errorf("%s COUNT by the member = %d, want %d (what the REQ returned)", name, c, len(got))
		}
	}
}

func TestWire_PrivateGroupNotBroadcastToOutsiders(t *testing.T) {
	h := newHarness(t)
	alice, bob := newUser(t), newUser(t)
	h.createGroup(t, "g-priv", alice, true)
	h.createGroup(t, "g-pub", alice, false)

	priv := alice.sign(t, 9, nostr.Tags{{"h", "g-priv"}}, "secret")
	pub := alice.sign(t, 9, nostr.Tags{{"h", "g-pub"}}, "hello")
	plain := alice.sign(t, 1, nil, "note")

	prevented := func(pk string, evt *nostr.Event) bool {
		ws := &khatru.WebSocket{AuthedPublicKey: pk}
		for _, pb := range h.r.PreventBroadcast {
			if pb(ws, evt) {
				return true
			}
		}
		return false
	}

	if !prevented("", priv) || !prevented(bob.pk, priv) {
		t.Error("private group message would be broadcast to a non-member subscriber")
	}
	if prevented(alice.pk, priv) {
		t.Error("private group message withheld from its member")
	}
	if prevented("", pub) || prevented("", plain) {
		t.Error("public group message or ordinary note withheld from a subscriber")
	}
}
