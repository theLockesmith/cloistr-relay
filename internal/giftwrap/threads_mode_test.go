package giftwrap

import (
	"context"
	"testing"

	"github.com/fiatjaf/khatru"
	"github.com/nbd-wtf/go-nostr"
)

// The bucketed-thread design publishes wraps with NO p tag. Under the NIP-59
// recommendation such an event is stored and served to nobody, so these tests
// pin the wiring that decides which mode is in force.
//
// They assert ADMISSION as well as refusal on purpose. A relay that refused every
// 1059 query would satisfy a refusal-only suite while making threads impossible,
// which is the failure mode this package is being changed to avoid.

func TestThreadsMode_RegistersNeitherFilter(t *testing.T) {
	r := khatru.NewRelay()
	before, beforeOW := len(r.RejectFilter), len(r.OverwriteFilter)

	RegisterHandlers(r, &Config{Enabled: true, RequireAuthForGiftWrap: false})

	if got := len(r.RejectFilter) - before; got != 0 {
		t.Errorf("threads mode registered %d reject filters, want 0", got)
	}
	if got := len(r.OverwriteFilter) - beforeOW; got != 0 {
		t.Errorf("threads mode registered %d overwrite filters, want 0; the overwrite "+
			"handler rewrites #p to the reader and silently empties every bucket query", got)
	}
}

func TestGatedMode_RegistersBothFilters(t *testing.T) {
	r := khatru.NewRelay()
	before, beforeOW := len(r.RejectFilter), len(r.OverwriteFilter)

	RegisterHandlers(r, &Config{Enabled: true, RequireAuthForGiftWrap: true})

	if got := len(r.RejectFilter) - before; got != 1 {
		t.Errorf("gated mode registered %d reject filters, want 1", got)
	}
	if got := len(r.OverwriteFilter) - beforeOW; got != 1 {
		t.Errorf("gated mode registered %d overwrite filters, want 1", got)
	}
}

// NOT UNIT TESTED, deliberately, and recorded so nobody thinks it was forgotten:
// the case that actually breaks threads is the gated overwrite handler rewriting an
// AUTHENTICATED reader's #p filter to their own pubkey, which empties every bucket
// query. khatru stores the authenticated pubkey on a connection reached through an
// unexported context key, so a test outside that package cannot construct one. It was
// proved against production instead: two wraps sharing a bucket tag, one p-tagged to
// the reader and one not, and a bucket query returned only the p-tagged one while the
// other was invisible even to the key that published it.

// ADMISSION, the half a refusal-only suite would miss: in threads mode a bucket
// query reaches the store unmodified and an anonymous reader is not refused.
func TestThreadsMode_BucketQueryPassesThroughUntouched(t *testing.T) {
	r := khatru.NewRelay()
	RegisterHandlers(r, &Config{Enabled: true, RequireAuthForGiftWrap: false})

	f := nostr.Filter{
		Kinds: []int{KindGiftWrap},
		Tags:  nostr.TagMap{"h": []string{"bucket-7f"}},
	}
	anon := context.Background()

	for i, reject := range r.RejectFilter {
		if blocked, msg := reject(anon, f); blocked {
			t.Fatalf("reject filter %d refused an anonymous bucket query: %s", i, msg)
		}
	}
	for _, overwrite := range r.OverwriteFilter {
		overwrite(anon, &f)
	}
	if _, injected := f.Tags["p"]; injected {
		t.Error("a #p filter was injected in threads mode; bucket queries would return nothing")
	}
	if f.Tags["h"][0] != "bucket-7f" {
		t.Errorf("bucket tag altered: %v", f.Tags)
	}
}

// Control: the gated mode still refuses an anonymous reader, so mode one is intact.
func TestGatedMode_StillRefusesAnonymous(t *testing.T) {
	h := NewHandler(&Config{Enabled: true, RequireAuthForGiftWrap: true})
	blocked, msg := h.RejectGiftWrapFilter()(context.Background(), nostr.Filter{Kinds: []int{KindGiftWrap}})
	if !blocked {
		t.Error("gated mode admitted an anonymous gift wrap query")
	}
	if msg == "" {
		t.Error("refusal carried no reason")
	}
}
