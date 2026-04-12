package auth

import (
	"net/http"
	"strings"
)

type Auth struct {
	editToken  string
	agentToken string
}

func New(editToken, agentToken string) *Auth {
	return &Auth{editToken: editToken, agentToken: agentToken}
}

// IsOwner checks if request carries the edit token
func (a *Auth) IsOwner(r *http.Request) bool {
	if a.editToken == "" {
		return false
	}
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == a.editToken {
			return true
		}
	}
	if r.Header.Get("X-Edit-Token") == a.editToken {
		return true
	}
	cookie, err := r.Cookie("edit_token")
	if err == nil && cookie.Value == a.editToken {
		return true
	}
	return false
}

// IsAgent checks if request carries the agent token
func (a *Auth) IsAgent(r *http.Request) bool {
	if a.agentToken == "" {
		return false
	}
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	return token != "" && token == a.agentToken
}

// RequireOwner — middleware, 401 jeśli nie owner
func (a *Auth) RequireOwner(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.IsOwner(r) {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":"unauthorized — edit token required"}`))
				return
			}
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireAgent — middleware, GET publiczne, POST/PUT/DELETE wymaga agent lub owner
func (a *Auth) RequireAgent(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			next.ServeHTTP(w, r)
			return
		}
		if !a.IsAgent(r) && !a.IsOwner(r) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}
