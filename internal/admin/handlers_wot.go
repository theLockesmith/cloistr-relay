package admin

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"git.coldforge.xyz/coldforge/cloistr-relay/internal/wot"
)

// WoTVisualizationData holds data for the WoT visualization page
type WoTVisualizationData struct {
	Enabled         bool
	OwnerPubkey     string
	OwnerDisplay    string
	TotalFollows    int
	UniquePubkeys   int
	TrustDistribution []TrustLevelCount
	TopFollowed     []FollowedPubkey
	TopFollowers    []FollowerPubkey
	RecentFollows   []RecentFollow
}

// TrustLevelCount holds count per trust level
type TrustLevelCount struct {
	Level int
	Name  string
	Count int64
}

// FollowedPubkey holds a pubkey and their follower count
type FollowedPubkey struct {
	Pubkey        string
	Display       string
	FollowerCount int
}

// FollowerPubkey holds a pubkey and who they follow
type FollowerPubkey struct {
	Pubkey      string
	Display     string
	FollowCount int
}

// RecentFollow holds a recent follow relationship
type RecentFollow struct {
	Follower        string
	FollowerDisplay string
	Followee        string
	FolloweeDisplay string
}

// GraphData holds data for D3.js visualization
type GraphData struct {
	Nodes []GraphNode `json:"nodes"`
	Links []GraphLink `json:"links"`
}

// GraphNode represents a node in the graph
type GraphNode struct {
	ID         string `json:"id"`
	Display    string `json:"display"`
	TrustLevel int    `json:"trustLevel"`
	Size       int    `json:"size"`
}

// GraphLink represents an edge in the graph
type GraphLink struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

func (h *Handler) handleWoTPage(w http.ResponseWriter, r *http.Request) {
	data := h.buildWoTVisualization()

	h.renderPage(w, r, "wot.html", PageData{
		Title:     "Web of Trust",
		ActiveNav: "wot",
		Content:   data,
	})
}

func (h *Handler) handleWoTStats(w http.ResponseWriter, r *http.Request) {
	data := h.buildWoTVisualization()
	h.renderPartial(w, "wot_stats.html", data)
}

func (h *Handler) handleWoTGraphData(w http.ResponseWriter, r *http.Request) {
	graph := h.buildWoTGraph()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(graph)
}

func (h *Handler) buildWoTVisualization() WoTVisualizationData {
	data := WoTVisualizationData{
		Enabled: false,
	}

	// Check if WoT is configured via HAVEN
	if h.havenConfig != nil && h.havenConfig.OwnerPubkey != "" {
		data.Enabled = true
		data.OwnerPubkey = h.havenConfig.OwnerPubkey
		data.OwnerDisplay = truncatePubkey(h.havenConfig.OwnerPubkey)
	}

	db := h.store.DB()
	if db == nil {
		return data
	}

	// Get total follow count
	_ = db.QueryRow(`SELECT COUNT(*) FROM wot_follows`).Scan(&data.TotalFollows)

	// Get unique pubkey count
	_ = db.QueryRow(`
		SELECT COUNT(DISTINCT pubkey) FROM (
			SELECT follower AS pubkey FROM wot_follows
			UNION
			SELECT followee AS pubkey FROM wot_follows
		) AS all_pubkeys
	`).Scan(&data.UniquePubkeys)

	// Get top followed pubkeys
	data.TopFollowed = h.getTopFollowed(db, 10)

	// Get top followers (most follows)
	data.TopFollowers = h.getTopFollowers(db, 10)

	// Get recent follows
	data.RecentFollows = h.getRecentFollows(db, 10)

	return data
}

func (h *Handler) getTopFollowed(db *sql.DB, limit int) []FollowedPubkey {
	rows, err := db.Query(`
		SELECT followee, COUNT(*) as follower_count
		FROM wot_follows
		GROUP BY followee
		ORDER BY follower_count DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()

	var result []FollowedPubkey
	for rows.Next() {
		var fp FollowedPubkey
		if err := rows.Scan(&fp.Pubkey, &fp.FollowerCount); err != nil {
			continue
		}
		fp.Display = truncatePubkey(fp.Pubkey)
		result = append(result, fp)
	}
	return result
}

func (h *Handler) getTopFollowers(db *sql.DB, limit int) []FollowerPubkey {
	rows, err := db.Query(`
		SELECT follower, COUNT(*) as follow_count
		FROM wot_follows
		GROUP BY follower
		ORDER BY follow_count DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()

	var result []FollowerPubkey
	for rows.Next() {
		var fp FollowerPubkey
		if err := rows.Scan(&fp.Pubkey, &fp.FollowCount); err != nil {
			continue
		}
		fp.Display = truncatePubkey(fp.Pubkey)
		result = append(result, fp)
	}
	return result
}

func (h *Handler) getRecentFollows(db *sql.DB, limit int) []RecentFollow {
	rows, err := db.Query(`
		SELECT follower, followee
		FROM wot_follows
		ORDER BY updated_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()

	var result []RecentFollow
	for rows.Next() {
		var rf RecentFollow
		if err := rows.Scan(&rf.Follower, &rf.Followee); err != nil {
			continue
		}
		rf.FollowerDisplay = truncatePubkey(rf.Follower)
		rf.FolloweeDisplay = truncatePubkey(rf.Followee)
		result = append(result, rf)
	}
	return result
}

func (h *Handler) buildWoTGraph() GraphData {
	graph := GraphData{
		Nodes: []GraphNode{},
		Links: []GraphLink{},
	}

	db := h.store.DB()
	if db == nil {
		return graph
	}

	// Get owner pubkey for trust level coloring
	ownerPubkey := ""
	if h.havenConfig != nil {
		ownerPubkey = h.havenConfig.OwnerPubkey
	}

	// Build node map
	nodeMap := make(map[string]*GraphNode)
	followerCounts := make(map[string]int)

	// Get all follows (limited to prevent huge graphs)
	rows, err := db.Query(`
		SELECT follower, followee FROM wot_follows LIMIT 500
	`)
	if err != nil {
		return graph
	}
	defer func() { _ = rows.Close() }()

	var links []GraphLink
	for rows.Next() {
		var follower, followee string
		if err := rows.Scan(&follower, &followee); err != nil {
			continue
		}

		// Count for sizing
		followerCounts[followee]++

		// Create nodes if not exist
		if _, ok := nodeMap[follower]; !ok {
			nodeMap[follower] = &GraphNode{
				ID:      follower,
				Display: truncatePubkey(follower),
			}
		}
		if _, ok := nodeMap[followee]; !ok {
			nodeMap[followee] = &GraphNode{
				ID:      followee,
				Display: truncatePubkey(followee),
			}
		}

		links = append(links, GraphLink{
			Source: follower,
			Target: followee,
		})
	}

	// Calculate trust levels and sizes
	ownerFollows := make(map[string]bool)
	if ownerPubkey != "" {
		// Get owner's follows for trust level 1
		followRows, err := db.Query(`SELECT followee FROM wot_follows WHERE follower = $1`, ownerPubkey)
		if err == nil {
			for followRows.Next() {
				var followee string
				if followRows.Scan(&followee) == nil {
					ownerFollows[followee] = true
				}
			}
			_ = followRows.Close()
		}
	}

	// Build final node list with trust levels
	for id, node := range nodeMap {
		if id == ownerPubkey {
			node.TrustLevel = 0 // Owner
		} else if ownerFollows[id] {
			node.TrustLevel = 1 // Followed by owner
		} else {
			node.TrustLevel = 2 // Follow of follow or unknown
		}
		node.Size = followerCounts[id] + 5 // Base size + followers
		if node.Size > 30 {
			node.Size = 30 // Cap size
		}
		graph.Nodes = append(graph.Nodes, *node)
	}

	graph.Links = links
	return graph
}

// SetWoTStore sets the WoT store reference for visualization
func (h *Handler) SetWoTStore(store *wot.Store) {
	// Store reference if needed for advanced queries
	// For now, we use direct SQL queries via h.store.DB()
}
