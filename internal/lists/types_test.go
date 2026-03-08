package lists

import (
	"testing"
)

func TestIsListKind(t *testing.T) {
	tests := []struct {
		kind     int
		expected bool
	}{
		{KindMuteList, true},
		{KindPinList, true},
		{KindRelayList, true},
		{KindBookmarkList, true},
		{KindCommunitiesList, true},
		{KindPublicChatsList, true},
		{KindBlockedRelaysList, true},
		{KindSearchRelaysList, true},
		{KindInterestsList, true},
		{KindEmojiList, true},
		{KindCategorizedPeopleList, true},
		{KindCategorizedBookmarkList, true},
		{KindRelaySets, true},
		{KindBookmarkSets, true},
		// Non-list kinds
		{0, false},   // Metadata
		{1, false},   // Text note
		{3, false},   // Contact list
		{4, false},   // DM
		{7, false},   // Reaction
		{9735, false}, // Zap
	}

	for _, tt := range tests {
		t.Run(KindName(tt.kind), func(t *testing.T) {
			if got := IsListKind(tt.kind); got != tt.expected {
				t.Errorf("IsListKind(%d) = %v, want %v", tt.kind, got, tt.expected)
			}
		})
	}
}

func TestIsPrivateListKind(t *testing.T) {
	privateKinds := []int{
		KindMuteList,
		KindBookmarkList,
		KindBlockedRelaysList,
		KindCategorizedPeopleList,
		KindCategorizedBookmarkList,
		KindBookmarkSets,
	}

	publicKinds := []int{
		KindPinList,
		KindRelayList,
		KindCommunitiesList,
		KindPublicChatsList,
		KindSearchRelaysList,
		KindInterestsList,
		KindEmojiList,
		KindRelaySets,
	}

	for _, kind := range privateKinds {
		if !IsPrivateListKind(kind) {
			t.Errorf("IsPrivateListKind(%d) = false, expected true for %s", kind, KindName(kind))
		}
	}

	for _, kind := range publicKinds {
		if IsPrivateListKind(kind) {
			t.Errorf("IsPrivateListKind(%d) = true, expected false for %s", kind, KindName(kind))
		}
	}
}

func TestIsReplaceableListKind(t *testing.T) {
	replaceableKinds := []int{
		KindMuteList,        // 10000
		KindPinList,         // 10001
		KindRelayList,       // 10002
		KindBookmarkList,    // 10003
		KindCommunitiesList, // 10004
		KindPublicChatsList, // 10005
		KindBlockedRelaysList, // 10006
		KindSearchRelaysList, // 10007
		KindInterestsList,   // 10015
		KindEmojiList,       // 10030
	}

	parameterizedKinds := []int{
		KindCategorizedPeopleList,   // 30000
		KindCategorizedBookmarkList, // 30001
		KindRelaySets,               // 30002
		KindBookmarkSets,            // 30003
	}

	for _, kind := range replaceableKinds {
		if !IsReplaceableListKind(kind) {
			t.Errorf("IsReplaceableListKind(%d) = false, expected true", kind)
		}
		if IsParameterizedListKind(kind) {
			t.Errorf("IsParameterizedListKind(%d) = true, expected false for replaceable kind", kind)
		}
	}

	for _, kind := range parameterizedKinds {
		if IsReplaceableListKind(kind) {
			t.Errorf("IsReplaceableListKind(%d) = true, expected false for parameterized kind", kind)
		}
		if !IsParameterizedListKind(kind) {
			t.Errorf("IsParameterizedListKind(%d) = false, expected true", kind)
		}
	}
}

func TestPrivateKinds(t *testing.T) {
	kinds := PrivateKinds()
	if len(kinds) == 0 {
		t.Error("PrivateKinds() returned empty slice")
	}

	for _, kind := range kinds {
		if !IsPrivateListKind(kind) {
			t.Errorf("PrivateKinds() contains %d which IsPrivateListKind returns false for", kind)
		}
	}
}

func TestPublicKinds(t *testing.T) {
	kinds := PublicKinds()
	if len(kinds) == 0 {
		t.Error("PublicKinds() returned empty slice")
	}

	for _, kind := range kinds {
		if IsPrivateListKind(kind) {
			t.Errorf("PublicKinds() contains %d which IsPrivateListKind returns true for", kind)
		}
	}
}

func TestAllKinds(t *testing.T) {
	all := AllKinds()
	private := PrivateKinds()
	public := PublicKinds()

	expectedLen := len(private) + len(public)
	if len(all) != expectedLen {
		t.Errorf("AllKinds() has %d kinds, expected %d", len(all), expectedLen)
	}

	// Verify all kinds are list kinds
	for _, kind := range all {
		if !IsListKind(kind) {
			t.Errorf("AllKinds() contains %d which IsListKind returns false for", kind)
		}
	}
}

func TestKindName(t *testing.T) {
	// Ensure all list kinds have proper names
	for _, kind := range AllKinds() {
		name := KindName(kind)
		if name == "Unknown List" {
			t.Errorf("KindName(%d) returned 'Unknown List'", kind)
		}
		if name == "" {
			t.Errorf("KindName(%d) returned empty string", kind)
		}
	}

	// Non-list kind should return "Unknown List"
	if name := KindName(999); name != "Unknown List" {
		t.Errorf("KindName(999) = %q, want 'Unknown List'", name)
	}
}

func TestKindConstants(t *testing.T) {
	// Verify kind values match NIP-51 specification
	tests := []struct {
		name  string
		kind  int
		value int
	}{
		{"MuteList", KindMuteList, 10000},
		{"PinList", KindPinList, 10001},
		{"RelayList", KindRelayList, 10002},
		{"BookmarkList", KindBookmarkList, 10003},
		{"CommunitiesList", KindCommunitiesList, 10004},
		{"PublicChatsList", KindPublicChatsList, 10005},
		{"BlockedRelaysList", KindBlockedRelaysList, 10006},
		{"SearchRelaysList", KindSearchRelaysList, 10007},
		{"InterestsList", KindInterestsList, 10015},
		{"EmojiList", KindEmojiList, 10030},
		{"CategorizedPeopleList", KindCategorizedPeopleList, 30000},
		{"CategorizedBookmarkList", KindCategorizedBookmarkList, 30001},
		{"RelaySets", KindRelaySets, 30002},
		{"BookmarkSets", KindBookmarkSets, 30003},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.kind != tt.value {
				t.Errorf("Kind%s = %d, want %d", tt.name, tt.kind, tt.value)
			}
		})
	}
}
