package auth

import (
	"net/http"
	"strings"
)

type Auth struct {
	editToken string
}

func New(editToken string) *Auth {
	return &Auth{editToken: editToken}
}

// IsOwner checks if request carries the edit token
func (a *Auth) IsOwner(r *http.Request) bool {
	if a.editToken == "" {
		return false
	}
	// Check Authorization: Bearer <token>
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == a.editToken {
			return true
		}
	}
	// Check X-Edit-Token header
	if r.Header.Get("X-Edit-Token") == a.editToken {
		return true
	}
	// Check cookie (for web UI)
	cookie, err := r.Cookie("edit_token")
	if err == nil && cookie.Value == a.editToken {
		return true
	}
	return false
}

// RequireOwner is middleware that 401s if not owner (JSON for API, redirect for browser)
// RequireAgent sprawdza Authorization: Bearer <AGENT_TOKEN> w nagłówku.
// Używany przez endpointy które trusted agenci mogą zapisywać.
func (a *Auth) RequireAgent(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token == "" || token == a.agentToken || a.IsOwner(r) {
			// Brak tokenu = tylko GET (publiczne), agent token lub owner = pełny dostęp
			if r.Method != http.MethodGet && token != a.agentToken && !a.IsOwner(r) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":"unauthorized"}`))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// IsAgent sprawdza czy request pochodzi od trusted agenta.
func (a *Auth) IsAgent(r *http.Request) bool {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	return token != "" && token == a.agentToken
}

func (a *Auth) RequireOwner(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.IsOwner(r) {
			// API paths get JSON error
			if strings.HasPrefix(r.URL.Path, "/api/") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":"unauthorized — edit token required"}`))
				return
			}
			// Browser pages get redirect to login
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}
