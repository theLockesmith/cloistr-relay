package management

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/fiatjaf/khatru"
	"github.com/nbd-wtf/go-nostr"
)

// Handler handles NIP-86 management API requests
type Handler struct {
	store        *Store
	methods      *MethodHandler
	adminPubkeys []string
}

// NewHandler creates a new NIP-86 management handler
func NewHandler(store *Store, adminPubkeys []string) *Handler {
	return &Handler{
		store:        store,
		methods:      NewMethodHandler(store),
		adminPubkeys: adminPubkeys,
	}
}

// ServeHTTP implements http.Handler for the management endpoint
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Only accept POST requests
	if r.Method != http.MethodPost {
		writeError(w, "error", "method not allowed")
		return
	}

	// Validate Content-Type
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/nostr+json+rpc" {
		writeError(w, "error", "invalid content-type, expected application/nostr+json+rpc")
		return
	}

	// Validate NIP-98 authentication
	pubkey, err := ValidateNIP98Auth(r, h.adminPubkeys)
	if err != nil {
		log.Printf("Management auth failed: %v", err)
		writeAuthError(w, err)
		return
	}

	// Parse JSON-RPC request
	var req JSONRPCRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "error", "invalid JSON-RPC request")
		return
	}

	log.Printf("Management request from %s: %s", pubkey, req.Method)

	// Dispatch to method handler
	result, err := h.methods.Dispatch(req.Method, req.Params)
	if err != nil {
		writeError(w, "error", err.Error())
		return
	}

	// Write success response
	writeSuccess(w, result)
}

// writeSuccess writes a successful JSON-RPC response
func writeSuccess(w http.ResponseWriter, result interface{}) {
	w.Header().Set("Content-Type", "application/nostr+json+rpc")
	json.NewEncoder(w).Encode(JSONRPCResponse{
		Result: result,
	})
}

// writeError writes an error JSON-RPC response
func writeError(w http.ResponseWriter, errType, message string) {
	w.Header().Set("Content-Type", "application/nostr+json+rpc")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(JSONRPCResponse{
		Error: message,
	})
}

// writeAuthError writes an authentication error response
func writeAuthError(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/nostr+json+rpc")
	w.WriteHeader(http.StatusUnauthorized)

	errMsg := "auth-required: " + err.Error()
	json.NewEncoder(w).Encode(JSONRPCResponse{
		Error: errMsg,
	})
}

// RegisterBanHandlers registers event rejection handlers for banned pubkeys and events
func RegisterBanHandlers(relay *khatru.Relay, store *Store) {
	// Reject events from banned pubkeys
	relay.RejectEvent = append(relay.RejectEvent, func(ctx context.Context, event *nostr.Event) (bool, string) {
		if store.IsPubkeyBanned(event.PubKey) {
			return true, "blocked: pubkey banned"
		}
		return false, ""
	})

	// Reject banned events by ID (for deletion/moderation)
	relay.RejectEvent = append(relay.RejectEvent, func(ctx context.Context, event *nostr.Event) (bool, string) {
		if store.IsEventBanned(event.ID) {
			return true, "blocked: event banned"
		}
		return false, ""
	})

	// Reject events of disallowed kinds (if kind restrictions are enabled)
	relay.RejectEvent = append(relay.RejectEvent, func(ctx context.Context, event *nostr.Event) (bool, string) {
		if !store.IsKindAllowed(event.Kind) {
			return true, "blocked: event kind not allowed"
		}
		return false, ""
	})

	log.Println("NIP-86 ban checking handlers registered")
}
