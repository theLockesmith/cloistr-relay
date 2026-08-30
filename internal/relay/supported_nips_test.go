package relay

import (
	"testing"

	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/config"
)

func hasNIP(nips []any, want int) bool {
	for _, n := range nips {
		if v, ok := n.(int); ok && v == want {
			return true
		}
	}
	return false
}

// NIP-11 must not claim NIP-29 unless relay29 will actually initialise. The
// conditions here mirror the guard in cmd/relay/main.go: groups enabled AND a
// secret key present. Anything else and a client checking supported_nips before
// publishing a kind:9007 is being misled.
func TestSupportedNIPs_Advertises29OnlyWhenGroupsReallyRun(t *testing.T) {
	tests := []struct {
		name   string
		cfg    *config.Config
		want29 bool
	}{
		{
			name:   "groups disabled (production default)",
			cfg:    &config.Config{GroupsEnabled: false},
			want29: false,
		},
		{
			name:   "groups enabled but no secret key -- main.go skips init, so do not advertise",
			cfg:    &config.Config{GroupsEnabled: true, GroupsSecretKey: ""},
			want29: false,
		},
		{
			name:   "groups enabled with secret key -- relay29 initialises",
			cfg:    &config.Config{GroupsEnabled: true, GroupsSecretKey: "aa" + "00000000000000000000000000000000000000000000000000000000000000"},
			want29: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := supportedNIPs(tc.cfg)
			if hasNIP(got, 29) != tc.want29 {
				t.Errorf("NIP-29 advertised = %v, want %v (list: %v)", !tc.want29, tc.want29, got)
			}
		})
	}
}

// Gating 29 must not disturb the NIPs the relay genuinely serves regardless.
func TestSupportedNIPs_OtherNIPsUnaffectedByGroupsFlag(t *testing.T) {
	always := []int{1, 9, 11, 13, 17, 22, 33, 40, 42, 45, 46, 50, 57, 59, 66, 70, 77, 86, 94}
	for _, cfg := range []*config.Config{
		{GroupsEnabled: false},
		{GroupsEnabled: true, GroupsSecretKey: "ab00000000000000000000000000000000000000000000000000000000000000"},
	} {
		got := supportedNIPs(cfg)
		for _, n := range always {
			if !hasNIP(got, n) {
				t.Errorf("NIP-%d missing from advertisement (GroupsEnabled=%v): %v", n, cfg.GroupsEnabled, got)
			}
		}
	}
}

// The list stays in ascending order so 29 lands between 22 and 33 rather than
// being tacked on the end.
func TestSupportedNIPs_StaysSorted(t *testing.T) {
	got := supportedNIPs(&config.Config{GroupsEnabled: true, GroupsSecretKey: "ac00000000000000000000000000000000000000000000000000000000000000"})
	prev := -1
	for i, n := range got {
		v, ok := n.(int)
		if !ok {
			t.Fatalf("element %d is not an int: %#v", i, n)
		}
		if v <= prev {
			t.Errorf("supported_nips not ascending at index %d: %v", i, got)
		}
		prev = v
	}
}
