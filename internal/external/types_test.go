package external

import (
	"testing"

	"github.com/nbd-wtf/go-nostr"
)

func TestParseExternalRefs(t *testing.T) {
	event := &nostr.Event{
		Tags: nostr.Tags{
			{"i", "isbn:9780141036144"},
			{"i", "doi:10.1000/xyz123"},
			{"i", "imdb:tt0111161"},
			{"p", "somepubkey"}, // Not an i-tag
		},
	}

	refs := ParseExternalRefs(event)

	if len(refs) != 3 {
		t.Errorf("Refs count = %d, want 3", len(refs))
	}

	// Check first ref
	if refs[0].Type != "isbn" || refs[0].ID != "9780141036144" {
		t.Errorf("Ref[0] = %+v, want isbn:9780141036144", refs[0])
	}

	// Check DOI
	if refs[1].Type != "doi" || refs[1].ID != "10.1000/xyz123" {
		t.Errorf("Ref[1] = %+v, want doi:10.1000/xyz123", refs[1])
	}
}

func TestFormatITag(t *testing.T) {
	ref := ExternalRef{Type: "isbn", ID: "9780141036144"}
	result := FormatITag(ref)

	if result != "isbn:9780141036144" {
		t.Errorf("FormatITag() = %s, want isbn:9780141036144", result)
	}
}

func TestAddExternalRef(t *testing.T) {
	event := &nostr.Event{Tags: nostr.Tags{}}

	ref := ExternalRef{Type: "doi", ID: "10.1000/xyz123"}
	AddExternalRef(event, ref)

	if len(event.Tags) != 1 {
		t.Errorf("Tags count = %d, want 1", len(event.Tags))
	}

	if event.Tags[0][0] != "i" || event.Tags[0][1] != "doi:10.1000/xyz123" {
		t.Errorf("Tag = %v, want [i, doi:10.1000/xyz123]", event.Tags[0])
	}
}

func TestParseKTags(t *testing.T) {
	event := &nostr.Event{
		Tags: nostr.Tags{
			{"k", "book"},
			{"k", "review"},
			{"i", "isbn:123"},
		},
	}

	kinds := ParseKTags(event)

	if len(kinds) != 2 {
		t.Errorf("Kinds count = %d, want 2", len(kinds))
	}

	if kinds[0] != "book" || kinds[1] != "review" {
		t.Errorf("Kinds = %v, want [book, review]", kinds)
	}
}

func TestIsValidISBN(t *testing.T) {
	tests := []struct {
		isbn  string
		valid bool
	}{
		{"9780141036144", true},     // ISBN-13
		{"0-14-103614-4", true},     // ISBN-10 with hyphens
		{"978 0 14 103614 4", true}, // ISBN-13 with spaces
		{"123", false},              // Too short
		{"12345678901234", false},   // Too long
	}

	for _, tt := range tests {
		t.Run(tt.isbn, func(t *testing.T) {
			if got := IsValidISBN(tt.isbn); got != tt.valid {
				t.Errorf("IsValidISBN(%s) = %v, want %v", tt.isbn, got, tt.valid)
			}
		})
	}
}

func TestIsValidDOI(t *testing.T) {
	tests := []struct {
		doi   string
		valid bool
	}{
		{"10.1000/xyz123", true},
		{"10.1038/nphys1170", true},
		{"doi:10.1000/xyz", false}, // Has prefix
		{"11.1000/xyz", false},     // Wrong prefix
		{"10.1000", false},         // No suffix
	}

	for _, tt := range tests {
		t.Run(tt.doi, func(t *testing.T) {
			if got := IsValidDOI(tt.doi); got != tt.valid {
				t.Errorf("IsValidDOI(%s) = %v, want %v", tt.doi, got, tt.valid)
			}
		})
	}
}

func TestIsExternalRefTag(t *testing.T) {
	tests := []struct {
		tag      nostr.Tag
		expected bool
	}{
		{nostr.Tag{"i", "isbn:123"}, true},
		{nostr.Tag{"i", "doi:10.1/x"}, true},
		{nostr.Tag{"p", "pubkey"}, false},
		{nostr.Tag{"e", "eventid"}, false},
		{nostr.Tag{"i"}, false}, // Too short
	}

	for _, tt := range tests {
		if got := IsExternalRefTag(tt.tag); got != tt.expected {
			t.Errorf("IsExternalRefTag(%v) = %v, want %v", tt.tag, got, tt.expected)
		}
	}
}

func TestGetRefsOfType(t *testing.T) {
	refs := []ExternalRef{
		{Type: "isbn", ID: "123"},
		{Type: "doi", ID: "10.1/x"},
		{Type: "isbn", ID: "456"},
	}

	isbns := GetRefsOfType(refs, "isbn")
	if len(isbns) != 2 {
		t.Errorf("ISBN refs count = %d, want 2", len(isbns))
	}

	dois := GetRefsOfType(refs, "doi")
	if len(dois) != 1 {
		t.Errorf("DOI refs count = %d, want 1", len(dois))
	}
}

func TestCommonExternalTypes(t *testing.T) {
	types := CommonExternalTypes()

	if len(types) == 0 {
		t.Error("CommonExternalTypes() returned empty")
	}

	// Check that known types are included
	expected := map[string]bool{
		"isbn":    false,
		"doi":     false,
		"imdb":    false,
		"spotify": false,
	}

	for _, typ := range types {
		if _, ok := expected[typ]; ok {
			expected[typ] = true
		}
	}

	for typ, found := range expected {
		if !found {
			t.Errorf("Missing expected type: %s", typ)
		}
	}
}

func TestTypeConstants(t *testing.T) {
	if TypeISBN != "isbn" {
		t.Errorf("TypeISBN = %s, want isbn", TypeISBN)
	}
	if TypeDOI != "doi" {
		t.Errorf("TypeDOI = %s, want doi", TypeDOI)
	}
	if TypeIMDB != "imdb" {
		t.Errorf("TypeIMDB = %s, want imdb", TypeIMDB)
	}
}
