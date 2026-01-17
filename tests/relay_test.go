package tests

import (
	"testing"

	"gitlab.com/coldforge/coldforge-relay/internal/config"
)

// TestConfigLoad tests configuration loading from environment variables
func TestConfigLoad(t *testing.T) {
	t.Run("default configuration", func(t *testing.T) {
		cfg, err := config.Load()
		if err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}

		if cfg.Port != 3334 {
			t.Errorf("Expected port 3334, got %d", cfg.Port)
		}

		if cfg.RelayName != "coldforge-relay" {
			t.Errorf("Expected relay name 'coldforge-relay', got %s", cfg.RelayName)
		}

		if cfg.DBHost != "localhost" {
			t.Errorf("Expected DB host 'localhost', got %s", cfg.DBHost)
		}
	})
}

// TestRelayInitialization is a placeholder for integration tests
func TestRelayInitialization(t *testing.T) {
	t.Skip("Integration test - requires database setup")

	// TODO: Add full relay initialization test
	// This would:
	// 1. Start a test PostgreSQL instance
	// 2. Create a relay instance
	// 3. Verify it connects to the database
	// 4. Check that event handlers are registered
}

// TestEventHandling is a placeholder for event handling tests
func TestEventHandling(t *testing.T) {
	t.Skip("Integration test - requires relay instance")

	// TODO: Add event handling tests
	// This would:
	// 1. Create a test event
	// 2. Send it to the relay
	// 3. Verify it's stored in the database
	// 4. Query it back and verify the data
}
