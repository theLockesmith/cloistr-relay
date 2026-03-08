// Package lists implements NIP-51 list event handling
//
// NIP-51 defines standard list kinds for organizing pubkeys, events, relays,
// and other references. Lists can be public or private (encrypted).
//
// Public list kinds (10000-10999):
//   - 10000: Mute list (private - users don't want others to know who they muted)
//   - 10001: Pin list (curated public content)
//   - 10002: Relay list metadata (NIP-65)
//   - 10003: Bookmark list (private)
//   - 10004: Communities list
//   - 10005: Public chats list
//   - 10006: Blocked relays list (private)
//   - 10007: Search relays list
//   - 10015: Interests list
//   - 10030: User emoji list
//
// Parameterized list kinds (30000-30999):
//   - 30000: Categorized people list (follow sets, can include mutes)
//   - 30001: Categorized bookmark list
//   - 30002: Relay sets
//   - 30003: Bookmark sets (private)
//
// Reference: https://github.com/nostr-protocol/nips/blob/master/51.md
package lists

// Event kinds defined by NIP-51
const (
	// KindMuteList (10000) stores muted pubkeys and event IDs
	// Tags: p (pubkey), e (event), t (hashtag), word (muted word)
	// Privacy: Private (encrypted content field, public tags for client-side filtering)
	KindMuteList = 10000

	// KindPinList (10001) stores pinned/highlighted events
	// Tags: e (event to pin)
	// Privacy: Public
	KindPinList = 10001

	// KindRelayList (10002) stores user's relay preferences (NIP-65)
	// Tags: r (relay URL with optional read/write marker)
	// Privacy: Public
	KindRelayList = 10002

	// KindBookmarkList (10003) stores bookmarked events
	// Tags: e (event), a (addressable event reference)
	// Privacy: Private (content can be encrypted)
	KindBookmarkList = 10003

	// KindCommunitiesList (10004) stores communities the user follows
	// Tags: a (community references like 34550:pubkey:community-name)
	// Privacy: Public
	KindCommunitiesList = 10004

	// KindPublicChatsList (10005) stores public chat channels the user is in
	// Tags: e (channel root event)
	// Privacy: Public
	KindPublicChatsList = 10005

	// KindBlockedRelaysList (10006) stores relays the user wants to avoid
	// Tags: r (relay URL)
	// Privacy: Private
	KindBlockedRelaysList = 10006

	// KindSearchRelaysList (10007) stores preferred relays for search queries
	// Tags: r (relay URL supporting NIP-50)
	// Privacy: Public
	KindSearchRelaysList = 10007

	// KindInterestsList (10015) stores topics/hashtags the user is interested in
	// Tags: t (hashtag), a (topic reference)
	// Privacy: Public
	KindInterestsList = 10015

	// KindEmojiList (10030) stores user's custom emoji
	// Tags: emoji (shortcode, URL pairs)
	// Privacy: Public
	KindEmojiList = 10030

	// KindCategorizedPeopleList (30000) stores categorized pubkey lists
	// Tags: d (category name like "friends", "family", "mutes"), p (pubkeys)
	// Privacy: Depends on category (can be encrypted)
	KindCategorizedPeopleList = 30000

	// KindCategorizedBookmarkList (30001) stores categorized bookmarks
	// Tags: d (category name), e (events), a (addressable events)
	// Privacy: Private
	KindCategorizedBookmarkList = 30001

	// KindRelaySets (30002) stores categorized relay groups
	// Tags: d (set name like "fast", "free-speech", "local"), r (relay URLs)
	// Privacy: Public
	KindRelaySets = 30002

	// KindBookmarkSets (30003) stores named bookmark collections
	// Tags: d (set name), e (events), a (addressable events)
	// Privacy: Private
	KindBookmarkSets = 30003
)

// PrivateKinds returns list kinds that should be stored privately
// These contain sensitive information the owner may not want public
func PrivateKinds() []int {
	return []int{
		KindMuteList,               // Don't reveal who user muted
		KindBookmarkList,           // Personal bookmarks
		KindBlockedRelaysList,      // Private relay preferences
		KindCategorizedPeopleList,  // Can contain mute categories
		KindCategorizedBookmarkList, // Personal categorized bookmarks
		KindBookmarkSets,           // Personal bookmark sets
	}
}

// PublicKinds returns list kinds that are typically public
func PublicKinds() []int {
	return []int{
		KindPinList,           // Curated public content
		KindRelayList,         // NIP-65 relay preferences
		KindCommunitiesList,   // Communities user follows
		KindPublicChatsList,   // Public chats user is in
		KindSearchRelaysList,  // Search relay preferences
		KindInterestsList,     // Topics of interest
		KindEmojiList,         // Custom emoji
		KindRelaySets,         // Relay sets
	}
}

// AllKinds returns all NIP-51 list kinds
func AllKinds() []int {
	return append(PrivateKinds(), PublicKinds()...)
}

// IsListKind returns true if the kind is a NIP-51 list kind
func IsListKind(kind int) bool {
	switch kind {
	case KindMuteList, KindPinList, KindRelayList, KindBookmarkList,
		KindCommunitiesList, KindPublicChatsList, KindBlockedRelaysList,
		KindSearchRelaysList, KindInterestsList, KindEmojiList,
		KindCategorizedPeopleList, KindCategorizedBookmarkList,
		KindRelaySets, KindBookmarkSets:
		return true
	default:
		return false
	}
}

// IsPrivateListKind returns true if the list kind should be private
func IsPrivateListKind(kind int) bool {
	switch kind {
	case KindMuteList, KindBookmarkList, KindBlockedRelaysList,
		KindCategorizedPeopleList, KindCategorizedBookmarkList, KindBookmarkSets:
		return true
	default:
		return false
	}
}

// IsReplaceableListKind returns true if the kind is a replaceable list (10xxx)
func IsReplaceableListKind(kind int) bool {
	return kind >= 10000 && kind < 20000 && IsListKind(kind)
}

// IsParameterizedListKind returns true if the kind is a parameterized replaceable list (30xxx)
func IsParameterizedListKind(kind int) bool {
	return kind >= 30000 && kind < 40000 && IsListKind(kind)
}

// KindName returns a human-readable name for the list kind
func KindName(kind int) string {
	switch kind {
	case KindMuteList:
		return "Mute List"
	case KindPinList:
		return "Pin List"
	case KindRelayList:
		return "Relay List"
	case KindBookmarkList:
		return "Bookmark List"
	case KindCommunitiesList:
		return "Communities List"
	case KindPublicChatsList:
		return "Public Chats List"
	case KindBlockedRelaysList:
		return "Blocked Relays List"
	case KindSearchRelaysList:
		return "Search Relays List"
	case KindInterestsList:
		return "Interests List"
	case KindEmojiList:
		return "Emoji List"
	case KindCategorizedPeopleList:
		return "Categorized People List"
	case KindCategorizedBookmarkList:
		return "Categorized Bookmark List"
	case KindRelaySets:
		return "Relay Sets"
	case KindBookmarkSets:
		return "Bookmark Sets"
	default:
		return "Unknown List"
	}
}
