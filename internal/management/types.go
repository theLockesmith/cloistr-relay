package management

import (
	"encoding/json"
	"time"
)

// JSONRPCRequest represents a NIP-86 JSON-RPC request
type JSONRPCRequest struct {
	Method string            `json:"method"`
	Params []json.RawMessage `json:"params"`
}

// JSONRPCResponse represents a NIP-86 JSON-RPC response
type JSONRPCResponse struct {
	Result interface{} `json:"result,omitempty"`
	Error  string      `json:"error,omitempty"`
}

// BannedPubkey represents a banned pubkey entry
type BannedPubkey struct {
	Pubkey    string    `json:"pubkey"`
	Reason    string    `json:"reason,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// AllowedPubkey represents an allowed pubkey entry (whitelist)
type AllowedPubkey struct {
	Pubkey    string    `json:"pubkey"`
	CreatedAt time.Time `json:"created_at"`
}

// BannedEvent represents a banned event entry
type BannedEvent struct {
	EventID   string    `json:"event_id"`
	Reason    string    `json:"reason,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// ModerationItem represents an event in the moderation queue
type ModerationItem struct {
	ID         int64           `json:"id"`
	EventID    string          `json:"event_id"`
	EventJSON  json.RawMessage `json:"event"`
	ReportedAt time.Time       `json:"reported_at"`
	Status     string          `json:"status"`
}

// BlockedIP represents a blocked IP address
type BlockedIP struct {
	IP        string    `json:"ip"`
	Reason    string    `json:"reason,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// RelaySetting represents a key-value relay setting
type RelaySetting struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// SupportedMethodsResponse is the response for supportedmethods
type SupportedMethodsResponse struct {
	Methods []string `json:"methods"`
}

// PubkeyParam is used for methods that take a pubkey as parameter
type PubkeyParam struct {
	Pubkey string `json:"pubkey"`
	Reason string `json:"reason,omitempty"`
}

// EventIDParam is used for methods that take an event ID as parameter
type EventIDParam struct {
	EventID string `json:"event_id"`
	Reason  string `json:"reason,omitempty"`
}

// IPParam is used for methods that take an IP address as parameter
type IPParam struct {
	IP     string `json:"ip"`
	Reason string `json:"reason,omitempty"`
}

// KindParam is used for methods that take a kind number as parameter
type KindParam struct {
	Kind int `json:"kind"`
}

// RelaySettingParam is used for changing relay settings
type RelaySettingParam struct {
	Value string `json:"value"`
}

// ListParams is used for list methods with optional pagination
type ListParams struct {
	Limit  int `json:"limit,omitempty"`
	Offset int `json:"offset,omitempty"`
}

// Supported methods as defined by NIP-86
var SupportedMethods = []string{
	"supportedmethods",
	"banpubkey",
	"listbannedpubkeys",
	"allowpubkey",
	"listallowedpubkeys",
	"listeventsneedingmoderation",
	"allowevent",
	"banevent",
	"listbannedevents",
	"changerelayname",
	"changerelaydescription",
	"changerelayicon",
	"allowkind",
	"disallowkind",
	"listallowedkinds",
	"blockip",
	"unblockip",
	"listblockedips",
}
