package management

import (
	"encoding/json"
	"testing"
)

func TestMethodHandler_SupportedMethods(t *testing.T) {
	handler := NewMethodHandler(nil) // Store not needed for this method

	result, err := handler.SupportedMethods()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	methods, ok := result.([]string)
	if !ok {
		t.Fatalf("expected []string, got %T", result)
	}

	// Verify we have the expected number of methods
	if len(methods) != 18 {
		t.Errorf("expected 18 methods, got %d", len(methods))
	}

	// Verify some key methods are present
	expectedMethods := []string{
		"supportedmethods",
		"banpubkey",
		"listbannedpubkeys",
		"allowpubkey",
		"banevent",
		"blockip",
	}

	for _, expected := range expectedMethods {
		found := false
		for _, method := range methods {
			if method == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected method %q not found in supported methods", expected)
		}
	}
}

func TestMethodHandler_Dispatch(t *testing.T) {
	handler := NewMethodHandler(nil)

	t.Run("unsupported method", func(t *testing.T) {
		_, err := handler.Dispatch("nonexistent", nil)
		if err == nil {
			t.Error("expected error for unsupported method")
		}
	})

	t.Run("supportedmethods", func(t *testing.T) {
		result, err := handler.Dispatch("supportedmethods", nil)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if result == nil {
			t.Error("expected non-nil result")
		}
	})
}

func TestParseListParams(t *testing.T) {
	t.Run("no params", func(t *testing.T) {
		limit, offset := parseListParams(nil)
		if limit != 100 {
			t.Errorf("expected default limit 100, got %d", limit)
		}
		if offset != 0 {
			t.Errorf("expected default offset 0, got %d", offset)
		}
	})

	t.Run("with limit only", func(t *testing.T) {
		limitJSON, _ := json.Marshal(50)
		params := []json.RawMessage{limitJSON}
		limit, offset := parseListParams(params)
		if limit != 50 {
			t.Errorf("expected limit 50, got %d", limit)
		}
		if offset != 0 {
			t.Errorf("expected default offset 0, got %d", offset)
		}
	})

	t.Run("with limit and offset", func(t *testing.T) {
		limitJSON, _ := json.Marshal(25)
		offsetJSON, _ := json.Marshal(10)
		params := []json.RawMessage{limitJSON, offsetJSON}
		limit, offset := parseListParams(params)
		if limit != 25 {
			t.Errorf("expected limit 25, got %d", limit)
		}
		if offset != 10 {
			t.Errorf("expected offset 10, got %d", offset)
		}
	})
}
