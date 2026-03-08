package calendar

import (
	"testing"
	"time"

	"github.com/nbd-wtf/go-nostr"
)

func TestKindConstants(t *testing.T) {
	tests := []struct {
		name  string
		kind  int
		value int
	}{
		{"DateEvent", KindDateEvent, 31922},
		{"TimeEvent", KindTimeEvent, 31923},
		{"Calendar", KindCalendar, 31924},
		{"RSVP", KindRSVP, 31925},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.kind != tt.value {
				t.Errorf("Kind%s = %d, want %d", tt.name, tt.kind, tt.value)
			}
		})
	}
}

func TestIsCalendarKind(t *testing.T) {
	calendarKinds := []int{
		KindDateEvent,
		KindTimeEvent,
		KindCalendar,
		KindRSVP,
	}

	for _, kind := range calendarKinds {
		if !IsCalendarKind(kind) {
			t.Errorf("IsCalendarKind(%d) = false, want true", kind)
		}
	}

	nonCalendarKinds := []int{0, 1, 3, 4, 7, 1985, 10002}
	for _, kind := range nonCalendarKinds {
		if IsCalendarKind(kind) {
			t.Errorf("IsCalendarKind(%d) = true, want false", kind)
		}
	}
}

func TestParseCalendarEventDateBased(t *testing.T) {
	event := &nostr.Event{
		Kind: KindDateEvent,
		Tags: nostr.Tags{
			{"d", "meetup-2024"},
			{"title", "Monthly Meetup"},
			{"summary", "Our monthly gathering"},
			{"start", "2024-06-15"},
			{"end", "2024-06-16"},
			{"location", "New York"},
			{"g", "dr5regw"},
			{"p", "participant1"},
			{"p", "participant2"},
			{"t", "nostr"},
			{"t", "meetup"},
		},
	}

	calEvent := ParseCalendarEvent(event)

	if calEvent == nil {
		t.Fatal("ParseCalendarEvent returned nil")
	}

	if !calEvent.IsDateBased {
		t.Error("Should be date-based")
	}

	if calEvent.ID != "meetup-2024" {
		t.Errorf("ID = %s, want meetup-2024", calEvent.ID)
	}

	if calEvent.Title != "Monthly Meetup" {
		t.Errorf("Title = %s, want 'Monthly Meetup'", calEvent.Title)
	}

	if calEvent.Summary != "Our monthly gathering" {
		t.Errorf("Summary = %s, want 'Our monthly gathering'", calEvent.Summary)
	}

	expectedStart := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
	if !calEvent.Start.Equal(expectedStart) {
		t.Errorf("Start = %v, want %v", calEvent.Start, expectedStart)
	}

	expectedEnd := time.Date(2024, 6, 16, 0, 0, 0, 0, time.UTC)
	if !calEvent.End.Equal(expectedEnd) {
		t.Errorf("End = %v, want %v", calEvent.End, expectedEnd)
	}

	if calEvent.Location != "New York" {
		t.Errorf("Location = %s, want 'New York'", calEvent.Location)
	}

	if calEvent.Geohash != "dr5regw" {
		t.Errorf("Geohash = %s, want 'dr5regw'", calEvent.Geohash)
	}

	if len(calEvent.Participants) != 2 {
		t.Errorf("Participants count = %d, want 2", len(calEvent.Participants))
	}

	if len(calEvent.Hashtags) != 2 {
		t.Errorf("Hashtags count = %d, want 2", len(calEvent.Hashtags))
	}
}

func TestParseCalendarEventTimeBased(t *testing.T) {
	startTS := int64(1718438400) // 2024-06-15 12:00:00 UTC
	endTS := int64(1718449200)   // 2024-06-15 15:00:00 UTC

	event := &nostr.Event{
		Kind: KindTimeEvent,
		Tags: nostr.Tags{
			{"d", "meeting-123"},
			{"title", "Team Meeting"},
			{"start", "1718438400"},
			{"end", "1718449200"},
			{"start_tzid", "America/New_York"},
			{"end_tzid", "America/New_York"},
		},
	}

	calEvent := ParseCalendarEvent(event)

	if calEvent == nil {
		t.Fatal("ParseCalendarEvent returned nil")
	}

	if calEvent.IsDateBased {
		t.Error("Should not be date-based")
	}

	if calEvent.Start.Unix() != startTS {
		t.Errorf("Start = %d, want %d", calEvent.Start.Unix(), startTS)
	}

	if calEvent.End.Unix() != endTS {
		t.Errorf("End = %d, want %d", calEvent.End.Unix(), endTS)
	}

	if calEvent.StartTzid != "America/New_York" {
		t.Errorf("StartTzid = %s, want 'America/New_York'", calEvent.StartTzid)
	}

	if calEvent.EndTzid != "America/New_York" {
		t.Errorf("EndTzid = %s, want 'America/New_York'", calEvent.EndTzid)
	}
}

func TestParseCalendarEventWrongKind(t *testing.T) {
	event := &nostr.Event{
		Kind: 1, // Wrong kind
	}

	calEvent := ParseCalendarEvent(event)
	if calEvent != nil {
		t.Error("Should return nil for wrong kind")
	}
}

func TestCreateDateEventTags(t *testing.T) {
	calEvent := &CalendarEvent{
		ID:       "test-event",
		Title:    "Test Event",
		Summary:  "A test",
		Start:    time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC),
		End:      time.Date(2024, 6, 16, 0, 0, 0, 0, time.UTC),
		Location: "Test Location",
	}

	tags := CreateDateEventTags(calEvent)

	// Check required tags
	foundD := false
	foundTitle := false
	foundStart := false

	for _, tag := range tags {
		if len(tag) >= 2 {
			switch tag[0] {
			case "d":
				foundD = true
				if tag[1] != "test-event" {
					t.Errorf("d tag = %s, want test-event", tag[1])
				}
			case "title":
				foundTitle = true
			case "start":
				foundStart = true
				if tag[1] != "2024-06-15" {
					t.Errorf("start tag = %s, want 2024-06-15", tag[1])
				}
			}
		}
	}

	if !foundD {
		t.Error("Missing d tag")
	}
	if !foundTitle {
		t.Error("Missing title tag")
	}
	if !foundStart {
		t.Error("Missing start tag")
	}
}

func TestCreateTimeEventTags(t *testing.T) {
	startTime := time.Unix(1718438400, 0)
	endTime := time.Unix(1718449200, 0)

	calEvent := &CalendarEvent{
		ID:        "meeting-123",
		Title:     "Meeting",
		Start:     startTime,
		End:       endTime,
		StartTzid: "America/New_York",
	}

	tags := CreateTimeEventTags(calEvent)

	// Check for D tag (day-granularity)
	foundD := false
	for _, tag := range tags {
		if len(tag) >= 2 && tag[0] == "D" {
			foundD = true
		}
	}

	if !foundD {
		t.Error("Missing D tag for day-granularity indexing")
	}

	// Check for start_tzid tag
	foundTzid := false
	for _, tag := range tags {
		if len(tag) >= 2 && tag[0] == "start_tzid" && tag[1] == "America/New_York" {
			foundTzid = true
		}
	}

	if !foundTzid {
		t.Error("Missing start_tzid tag")
	}
}

func TestParseRSVP(t *testing.T) {
	event := &nostr.Event{
		Kind: KindRSVP,
		Tags: nostr.Tags{
			{"d", "31923:pubkey:meeting-123"},
			{"a", "31923:pubkey:meeting-123"},
			{"e", "eventid123"},
			{"status", "accepted"},
			{"fb", "busy"},
		},
	}

	rsvp := ParseRSVP(event)

	if rsvp == nil {
		t.Fatal("ParseRSVP returned nil")
	}

	if rsvp.EventRef != "31923:pubkey:meeting-123" {
		t.Errorf("EventRef = %s, want '31923:pubkey:meeting-123'", rsvp.EventRef)
	}

	if rsvp.EventID != "eventid123" {
		t.Errorf("EventID = %s, want 'eventid123'", rsvp.EventID)
	}

	if rsvp.Status != "accepted" {
		t.Errorf("Status = %s, want 'accepted'", rsvp.Status)
	}

	if rsvp.FreeBusy != "busy" {
		t.Errorf("FreeBusy = %s, want 'busy'", rsvp.FreeBusy)
	}
}

func TestParseRSVPWrongKind(t *testing.T) {
	event := &nostr.Event{
		Kind: 1, // Wrong kind
	}

	rsvp := ParseRSVP(event)
	if rsvp != nil {
		t.Error("Should return nil for wrong kind")
	}
}

func TestCreateRSVPEvent(t *testing.T) {
	rsvp := &RSVP{
		EventRef: "31923:pubkey:meeting-123",
		EventID:  "eventid123",
		Status:   RSVPAccepted,
		FreeBusy: FreeBusyBusy,
	}

	event := CreateRSVPEvent("userpubkey", rsvp)

	if event.Kind != KindRSVP {
		t.Errorf("Kind = %d, want %d", event.Kind, KindRSVP)
	}

	if event.PubKey != "userpubkey" {
		t.Errorf("PubKey = %s, want userpubkey", event.PubKey)
	}

	// Check tags
	foundA := false
	foundStatus := false
	foundFb := false

	for _, tag := range event.Tags {
		if len(tag) >= 2 {
			switch tag[0] {
			case "a":
				foundA = true
			case "status":
				foundStatus = true
				if tag[1] != "accepted" {
					t.Errorf("status = %s, want accepted", tag[1])
				}
			case "fb":
				foundFb = true
				if tag[1] != "busy" {
					t.Errorf("fb = %s, want busy", tag[1])
				}
			}
		}
	}

	if !foundA {
		t.Error("Missing a tag")
	}
	if !foundStatus {
		t.Error("Missing status tag")
	}
	if !foundFb {
		t.Error("Missing fb tag")
	}
}

func TestParseCalendar(t *testing.T) {
	event := &nostr.Event{
		Kind: KindCalendar,
		Tags: nostr.Tags{
			{"d", "my-calendar"},
			{"title", "My Calendar"},
			{"summary", "Personal events"},
			{"a", "31923:pubkey:event1"},
			{"a", "31922:pubkey:event2"},
		},
	}

	calendar := ParseCalendar(event)

	if calendar == nil {
		t.Fatal("ParseCalendar returned nil")
	}

	if calendar.ID != "my-calendar" {
		t.Errorf("ID = %s, want my-calendar", calendar.ID)
	}

	if calendar.Title != "My Calendar" {
		t.Errorf("Title = %s, want 'My Calendar'", calendar.Title)
	}

	if len(calendar.EventRefs) != 2 {
		t.Errorf("EventRefs count = %d, want 2", len(calendar.EventRefs))
	}
}

func TestParseCalendarWrongKind(t *testing.T) {
	event := &nostr.Event{
		Kind: 1, // Wrong kind
	}

	calendar := ParseCalendar(event)
	if calendar != nil {
		t.Error("Should return nil for wrong kind")
	}
}

func TestIsValidRSVPStatus(t *testing.T) {
	validStatuses := []string{RSVPAccepted, RSVPDeclined, RSVPTentative}
	for _, status := range validStatuses {
		if !IsValidRSVPStatus(status) {
			t.Errorf("IsValidRSVPStatus(%s) = false, want true", status)
		}
	}

	if IsValidRSVPStatus("invalid") {
		t.Error("IsValidRSVPStatus(invalid) = true, want false")
	}
}

func TestIsValidFreeBusy(t *testing.T) {
	validFb := []string{FreeBusyFree, FreeBusyBusy}
	for _, fb := range validFb {
		if !IsValidFreeBusy(fb) {
			t.Errorf("IsValidFreeBusy(%s) = false, want true", fb)
		}
	}

	if IsValidFreeBusy("invalid") {
		t.Error("IsValidFreeBusy(invalid) = true, want false")
	}
}

func TestRSVPStatusConstants(t *testing.T) {
	if RSVPAccepted != "accepted" {
		t.Errorf("RSVPAccepted = %s, want accepted", RSVPAccepted)
	}
	if RSVPDeclined != "declined" {
		t.Errorf("RSVPDeclined = %s, want declined", RSVPDeclined)
	}
	if RSVPTentative != "tentative" {
		t.Errorf("RSVPTentative = %s, want tentative", RSVPTentative)
	}
}

func TestFreeBusyConstants(t *testing.T) {
	if FreeBusyFree != "free" {
		t.Errorf("FreeBusyFree = %s, want free", FreeBusyFree)
	}
	if FreeBusyBusy != "busy" {
		t.Errorf("FreeBusyBusy = %s, want busy", FreeBusyBusy)
	}
}
