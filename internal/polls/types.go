// Package polls implements NIP-88 polls
//
// NIP-88 provides polling functionality where:
// - Users create poll events with multiple options
// - Others respond by selecting options
// - Results can be aggregated by tallying responses
//
// Reference: https://github.com/nostr-protocol/nips/blob/master/88.md
package polls

import (
	"strconv"

	"github.com/nbd-wtf/go-nostr"
)

// Event kinds for NIP-88
const (
	// KindPoll is a poll question with options
	KindPoll = 1068

	// KindPollResponse is a response to a poll
	KindPollResponse = 1018
)

// Poll represents a NIP-88 poll
type Poll struct {
	// ID is the poll event ID
	ID string
	// Question is the poll question (in content)
	Question string
	// Options are the poll options (from poll_option tags)
	Options []PollOption
	// ClosedAt is the optional closing time (Unix timestamp)
	ClosedAt int64
	// MinChoices is minimum selections (default: 1)
	MinChoices int
	// MaxChoices is maximum selections (default: 1)
	MaxChoices int
}

// PollOption represents a poll option
type PollOption struct {
	// Index is the option index (0-based)
	Index int
	// Label is the option text
	Label string
}

// PollResponse represents a response to a poll
type PollResponse struct {
	// PollID is the event ID of the poll being responded to
	PollID string
	// PollRef is the a-tag reference if poll is addressable
	PollRef string
	// Selections are the selected option indices
	Selections []int
}

// ParsePoll parses a kind 1068 poll event
func ParsePoll(event *nostr.Event) *Poll {
	if event.Kind != KindPoll {
		return nil
	}

	poll := &Poll{
		ID:         event.ID,
		Question:   event.Content,
		MinChoices: 1,
		MaxChoices: 1,
	}

	for _, tag := range event.Tags {
		if len(tag) < 2 {
			continue
		}

		switch tag[0] {
		case "poll_option":
			if len(tag) >= 3 {
				idx, err := strconv.Atoi(tag[1])
				if err == nil {
					poll.Options = append(poll.Options, PollOption{
						Index: idx,
						Label: tag[2],
					})
				}
			}
		case "closed_at":
			if ts, err := strconv.ParseInt(tag[1], 10, 64); err == nil {
				poll.ClosedAt = ts
			}
		case "min_choices":
			if n, err := strconv.Atoi(tag[1]); err == nil {
				poll.MinChoices = n
			}
		case "max_choices":
			if n, err := strconv.Atoi(tag[1]); err == nil {
				poll.MaxChoices = n
			}
		}
	}

	return poll
}

// CreatePollEvent creates a kind 1068 poll event
func CreatePollEvent(pubkey string, poll *Poll) *nostr.Event {
	event := &nostr.Event{
		Kind:      KindPoll,
		PubKey:    pubkey,
		CreatedAt: nostr.Now(),
		Content:   poll.Question,
		Tags:      nostr.Tags{},
	}

	// Add options
	for _, opt := range poll.Options {
		event.Tags = append(event.Tags, nostr.Tag{
			"poll_option",
			strconv.Itoa(opt.Index),
			opt.Label,
		})
	}

	// Add optional settings
	if poll.ClosedAt > 0 {
		event.Tags = append(event.Tags, nostr.Tag{"closed_at", strconv.FormatInt(poll.ClosedAt, 10)})
	}

	if poll.MinChoices != 1 {
		event.Tags = append(event.Tags, nostr.Tag{"min_choices", strconv.Itoa(poll.MinChoices)})
	}

	if poll.MaxChoices != 1 {
		event.Tags = append(event.Tags, nostr.Tag{"max_choices", strconv.Itoa(poll.MaxChoices)})
	}

	return event
}

// ParsePollResponse parses a kind 1018 poll response
func ParsePollResponse(event *nostr.Event) *PollResponse {
	if event.Kind != KindPollResponse {
		return nil
	}

	response := &PollResponse{}

	for _, tag := range event.Tags {
		if len(tag) < 2 {
			continue
		}

		switch tag[0] {
		case "e":
			response.PollID = tag[1]
		case "a":
			response.PollRef = tag[1]
		case "response":
			if idx, err := strconv.Atoi(tag[1]); err == nil {
				response.Selections = append(response.Selections, idx)
			}
		}
	}

	return response
}

// CreatePollResponseEvent creates a kind 1018 poll response
func CreatePollResponseEvent(pubkey string, response *PollResponse) *nostr.Event {
	event := &nostr.Event{
		Kind:      KindPollResponse,
		PubKey:    pubkey,
		CreatedAt: nostr.Now(),
		Content:   "",
		Tags:      nostr.Tags{},
	}

	// Reference the poll
	if response.PollID != "" {
		event.Tags = append(event.Tags, nostr.Tag{"e", response.PollID})
	}
	if response.PollRef != "" {
		event.Tags = append(event.Tags, nostr.Tag{"a", response.PollRef})
	}

	// Add selections
	for _, idx := range response.Selections {
		event.Tags = append(event.Tags, nostr.Tag{"response", strconv.Itoa(idx)})
	}

	return event
}

// IsPollKind returns true if the kind is NIP-88 related
func IsPollKind(kind int) bool {
	return kind == KindPoll || kind == KindPollResponse
}

// ValidateResponse checks if a response is valid for a poll
func ValidateResponse(poll *Poll, response *PollResponse) bool {
	// Check selection count
	if len(response.Selections) < poll.MinChoices {
		return false
	}
	if len(response.Selections) > poll.MaxChoices {
		return false
	}

	// Check that all selections are valid option indices
	optionIndices := make(map[int]bool)
	for _, opt := range poll.Options {
		optionIndices[opt.Index] = true
	}

	for _, sel := range response.Selections {
		if !optionIndices[sel] {
			return false
		}
	}

	return true
}
