package wot

import (
	"context"
	"strings"
	"testing"

	"github.com/nbd-wtf/go-nostr"
)

// Collaborative editors publish one sync event (kind 25078) per keystroke:
// measured against the real Docs stack (TipTap + y-prosemirror), 120
// keystrokes produced exactly 120 Y.Doc updates. With one shared per-pubkey
// bucket, a brand-new (unknown-trust, 5/s) user typing at an ordinary pace
// drained it, and the document's snapshot save (kind 30078) landing in that
// window was refused "rate-limited: trust level unknown allows 5 events/sec".
// R&D measured 1 in 5 new users' first Docs save failing this way.

const collabTestPubkey = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

func newCollabTestHandler(collabRate int) *Handler {
	// No store and no trust roots: every pubkey resolves to unknown.
	return NewHandler(nil, nil, &Config{
		Enabled:               true,
		Policies:              DefaultPolicies(),
		CollabKinds:           []int{25078, 25079},
		CollabEventsPerSecond: collabRate,
	})
}

func publish(h *Handler, kind int) (bool, string) {
	return h.RejectEventByTrust()(context.Background(), &nostr.Event{PubKey: collabTestPubkey, Kind: kind})
}

func TestCollabTypingDoesNotStarveSnapshotSave(t *testing.T) {
	h := newCollabTestHandler(20)

	// A burst of typing well past the unknown general limit (5/s).
	for i := 0; i < 15; i++ {
		if rejected, msg := publish(h, 25078); rejected {
			t.Fatalf("keystroke %d rejected: %s", i, msg)
		}
	}

	// The snapshot save still has its whole general budget.
	if rejected, msg := publish(h, 30078); rejected {
		t.Fatalf("snapshot save rejected after typing: %s", msg)
	}
}

func TestCollabBucketStillCapsFlood(t *testing.T) {
	h := newCollabTestHandler(20)

	accepted := 0
	var lastMsg string
	for i := 0; i < 200; i++ {
		rejected, msg := publish(h, 25078)
		if !rejected {
			accepted++
		} else {
			lastMsg = msg
		}
	}
	// Bucket starts at one second's worth (20) and never holds more than the
	// 2x burst (40), so however slow the runner, 200 back-to-back cannot pass.
	if accepted < 20 || accepted > 40 {
		t.Fatalf("flood of 200 accepted %d, want 20..40", accepted)
	}
	if !strings.Contains(lastMsg, "rate-limited: trust level unknown allows 20 collaboration events/sec") {
		t.Fatalf("unexpected rejection message: %q", lastMsg)
	}
}

func TestGeneralBucketUnchangedForNonCollabKinds(t *testing.T) {
	h := newCollabTestHandler(20)

	accepted := 0
	for i := 0; i < 50; i++ {
		if rejected, _ := publish(h, 1); !rejected {
			accepted++
		}
	}
	// Unknown trust: 5/s, bucket starts at 5.
	if accepted < 5 || accepted > 10 {
		t.Fatalf("flood of kind 1 accepted %d, want ~5", accepted)
	}
}

func TestCollabFloodDoesNotDrainGeneralBucket(t *testing.T) {
	h := newCollabTestHandler(20)
	for i := 0; i < 200; i++ {
		publish(h, 25078)
	}
	if rejected, msg := publish(h, 30078); rejected {
		t.Fatalf("snapshot save rejected after collab flood: %s", msg)
	}
}

func TestCollabRateZeroSharesGeneralBucket(t *testing.T) {
	h := newCollabTestHandler(0)

	for i := 0; i < 15; i++ {
		publish(h, 25078)
	}
	rejected, msg := publish(h, 30078)
	if !rejected || !strings.Contains(msg, "allows 5 events/sec") {
		t.Fatalf("with collab bucket disabled, typing should drain the shared bucket; got rejected=%v msg=%q", rejected, msg)
	}
}

func TestCollabRateIsFloorNotCeiling(t *testing.T) {
	// A level whose general rate exceeds the collab floor keeps its own rate.
	h := newCollabTestHandler(20)
	policy := h.policies[TrustLevelUnknown]
	policy.EventsPerSecond = 50
	h.policies[TrustLevelUnknown] = policy

	accepted := 0
	for i := 0; i < 200; i++ {
		if rejected, _ := publish(h, 25078); !rejected {
			accepted++
		}
	}
	if accepted < 50 {
		t.Fatalf("collab bucket at a 50/s level accepted %d, want >= 50", accepted)
	}
}

func TestCollabUnlimitedLevelStaysUnlimited(t *testing.T) {
	// EventsPerSecond == 0 means unlimited; the collab floor must not turn
	// that into a 20/s cap.
	h := newCollabTestHandler(20)
	policy := h.policies[TrustLevelUnknown]
	policy.EventsPerSecond = 0
	h.policies[TrustLevelUnknown] = policy

	for i := 0; i < 200; i++ {
		if rejected, msg := publish(h, 25078); rejected {
			t.Fatalf("unlimited level rejected collab event %d: %s", i, msg)
		}
	}
}
