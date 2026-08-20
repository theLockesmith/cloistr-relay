package wot

import "testing"

// An AUTHENTICATED author is not an unknown.
//
// PoW prices ANONYMOUS spam. It buys nothing against someone who has already
// proven they hold the key, and this relay runs AUTH_POLICY=auth-write so every
// write is authenticated regardless.
//
// Without the exemption, first-party Cloistr data was gated as though it were
// spam from a stranger: stash publishes file and folder metadata as kind
// 30078/30079, the relay answered
//     "pow: low trust requires proof of work (got 0, need 8)"
// and stash hung forever on "Connecting to your account…". Every user not
// hand-listed in ALLOWED_PUBKEYS was locked out of the product — on a
// multi-user service that is everyone except the operator.
func TestAuthorIsAuthenticated(t *testing.T) {
	tests := []struct {
		name   string
		authed string
		author string
		want   bool
	}{
		{"authenticated as the author", "abc123", "abc123", true},
		{"not authenticated at all", "", "abc123", false},
		{
			// The exemption is tied to the AUTHOR, not to the mere presence of
			// an authenticated connection. Otherwise one account becomes an
			// open relay for unmined events on behalf of anyone.
			name: "authenticated as someone else", authed: "attacker", author: "victim", want: false,
		},
		{"empty author with empty auth is not a match", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := authorIsAuthenticated(tt.authed, tt.author); got != tt.want {
				t.Errorf("authorIsAuthenticated(%q, %q) = %v, want %v",
					tt.authed, tt.author, got, tt.want)
			}
		})
	}
}
