package management

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// BrowserFilter holds event browser query filters
type BrowserFilter struct {
	Pubkey    string
	Kind      string
	Search    string
	StartDate string
	EndDate   string
}

// BrowserEvent represents a single event returned by the event browser
type BrowserEvent struct {
	ID        string    `json:"id"`
	Pubkey    string    `json:"pubkey"`
	Kind      int       `json:"kind"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
	Tags      string    `json:"tags,omitempty"`
	IsBanned  bool      `json:"is_banned"`
}

// BrowserResult holds a paginated event browser response
type BrowserResult struct {
	Events  []BrowserEvent `json:"events"`
	Total   int            `json:"total"`
	Limit   int            `json:"limit"`
	Offset  int            `json:"offset"`
	HasMore bool           `json:"has_more"`
}

// QueryBrowserEvents queries the event table with optional filters and pagination.
// limit is capped at 200; minimum 1.
func (s *Store) QueryBrowserEvents(filters BrowserFilter, limit, offset int) (*BrowserResult, error) {
	db := s.db
	if db == nil {
		return nil, fmt.Errorf("database not available")
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	where := []string{"1=1"}
	args := []interface{}{}
	argNum := 1

	if filters.Pubkey != "" {
		where = append(where, fmt.Sprintf("pubkey = $%d", argNum))
		args = append(args, strings.ToLower(filters.Pubkey))
		argNum++
	}

	if filters.Kind != "" {
		if kind, err := strconv.Atoi(filters.Kind); err == nil {
			where = append(where, fmt.Sprintf("kind = $%d", argNum))
			args = append(args, kind)
			argNum++
		}
	}

	if filters.Search != "" {
		where = append(where, fmt.Sprintf("content ILIKE $%d", argNum))
		args = append(args, "%"+filters.Search+"%")
		argNum++
	}

	if filters.StartDate != "" {
		if t, err := time.Parse("2006-01-02", filters.StartDate); err == nil {
			where = append(where, fmt.Sprintf("created_at >= $%d", argNum))
			args = append(args, t.Unix())
			argNum++
		}
	}

	if filters.EndDate != "" {
		if t, err := time.Parse("2006-01-02", filters.EndDate); err == nil {
			// Include all events up to end of day
			t = t.Add(24*time.Hour - time.Second)
			where = append(where, fmt.Sprintf("created_at <= $%d", argNum))
			args = append(args, t.Unix())
			argNum++
		}
	}

	whereClause := strings.Join(where, " AND ")

	var total int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM event WHERE %s", whereClause)
	if err := db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("count query failed: %w", err)
	}

	// Fetch limit+1 rows to determine hasMore without a second count query
	fetchArgs := make([]interface{}, len(args)+2)
	copy(fetchArgs, args)
	fetchArgs[len(args)] = limit + 1
	fetchArgs[len(args)+1] = offset

	query := fmt.Sprintf(`
		SELECT e.id, e.pubkey, e.kind, e.content, e.created_at, e.tagvalues,
		       EXISTS(SELECT 1 FROM management_banned_events b WHERE b.event_id = e.id) AS is_banned
		FROM event e
		WHERE %s
		ORDER BY e.created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argNum, argNum+1)

	rows, err := db.Query(query, fetchArgs...)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var events []BrowserEvent
	for rows.Next() {
		var e BrowserEvent
		var createdAt int64
		var tags sql.NullString

		if err := rows.Scan(&e.ID, &e.Pubkey, &e.Kind, &e.Content, &createdAt, &tags, &e.IsBanned); err != nil {
			continue
		}
		e.CreatedAt = time.Unix(createdAt, 0)
		if tags.Valid {
			e.Tags = tags.String
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	hasMore := len(events) > limit
	if hasMore {
		events = events[:limit]
	}

	return &BrowserResult{
		Events:  events,
		Total:   total,
		Limit:   limit,
		Offset:  offset,
		HasMore: hasMore,
	}, nil
}

// KindStat holds event count for a specific event kind
type KindStat struct {
	Kind  int   `json:"kind"`
	Count int64 `json:"count"`
}

// StatsResult holds relay event and database statistics
type StatsResult struct {
	TotalEvents  int64      `json:"total_events"`
	DatabaseSize string     `json:"database_size"`
	OldestEvent  *time.Time `json:"oldest_event,omitempty"`
	NewestEvent  *time.Time `json:"newest_event,omitempty"`
	TopKinds     []KindStat `json:"top_kinds"`
}

// QueryStats returns event counts, database size, and top event kinds.
func (s *Store) QueryStats() (*StatsResult, error) {
	db := s.db
	if db == nil {
		return nil, fmt.Errorf("database not available")
	}

	result := &StatsResult{
		TopKinds: []KindStat{},
	}

	if err := db.QueryRow("SELECT COUNT(*) FROM event").Scan(&result.TotalEvents); err != nil {
		return nil, fmt.Errorf("event count failed: %w", err)
	}

	_ = db.QueryRow("SELECT pg_size_pretty(pg_database_size(current_database()))").Scan(&result.DatabaseSize)

	var oldest, newest sql.NullInt64
	_ = db.QueryRow("SELECT MIN(created_at) FROM event").Scan(&oldest)
	_ = db.QueryRow("SELECT MAX(created_at) FROM event").Scan(&newest)
	if oldest.Valid {
		t := time.Unix(oldest.Int64, 0)
		result.OldestEvent = &t
	}
	if newest.Valid {
		t := time.Unix(newest.Int64, 0)
		result.NewestEvent = &t
	}

	rows, err := db.Query(`
		SELECT kind, COUNT(*) AS count
		FROM event
		GROUP BY kind
		ORDER BY count DESC
		LIMIT 10
	`)
	if err == nil {
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var ks KindStat
			if err := rows.Scan(&ks.Kind, &ks.Count); err == nil {
				result.TopKinds = append(result.TopKinds, ks)
			}
		}
	}

	return result, nil
}

// WoTNode represents a node in the WoT graph
type WoTNode struct {
	ID         string `json:"id"`
	Display    string `json:"display"`
	TrustLevel int    `json:"trust_level"`
	Size       int    `json:"size"`
}

// WoTLink represents a directed edge in the WoT graph
type WoTLink struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

// WoTGraph holds the complete WoT graph data (capped at 500 edges)
type WoTGraph struct {
	Nodes []WoTNode `json:"nodes"`
	Links []WoTLink `json:"links"`
}

// FollowEntry holds a pubkey with an associated count
type FollowEntry struct {
	Pubkey string `json:"pubkey"`
	Count  int    `json:"count"`
}

// RecentFollowEntry holds a single follower → followee pair
type RecentFollowEntry struct {
	Follower string `json:"follower"`
	Followee string `json:"followee"`
}

// WoTStats holds aggregate WoT statistics
type WoTStats struct {
	Enabled       bool                `json:"enabled"`
	OwnerPubkey   string              `json:"owner_pubkey,omitempty"`
	TotalFollows  int                 `json:"total_follows"`
	UniquePubkeys int                 `json:"unique_pubkeys"`
	TopFollowed   []FollowEntry       `json:"top_followed"`
	TopFollowers  []FollowEntry       `json:"top_followers"`
	RecentFollows []RecentFollowEntry `json:"recent_follows"`
}

// QueryWoTGraph builds the WoT follow graph, limited to 500 edges.
// ownerPubkey is used to assign trust levels; pass "" when unknown.
func (s *Store) QueryWoTGraph(ownerPubkey string) (*WoTGraph, error) {
	db := s.db
	if db == nil {
		return nil, fmt.Errorf("database not available")
	}

	graph := &WoTGraph{
		Nodes: []WoTNode{},
		Links: []WoTLink{},
	}

	nodeMap := make(map[string]*WoTNode)
	followerCounts := make(map[string]int)

	rows, err := db.Query(`SELECT follower, followee FROM wot_follows LIMIT 500`)
	if err != nil {
		// Return empty graph rather than propagating a missing-table error
		return graph, nil
	}
	defer func() { _ = rows.Close() }()

	var links []WoTLink
	for rows.Next() {
		var follower, followee string
		if err := rows.Scan(&follower, &followee); err != nil {
			continue
		}
		followerCounts[followee]++

		if _, ok := nodeMap[follower]; !ok {
			nodeMap[follower] = &WoTNode{
				ID:      follower,
				Display: wotTruncatePubkey(follower),
			}
		}
		if _, ok := nodeMap[followee]; !ok {
			nodeMap[followee] = &WoTNode{
				ID:      followee,
				Display: wotTruncatePubkey(followee),
			}
		}
		links = append(links, WoTLink{Source: follower, Target: followee})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Resolve trust levels using the owner's first-hop follows
	ownerFollows := make(map[string]bool)
	if ownerPubkey != "" {
		fRows, err := db.Query(`SELECT followee FROM wot_follows WHERE follower = $1`, ownerPubkey)
		if err == nil {
			for fRows.Next() {
				var followee string
				if fRows.Scan(&followee) == nil {
					ownerFollows[followee] = true
				}
			}
			_ = fRows.Close()
		}
	}

	for id, node := range nodeMap {
		switch {
		case id == ownerPubkey:
			node.TrustLevel = 0
		case ownerFollows[id]:
			node.TrustLevel = 1
		default:
			node.TrustLevel = 2
		}
		size := followerCounts[id] + 5
		if size > 30 {
			size = 30
		}
		node.Size = size
		graph.Nodes = append(graph.Nodes, *node)
	}

	graph.Links = links
	return graph, nil
}

// QueryWoTStats returns aggregate statistics about the WoT follow graph.
// ownerPubkey is used to populate the enabled/owner fields; pass "" when unknown.
func (s *Store) QueryWoTStats(ownerPubkey string) (*WoTStats, error) {
	db := s.db
	if db == nil {
		return nil, fmt.Errorf("database not available")
	}

	result := &WoTStats{
		Enabled:       ownerPubkey != "",
		OwnerPubkey:   ownerPubkey,
		TopFollowed:   []FollowEntry{},
		TopFollowers:  []FollowEntry{},
		RecentFollows: []RecentFollowEntry{},
	}

	_ = db.QueryRow(`SELECT COUNT(*) FROM wot_follows`).Scan(&result.TotalFollows)
	_ = db.QueryRow(`
		SELECT COUNT(DISTINCT pubkey) FROM (
			SELECT follower AS pubkey FROM wot_follows
			UNION
			SELECT followee AS pubkey FROM wot_follows
		) AS all_pubkeys
	`).Scan(&result.UniquePubkeys)

	// Most-followed pubkeys
	tfRows, err := db.Query(`
		SELECT followee, COUNT(*) AS follower_count
		FROM wot_follows
		GROUP BY followee
		ORDER BY follower_count DESC
		LIMIT 10
	`)
	if err == nil {
		defer func() { _ = tfRows.Close() }()
		for tfRows.Next() {
			var e FollowEntry
			if err := tfRows.Scan(&e.Pubkey, &e.Count); err == nil {
				result.TopFollowed = append(result.TopFollowed, e)
			}
		}
	}

	// Pubkeys that follow the most others
	flRows, err := db.Query(`
		SELECT follower, COUNT(*) AS follow_count
		FROM wot_follows
		GROUP BY follower
		ORDER BY follow_count DESC
		LIMIT 10
	`)
	if err == nil {
		defer func() { _ = flRows.Close() }()
		for flRows.Next() {
			var e FollowEntry
			if err := flRows.Scan(&e.Pubkey, &e.Count); err == nil {
				result.TopFollowers = append(result.TopFollowers, e)
			}
		}
	}

	// Most recent follow relationships
	rfRows, err := db.Query(`
		SELECT follower, followee
		FROM wot_follows
		ORDER BY updated_at DESC
		LIMIT 10
	`)
	if err == nil {
		defer func() { _ = rfRows.Close() }()
		for rfRows.Next() {
			var rf RecentFollowEntry
			if err := rfRows.Scan(&rf.Follower, &rf.Followee); err == nil {
				result.RecentFollows = append(result.RecentFollows, rf)
			}
		}
	}

	return result, nil
}

// wotTruncatePubkey returns an 8…8 abbreviated display of a pubkey
func wotTruncatePubkey(pubkey string) string {
	if len(pubkey) <= 16 {
		return pubkey
	}
	return pubkey[:8] + "..." + pubkey[len(pubkey)-8:]
}
