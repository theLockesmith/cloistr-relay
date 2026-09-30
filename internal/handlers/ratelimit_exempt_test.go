package handlers

import (
	"context"
	"testing"

	"github.com/nbd-wtf/go-nostr"
)

type stubExemptChecker struct {
	exempt map[string]bool
}

func (s *stubExemptChecker) IsRateLimitExempt(pubkey string) bool {
	return s.exempt[pubkey]
}

func TestRateLimitExemptChecker_Interface(t *testing.T) {
	checker := &stubExemptChecker{exempt: map[string]bool{
		"aaaa": true,
	}}

	var c RateLimitExemptChecker = checker
	if !c.IsRateLimitExempt("aaaa") {
		t.Error("expected aaaa to be exempt")
	}
	if c.IsRateLimitExempt("bbbb") {
		t.Error("expected bbbb to NOT be exempt")
	}
}

func TestBuildRateLimitRejecter_StaticExemptPubkey(t *testing.T) {
	exempt := map[string]bool{"static_pk": true}
	kinds := map[int]bool{}

	rejecter := buildRateLimitRejecter(exempt, kinds, nil, nil)

	event := &nostr.Event{PubKey: "static_pk", Kind: 1}
	reject, _ := rejecter(context.Background(), event)
	if reject {
		t.Error("static exempt pubkey should not be rejected")
	}
}

func TestBuildRateLimitRejecter_DynamicExemptPubkey(t *testing.T) {
	exempt := map[string]bool{}
	kinds := map[int]bool{}
	checker := &stubExemptChecker{exempt: map[string]bool{"dynamic_pk": true}}

	rejecter := buildRateLimitRejecter(exempt, kinds, checker, nil)

	event := &nostr.Event{PubKey: "dynamic_pk", Kind: 1}
	reject, _ := rejecter(context.Background(), event)
	if reject {
		t.Error("dynamically exempt pubkey should not be rejected")
	}
}

func TestBuildRateLimitRejecter_ExemptKind(t *testing.T) {
	exempt := map[string]bool{}
	kinds := map[int]bool{24133: true}

	rejecter := buildRateLimitRejecter(exempt, kinds, nil, nil)

	event := &nostr.Event{PubKey: "some_pk", Kind: 24133}
	reject, _ := rejecter(context.Background(), event)
	if reject {
		t.Error("exempt kind should not be rejected")
	}
}

func TestBuildRateLimitRejecter_NonExemptHitsLimiter(t *testing.T) {
	exempt := map[string]bool{}
	kinds := map[int]bool{}
	called := false
	fakeLimiter := func(ctx context.Context, event *nostr.Event) (bool, string) {
		called = true
		return true, "rate-limited"
	}

	rejecter := buildRateLimitRejecter(exempt, kinds, nil, fakeLimiter)

	event := &nostr.Event{PubKey: "not_exempt", Kind: 1}
	reject, msg := rejecter(context.Background(), event)
	if !called {
		t.Error("expected base limiter to be called for non-exempt pubkey")
	}
	if !reject {
		t.Error("expected rejection from base limiter")
	}
	if msg != "rate-limited" {
		t.Errorf("expected 'rate-limited', got %q", msg)
	}
}
