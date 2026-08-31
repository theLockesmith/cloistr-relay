package arbiterclaim

import (
	"testing"

	"github.com/nbd-wtf/go-nostr"
)

func ev(kind int, tags nostr.Tags) *nostr.Event {
	return &nostr.Event{ID: "ffff", Kind: kind, PubKey: "aa", Tags: tags}
}

// Real 30078 traffic observed on relay.cloistr.xyz. None of this may be treated
// as a claim.
func TestIsClaim_RejectsRealNonClaimTraffic(t *testing.T) {
	cases := []struct {
		name string
		evt  *nostr.Event
	}{
		{"WoT settings", ev(30078, nostr.Tags{{"d", "cloistr-wot-settings"}})},
		{"HAVEN settings", ev(30078, nostr.Tags{{"d", "cloistr-haven-settings"}})},
		{"stash folder metadata", ev(30078, nostr.Tags{{"d", "folder-1787602143334-385c99eb"}})},
		{"cloistr docs", ev(30078, nostr.Tags{{"d", "doc-1787602143334-385c99eb"}, {"t", "doc"}, {"client", "cloistr"}})},
		{"cloistr sheets", ev(30078, nostr.Tags{{"d", "sheet-1787676215483-dedae12b"}, {"t", "sheet"}})},
		{"third-party Amethyst", ev(30078, nostr.Tags{{"d", "AmethystSettings"}, {"alt", "x"}})},
		{"root-key", ev(30078, nostr.Tags{{"d", "root-key"}, {"key", "x"}})},
		{"no d tag at all", ev(30078, nostr.Tags{{"t", "arbiter-claim"}})},
		{"different kind, claim-shaped d", ev(30079, nostr.Tags{{"d", DTagPrefix + "t1"}})},
		{"kind 1 note", ev(1, nostr.Tags{{"d", DTagPrefix + "t1"}})},
		{"nil event", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if IsClaim(tc.evt) {
				t.Errorf("IsClaim must be false for %s", tc.name)
			}
		})
	}
}

func TestIsClaim_AcceptsClaimNamespace(t *testing.T) {
	e := ev(30078, nostr.Tags{{"d", DTagPrefix + "task-42"}, {"t", TagMarker}})
	if !IsClaim(e) {
		t.Fatal("a d=arbiter:claim:* event on kind 30078 must be recognised as a claim")
	}
	if got := TaskIDOf(e); got != "task-42" {
		t.Errorf("TaskIDOf = %q, want %q", got, "task-42")
	}
}

// A claim that omits the marker tag must be REJECTED, not silently treated as
// non-claim traffic -- otherwise enforcement is bypassable by leaving a tag off.
func TestValidate_MalformedClaimsAreRejectedNotIgnored(t *testing.T) {
	cases := []struct {
		name string
		evt  *nostr.Event
	}{
		{"missing marker tag", ev(30078, nostr.Tags{{"d", DTagPrefix + "task-1"}})},
		{"empty task id", ev(30078, nostr.Tags{{"d", DTagPrefix}, {"t", TagMarker}})},
		{"whitespace task id", ev(30078, nostr.Tags{{"d", DTagPrefix + "   "}, {"t", TagMarker}})},
		{"wrong marker value", ev(30078, nostr.Tags{{"d", DTagPrefix + "task-1"}, {"t", "something-else"}})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !IsClaim(tc.evt) {
				t.Fatal("precondition: should still be in the claim namespace")
			}
			if _, err := Validate(tc.evt); err == nil {
				t.Error("Validate must reject a malformed claim, not accept it")
			}
		})
	}
}

// The tie-break is the lowest event id and must NOT depend on created_at, which
// is self-asserted.
func TestWinner_LowestEventIDWinsRegardlessOfClaimedTime(t *testing.T) {
	claims := []Claim{
		{TaskID: "t", EventID: "ccc", Claimant: "c", At: 1},          // earliest created_at
		{TaskID: "t", EventID: "aaa", Claimant: "a", At: 9999999999}, // latest created_at, lowest id
		{TaskID: "t", EventID: "bbb", Claimant: "b", At: 500},
	}
	w, ok := Winner(claims)
	if !ok {
		t.Fatal("Winner returned no winner for a non-empty set")
	}
	if w.EventID != "aaa" {
		t.Errorf("winner = %s, want aaa (lowest id); a created_at tie-break would have picked ccc", w.EventID)
	}
}

func TestWinner_IsDeterministicAndDoesNotMutateInput(t *testing.T) {
	in := []Claim{{EventID: "ccc"}, {EventID: "aaa"}, {EventID: "bbb"}}
	first, _ := Winner(in)
	second, _ := Winner(in)
	if first.EventID != second.EventID {
		t.Error("Winner must be deterministic")
	}
	if in[0].EventID != "ccc" {
		t.Errorf("Winner must not reorder its input; got %v", in)
	}
	if _, ok := Winner(nil); ok {
		t.Error("Winner on an empty set must report no winner")
	}
}

func TestBuildClaimEvent_IsRecognisedByItsOwnDiscriminator(t *testing.T) {
	e := BuildClaimEvent("task-7", 1000)
	e.ID = "deadbeef"
	if !IsClaim(e) {
		t.Fatal("BuildClaimEvent output must satisfy IsClaim")
	}
	c, err := Validate(e)
	if err != nil {
		t.Fatalf("BuildClaimEvent output must validate: %v", err)
	}
	if c.TaskID != "task-7" {
		t.Errorf("TaskID = %q, want task-7", c.TaskID)
	}
}
