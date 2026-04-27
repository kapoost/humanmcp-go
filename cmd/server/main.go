package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/kapoost/humanmcp-go/internal/auth"
	"github.com/kapoost/humanmcp-go/internal/config"
	"github.com/kapoost/humanmcp-go/internal/content"
	"github.com/kapoost/humanmcp-go/internal/mcp"
	"github.com/kapoost/humanmcp-go/internal/oauth"
	"github.com/kapoost/humanmcp-go/internal/web"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	if cfg.EditToken == "" {
		log.Println("WARNING: EDIT_TOKEN is not set — owner API is disabled")
	}

	// Ensure content directory exists
	if err := os.MkdirAll(cfg.ContentDir, 0755); err != nil {
		log.Fatalf("content dir: %v", err)
	}

	store := content.NewStore(cfg.ContentDir)
	if err := store.Load(); err != nil {
		log.Printf("initial content load: %v", err)
	}

	a := auth.New(cfg.EditToken, cfg.AgentToken)

	// Shared stores — one instance, wired into both handlers
	sessionCode := content.NewSessionCode(time.Duration(cfg.SessionRotateHours) * time.Hour)
	memoryStore := content.NewMemoryStore(cfg.ContentDir)
	skillStore := content.NewSkillStore(cfg.ContentDir)

	// Listing + subscription stores
	listingStore := content.NewListingStore(cfg.ContentDir)
	subStore := content.NewSubscriptionStore(cfg.ContentDir)
	statStore := content.NewStatStore(cfg.ContentDir)

	// Notifier — delivers webhooks to subscribers
	notifier := content.NewNotifier(listingStore, subStore, statStore, log.Default())
	notifierInterval := parseDurationEnv("NOTIFIER_INTERVAL", time.Minute)
	go notifier.Run(context.Background(), notifierInterval)

	// OAuth 2.1 provider — consent screen uses edit_token as password
	issuer := "https://" + cfg.Domain
	oauthProvider := oauth.NewProvider(issuer, cfg.EditToken)

	mcpHandler := mcp.NewHandler(cfg, store, a, sessionCode, memoryStore, skillStore, oauthProvider, listingStore, subStore)
	webHandler := web.NewHandler(cfg, store, a, sessionCode, memoryStore, skillStore, listingStore, subStore)
	webHandler.SetToolCounter(mcpHandler)

	mux := http.NewServeMux()

	// OAuth endpoints
	oauthProvider.RegisterRoutes(mux)

	// MCP endpoint
	mux.Handle("/mcp", corsMiddleware(mcpHandler))
	mux.Handle("/mcp/", corsMiddleware(mcpHandler))

	// Web UI + REST API
	webHandler.RegisterRoutes(mux)

	addr := cfg.Host + ":" + cfg.Port
	log.Printf("humanMCP starting on %s", addr)
	log.Printf("  author:  %s", cfg.AuthorName)
	log.Printf("  domain:  %s", cfg.Domain)
	log.Printf("  content: %s", cfg.ContentDir)
	log.Printf("  mcp:     http://%s/mcp", addr)

	if err := http.ListenAndServe(addr, secureMiddleware(mux)); err != nil {
		log.Fatalf("server: %v", err)
	}
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Edit-Token")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func parseDurationEnv(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}

// secureMiddleware adds security + cache headers to all responses
func secureMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Security headers
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		// Cache: MCP endpoints never cache, HTML pages short TTL
		if r.URL.Path == "/mcp" || strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}
