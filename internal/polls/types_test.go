package polls

import (
	"testing"

	"github.com/nbd-wtf/go-nostr"
)

func TestKindConstants(t *testing.T) {
	if KindPoll != 1068 {
		t.Errorf("KindPoll = %d, want 1068", KindPoll)
	}
	if KindPollResponse != 1018 {
		t.Errorf("KindPollResponse = %d, want 1018", KindPollResponse)
	}
}

func TestIsPollKind(t *testing.T) {
	if !IsPollKind(KindPoll) {
		t.Error("IsPollKind(KindPoll) = false, want true")
	}
	if !IsPollKind(KindPollResponse) {
		t.Error("IsPollKind(KindPollResponse) = false, want true")
	}
	if IsPollKind(1) {
		t.Error("IsPollKind(1) = true, want false")
	}
}

func TestParsePoll(t *testing.T) {
	event := &nostr.Event{
		Kind:    KindPoll,
		Content: "What is your favorite color?",
		Tags: nostr.Tags{
			{"poll_option", "0", "Red"},
			{"poll_option", "1", "Blue"},
			{"poll_option", "2", "Green"},
			{"closed_at", "1718438400"},
			{"min_choices", "1"},
			{"max_choices", "2"},
		},
	}

	poll := ParsePoll(event)

	if poll == nil {
		t.Fatal("ParsePoll returned nil")
	}

	if poll.Question != "What is your favorite color?" {
		t.Errorf("Question = %s, want 'What is your favorite color?'", poll.Question)
	}

	if len(poll.Options) != 3 {
		t.Errorf("Options count = %d, want 3", len(poll.Options))
	}

	if poll.Options[0].Label != "Red" {
		t.Errorf("Options[0].Label = %s, want Red", poll.Options[0].Label)
	}

	if poll.ClosedAt != 1718438400 {
		t.Errorf("ClosedAt = %d, want 1718438400", poll.ClosedAt)
	}

	if poll.MinChoices != 1 {
		t.Errorf("MinChoices = %d, want 1", poll.MinChoices)
	}

	if poll.MaxChoices != 2 {
		t.Errorf("MaxChoices = %d, want 2", poll.MaxChoices)
	}
}

func TestParsePollWrongKind(t *testing.T) {
	event := &nostr.Event{Kind: 1}
	if ParsePoll(event) != nil {
		t.Error("Should return nil for wrong kind")
	}
}

func TestCreatePollEvent(t *testing.T) {
	poll := &Poll{
		Question: "Test question?",
		Options: []PollOption{
			{Index: 0, Label: "Option A"},
			{Index: 1, Label: "Option B"},
		},
		ClosedAt:   1718438400,
		MinChoices: 1,
		MaxChoices: 1,
	}

	event := CreatePollEvent("pubkey123", poll)

	if event.Kind != KindPoll {
		t.Errorf("Kind = %d, want %d", event.Kind, KindPoll)
	}

	if event.Content != "Test question?" {
		t.Errorf("Content = %s, want 'Test question?'", event.Content)
	}

	// Verify options exist
	optionCount := 0
	for _, tag := range event.Tags {
		if tag[0] == "poll_option" {
			optionCount++
		}
	}

	if optionCount != 2 {
		t.Errorf("poll_option count = %d, want 2", optionCount)
	}
}

func TestParsePollResponse(t *testing.T) {
	event := &nostr.Event{
		Kind: KindPollResponse,
		Tags: nostr.Tags{
			{"e", "pollid123"},
			{"response", "0"},
			{"response", "2"},
		},
	}

	response := ParsePollResponse(event)

	if response == nil {
		t.Fatal("ParsePollResponse returned nil")
	}

	if response.PollID != "pollid123" {
		t.Errorf("PollID = %s, want pollid123", response.PollID)
	}

	if len(response.Selections) != 2 {
		t.Errorf("Selections count = %d, want 2", len(response.Selections))
	}

	if response.Selections[0] != 0 || response.Selections[1] != 2 {
		t.Errorf("Selections = %v, want [0, 2]", response.Selections)
	}
}

func TestParsePollResponseWrongKind(t *testing.T) {
	event := &nostr.Event{Kind: 1}
	if ParsePollResponse(event) != nil {
		t.Error("Should return nil for wrong kind")
	}
}

func TestCreatePollResponseEvent(t *testing.T) {
	response := &PollResponse{
		PollID:     "pollid123",
		Selections: []int{1, 2},
	}

	event := CreatePollResponseEvent("userpubkey", response)

	if event.Kind != KindPollResponse {
		t.Errorf("Kind = %d, want %d", event.Kind, KindPollResponse)
	}

	// Verify e-tag exists
	foundE := false
	for _, tag := range event.Tags {
		if tag[0] == "e" && tag[1] == "pollid123" {
			foundE = true
			break
		}
	}
	if !foundE {
		t.Error("Missing e-tag for poll reference")
	}

	// Verify response tags
	responseCount := 0
	for _, tag := range event.Tags {
		if tag[0] == "response" {
			responseCount++
		}
	}
	if responseCount != 2 {
		t.Errorf("response tag count = %d, want 2", responseCount)
	}
}

func TestValidateResponse(t *testing.T) {
	poll := &Poll{
		Options: []PollOption{
			{Index: 0, Label: "A"},
			{Index: 1, Label: "B"},
			{Index: 2, Label: "C"},
		},
		MinChoices: 1,
		MaxChoices: 2,
	}

	tests := []struct {
		name       string
		selections []int
		valid      bool
	}{
		{"valid single", []int{0}, true},
		{"valid double", []int{0, 1}, true},
		{"too few", []int{}, false},
		{"too many", []int{0, 1, 2}, false},
		{"invalid index", []int{5}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := &PollResponse{Selections: tt.selections}
			if got := ValidateResponse(poll, response); got != tt.valid {
				t.Errorf("ValidateResponse() = %v, want %v", got, tt.valid)
			}
		})
	}
}
