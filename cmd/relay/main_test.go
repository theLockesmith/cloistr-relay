package main

import (
	"testing"

	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/auth"
	"git.aegis-hq.xyz/coldforge/cloistr-relay/internal/config"
)

// This one line is where the defect lived.
//
// parseAuthConfig used to copy cfg.AllowedPubkeys -- the WoT BYPASS list -- into
// the auth package's RESTRICTIVE write whitelist. Production carried four entries
// there, so the relay refused every other pubkey: a new user completed NIP-42 AUTH
// and was told "restricted: your pubkey is not on the whitelist", and could not
// save a profile, a follow, or a file.
//
// Auth is registered before the WoT gate and khatru runs RejectEvent handlers in
// registration order, so this rejection fired first and WoT's authed-author
// exemption -- written specifically to open the relay up -- was never reached.
func TestParseAuthConfig_AllowedPubkeysDoNotRestrictWrites(t *testing.T) {
	cfg := &config.Config{
		AuthPolicy:     "auth-write",
		AllowedPubkeys: []string{"wotbypass1", "wotbypass2", "wotbypass3", "wotbypass4"},
	}

	got := parseAuthConfig(cfg)

	if len(got.WriteWhitelist) != 0 {
		t.Fatalf("WriteWhitelist = %v, want empty. AllowedPubkeys is a WoT bypass "+
			"grant; copying it here refuses every pubkey that is not listed.",
			got.WriteWhitelist)
	}
	if got.Policy != auth.PolicyAuthWrite {
		t.Errorf("Policy = %v, want PolicyAuthWrite", got.Policy)
	}
}

// The restrictive whitelist still works -- it just has its own input now, so a
// self-hosted or invite-only relay can opt in without side effects on WoT.
func TestParseAuthConfig_WriteWhitelistIsWired(t *testing.T) {
	cfg := &config.Config{
		AuthPolicy:            "auth-write",
		AllowedPubkeys:        []string{"wotbypass1"},
		WriteWhitelistPubkeys: []string{"invited1", "invited2"},
	}

	got := parseAuthConfig(cfg)

	if len(got.WriteWhitelist) != 2 ||
		got.WriteWhitelist[0] != "invited1" || got.WriteWhitelist[1] != "invited2" {
		t.Fatalf("WriteWhitelist = %v, want [invited1 invited2]", got.WriteWhitelist)
	}
}
