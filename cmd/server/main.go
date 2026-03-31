package main

import (
	"log"
	"net/http"
	"os"

	"github.com/kapoost/humanmcp-go/internal/auth"
	"github.com/kapoost/humanmcp-go/internal/config"
	"github.com/kapoost/humanmcp-go/internal/content"
	"github.com/kapoost/humanmcp-go/internal/mcp"
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

	a := auth.New(cfg.EditToken)
	mcpHandler := mcp.NewHandler(cfg, store, a)
	webHandler := web.NewHandler(cfg, store, a)

	mux := http.NewServeMux()

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

	if err := http.ListenAndServe(addr, mux); err != nil {
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
