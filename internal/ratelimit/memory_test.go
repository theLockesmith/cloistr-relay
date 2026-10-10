package ratelimit

import (
	"fmt"
	"net/http/httptest"
	"testing"
	"time"
)

// One real client (X-Real-IP, set by the edge) rotating a forged
// X-Forwarded-For on every request must still share one budget. khatru's
// policies.ConnectionRateLimiter keyed on the leftmost public XFF entry, so
// each forged value got a fresh budget and this test accepted all 50.
func TestConnectionRateLimiterIgnoresForgedXFF(t *testing.T) {
	reject := ConnectionRateLimiter(1, time.Hour, 5)

	accepted := 0
	for i := 0; i < 50; i++ {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = "10.128.0.5:41000"
		r.Header.Set("X-Real-IP", "198.51.100.7")
		r.Header.Set("X-Forwarded-For", fmt.Sprintf("203.0.113.%d, 198.51.100.7", i+1))
		if !reject(r) {
			accepted++
		}
	}
	if accepted != 5 {
		t.Fatalf("accepted %d of 50 connections with rotating forged XFF, want 5", accepted)
	}
}

func TestConnectionRateLimiterSeparatesRealClients(t *testing.T) {
	reject := ConnectionRateLimiter(1, time.Hour, 2)
	for _, ip := range []string{"198.51.100.7", "198.51.100.8"} {
		for i := 0; i < 2; i++ {
			r := httptest.NewRequest("GET", "/", nil)
			r.Header.Set("X-Real-IP", ip)
			if reject(r) {
				t.Fatalf("client %s rejected on connection %d; budgets must be per real IP", ip, i+1)
			}
		}
	}
}
