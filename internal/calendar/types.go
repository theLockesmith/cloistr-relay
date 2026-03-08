// Package calendar implements NIP-52 calendar events
//
// NIP-52 provides calendar functionality including:
// - Date-based events (all-day, multi-day)
// - Time-based events (specific start/end times)
// - Calendar collections
// - RSVPs (accepted, declined, tentative)
//
// Reference: https://github.com/nostr-protocol/nips/blob/master/52.md
package calendar

import (
	"strconv"
	"time"

	"github.com/nbd-wtf/go-nostr"
)

// Event kinds for NIP-52
const (
	// KindDateEvent is for date-based calendar events (all-day/multi-day)
	KindDateEvent = 31922

	// KindTimeEvent is for time-based calendar events (specific times)
	KindTimeEvent = 31923

	// KindCalendar is for calendar collections
	KindCalendar = 31924

	// KindRSVP is for event RSVP responses
	KindRSVP = 31925
)

// RSVP status values
const (
	RSVPAccepted  = "accepted"
	RSVPDeclined  = "declined"
	RSVPTentative = "tentative"
)

// FreeBusy status values
const (
	FreeBusyFree = "free"
	FreeBusyBusy = "busy"
)

// CalendarEvent represents a parsed calendar event (date or time based)
type CalendarEvent struct {
	// ID is the d-tag identifier
	ID string
	// Title is the event title
	Title string
	// Summary is an optional description
	Summary string
	// Image is an optional image URL
	Image string
	// Location is the event location
	Location string
	// Geohash is the location geohash
	Geohash string
	// Start is the start date/time
	Start time.Time
	// End is the optional end date/time
	End time.Time
	// StartTzid is the start timezone (IANA)
	StartTzid string
	// EndTzid is the end timezone (IANA)
	EndTzid string
	// Participants are p-tagged pubkeys
	Participants []string
	// Hashtags are t-tagged topics
	Hashtags []string
	// References are r-tagged URLs
	References []string
	// IsDateBased indicates if this is a date-based (vs time-based) event
	IsDateBased bool
}

// RSVP represents a calendar event RSVP
type RSVP struct {
	// EventRef is the a-tag reference to the event
	EventRef string
	// Status is accepted, declined, or tentative
	Status string
	// FreeBusy is optional free/busy indicator
	FreeBusy string
	// EventID is the specific event revision (optional)
	EventID string
}

// Calendar represents a calendar collection
type Calendar struct {
	// ID is the d-tag identifier
	ID string
	// Title is the calendar name
	Title string
	// Summary is an optional description
	Summary string
	// EventRefs are a-tag references to events
	EventRefs []string
}

// ParseCalendarEvent parses a kind 31922 or 31923 event
func ParseCalendarEvent(event *nostr.Event) *CalendarEvent {
	if event.Kind != KindDateEvent && event.Kind != KindTimeEvent {
		return nil
	}

	calEvent := &CalendarEvent{
		IsDateBased: event.Kind == KindDateEvent,
	}

	for _, tag := range event.Tags {
		if len(tag) < 2 {
			continue
		}

		switch tag[0] {
		case "d":
			calEvent.ID = tag[1]
		case "title":
			calEvent.Title = tag[1]
		case "summary":
			calEvent.Summary = tag[1]
		case "image":
			calEvent.Image = tag[1]
		case "location":
			calEvent.Location = tag[1]
		case "g":
			calEvent.Geohash = tag[1]
		case "start":
			if calEvent.IsDateBased {
				// ISO 8601 date format (YYYY-MM-DD)
				if t, err := time.Parse("2006-01-02", tag[1]); err == nil {
					calEvent.Start = t
				}
			} else {
				// Unix timestamp
				if ts, err := strconv.ParseInt(tag[1], 10, 64); err == nil {
					calEvent.Start = time.Unix(ts, 0)
				}
			}
		case "end":
			if calEvent.IsDateBased {
				if t, err := time.Parse("2006-01-02", tag[1]); err == nil {
					calEvent.End = t
				}
			} else {
				if ts, err := strconv.ParseInt(tag[1], 10, 64); err == nil {
					calEvent.End = time.Unix(ts, 0)
				}
			}
		case "start_tzid":
			calEvent.StartTzid = tag[1]
		case "end_tzid":
			calEvent.EndTzid = tag[1]
		case "p":
			calEvent.Participants = append(calEvent.Participants, tag[1])
		case "t":
			calEvent.Hashtags = append(calEvent.Hashtags, tag[1])
		case "r":
			calEvent.References = append(calEvent.References, tag[1])
		}
	}

	return calEvent
}

// CreateDateEventTags creates tags for a date-based event
func CreateDateEventTags(calEvent *CalendarEvent) nostr.Tags {
	tags := nostr.Tags{
		{"d", calEvent.ID},
		{"title", calEvent.Title},
	}

	if !calEvent.Start.IsZero() {
		tags = append(tags, nostr.Tag{"start", calEvent.Start.Format("2006-01-02")})
	}

	if !calEvent.End.IsZero() {
		tags = append(tags, nostr.Tag{"end", calEvent.End.Format("2006-01-02")})
	}

	return appendOptionalTags(tags, calEvent)
}

// CreateTimeEventTags creates tags for a time-based event
func CreateTimeEventTags(calEvent *CalendarEvent) nostr.Tags {
	tags := nostr.Tags{
		{"d", calEvent.ID},
		{"title", calEvent.Title},
	}

	if !calEvent.Start.IsZero() {
		tags = append(tags, nostr.Tag{"start", strconv.FormatInt(calEvent.Start.Unix(), 10)})
		// Add D tag for day-granularity indexing
		dayStart := time.Date(calEvent.Start.Year(), calEvent.Start.Month(), calEvent.Start.Day(), 0, 0, 0, 0, time.UTC)
		tags = append(tags, nostr.Tag{"D", strconv.FormatInt(dayStart.Unix(), 10)})
	}

	if !calEvent.End.IsZero() {
		tags = append(tags, nostr.Tag{"end", strconv.FormatInt(calEvent.End.Unix(), 10)})
	}

	if calEvent.StartTzid != "" {
		tags = append(tags, nostr.Tag{"start_tzid", calEvent.StartTzid})
	}

	if calEvent.EndTzid != "" {
		tags = append(tags, nostr.Tag{"end_tzid", calEvent.EndTzid})
	}

	return appendOptionalTags(tags, calEvent)
}

func appendOptionalTags(tags nostr.Tags, calEvent *CalendarEvent) nostr.Tags {
	if calEvent.Summary != "" {
		tags = append(tags, nostr.Tag{"summary", calEvent.Summary})
	}

	if calEvent.Image != "" {
		tags = append(tags, nostr.Tag{"image", calEvent.Image})
	}

	if calEvent.Location != "" {
		tags = append(tags, nostr.Tag{"location", calEvent.Location})
	}

	if calEvent.Geohash != "" {
		tags = append(tags, nostr.Tag{"g", calEvent.Geohash})
	}

	for _, p := range calEvent.Participants {
		tags = append(tags, nostr.Tag{"p", p})
	}

	for _, t := range calEvent.Hashtags {
		tags = append(tags, nostr.Tag{"t", t})
	}

	for _, r := range calEvent.References {
		tags = append(tags, nostr.Tag{"r", r})
	}

	return tags
}

// ParseRSVP parses a kind 31925 RSVP event
func ParseRSVP(event *nostr.Event) *RSVP {
	if event.Kind != KindRSVP {
		return nil
	}

	rsvp := &RSVP{}

	for _, tag := range event.Tags {
		if len(tag) < 2 {
			continue
		}

		switch tag[0] {
		case "a":
			rsvp.EventRef = tag[1]
		case "e":
			rsvp.EventID = tag[1]
		case "status":
			rsvp.Status = tag[1]
		case "fb":
			rsvp.FreeBusy = tag[1]
		}
	}

	return rsvp
}

// CreateRSVPEvent creates a kind 31925 RSVP event
func CreateRSVPEvent(pubkey string, rsvp *RSVP) *nostr.Event {
	event := &nostr.Event{
		Kind:      KindRSVP,
		PubKey:    pubkey,
		CreatedAt: nostr.Now(),
		Content:   "",
		Tags:      nostr.Tags{},
	}

	// d-tag should be the event reference for deduplication
	event.Tags = append(event.Tags, nostr.Tag{"d", rsvp.EventRef})

	if rsvp.EventRef != "" {
		event.Tags = append(event.Tags, nostr.Tag{"a", rsvp.EventRef})
	}

	if rsvp.EventID != "" {
		event.Tags = append(event.Tags, nostr.Tag{"e", rsvp.EventID})
	}

	if rsvp.Status != "" {
		event.Tags = append(event.Tags, nostr.Tag{"status", rsvp.Status})
	}

	if rsvp.FreeBusy != "" {
		event.Tags = append(event.Tags, nostr.Tag{"fb", rsvp.FreeBusy})
	}

	return event
}

// ParseCalendar parses a kind 31924 calendar collection
func ParseCalendar(event *nostr.Event) *Calendar {
	if event.Kind != KindCalendar {
		return nil
	}

	calendar := &Calendar{}

	for _, tag := range event.Tags {
		if len(tag) < 2 {
			continue
		}

		switch tag[0] {
		case "d":
			calendar.ID = tag[1]
		case "title":
			calendar.Title = tag[1]
		case "summary":
			calendar.Summary = tag[1]
		case "a":
			calendar.EventRefs = append(calendar.EventRefs, tag[1])
		}
	}

	return calendar
}

// IsCalendarKind returns true if the kind is NIP-52 related
func IsCalendarKind(kind int) bool {
	switch kind {
	case KindDateEvent, KindTimeEvent, KindCalendar, KindRSVP:
		return true
	default:
		return false
	}
}

// IsValidRSVPStatus checks if the RSVP status is valid
func IsValidRSVPStatus(status string) bool {
	switch status {
	case RSVPAccepted, RSVPDeclined, RSVPTentative:
		return true
	default:
		return false
	}
}

// IsValidFreeBusy checks if the free/busy status is valid
func IsValidFreeBusy(fb string) bool {
	switch fb {
	case FreeBusyFree, FreeBusyBusy:
		return true
	default:
		return false
	}
}
