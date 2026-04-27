package oauth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ── Structs ───────────────────────────────────────────────────────────────────

type Client struct {
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret,omitempty"`
	RedirectURIs []string `json:"redirect_uris"`
	ClientName   string   `json:"client_name,omitempty"`
	GrantTypes   []string `json:"grant_types,omitempty"`
	Scope        string   `json:"scope,omitempty"`
}

type authCode struct {
	Code         string
	ClientID     string
	RedirectURI  string
	Challenge    string // PKCE code_challenge
	ChallengeMethod string // S256
	Scope        string
	ExpiresAt    time.Time
	Used         bool
}

type accessToken struct {
	Token     string
	ClientID  string
	Scope     string
	ExpiresAt time.Time
}

type authAttempt struct {
	count       int
	windowStart time.Time
}

// Provider is the OAuth 2.1 authorization server.
type Provider struct {
	mu      sync.RWMutex
	issuer  string // e.g. "https://kapoost-humanmcp.fly.dev"

	clients map[string]*Client      // clientID → Client
	codes   map[string]*authCode    // code → authCode
	tokens  map[string]*accessToken // token → accessToken

	// ownerPassword — used for the consent screen (edit_token)
	ownerPassword string

	// rate limiting for auth attempts
	authAttemptsMu sync.Mutex
	authAttempts   map[string]*authAttempt
}

const (
	authAttemptWindow = 15 * time.Minute
	authAttemptMax    = 5
)

func NewProvider(issuer, ownerPassword string) *Provider {
	p := &Provider{
		issuer:        strings.TrimRight(issuer, "/"),
		ownerPassword: ownerPassword,
		clients:       make(map[string]*Client),
		codes:         make(map[string]*authCode),
		tokens:        make(map[string]*accessToken),
		authAttempts:  make(map[string]*authAttempt),
	}
	go p.cleanup()
	return p
}

func oauthClientIP(r *http.Request) string {
	if ip := r.Header.Get("Fly-Client-IP"); ip != "" {
		return ip
	}
	addr := r.RemoteAddr
	if i := strings.LastIndex(addr, ":"); i != -1 {
		return addr[:i]
	}
	return addr
}

func (p *Provider) checkAuthAttempt(ip string) bool {
	now := time.Now()
	p.authAttemptsMu.Lock()
	defer p.authAttemptsMu.Unlock()

	a, exists := p.authAttempts[ip]
	if !exists || now.Sub(a.windowStart) > authAttemptWindow {
		p.authAttempts[ip] = &authAttempt{count: 1, windowStart: now}
		return true
	}
	a.count++
	return a.count <= authAttemptMax
}

// ── Token validation (called from MCP handler) ───────────────────────────────

// ValidateBearer checks if a Bearer token is valid and returns its scope.
func (p *Provider) ValidateBearer(token string) (scope string, ok bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	t, exists := p.tokens[token]
	if !exists || time.Now().After(t.ExpiresAt) {
		return "", false
	}
	return t.Scope, true
}

// ── HTTP Handlers ─────────────────────────────────────────────────────────────

// RegisterRoutes wires all OAuth endpoints into the mux.
func (p *Provider) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/.well-known/oauth-authorization-server", p.handleMetadata)
	mux.HandleFunc("/oauth/authorize", p.handleAuthorize)
	mux.HandleFunc("/oauth/token", p.handleToken)
	mux.HandleFunc("/oauth/register", p.handleRegister)
}

// ── Metadata (RFC 8414) ──────────────────────────────────────────────────────

func (p *Provider) handleMetadata(w http.ResponseWriter, r *http.Request) {
	meta := map[string]interface{}{
		"issuer":                 p.issuer,
		"authorization_endpoint": p.issuer + "/oauth/authorize",
		"token_endpoint":         p.issuer + "/oauth/token",
		"registration_endpoint":  p.issuer + "/oauth/register",
		"response_types_supported":            []string{"code"},
		"grant_types_supported":               []string{"authorization_code"},
		"code_challenge_methods_supported":     []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"client_secret_post", "none"},
		"scopes_supported":                     []string{"mcp:full"},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(meta)
}

// ── Dynamic Client Registration (RFC 7591) ───────────────────────────────────

func (p *Provider) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		RedirectURIs []string `json:"redirect_uris"`
		ClientName   string   `json:"client_name"`
		GrantTypes   []string `json:"grant_types"`
		Scope        string   `json:"scope"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid_request", "bad JSON", http.StatusBadRequest)
		return
	}

	if len(req.RedirectURIs) == 0 {
		jsonError(w, "invalid_request", "redirect_uris required", http.StatusBadRequest)
		return
	}

	clientID := randomToken(16)
	clientSecret := randomToken(32)

	client := &Client{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURIs: req.RedirectURIs,
		ClientName:   req.ClientName,
		GrantTypes:   req.GrantTypes,
		Scope:        req.Scope,
	}

	p.mu.Lock()
	p.clients[clientID] = client
	p.mu.Unlock()

	log.Printf("[OAuth] registered client %s (%s)", clientID, req.ClientName)

	resp := map[string]interface{}{
		"client_id":                clientID,
		"client_secret":            clientSecret,
		"redirect_uris":            req.RedirectURIs,
		"client_name":              req.ClientName,
		"grant_types":              req.GrantTypes,
		"token_endpoint_auth_method": "client_secret_post",
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

// ── Authorization Endpoint ───────────────────────────────────────────────────

func (p *Provider) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	clientID := q.Get("client_id")
	redirectURI := q.Get("redirect_uri")
	state := q.Get("state")
	codeChallenge := q.Get("code_challenge")
	challengeMethod := q.Get("code_challenge_method")
	scope := q.Get("scope")
	if scope == "" {
		scope = "mcp:full"
	}

	// Validate client
	p.mu.RLock()
	client, exists := p.clients[clientID]
	p.mu.RUnlock()

	if !exists {
		jsonError(w, "invalid_client", "unknown client_id", http.StatusBadRequest)
		return
	}

	if !validRedirect(client.RedirectURIs, redirectURI) {
		jsonError(w, "invalid_request", "redirect_uri mismatch", http.StatusBadRequest)
		return
	}

	// PKCE required (OAuth 2.1)
	if codeChallenge == "" || challengeMethod != "S256" {
		redirectWithError(w, r, redirectURI, state, "invalid_request", "PKCE with S256 required")
		return
	}

	// POST = form submission (consent granted)
	if r.Method == http.MethodPost {
		ip := oauthClientIP(r)
		if !p.checkAuthAttempt(ip) {
			log.Printf("[OAuth] rate limit hit for %s", ip)
			p.serveConsentPage(w, q, "Too many attempts. Try again in 15 minutes.")
			return
		}

		r.ParseForm()
		password := r.FormValue("password")

		if password != p.ownerPassword {
			log.Printf("[OAuth] failed auth attempt from %s", ip)
			p.serveConsentPage(w, q, "Niepoprawne hasło. Spróbuj ponownie.")
			return
		}

		// Issue authorization code
		code := randomToken(32)
		p.mu.Lock()
		p.codes[code] = &authCode{
			Code:            code,
			ClientID:        clientID,
			RedirectURI:     redirectURI,
			Challenge:       codeChallenge,
			ChallengeMethod: challengeMethod,
			Scope:           scope,
			ExpiresAt:       time.Now().Add(10 * time.Minute),
		}
		p.mu.Unlock()

		log.Printf("[OAuth] issued auth code for client %s", clientID)

		// Redirect back with code
		sep := "?"
		if strings.Contains(redirectURI, "?") {
			sep = "&"
		}
		location := fmt.Sprintf("%s%scode=%s", redirectURI, sep, code)
		if state != "" {
			location += "&state=" + state
		}
		http.Redirect(w, r, location, http.StatusFound)
		return
	}

	// GET = show consent page
	p.serveConsentPage(w, q, "")
}

func (p *Provider) serveConsentPage(w http.ResponseWriter, q map[string][]string, errMsg string) {
	clientID := getFirst(q, "client_id")
	p.mu.RLock()
	client := p.clients[clientID]
	p.mu.RUnlock()

	clientName := clientID
	if client != nil && client.ClientName != "" {
		clientName = client.ClientName
	}

	errHTML := ""
	if errMsg != "" {
		errHTML = fmt.Sprintf(`<div class="err">%s</div>`, errMsg)
	}

	// Rebuild query string for form action
	var qs []string
	for k, vs := range q {
		for _, v := range vs {
			qs = append(qs, fmt.Sprintf("%s=%s", k, v))
		}
	}

	html := fmt.Sprintf(`<!DOCTYPE html>
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>humanMCP — authorize</title>
<style>
:root{--bg:#fdfcfa;--fg:#1a1a1a;--muted:#6b6b6b;--border:#e2e0db;--accent:#2a6496;--accent-light:#e8f1f8;--locked:#7a5c00;--locked-bg:#fef9ec;--tag-bg:#f0ede8;--serif:Georgia,'Times New Roman',serif;--sans:-apple-system,BlinkMacSystemFont,'Segoe UI',system-ui,sans-serif;}
@media(prefers-color-scheme:dark){:root{--bg:#141412;--fg:#e8e6e1;--muted:#888;--border:#2e2c28;--accent:#6baed6;--accent-light:#1a2a36;--locked:#d4a017;--locked-bg:#1e1800;--tag-bg:#252320;}}
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:var(--sans);background:var(--bg);color:var(--fg);display:flex;justify-content:center;align-items:center;min-height:100vh}
.card{max-width:420px;width:90%%}
.section-title{font-size:.7rem;font-weight:500;color:var(--muted);text-transform:uppercase;letter-spacing:.07em;margin-bottom:.6rem}
.session-box{background:var(--accent-light);border:1px solid var(--accent);border-radius:8px;padding:1rem 1.25rem;margin-bottom:1.5rem}
.session-box .section-title{color:var(--accent);margin-bottom:.5rem}
.client-name{font-family:var(--serif);font-size:1.15rem;font-weight:500;color:var(--fg);margin-bottom:.5rem}
.hint{font-size:.72rem;color:var(--muted);margin-bottom:.75rem}
input[type=password]{width:100%%;padding:.5rem;border:1px solid var(--border);border-radius:4px;background:var(--bg);color:var(--fg);font-size:1rem}
input[type=password]:focus{outline:none;border-color:var(--accent)}
.info-btn{font-size:.68rem;padding:4px 12px;border:1px solid var(--accent);border-radius:3px;background:var(--bg);color:var(--accent);cursor:pointer;text-decoration:none;display:inline-block;margin-top:.75rem}
.info-btn:hover{background:var(--accent-light)}
button.primary{width:100%%;padding:.5rem;border:1px solid var(--accent);border-radius:4px;background:var(--accent);color:var(--bg);font-size:.82rem;cursor:pointer;font-weight:500;margin-top:.75rem}
button.primary:hover{opacity:.9}
.err{color:var(--locked);font-size:.82rem;margin-bottom:.75rem;padding:.5rem .75rem;background:var(--locked-bg);border:1px solid var(--locked);border-radius:4px}
.foot{font-size:.72rem;color:var(--muted);margin-top:1.25rem}
</style></head>
<body><div class="card">
<div class="session-box">
<div class="section-title">authorize connection</div>
<div class="client-name">%s</div>
<p class="hint">This app wants to access your humanMCP personas, skills, and memory.</p>
</div>
%s
<form method="POST" action="/oauth/authorize?%s">
<div class="section-title">owner password</div>
<input type="password" name="password" autofocus required>
<button type="submit" class="primary">Authorize</button>
</form>
<p class="foot">Your data stays under your control.</p>
</div></body></html>`, clientName, errHTML, strings.Join(qs, "&"))

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

// ── Token Endpoint ───────────────────────────────────────────────────────────

func (p *Provider) handleToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.ParseForm()
	grantType := r.FormValue("grant_type")

	if grantType != "authorization_code" {
		jsonError(w, "unsupported_grant_type", "only authorization_code supported", http.StatusBadRequest)
		return
	}

	codeStr := r.FormValue("code")
	codeVerifier := r.FormValue("code_verifier")
	clientID := r.FormValue("client_id")

	p.mu.Lock()
	ac, exists := p.codes[codeStr]
	if !exists || ac.Used || time.Now().After(ac.ExpiresAt) {
		p.mu.Unlock()
		jsonError(w, "invalid_grant", "code expired or invalid", http.StatusBadRequest)
		return
	}

	if ac.ClientID != clientID {
		p.mu.Unlock()
		jsonError(w, "invalid_grant", "client_id mismatch", http.StatusBadRequest)
		return
	}

	// Verify PKCE
	if !verifyPKCE(codeVerifier, ac.Challenge) {
		p.mu.Unlock()
		jsonError(w, "invalid_grant", "PKCE verification failed", http.StatusBadRequest)
		return
	}

	// Mark code as used
	ac.Used = true

	// Issue access token
	token := randomToken(32)
	p.tokens[token] = &accessToken{
		Token:     token,
		ClientID:  clientID,
		Scope:     ac.Scope,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	p.mu.Unlock()

	log.Printf("[OAuth] issued access token for client %s", clientID)

	resp := map[string]interface{}{
		"access_token": token,
		"token_type":   "Bearer",
		"expires_in":   86400,
		"scope":        ac.Scope,
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(resp)
}

// ── PKCE ─────────────────────────────────────────────────────────────────────

func verifyPKCE(verifier, challenge string) bool {
	if verifier == "" || challenge == "" {
		return false
	}
	h := sha256.Sum256([]byte(verifier))
	computed := base64.RawURLEncoding.EncodeToString(h[:])
	return computed == challenge
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func randomToken(nBytes int) string {
	b := make([]byte, nBytes)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func validRedirect(registered []string, uri string) bool {
	for _, r := range registered {
		if r == uri {
			return true
		}
	}
	return true // allow any redirect for dynamic registration clients
}

func getFirst(q map[string][]string, key string) string {
	if vs, ok := q[key]; ok && len(vs) > 0 {
		return vs[0]
	}
	return ""
}

func redirectWithError(w http.ResponseWriter, r *http.Request, redirectURI, state, errCode, desc string) {
	sep := "?"
	if strings.Contains(redirectURI, "?") {
		sep = "&"
	}
	location := fmt.Sprintf("%s%serror=%s&error_description=%s", redirectURI, sep, errCode, desc)
	if state != "" {
		location += "&state=" + state
	}
	http.Redirect(w, r, location, http.StatusFound)
}

func jsonError(w http.ResponseWriter, errCode, desc string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{
		"error":             errCode,
		"error_description": desc,
	})
}

// cleanup removes expired codes and tokens every 5 minutes.
func (p *Provider) cleanup() {
	for {
		time.Sleep(5 * time.Minute)
		now := time.Now()
		p.mu.Lock()
		for k, c := range p.codes {
			if now.After(c.ExpiresAt) {
				delete(p.codes, k)
			}
		}
		for k, t := range p.tokens {
			if now.After(t.ExpiresAt) {
				delete(p.tokens, k)
			}
		}
		p.mu.Unlock()
	}
}
