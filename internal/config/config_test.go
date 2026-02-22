package config

import (
	"os"
	"testing"
)

// TestLoad_DefaultValues tests that default configuration values are set correctly
func TestLoad_DefaultValues(t *testing.T) {
	// Clear environment variables
	clearEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	// Verify all default values
	tests := []struct {
		name     string
		got      interface{}
		expected interface{}
	}{
		{"Port", cfg.Port, 3334},
		{"RelayName", cfg.RelayName, "Cloistr Relay"},
		{"RelayURL", cfg.RelayURL, "ws://localhost:3334"},
		{"DBHost", cfg.DBHost, "localhost"},
		{"DBPort", cfg.DBPort, 5432},
		{"DBName", cfg.DBName, "nostr"},
		{"DBUser", cfg.DBUser, "postgres"},
		{"DBPassword", cfg.DBPassword, ""},
		{"RelayPubkey", cfg.RelayPubkey, ""},
		{"RelayContact", cfg.RelayContact, ""},
		{"AuthPolicy", cfg.AuthPolicy, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("Default %s = %v, want %v", tt.name, tt.got, tt.expected)
			}
		})
	}
}

// TestLoad_RelayPortOverride tests RELAY_PORT environment variable
func TestLoad_RelayPortOverride(t *testing.T) {
	clearEnv(t)
	os.Setenv("RELAY_PORT", "8080")
	defer os.Unsetenv("RELAY_PORT")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Port)
	}
}

// TestLoad_RelayPortInvalid tests invalid RELAY_PORT value
func TestLoad_RelayPortInvalid(t *testing.T) {
	clearEnv(t)
	os.Setenv("RELAY_PORT", "not-a-number")
	defer os.Unsetenv("RELAY_PORT")

	_, err := Load()
	if err == nil {
		t.Error("Load() with invalid RELAY_PORT should return error")
	}
}

// TestLoad_RelayNameOverride tests RELAY_NAME environment variable
func TestLoad_RelayNameOverride(t *testing.T) {
	clearEnv(t)
	os.Setenv("RELAY_NAME", "test-relay")
	defer os.Unsetenv("RELAY_NAME")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.RelayName != "test-relay" {
		t.Errorf("RelayName = %s, want test-relay", cfg.RelayName)
	}
}

// TestLoad_RelayURLOverride tests RELAY_URL environment variable
func TestLoad_RelayURLOverride(t *testing.T) {
	clearEnv(t)
	os.Setenv("RELAY_URL", "wss://relay.example.com")
	defer os.Unsetenv("RELAY_URL")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.RelayURL != "wss://relay.example.com" {
		t.Errorf("RelayURL = %s, want wss://relay.example.com", cfg.RelayURL)
	}
}

// TestLoad_RelayPubkeyOverride tests RELAY_PUBKEY environment variable
func TestLoad_RelayPubkeyOverride(t *testing.T) {
	clearEnv(t)
	testPubkey := "npub1test1234567890abcdefghijklmnopqrstuvwxyz"
	os.Setenv("RELAY_PUBKEY", testPubkey)
	defer os.Unsetenv("RELAY_PUBKEY")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.RelayPubkey != testPubkey {
		t.Errorf("RelayPubkey = %s, want %s", cfg.RelayPubkey, testPubkey)
	}
}

// TestLoad_RelayContactOverride tests RELAY_CONTACT environment variable
func TestLoad_RelayContactOverride(t *testing.T) {
	clearEnv(t)
	os.Setenv("RELAY_CONTACT", "admin@example.com")
	defer os.Unsetenv("RELAY_CONTACT")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.RelayContact != "admin@example.com" {
		t.Errorf("RelayContact = %s, want admin@example.com", cfg.RelayContact)
	}
}

// TestLoad_DBHostOverride tests DB_HOST environment variable
func TestLoad_DBHostOverride(t *testing.T) {
	clearEnv(t)
	os.Setenv("DB_HOST", "postgres.example.com")
	defer os.Unsetenv("DB_HOST")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.DBHost != "postgres.example.com" {
		t.Errorf("DBHost = %s, want postgres.example.com", cfg.DBHost)
	}
}

// TestLoad_DBPortOverride tests DB_PORT environment variable
func TestLoad_DBPortOverride(t *testing.T) {
	clearEnv(t)
	os.Setenv("DB_PORT", "5433")
	defer os.Unsetenv("DB_PORT")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.DBPort != 5433 {
		t.Errorf("DBPort = %d, want 5433", cfg.DBPort)
	}
}

// TestLoad_DBPortInvalid tests invalid DB_PORT value
func TestLoad_DBPortInvalid(t *testing.T) {
	clearEnv(t)
	os.Setenv("DB_PORT", "invalid")
	defer os.Unsetenv("DB_PORT")

	_, err := Load()
	if err == nil {
		t.Error("Load() with invalid DB_PORT should return error")
	}
}

// TestLoad_DBNameOverride tests DB_NAME environment variable
func TestLoad_DBNameOverride(t *testing.T) {
	clearEnv(t)
	os.Setenv("DB_NAME", "test_db")
	defer os.Unsetenv("DB_NAME")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.DBName != "test_db" {
		t.Errorf("DBName = %s, want test_db", cfg.DBName)
	}
}

// TestLoad_DBUserOverride tests DB_USER environment variable
func TestLoad_DBUserOverride(t *testing.T) {
	clearEnv(t)
	os.Setenv("DB_USER", "testuser")
	defer os.Unsetenv("DB_USER")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.DBUser != "testuser" {
		t.Errorf("DBUser = %s, want testuser", cfg.DBUser)
	}
}

// TestLoad_DBPasswordOverride tests DB_PASSWORD environment variable
func TestLoad_DBPasswordOverride(t *testing.T) {
	clearEnv(t)
	os.Setenv("DB_PASSWORD", "secretpassword")
	defer os.Unsetenv("DB_PASSWORD")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.DBPassword != "secretpassword" {
		t.Errorf("DBPassword = %s, want secretpassword", cfg.DBPassword)
	}
}

// TestLoad_AuthPolicyOverride tests AUTH_POLICY environment variable
func TestLoad_AuthPolicyOverride(t *testing.T) {
	testCases := []string{"open", "auth-read", "auth-write", "auth-all"}

	for _, tc := range testCases {
		t.Run(tc, func(t *testing.T) {
			clearEnv(t)
			os.Setenv("AUTH_POLICY", tc)
			defer os.Unsetenv("AUTH_POLICY")

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() returned error: %v", err)
			}

			if cfg.AuthPolicy != tc {
				t.Errorf("AuthPolicy = %s, want %s", cfg.AuthPolicy, tc)
			}
		})
	}
}

// TestLoad_AllowedPubkeysOverride tests ALLOWED_PUBKEYS environment variable
func TestLoad_AllowedPubkeysOverride(t *testing.T) {
	clearEnv(t)
	os.Setenv("ALLOWED_PUBKEYS", "pubkey1,pubkey2,pubkey3")
	defer os.Unsetenv("ALLOWED_PUBKEYS")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	expected := []string{"pubkey1", "pubkey2", "pubkey3"}
	if len(cfg.AllowedPubkeys) != len(expected) {
		t.Fatalf("AllowedPubkeys length = %d, want %d", len(cfg.AllowedPubkeys), len(expected))
	}

	for i, pk := range expected {
		if cfg.AllowedPubkeys[i] != pk {
			t.Errorf("AllowedPubkeys[%d] = %s, want %s", i, cfg.AllowedPubkeys[i], pk)
		}
	}
}

// TestLoad_AllowedPubkeysWithSpaces tests ALLOWED_PUBKEYS with extra whitespace
func TestLoad_AllowedPubkeysWithSpaces(t *testing.T) {
	clearEnv(t)
	os.Setenv("ALLOWED_PUBKEYS", " pubkey1 , pubkey2 , pubkey3 ")
	defer os.Unsetenv("ALLOWED_PUBKEYS")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	expected := []string{"pubkey1", "pubkey2", "pubkey3"}
	if len(cfg.AllowedPubkeys) != len(expected) {
		t.Fatalf("AllowedPubkeys length = %d, want %d", len(cfg.AllowedPubkeys), len(expected))
	}

	for i, pk := range expected {
		if cfg.AllowedPubkeys[i] != pk {
			t.Errorf("AllowedPubkeys[%d] = %s, want %s", i, cfg.AllowedPubkeys[i], pk)
		}
	}
}

// TestLoad_MultipleOverrides tests multiple environment variables at once
func TestLoad_MultipleOverrides(t *testing.T) {
	clearEnv(t)
	os.Setenv("RELAY_PORT", "9000")
	os.Setenv("RELAY_NAME", "multi-test")
	os.Setenv("DB_HOST", "db.example.com")
	os.Setenv("DB_PORT", "5433")
	os.Setenv("AUTH_POLICY", "auth-write")
	defer func() {
		os.Unsetenv("RELAY_PORT")
		os.Unsetenv("RELAY_NAME")
		os.Unsetenv("DB_HOST")
		os.Unsetenv("DB_PORT")
		os.Unsetenv("AUTH_POLICY")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.Port != 9000 {
		t.Errorf("Port = %d, want 9000", cfg.Port)
	}
	if cfg.RelayName != "multi-test" {
		t.Errorf("RelayName = %s, want multi-test", cfg.RelayName)
	}
	if cfg.DBHost != "db.example.com" {
		t.Errorf("DBHost = %s, want db.example.com", cfg.DBHost)
	}
	if cfg.DBPort != 5433 {
		t.Errorf("DBPort = %d, want 5433", cfg.DBPort)
	}
	if cfg.AuthPolicy != "auth-write" {
		t.Errorf("AuthPolicy = %s, want auth-write", cfg.AuthPolicy)
	}
}

// TestParseCommaSeparated_EmptyString tests parsing empty string
func TestParseCommaSeparated_EmptyString(t *testing.T) {
	result := parseCommaSeparated("")
	if result != nil {
		t.Errorf("parseCommaSeparated(\"\") = %v, want nil", result)
	}
}

// TestParseCommaSeparated_SingleValue tests parsing single value
func TestParseCommaSeparated_SingleValue(t *testing.T) {
	result := parseCommaSeparated("value1")
	if len(result) != 1 || result[0] != "value1" {
		t.Errorf("parseCommaSeparated(\"value1\") = %v, want [value1]", result)
	}
}

// TestParseCommaSeparated_MultipleValues tests parsing multiple values
func TestParseCommaSeparated_MultipleValues(t *testing.T) {
	result := parseCommaSeparated("value1,value2,value3")
	expected := []string{"value1", "value2", "value3"}

	if len(result) != len(expected) {
		t.Fatalf("length = %d, want %d", len(result), len(expected))
	}

	for i, v := range expected {
		if result[i] != v {
			t.Errorf("result[%d] = %s, want %s", i, result[i], v)
		}
	}
}

// TestParseCommaSeparated_WithWhitespace tests parsing with whitespace
func TestParseCommaSeparated_WithWhitespace(t *testing.T) {
	result := parseCommaSeparated(" value1 , value2 , value3 ")
	expected := []string{"value1", "value2", "value3"}

	if len(result) != len(expected) {
		t.Fatalf("length = %d, want %d", len(result), len(expected))
	}

	for i, v := range expected {
		if result[i] != v {
			t.Errorf("result[%d] = %s, want %s", i, result[i], v)
		}
	}
}

// TestParseCommaSeparated_EmptyElements tests parsing with empty elements
func TestParseCommaSeparated_EmptyElements(t *testing.T) {
	result := parseCommaSeparated("value1,,value2,  ,value3")
	expected := []string{"value1", "value2", "value3"}

	if len(result) != len(expected) {
		t.Fatalf("length = %d, want %d", len(result), len(expected))
	}

	for i, v := range expected {
		if result[i] != v {
			t.Errorf("result[%d] = %s, want %s", i, result[i], v)
		}
	}
}

// clearEnv clears all environment variables that affect configuration
func clearEnv(t *testing.T) {
	t.Helper()
	envVars := []string{
		"RELAY_PORT", "RELAY_NAME", "RELAY_URL", "RELAY_PUBKEY", "RELAY_CONTACT",
		"DB_HOST", "DB_PORT", "DB_NAME", "DB_USER", "DB_PASSWORD",
		"AUTH_POLICY", "ALLOWED_PUBKEYS",
	}
	for _, env := range envVars {
		os.Unsetenv(env)
	}
}
