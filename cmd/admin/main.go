package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	_ "github.com/lib/pq"

	"git.coldforge.xyz/coldforge/cloistr-relay/internal/admin"
	"git.coldforge.xyz/coldforge/cloistr-relay/internal/management"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Parse config from environment
	port := 8080
	if p := os.Getenv("ADMIN_PORT"); p != "" {
		if v, err := strconv.Atoi(p); err == nil {
			port = v
		}
	}

	adminPubkeys := parseCommaSeparated(os.Getenv("ADMIN_PUBKEYS"))
	if len(adminPubkeys) == 0 {
		log.Fatal("ADMIN_PUBKEYS is required")
	}

	// Connect to database (same DB as relay)
	dbHost := envOrDefault("DB_HOST", "localhost")
	dbPort := envOrDefault("DB_PORT", "5432")
	dbName := envOrDefault("DB_NAME", "nostr")
	dbUser := envOrDefault("DB_USER", "postgres")
	dbPassword := os.Getenv("DB_PASSWORD")

	dsn := fmt.Sprintf("host=%s port=%s dbname=%s user=%s password=%s sslmode=disable",
		dbHost, dbPort, dbName, dbUser, dbPassword)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("Database unreachable: %v", err)
	}

	// Initialize management store
	store := management.NewStore(db)
	if err := store.Init(); err != nil {
		log.Fatalf("Failed to initialize management store: %v", err)
	}

	// Set up admin handler
	handler := admin.NewHandler(store, adminPubkeys)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if err := db.Ping(); err != nil {
			http.Error(w, "db unhealthy", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	addr := fmt.Sprintf(":%d", port)
	log.Printf("Relay admin UI starting on %s", addr)
	log.Printf("Authorized admin pubkeys: %d", len(adminPubkeys))

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("Failed to start admin server: %v", err)
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func parseCommaSeparated(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var result []string
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
