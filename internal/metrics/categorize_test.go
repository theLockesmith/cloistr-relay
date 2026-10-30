package metrics

import "testing"

func TestCategorizeRejection(t *testing.T) {
	cases := []struct{ msg, want string }{
		// Filter-size limits get their own reason instead of hiding under "error".
		{"error: too many authors in filter (max 100)", "too-many-authors"},
		{"error: too many ids in filter (max 500)", "too-many-ids"},
		{"error: too many kinds in filter (max 150)", "too-many-kinds"},
		{"error: too many widgets", "too-many-widgets"},
		// Everything else keeps the prefix-before-colon behaviour.
		{"error: could not check deletion status (timeout), refusing", "error"},
		{"error: internal server error", "error"},
		{"auth-required: authentication required to publish events", "auth-required"},
		{"rate-limited: too many events, slow down", "rate-limited"},
		{"blocked: pubkey banned", "blocked"},
		{"short", "short"},
	}
	for _, c := range cases {
		if got := categorizeRejection(c.msg); got != c.want {
			t.Errorf("categorizeRejection(%q) = %q, want %q", c.msg, got, c.want)
		}
	}
}
