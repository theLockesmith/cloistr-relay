package admin

import (
	"fmt"
	"net/http"
	"strconv"
)

// KindsListData holds data for the kinds list partial
type KindsListData struct {
	Items      []int
	TotalCount int
}

func (h *Handler) handleKindsPage(w http.ResponseWriter, r *http.Request) {
	h.renderPage(w, r, "kinds.html", PageData{
		Title:     "Kind Management",
		ActiveNav: "kinds",
	})
}

func (h *Handler) handleListAllowedKinds(w http.ResponseWriter, r *http.Request) {
	kinds, err := h.store.ListAllowedKinds()
	if err != nil {
		h.renderError(w, r, fmt.Sprintf("Failed to list allowed kinds: %v", err), http.StatusInternalServerError)
		return
	}

	data := KindsListData{
		Items:      kinds,
		TotalCount: len(kinds),
	}

	h.renderPartial(w, "allowed_kinds_list.html", data)
}

func (h *Handler) handleAllowKind(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.renderError(w, r, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, "Invalid form data", http.StatusBadRequest)
		return
	}

	kindStr := r.FormValue("kind")

	if kindStr == "" {
		h.renderError(w, r, "Kind number is required", http.StatusBadRequest)
		return
	}

	kind, err := strconv.Atoi(kindStr)
	if err != nil {
		h.renderError(w, r, "Kind must be a valid integer", http.StatusBadRequest)
		return
	}

	if kind < 0 || kind > 65535 {
		h.renderError(w, r, "Kind must be between 0 and 65535", http.StatusBadRequest)
		return
	}

	if err := h.store.AllowKind(kind); err != nil {
		h.renderError(w, r, fmt.Sprintf("Failed to allow kind: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", "refreshAllowedKinds")
	h.renderSuccess(w, fmt.Sprintf("Allowed kind %d", kind))
}

func (h *Handler) handleDisallowKind(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.renderError(w, r, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.renderError(w, r, "Invalid form data", http.StatusBadRequest)
		return
	}

	kindStr := r.FormValue("kind")

	if kindStr == "" {
		h.renderError(w, r, "Kind number is required", http.StatusBadRequest)
		return
	}

	kind, err := strconv.Atoi(kindStr)
	if err != nil {
		h.renderError(w, r, "Kind must be a valid integer", http.StatusBadRequest)
		return
	}

	if err := h.store.DisallowKind(kind); err != nil {
		h.renderError(w, r, fmt.Sprintf("Failed to disallow kind: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Trigger", "refreshAllowedKinds")
	h.renderSuccess(w, fmt.Sprintf("Disallowed kind %d", kind))
}

// getKindDescription returns a human-readable description of a Nostr event kind
func getKindDescription(kind int) string {
	descriptions := map[int]string{
		0:     "Metadata",
		1:     "Short Text Note",
		2:     "Recommend Relay (deprecated)",
		3:     "Contacts",
		4:     "Encrypted DM (deprecated)",
		5:     "Event Deletion",
		6:     "Repost",
		7:     "Reaction",
		8:     "Badge Award",
		9:     "Group Chat Message",
		10:    "Group Chat Threaded Reply",
		11:    "Group Thread",
		12:    "Group Thread Reply",
		13:    "Seal",
		14:    "Direct Message",
		16:    "Generic Repost",
		17:    "Reaction to Website",
		40:    "Channel Creation",
		41:    "Channel Metadata",
		42:    "Channel Message",
		43:    "Channel Hide Message",
		44:    "Channel Mute User",
		1021:  "Bid",
		1022:  "Bid Confirmation",
		1040:  "OpenTimestamps",
		1059:  "Gift Wrap",
		1063:  "File Metadata",
		1311:  "Live Chat Message",
		1617:  "Patches",
		1621:  "Issues",
		1622:  "Replies",
		1971:  "Problem Tracker",
		1984:  "Reporting",
		1985:  "Label",
		4550:  "Community Post Approval",
		5000:  "Job Request (range start)",
		6000:  "Job Result (range start)",
		7000:  "Job Feedback",
		9041:  "Zap Goal",
		9734:  "Zap Request",
		9735:  "Zap Receipt",
		10000: "Mute List",
		10001: "Pin List",
		10002: "Relay List Metadata",
		10003: "Bookmark List",
		10004: "Communities List",
		10005: "Public Chats List",
		10006: "Blocked Relays List",
		10007: "Search Relays List",
		10009: "User Groups List",
		10015: "Interests List",
		10030: "User Emoji List",
		10050: "DM Relay List",
		10096: "File Storage Server List",
		13194: "Wallet Info",
		21000: "Lightning Pub RPC",
		22242: "Client Authentication",
		23194: "Wallet Request",
		23195: "Wallet Response",
		24133: "Nostr Connect",
		27235: "HTTP Auth",
		30000: "Categorized People List",
		30001: "Categorized Bookmark List",
		30002: "Relay Sets",
		30003: "Bookmark Sets",
		30004: "Curation Sets",
		30008: "Profile Badges",
		30009: "Badge Definition",
		30015: "Interest Sets",
		30017: "Create or Update Stall",
		30018: "Create or Update Product",
		30019: "Marketplace UI/UX",
		30020: "Product Sold as Auction",
		30023: "Long-form Content",
		30024: "Draft Long-form",
		30030: "User App Specific Data",
		30040: "Slides",
		30078: "App Specific Data",
		30311: "Live Event",
		30315: "Live User Statuses",
		30402: "Classified Listing",
		30403: "Draft Classified",
		31922: "Date-Based Calendar Event",
		31923: "Time-Based Calendar Event",
		31924: "Calendar",
		31925: "Calendar RSVP",
		31989: "Used App Recommendation",
		31990: "App Recommendation",
		34235: "Video Event",
		34236: "Short Video Event",
		34237: "Video View",
		34550: "Community Definition",
	}

	if desc, ok := descriptions[kind]; ok {
		return desc
	}

	// Ranges
	if kind >= 5000 && kind < 6000 {
		return "Job Request"
	}
	if kind >= 6000 && kind < 7000 {
		return "Job Result"
	}
	if kind >= 10000 && kind < 20000 {
		return "Replaceable Event"
	}
	if kind >= 20000 && kind < 30000 {
		return "Ephemeral Event"
	}
	if kind >= 30000 && kind < 40000 {
		return "Parameterized Replaceable Event"
	}

	return "Unknown"
}
