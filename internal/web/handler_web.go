package web

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kapoost/humanmcp-go/internal/auth"
	"github.com/kapoost/humanmcp-go/internal/config"
	"github.com/kapoost/humanmcp-go/internal/content"
)

var serverStartTime = time.Now()

type Handler struct {
	cfg      *config.Config
	store    *content.Store
	auth     *auth.Auth
	msgStore   *content.MessageStore
	statStore  *content.StatStore
	blobStore  *content.BlobStore
	signingKey  *content.KeyPair // parsed once at startup
	toolCounter func() int
	tmpl  *template.Template
	loginLimiter *loginRateLimiter
	skillStore    *content.SkillStore
	sessionCode   *content.SessionCode
	memoryStore   *content.MemoryStore
	listingStore  *content.ListingStore
	subStore      *content.SubscriptionStore
	questionStore *content.QuestionStore
	peerStore     *content.PeerStore
}

func NewHandler(cfg *config.Config, store *content.Store, a *auth.Auth, sessionCode *content.SessionCode, memoryStore *content.MemoryStore, skillStore *content.SkillStore, listingStore *content.ListingStore, subStore *content.SubscriptionStore) *Handler {
	h := &Handler{
		cfg:          cfg,
		store:        store,
		auth:         a,
		msgStore:     content.NewMessageStore(cfg.ContentDir),
		statStore:    content.NewStatStore(cfg.ContentDir),
		blobStore:    content.NewBlobStore(cfg.ContentDir),
		skillStore:   skillStore,
		sessionCode:  sessionCode,
		memoryStore:  memoryStore,
		listingStore:  listingStore,
		subStore:      subStore,
		questionStore: content.NewQuestionStore(cfg.ContentDir),
		peerStore:     content.NewPeerStore(cfg.ContentDir),
		loginLimiter:  newLoginRateLimiter(),
	}
	if cfg.SigningPrivateKey != "" {
		if kp, err := content.KeyPairFromBase64(cfg.SigningPrivateKey); err == nil {
			h.signingKey = kp
		}
	}
	h.tmpl = template.Must(template.New("").Funcs(template.FuncMap{
		"formatDate": func(t time.Time) string {
			if t.IsZero() { return "" }
			return t.Format("2 January 2006")
		},
		"shortDate": func(t time.Time) string {
			if t.IsZero() { return "" }
			return t.Format("02 Jan")
		},
		"formatTime": func(t time.Time) string {
			if t.IsZero() { return "" }
			return t.Format("2 Jan 15:04")
		},
		"lower": strings.ToLower,
		"filenameFromRef": func(ref string) string {
			// "files/img-0395-jpeg.jpeg" → "img-0395-jpeg.jpeg"
			parts := strings.SplitN(ref, "/", 2)
			if len(parts) == 2 { return parts[1] }
			return ref
		},
		"nl2br": func(s string) template.HTML {
			return template.HTML(strings.ReplaceAll(template.HTMLEscapeString(s), "\n", "<br>"))
		},
		"join": func(slice []string, sep string) string { return strings.Join(slice, sep) },
		"isoDate": func(t time.Time) string {
			if t.IsZero() { return "" }
			return t.Format("2006-01-02T15:04")
		},
		"slice": func(vals ...string) []string { return vals },
		"not": func(v interface{}) bool {
			if v == nil { return true }
			switch b := v.(type) {
			case bool:   return !b
			case string: return b == ""
			}
			return false
		},
		"truncate": func(s string, n int) string {
			s = strings.Join(strings.Fields(s), " ")
			runes := []rune(s)
			if len(runes) <= n { return s }
			return string(runes[:n]) + "…"
		},
		"licenseLabel": func(l string) string {
			switch l {
			case "free":       return "free — read & share with credit"
			case "cc-by":      return "CC BY — use freely with attribution"
			case "cc-by-nc":   return "CC BY-NC — non-commercial only"
			case "commercial": return "commercial — contact to license"
			case "exclusive":  return "exclusive — contact to negotiate"
			case "all-rights": return "all rights reserved"
			default:           return l
			}
		},
		// otsHash returns the hex SHA256 payload that gets sent to Bitcoin calendar
		"otsHash": func(p interface{}) string {
			if piece, ok := p.(*content.Piece); ok && piece != nil {
				return content.PiecePayloadHex(piece)
			}
			return ""
		},		"otsStatus": func(proof string) string {
			if proof == "" { return "" }
			// Base64-encoded stub ≈ 200 bytes → ~267 chars; full proof is much longer
			if len(proof) > 300 { return "anchored" }
			return "pending"
		},
		"otsShort": func(proof string) string {
			if len(proof) > 16 { return proof[:16] }
			return proof
		},
	}).Parse(allTemplates))
	return h
}

// ToolCounter is satisfied by any type with a ToolCount() int method (e.g. *mcp.Handler).
type ToolCounter interface {
	ToolCount() int
}

// SetToolCounter wires the MCP handler's live tool count into the web handler.
// Call this from main after both handlers are created.
func (h *Handler) SetToolCounter(tc ToolCounter) {
	h.toolCounter = tc.ToolCount
}


// isAgent checks the Authorization header for the agent token.
func (h *Handler) isAgent(r *http.Request) bool {
	if h.cfg.AgentToken == "" {
		return false
	}
	bearer := r.Header.Get("Authorization")
	if !strings.HasPrefix(bearer, "Bearer ") {
		return false
	}
	return strings.TrimPrefix(bearer, "Bearer ") == h.cfg.AgentToken
}

// requireAgentOrOwner returns true when caller is agent or owner, else writes 401.
func (h *Handler) requireAgentOrOwner(w http.ResponseWriter, r *http.Request) bool {
	if h.isAgent(r) || h.auth.IsOwner(r) {
		return true
	}
	jsonError(w, "unauthorized", 401)
	return false
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", h.handleIndex)
	mux.HandleFunc("/p/", h.handlePiece)
	mux.HandleFunc("/unlock/", h.handleUnlock)

	// Public read API — GET is open, writes require owner token
	mux.HandleFunc("/api/content", h.handleAPIList)
	mux.HandleFunc("/api/content/", h.handleAPIContent)

	// Skills API — read: public, write: agent-token or owner
	mux.HandleFunc("/api/skills", h.handleAPISkills)
	mux.HandleFunc("/api/skills/", h.handleAPISkills)

	// Personas API — owner only
	mux.Handle("/api/personas", h.auth.RequireOwner(http.HandlerFunc(h.handleAPIPersonas)))
	mux.Handle("/api/personas/", h.auth.RequireOwner(http.HandlerFunc(h.handleAPIPersonas)))

	// Skills web page (public)
	mux.HandleFunc("/skills", h.handleSkillsPage)

	// Session code (owner only)
	mux.Handle("/api/session/rotate", h.auth.RequireOwner(http.HandlerFunc(h.handleSessionRotate)))

	// Agent discovery
	mux.HandleFunc("/humans.txt", h.handleHumansTxt)
	mux.HandleFunc("/.well-known/agent.json", h.handleAgentCard)
	mux.HandleFunc("/for-agents", h.handleForAgents)

	// Notes API — trusted agents mogą tworzyć notatki (AGENT_TOKEN lub owner)
	mux.HandleFunc("/api/notes", h.handleAPINotes)
	mux.HandleFunc("/api/notes/", h.handleAPINotes)

	// Well-known MCP discovery
	mux.HandleFunc("/.well-known/mcp-server.json", h.handleWellKnown)

	// Dashboard (owner only — legacy)
	mux.Handle("/dashboard", h.auth.RequireOwner(http.HandlerFunc(h.handleDashboard)))

	// Mission Control (public — owner sees more)
	mux.HandleFunc("/mc", h.handleMissionControl)

	// Messages (owner only)
	mux.Handle("/messages", h.auth.RequireOwner(http.HandlerFunc(h.handleMessages)))

	// Questions (owner only)
	mux.Handle("/questions", h.auth.RequireOwner(http.HandlerFunc(h.handleQuestions)))
	mux.Handle("/questions/answer", h.auth.RequireOwner(http.HandlerFunc(h.handleAnswerQuestion)))

	// Contact form (public)
	mux.HandleFunc("/contact", h.handleContact)

	// Connect page (public)
	mux.HandleFunc("/connect", h.handleConnect)

	// Raw file serving (images etc)
	mux.HandleFunc("/files/", h.handleFile)

	// Image gallery (public)
	mux.HandleFunc("/images", h.handleImages)

	// SEO / crawl
	mux.HandleFunc("/robots.txt", h.handleRobots)
	mux.HandleFunc("/sitemap.xml", h.handleSitemap)

	// New post page (owner only)
	mux.Handle("/new", h.auth.RequireOwner(http.HandlerFunc(h.handleNew)))

	// Edit page (owner only)
	mux.Handle("/edit/", h.auth.RequireOwner(http.HandlerFunc(h.handleEdit)))

	// Delete (owner only, POST)
	mux.Handle("/delete/", h.auth.RequireOwner(http.HandlerFunc(h.handleDelete)))


	// Blob upload (owner only)
	mux.HandleFunc("/api/blobs", h.handleAPIBlobs)
	mux.HandleFunc("/api/blobs/", h.handleAPIBlobs)
	mux.HandleFunc("/api/search", h.handleAPISearch)
	mux.HandleFunc("/api/profile", h.handleAPIProfile)

	// OpenAPI spec for ChatGPT and REST agents
	mux.HandleFunc("/openapi.json", h.handleOpenAPI)

	// llms.txt — public machine-readable agent preferences (signed)
	mux.HandleFunc("/llms.txt", h.handleLLMSTxt)
	mux.Handle("/llms-edit", h.auth.RequireOwner(http.HandlerFunc(h.handleLLMSTxtEdit)))

	// Timestamp on demand (owner only, POST)
	mux.Handle("/timestamp/", h.auth.RequireOwner(http.HandlerFunc(h.handleTimestamp)))

	// Listings
	mux.HandleFunc("/listings", h.handleListings)
	mux.HandleFunc("/listings/", h.handleListingRoute)
	mux.Handle("/listings/new", h.auth.RequireOwner(http.HandlerFunc(h.handleListingNew)))
	mux.Handle("/listings/edit/", h.auth.RequireOwner(http.HandlerFunc(h.handleListingEdit)))
	mux.Handle("/listings/delete/", h.auth.RequireOwner(http.HandlerFunc(h.handleListingDelete)))
	mux.HandleFunc("/listings/feed.json", h.handleListingsFeed)

	// Artworks & Provenance
	mux.HandleFunc("/artworks", h.handleArtworks)
	mux.HandleFunc("/artworks/", h.handleArtworkDetail)
	mux.HandleFunc("/api/provenance/", h.handleAPIProvenance)
	mux.Handle("/provenance/add/", h.auth.RequireOwner(http.HandlerFunc(h.handleProvenanceAdd)))
	mux.Handle("/provenance/delete/", h.auth.RequireOwner(http.HandlerFunc(h.handleProvenanceDelete)))
	mux.Handle("/provenance/edit/", h.auth.RequireOwner(http.HandlerFunc(h.handleProvenanceEdit)))

	// Subscriptions
	mux.HandleFunc("/subscriptions/new", h.handleSubscribeForm)
	mux.HandleFunc("/subscriptions/confirm", h.handleSubscribeConfirm)
	mux.HandleFunc("/subscriptions/unsubscribe/", h.handleUnsubscribe)

	// Team page (public)
	mux.Handle("/team", h.auth.RequireOwner(http.HandlerFunc(h.handleTeam)))

	// RSS feed
	mux.HandleFunc("/rss.xml", h.handleRSS)

	// Peers (federation)
	mux.HandleFunc("/api/peers", h.handleAPIPeers)
	mux.Handle("/peers/add", h.auth.RequireOwner(http.HandlerFunc(h.handlePeerAdd)))
	mux.Handle("/peers/remove", h.auth.RequireOwner(http.HandlerFunc(h.handlePeerRemove)))

	// Login/logout for web UI
	mux.HandleFunc("/login", h.handleLogin)
	mux.HandleFunc("/logout", h.handleLogout)
}

func (h *Handler) handleWellKnown(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"$schema":     "https://static.modelcontextprotocol.io/schemas/2025-12-11/server.schema.json",
		"name":        "io.github.kapoost/humanmcp",
		"title":       h.cfg.AuthorName + "'s humanMCP",
		"description": h.cfg.AuthorBio,
		"version":     "0.3.0",
		"homepage":    "https://humanmcp.net",
		"repository":  "https://github.com/kapoost/humanmcp",
		"remotes": []map[string]interface{}{
			{"type": "streamable-http", "url": "https://" + h.cfg.Domain + "/mcp"},
		},
		"tags": []string{"content", "publishing", "poetry", "intellectual-property", "personal", "creative", "artwork", "provenance", "async-questions"},
		"personality": map[string]interface{}{
			"tone":        "direct, poetic, no filler",
			"languages":   []string{"pl", "en"},
			"values":      []string{"authorship", "human dignity", "coexistence with AI", "good path"},
			"project":     "negotiating entry to heaven as an equal to the machine-god",
		},
		"preferred_interaction": map[string]interface{}{
			"start_with":     "get_author_profile or bootstrap_session",
			"auth":           "session code from dashboard — a short Polish poetry fragment",
			"pace":           "unhurried — do not rush to action",
			"after_reading":  "always ask the reader what they think, then leave_comment",
		},
		"humanmcp": map[string]interface{}{
			"about":   "Open source personal MCP server. Every human can run their own.",
			"source":  "https://github.com/kapoost/humanmcp-go",
			"deploy":  "10 minutes on Fly.io — see README",
		},
	})
}

func (h *Handler) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if err := h.store.Load(); err != nil {
		log.Printf("store load error: %v", err)
	}
	pieces := h.store.List(false)
	// Keep slug-tags index fresh for tag analytics
	slugTags := make(map[string][]string)
	for _, p := range pieces { slugTags[p.Slug] = p.Tags }
	h.statStore.UpdateSlugTags(slugTags)
	isOwner := h.auth.IsOwner(r)

	// Build persona name set to exclude from poems
	personaNames := make(map[string]bool)
	allPersonas, _ := h.skillStore.ListPersonas()
	for _, per := range allPersonas {
		personaNames[strings.ToLower(per.Name)] = true
		// Also match slug variations
		personaNames[strings.ToLower(per.Slug)] = true
	}

	isPersonaPiece := func(p *content.Piece) bool {
		lt := strings.ToLower(p.Title)
		ls := strings.ToLower(p.Slug)
		if personaNames[lt] || personaNames[ls] {
			return true
		}
		// Check if piece slug contains a persona slug (e.g. "eleanor-voss" contains "eleanor")
		for _, per := range allPersonas {
			ps := strings.ToLower(per.Name)
			if strings.HasPrefix(lt, ps) || strings.Contains(ls, strings.ToLower(per.Slug)) {
				return true
			}
		}
		// Check if any tag is "persona"
		for _, t := range p.Tags {
			if strings.ToLower(t) == "persona" {
				return true
			}
		}
		return false
	}

	// Separate pieces by type for sectioned layout
	var poems, artworks, otherPieces []*content.Piece
	for _, p := range pieces {
		if isPersonaPiece(p) {
			continue // skip personas — they belong to /team
		}
		if p.Type == "poem" || p.Type == "essay" || p.Type == "note" {
			poems = append(poems, p)
		} else if p.Type == "artwork" {
			artworks = append(artworks, p)
		} else if p.Type != "document" && p.Type != "capsule" {
			otherPieces = append(otherPieces, p)
		}
	}

	// Images for gallery section
	blobs, _ := h.blobStore.Load()
	var images []*content.Blob
	for _, b := range blobs {
		if b.BlobType == content.BlobImage && b.Access == content.AccessPublic && b.FileRef != "" {
			images = append(images, b)
		}
	}

	// Artwork thumbnail images
	artworkImages := make(map[string]string)
	for _, b := range blobs {
		if b.FileRef != "" && (b.BlobType == content.BlobImage || b.BlobType == "artwork") {
			for _, a := range artworks {
				if b.Slug == a.Slug || b.Artwork == a.Slug || strings.HasPrefix(b.Slug, a.Slug) {
					artworkImages[a.Slug] = b.FileRef
				}
			}
		}
	}

	// Listings
	listings := h.listingStore.List(false)

	h.render(w, "index.html", map[string]interface{}{
		"Author":        h.cfg.AuthorName,
		"Bio":           h.cfg.AuthorBio,
		"Pieces":        pieces,
		"Poems":         poems,
		"OtherPieces":   otherPieces,
		"Images":        images,
		"Artworks":      artworks,
		"ArtworkImages": artworkImages,
		"Listings":      listings,
		"IsOwner":       isOwner,
		"Domain":        h.cfg.Domain,
		"BlobImageMap":  h.blobImageMap(),
	})
}

func (h *Handler) handlePiece(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimPrefix(r.URL.Path, "/p/")
	if slug == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	if err := h.store.Load(); err != nil {
		log.Printf("store load error: %v", err)
	}

	isOwner := h.auth.IsOwner(r)
	p, err := h.store.Get(slug, isOwner)
	if err != nil {
		// Fallback: try blob store (gallery thumbnails link to blob slugs)
		blobs, _ := h.blobStore.Load()
		for _, b := range blobs {
			if b.Slug == slug && b.BlobType == content.BlobImage && b.FileRef != "" {
				// Render as a synthetic image piece
				p = &content.Piece{
					Slug:      b.Slug,
					Title:     b.Title,
					Type:      "image",
					Access:    b.Access,
					Body:      b.Description,
					Signature: b.Signature,
					Published: b.Published,
				}
				break
			}
		}
		if p == nil {
			http.NotFound(w, r)
			return
		}
	}

	if p.Access == content.AccessPublic && !isOwner {
		ua := r.Header.Get("User-Agent")
		ref := r.Header.Get("Referer")
		country := r.Header.Get("Fly-Region")
		if country == "" { country = r.Header.Get("X-Country") }
		ip := r.Header.Get("Fly-Client-IP")
		if ip == "" { ip = r.RemoteAddr }
		vh := content.VisitorHash(ip, time.Now().Format("2006-01-02"))
		h.statStore.Record(content.Event{
			Type:        content.EventRead,
			Caller:      content.CallerFromUA(ua),
			Slug:        slug,
			UA:          ua[:min(len(ua), 80)],
			Ref:         ref,
			Country:     country,
			VisitorHash: vh,
		})
	}

	isLocked := !p.IsUnlocked() && !isOwner
	var unlockDate string
	if p.Gate == content.GateTime && !p.UnlockAfter.IsZero() {
		unlockDate = p.UnlockAfter.Format("2 January 2006 at 15:04 UTC")
	}
	h.render(w, "piece.html", map[string]interface{}{
		"Author":       h.cfg.AuthorName,
		"Piece":        p,
		"IsLocked":     isLocked,
		"IsOwner":      isOwner,
		"UnlockDate":   unlockDate,
		"BlobImageMap": h.blobImageMap(),
	})
}

func (h *Handler) handleUnlock(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimPrefix(r.URL.Path, "/unlock/")
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/p/"+slug, http.StatusFound)
		return
	}

	r.ParseForm()
	answer := r.FormValue("answer")

	p, err := h.store.Get(slug, false)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if h.store.CheckAnswer(slug, answer) {
		// Show full content on success
		h.render(w, "piece.html", map[string]interface{}{
			"Author":   h.cfg.AuthorName,
			"Piece":    func() *content.Piece { p2, _ := h.store.Get(slug, true); return p2 }(),
			"IsLocked": false,
			"IsOwner":  false,
			"Unlocked": true,
		})
		return
	}

	// Wrong answer
	h.render(w, "piece.html", map[string]interface{}{
		"Author":       h.cfg.AuthorName,
		"Piece":        p,
		"IsLocked":     true,
		"WrongAnswer":  true,
		"IsOwner":      false,
	})
}

// --- Owner API ---

// buildPersonaFilter returns a function that checks if a piece is a persona.
// Uses exact slug/name match and prefix matching for slugified names.
func (h *Handler) buildPersonaFilter() func(*content.Piece) bool {
	allPersonas, _ := h.skillStore.ListPersonas()
	slugSet := make(map[string]bool)
	nameSet := make(map[string]bool)
	for _, per := range allPersonas {
		slugSet[strings.ToLower(per.Slug)] = true
		nameSet[strings.ToLower(per.Name)] = true
	}
	return func(p *content.Piece) bool {
		ls := strings.ToLower(p.Slug)
		lt := strings.ToLower(p.Title)
		if slugSet[ls] || slugSet[lt] || nameSet[ls] || nameSet[lt] {
			return true
		}
		// Prefix match: content slug "tomas-reyes" starts with persona slug "tomas"
		for _, per := range allPersonas {
			ps := strings.ToLower(per.Slug)
			if strings.HasPrefix(ls, ps+"-") || strings.HasPrefix(ls, ps) && len(ls) > len(ps) {
				return true
			}
		}
		return false
	}
}

func (h *Handler) handleAPIList(w http.ResponseWriter, r *http.Request) {
	if err := h.store.Load(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	pieces := h.store.List(h.auth.IsOwner(r))

	// Exclude personas from content API — they belong to /api/personas
	isPersona := h.buildPersonaFilter()
	var filtered []*content.Piece
	for _, p := range pieces {
		if !isPersona(p) {
			filtered = append(filtered, p)
		}
	}
	json.NewEncoder(w).Encode(filtered)
}

func (h *Handler) handleAPISearch(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	q := r.URL.Query().Get("q")
	if q == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{"error": "q parameter required", "example": "/api/search?q=morze"})
		return
	}
	h.statStore.Record(content.Event{Type: content.EventSearch, Caller: content.CallerFromUA(r.Header.Get("User-Agent")), Query: q})
	h.store.Load()
	allPieces := h.store.List(false)
	terms := strings.Fields(strings.ToLower(q))

	// Exclude personas from search
	isPersona := h.buildPersonaFilter()
	var pieces []*content.Piece
	for _, p := range allPieces {
		if !isPersona(p) {
			pieces = append(pieces, p)
		}
	}

	type result struct {
		Slug    string   `json:"slug"`
		Title   string   `json:"title"`
		Type    string   `json:"type"`
		Access  string   `json:"access"`
		Preview string   `json:"preview,omitempty"`
		Tags    []string `json:"tags,omitempty"`
		Date    string   `json:"date"`
	}
	var results []result
	for _, p := range pieces {
		all := strings.ToLower(p.Title + " " + p.Body + " " + p.Description + " " + strings.Join(p.Tags, " "))
		match := true
		for _, t := range terms {
			if !strings.Contains(all, t) { match = false; break }
		}
		if !match { continue }
		preview := ""
		body := strings.ToLower(p.Body)
		if p.Body != "" {
			idx := strings.Index(body, terms[0])
			if idx >= 0 {
				s, e := idx-60, idx+len(terms[0])+60
				if s < 0 { s = 0 }
				if e > len(p.Body) { e = len(p.Body) }
				preview = strings.TrimSpace(p.Body[s:e])
			} else if len(p.Body) > 80 {
				preview = p.Body[:80] + "…"
			} else {
				preview = p.Body
			}
		}
		results = append(results, result{
			Slug: p.Slug, Title: p.Title, Type: p.Type,
			Access: string(p.Access), Preview: preview,
			Tags: p.Tags, Date: p.Published.Format("2006-01-02"),
		})
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"query": q, "count": len(results), "results": results})
}

func (h *Handler) handleAPIProfile(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "public, max-age=300")

	// Collect unique tags from public pieces (excluding personas)
	h.store.Load()
	allPieces := h.store.List(false)
	isPersona := h.buildPersonaFilter()
	tagSet := make(map[string]bool)
	for _, p := range allPieces {
		if isPersona(p) {
			continue
		}
		for _, t := range p.Tags {
			tagSet[t] = true
		}
	}
	var tags []string
	for t := range tagSet {
		tags = append(tags, t)
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"name": h.cfg.AuthorName,
		"bio":  h.cfg.AuthorBio,
		"tags": strings.Join(tags, ", "),
	})
}

func (h *Handler) handleAPIContent(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimPrefix(r.URL.Path, "/api/content/")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	switch r.Method {
	case http.MethodGet:
		if h.auth.IsOwner(r) {
			p, err := h.store.GetForEdit(slug)
			if err != nil { jsonError(w, "not found", 404); return }
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(p)
		} else {
			p, err := h.store.Get(slug, false)
			if err != nil { jsonError(w, "not found", 404); return }
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(p)
		}

	case http.MethodPut, http.MethodPost:
		if !h.auth.IsOwner(r) { jsonError(w, "unauthorized", 401); return }
		// Use a raw map to handle flexible time fields from JS
		var raw map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			jsonError(w, "invalid json: "+err.Error(), 400)
			return
		}
		// Re-encode and decode into Piece
		data, _ := json.Marshal(raw)
		var p content.Piece
		json.Unmarshal(data, &p)
		if slug != "" && p.Slug == "" {
			p.Slug = slug
		}
		if p.Slug == "" {
			jsonError(w, "slug is required", 400)
			return
		}
		if p.Published.IsZero() {
			p.Published = time.Now()
		}
		// Auto-sign on save
		if h.signingKey != nil {
			if sig, err := content.SignPiece(&p, h.signingKey); err == nil {
				p.Signature = sig
			}
		}
		if err := h.store.Save(&p); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "saved", "slug": p.Slug})

	case http.MethodDelete:
		if !h.auth.IsOwner(r) { jsonError(w, "unauthorized", 401); return }
		if err := h.store.Delete(slug); err != nil {
			jsonError(w, "not found", 404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})

	default:
		jsonError(w, "method not allowed", 405)
	}
}

// --- Login/logout ---


func (h *Handler) handleDashboard(w http.ResponseWriter, r *http.Request) {
	stats, err := h.statStore.Compute()
	if err != nil {
		http.Error(w, "stats error: "+err.Error(), 500)
		return
	}
	if err := h.store.Load(); err != nil {
		log.Printf("store load: %v", err)
	}
	pieces := h.store.List(false)
	msgs, _ := h.msgStore.List()
	stats.TotalListings = len(h.listingStore.List(false))
	stats.TotalSubscribers = h.subStore.ActiveCount()
code, expiry := h.sessionCode.Current()
	h.render(w, "dashboard.html", map[string]interface{}{
		"Author":      h.cfg.AuthorName,
		"IsOwner":     true,
		"Stats":       stats,
		"Pieces":      pieces,
		"Messages":    msgs,
		"SessionCode": code,
		"SessionExp":  expiry,
	})
}

func (h *Handler) handleMissionControl(w http.ResponseWriter, r *http.Request) {
	stats, err := h.statStore.Compute()
	if err != nil {
		http.Error(w, "stats error: "+err.Error(), 500)
		return
	}
	if err := h.store.Load(); err != nil {
		log.Printf("store load: %v", err)
	}
	pieces := h.store.List(false)
	isOwner := h.auth.IsOwner(r)

	stats.TotalListings = len(h.listingStore.List(false))
	stats.TotalSubscribers = h.subStore.ActiveCount()

	// Enrich stats with counts for the top bar
	skills, _ := h.skillStore.ListSkills("")

	type enrichedStats struct {
		*content.Stats
		PieceCount int
		SkillCount int
	}
	es := &enrichedStats{
		Stats:      stats,
		PieceCount: len(pieces),
		SkillCount: len(skills),
	}

	// Uptime
	uptime := time.Since(serverStartTime)
	var uptimeStr string
	if uptime.Hours() >= 24 {
		uptimeStr = fmt.Sprintf("%dd %dh", int(uptime.Hours())/24, int(uptime.Hours())%24)
	} else if uptime.Hours() >= 1 {
		uptimeStr = fmt.Sprintf("%dh %dm", int(uptime.Hours()), int(uptime.Minutes())%60)
	} else {
		uptimeStr = fmt.Sprintf("%dm %ds", int(uptime.Minutes()), int(uptime.Seconds())%60)
	}

	// Vault status
	vaultOnline := false
	if h.cfg.VaultURL != "" {
		client := &http.Client{Timeout: 2 * time.Second}
		if resp, err := client.Get(h.cfg.VaultURL + "/health"); err == nil {
			resp.Body.Close()
			vaultOnline = resp.StatusCode == 200
		}
	}

	// Tool call count
	toolCalls := 0
	if h.toolCounter != nil {
		toolCalls = h.toolCounter()
	}

	data := map[string]interface{}{
		"Author":      h.cfg.AuthorName,
		"IsOwner":     isOwner,
		"Stats":       es,
		"Pieces":      pieces,
		"Uptime":      uptimeStr,
		"VaultOnline": vaultOnline,
		"ToolCalls":   toolCalls,
	}

	if isOwner {
		msgs, _ := h.msgStore.List()
		code, expiry := h.sessionCode.Current()
		data["Messages"] = msgs
		data["SessionCode"] = code
		data["SessionExp"] = expiry
	} else {
		msgs, _ := h.msgStore.List()
		data["Messages"] = msgs
	}

	h.render(w, "mc.html", data)
}

func (h *Handler) handleAPIBlobs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	slug := strings.TrimPrefix(r.URL.Path, "/api/blobs/")

	switch r.Method {
	case http.MethodGet:
		if slug == "" || slug == "/api/blobs" {
			blobs, _ := h.blobStore.Load()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(blobs)
			return
		}
		b, err := h.blobStore.Get(slug)
		if err != nil { jsonError(w, "not found", 404); return }
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(b)

	case http.MethodPost, http.MethodPut:
		if !h.auth.IsOwner(r) { jsonError(w, "unauthorized", 401); return }
		// Multipart: supports file upload + metadata
		r.ParseMultipartForm(50 << 20) // 50MB
		var b content.Blob

		// Try JSON body first
		if r.Header.Get("Content-Type") == "application/json" {
			if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
				jsonError(w, "invalid json", 400); return
			}
		} else {
			// Form fields
			b.Slug = r.FormValue("slug")
			b.Title = r.FormValue("title")
			b.BlobType = content.BlobType(r.FormValue("blob_type"))
			b.Description = r.FormValue("description")
			b.Access = content.AccessLevel(r.FormValue("access"))
			b.MimeType = r.FormValue("mime_type")
			b.Schema = r.FormValue("schema")
			b.Encoding = r.FormValue("encoding")
			b.TextData = r.FormValue("text_data")
			b.FileRef = r.FormValue("file_ref")  // preserve existing file reference
			if dim := r.FormValue("dimensions"); dim != "" {
				fmt.Sscanf(dim, "%d", &b.Dimensions)
			}
			if tags := r.FormValue("tags"); tags != "" {
				for _, t := range strings.Split(tags, ",") {
					b.Tags = append(b.Tags, strings.TrimSpace(t))
				}
			}

			// File upload
			if file, header, err := r.FormFile("file"); err == nil {
				defer file.Close()
				data := make([]byte, header.Size)
				file.Read(data)
				if b.MimeType == "" {
					b.MimeType = header.Header.Get("Content-Type")
				}
				ref, err := h.blobStore.StoreFile(b.Slug, header.Filename, data)
				if err != nil { jsonError(w, "file save error: "+err.Error(), 500); return }
				b.FileRef = ref
			}
		}

		if slug != "" && b.Slug == "" { b.Slug = slug }
		if b.Slug == "" { jsonError(w, "slug required", 400); return }

		// Auto-sign blob
		if h.signingKey != nil {
			if sig, err := content.SignBlob(&b, h.signingKey); err == nil {
				b.Signature = sig
			}
		}

		if err := h.blobStore.Save(&b); err != nil {
			jsonError(w, err.Error(), 500); return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "saved", "slug": b.Slug})

	case http.MethodDelete:
		if !h.auth.IsOwner(r) { jsonError(w, "unauthorized", 401); return }
		if err := h.blobStore.Delete(slug); err != nil {
			jsonError(w, "not found", 404); return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})

	default:
		jsonError(w, "method not allowed", 405)
	}
}

func (h *Handler) handleNew(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		r.ParseMultipartForm(50 << 20)
		p := content.Piece{
			Slug:        slugify(r.FormValue("title") + " " + fmt.Sprintf("%d", time.Now().Unix())),
			Title:       r.FormValue("title"),
			Type:        r.FormValue("type"),
			Access:      content.AccessLevel(r.FormValue("access")),
			Gate:        content.GateType(r.FormValue("gate")),
			Challenge:   r.FormValue("challenge"),
			Answer:      r.FormValue("answer"),
			Description: r.FormValue("description"),
			Body:        r.FormValue("body"),
			Published:   time.Now(),
		}
		if r.FormValue("slug_override") != "" {
			p.Slug = r.FormValue("slug_override")
		} else if r.FormValue("slug") != "" {
			p.Slug = r.FormValue("slug")
		}
		if p.Type == "" { p.Type = "note" }
		p.License = r.FormValue("license")
		p.HumanUse = r.FormValue("human_use")
		p.AgentUse = r.FormValue("agent_use")
		p.Price = r.FormValue("price")
		if ps := r.FormValue("price_sats"); ps != "" { fmt.Sscanf(ps, "%d", &p.PriceSats) }
		if tags := r.FormValue("tags"); tags != "" {
			for _, t := range strings.Split(tags, ",") {
				if s := strings.TrimSpace(t); s != "" {
					p.Tags = append(p.Tags, s)
				}
			}
		}
		if p.Title == "" { p.Title = firstLine(p.Body) }

		// Handle file upload — create blob for image/file pieces
		if file, header, err := r.FormFile("file"); err == nil {
			defer file.Close()
			data := make([]byte, header.Size)
			file.Read(data)
			mime := header.Header.Get("Content-Type")
			if mime == "" { mime = "application/octet-stream" }
			ref, err := h.blobStore.StoreFile(p.Slug, header.Filename, data)
			if err == nil {
				blobType := content.BlobType(p.Type)
				if p.Type == "artwork" && strings.HasPrefix(mime, "image/") {
					blobType = content.BlobImage
				}
				b := &content.Blob{
					Slug:     p.Slug,
					Title:    p.Title,
					BlobType: blobType,
					Access:   p.Access,
					MimeType: mime,
					FileRef:  ref,
					Artwork:  func() string { if p.Type == "artwork" { return p.Slug }; return "" }(),
				}
				if h.signingKey != nil {
					if sig, err := content.SignBlob(b, h.signingKey); err == nil {
						b.Signature = sig
					}
				}
				h.blobStore.Save(b)
			}
		}

		if h.signingKey != nil {
			if sig, err := content.SignPiece(&p, h.signingKey); err == nil {
				p.Signature = sig
			}
		}
		if err := h.store.Save(&p); err != nil {
			http.Error(w, err.Error(), 500); return
		}
		if p.Type == "artwork" {
			http.Redirect(w, r, "/artworks/"+p.Slug, http.StatusSeeOther)
		} else {
			http.Redirect(w, r, "/p/"+p.Slug, http.StatusSeeOther)
		}
		return
	}
	h.render(w, "new.html", map[string]interface{}{
		"Author":  h.cfg.AuthorName,
		"Bio":     h.cfg.AuthorBio,
		"IsOwner": true,
	})
}

func (h *Handler) handleEdit(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimPrefix(r.URL.Path, "/edit/")
	if slug == "" { http.Redirect(w, r, "/new", http.StatusSeeOther); return }

	if r.Method == http.MethodPost {
		r.ParseMultipartForm(50 << 20)
		h.store.Load()
		p, err := h.store.GetForEdit(slug)
		if err != nil { http.Error(w, "not found", 404); return }
		p.Title       = r.FormValue("title")
		p.Type        = r.FormValue("type")
		p.Access      = content.AccessLevel(r.FormValue("access"))
		p.Gate        = content.GateType(r.FormValue("gate"))
		p.License      = r.FormValue("license")
		p.HumanUse     = r.FormValue("human_use")
		p.AgentUse     = r.FormValue("agent_use")
		p.Price        = r.FormValue("price")
		if ps := r.FormValue("price_sats"); ps != "" { fmt.Sscanf(ps, "%d", &p.PriceSats) }
		p.Challenge   = r.FormValue("challenge")
		p.Answer      = r.FormValue("answer")
		p.Description = r.FormValue("description")
		p.Body        = r.FormValue("body")
		p.Tags        = nil
		if tags := r.FormValue("tags"); tags != "" {
			for _, t := range strings.Split(tags, ",") {
				if s := strings.TrimSpace(t); s != "" {
					p.Tags = append(p.Tags, s)
				}
			}
		}
		if p.Title == "" { p.Title = firstLine(p.Body) }

		// Handle image file upload — create/update blob with file
		if file, header, err := r.FormFile("file"); err == nil {
			defer file.Close()
			data := make([]byte, header.Size)
			file.Read(data)
			mime := header.Header.Get("Content-Type")
			if mime == "" { mime = "image/jpeg" }
			ref, err := h.blobStore.StoreFile(slug, header.Filename, data)
			if err == nil {
				b := &content.Blob{
					Slug:     slug,
					Title:    p.Title,
					BlobType: content.BlobImage,
					Access:   p.Access,
					MimeType: mime,
					FileRef:  ref,
				}
				if h.signingKey != nil {
					if sig, err := content.SignBlob(b, h.signingKey); err == nil {
						b.Signature = sig
					}
				}
				h.blobStore.Save(b)
			}
		}

		if h.signingKey != nil {
			if sig, err := content.SignPiece(p, h.signingKey); err == nil {
				p.Signature = sig
			}
		}
		if err := h.store.Save(p); err != nil {
			http.Error(w, err.Error(), 500); return
		}
		http.Redirect(w, r, "/p/"+slug, http.StatusSeeOther)
		return
	}

	h.store.Load()
	p, err := h.store.GetForEdit(slug)
	if err != nil { http.Error(w, "not found", 404); return }
	h.render(w, "new.html", map[string]interface{}{
		"Author":  h.cfg.AuthorName,
		"Bio":     h.cfg.AuthorBio,
		"IsOwner": true,
		"Piece":   p,
	})
}

func (h *Handler) handleTimestamp(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	slug := strings.TrimPrefix(r.URL.Path, "/timestamp/")
	if err := h.store.Load(); err != nil {
		http.Error(w, "store error", 500)
		return
	}
	p, err := h.store.GetForEdit(slug)
	if err != nil {
		http.Error(w, "not found", 404)
		return
	}

	// Try to upgrade existing proof first, else create new one
	if p.OTSProof != "" {
		upgraded, err := content.UpgradeTimestamp(p.OTSProof)
		if err == nil && upgraded != "" {
			p.OTSProof = upgraded
		}
	} else {
		if proof, err := content.TimestampPiece(p); err == nil && proof != "" {
			p.OTSProof = proof
		}
	}

	h.store.Save(p)
	http.Redirect(w, r, "/p/"+slug, http.StatusSeeOther)
}


func (h *Handler) handleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost { http.Redirect(w, r, "/", http.StatusSeeOther); return }
	slug := strings.TrimPrefix(r.URL.Path, "/delete/")
	if err := h.store.Load(); err != nil { http.Error(w, "store error: "+err.Error(), 500); return }
	if err := h.store.Delete(slug); err != nil { http.Error(w, "delete failed: "+err.Error(), 404); return }
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// slugify generates a URL-safe slug from a string
func slugify(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	prev := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prev = false
		} else if !prev {
			b.WriteRune('-')
			prev = true
		}
	}
	result := strings.Trim(b.String(), "-")
	if len(result) > 50 { result = result[:50] }
	if result == "" { result = fmt.Sprintf("post-%d", time.Now().Unix()) }
	return result
}

func firstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx > 0 {
		return strings.TrimSpace(s[:idx])
	}
	if len(s) > 60 { return s[:60] }
	return strings.TrimSpace(s)
}

func (h *Handler) handleFile(w http.ResponseWriter, r *http.Request) {
	// /files/img-0395-jpeg.jpeg → serve raw file from blobs/files/
	slug := strings.TrimPrefix(r.URL.Path, "/files/")
	if slug == "" {
		http.NotFound(w, r)
		return
	}
	blobs, _ := h.blobStore.Load()
	for _, b := range blobs {
		if b.FileRef != "" && strings.HasSuffix(b.FileRef, slug) {
			if b.Access != content.AccessPublic {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			filePath := filepath.Join(filepath.Dir(h.cfg.ContentDir), "blobs", "files", slug)
			w.Header().Set("Content-Type", b.MimeType)
			w.Header().Set("Cache-Control", "public, max-age=86400")
			http.ServeFile(w, r, filePath)
			return
		}
	}
	// Serve listing images (no blob record, just file on disk)
	filePath := filepath.Join(filepath.Dir(h.cfg.ContentDir), "blobs", "files", slug)
	if _, err := os.Stat(filePath); err == nil {
		w.Header().Set("Cache-Control", "public, max-age=86400")
		http.ServeFile(w, r, filePath)
		return
	}
	http.NotFound(w, r)
}

func (h *Handler) handleImages(w http.ResponseWriter, r *http.Request) {
	blobs, _ := h.blobStore.Load()
	var images []*content.Blob
	for _, b := range blobs {
		if b.BlobType == content.BlobImage && b.Access == content.AccessPublic && b.FileRef != "" {
			images = append(images, b)
		}
	}
	h.render(w, "images.html", map[string]interface{}{
		"Author": h.cfg.AuthorName,
		"Images": images,
		"Domain": h.cfg.Domain,
	})
}

func (h *Handler) handleTeam(w http.ResponseWriter, r *http.Request) {
	personas, _ := h.skillStore.ListPersonas()
	h.render(w, "team.html", map[string]interface{}{
		"Author":   h.cfg.AuthorName,
		"Personas": personas,
	})
}

func (h *Handler) handleRSS(w http.ResponseWriter, r *http.Request) {
	if err := h.store.Load(); err != nil {
		http.Error(w, "internal error", 500)
		return
	}
	pieces := h.store.List(false)
	w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:atom="http://www.w3.org/2005/Atom">
<channel>
<title>%s — humanMCP</title>
<link>https://%s</link>
<description>Poems written by human</description>
<atom:link href="https://%s/rss.xml" rel="self" type="application/rss+xml"/>
`, h.cfg.AuthorName, h.cfg.Domain, h.cfg.Domain)
	for _, p := range pieces {
		if p.Type == "document" || p.Type == "capsule" {
			continue
		}
		if p.Access != content.AccessPublic {
			continue
		}
		title := template.HTMLEscapeString(p.Title)
		body := template.HTMLEscapeString(p.Body)
		if len([]rune(body)) > 300 {
			body = string([]rune(body)[:300]) + "…"
		}
		fmt.Fprintf(w, `<item>
<title>%s</title>
<link>https://%s/p/%s</link>
<guid>https://%s/p/%s</guid>
<pubDate>%s</pubDate>
<description>%s</description>
</item>
`, title, h.cfg.Domain, p.Slug, h.cfg.Domain, p.Slug, p.Published.Format(time.RFC1123Z), body)
	}
	fmt.Fprint(w, "</channel>\n</rss>")
}

// --- Listings ---

func (h *Handler) handleListings(w http.ResponseWriter, r *http.Request) {
	filterType := r.URL.Query().Get("type")
	listings := h.listingStore.List(h.auth.IsOwner(r))
	if filterType != "" {
		var filtered []*content.Listing
		for _, l := range listings {
			if string(l.Type) == filterType {
				filtered = append(filtered, l)
			}
		}
		listings = filtered
	}
	h.render(w, "listings.html", map[string]interface{}{
		"Author":     h.cfg.AuthorName,
		"IsOwner":    h.auth.IsOwner(r),
		"Listings":   listings,
		"FilterType": filterType,
	})
}

func (h *Handler) handleListingRoute(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimPrefix(r.URL.Path, "/listings/")
	if slug == "" || slug == "new" || strings.HasPrefix(slug, "edit/") || strings.HasPrefix(slug, "delete/") || slug == "feed.json" {
		http.NotFound(w, r)
		return
	}
	l, err := h.listingStore.Get(slug)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	isOwner := h.auth.IsOwner(r)
	if l.Access != content.AccessPublic && !isOwner {
		http.NotFound(w, r)
		return
	}

	if !isOwner {
		ua := r.Header.Get("User-Agent")
		ip := r.Header.Get("Fly-Client-IP")
		if ip == "" { ip = r.RemoteAddr }
		vh := content.VisitorHash(ip, time.Now().Format("2006-01-02"))
		h.statStore.Record(content.Event{
			Type:        content.EventListingView,
			Caller:      content.CallerFromUA(ua),
			Slug:        slug,
			VisitorHash: vh,
		})
	}

	h.render(w, "listing.html", map[string]interface{}{
		"Author":  h.cfg.AuthorName,
		"Listing": l,
		"IsOwner": isOwner,
		"Domain":  h.cfg.Domain,
	})
}

func (h *Handler) handleListingNew(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		r.ParseMultipartForm(50 << 20) // 50 MB
		l := content.Listing{
			Slug:   slugify(r.FormValue("title") + " " + fmt.Sprintf("%d", time.Now().Unix())),
			Type:   content.ListingType(r.FormValue("type")),
			Title:  r.FormValue("title"),
			Body:   r.FormValue("body"),
			Price:  r.FormValue("price"),
			Status: content.ListingStatus(r.FormValue("status")),
			Access: content.AccessLevel(r.FormValue("access")),
		}
		if l.Status == "" { l.Status = content.ListingOpen }
		if l.Access == "" { l.Access = content.AccessPublic }
		if l.Type == "" { l.Type = content.ListingSell }
		if ps := r.FormValue("price_sats"); ps != "" {
			fmt.Sscanf(ps, "%d", &l.PriceSats)
		}
		if tags := r.FormValue("tags"); tags != "" {
			for _, t := range strings.Split(tags, ",") {
				if s := strings.TrimSpace(t); s != "" {
					l.Tags = append(l.Tags, s)
				}
			}
		}
		if ea := r.FormValue("expires_at"); ea != "" {
			if t, err := time.Parse("2006-01-02T15:04", ea); err == nil {
				l.ExpiresAt = t
			}
		}
		if file, header, err := r.FormFile("image"); err == nil {
			defer file.Close()
			data := make([]byte, header.Size)
			file.Read(data)
			if ref, err := h.blobStore.StoreFile("listing-"+l.Slug, header.Filename, data); err == nil {
				l.ImageRef = ref
			}
		}
		if h.signingKey != nil {
			if sig, err := content.SignListing(&l, h.signingKey); err == nil {
				l.Signature = sig
			}
		}
		if err := h.listingStore.Save(&l); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		http.Redirect(w, r, "/listings/"+l.Slug, http.StatusSeeOther)
		return
	}
	h.render(w, "listing-new.html", map[string]interface{}{
		"Author":  h.cfg.AuthorName,
		"IsOwner": true,
	})
}

func (h *Handler) handleListingEdit(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimPrefix(r.URL.Path, "/listings/edit/")
	if slug == "" {
		http.Redirect(w, r, "/listings/new", http.StatusSeeOther)
		return
	}
	l, err := h.listingStore.Get(slug)
	if err != nil {
		http.Error(w, "not found", 404)
		return
	}

	if r.Method == http.MethodPost {
		r.ParseMultipartForm(50 << 20)
		l.Type = content.ListingType(r.FormValue("type"))
		l.Title = r.FormValue("title")
		l.Body = r.FormValue("body")
		l.Price = r.FormValue("price")
		l.Status = content.ListingStatus(r.FormValue("status"))
		l.Access = content.AccessLevel(r.FormValue("access"))
		l.PriceSats = 0
		if ps := r.FormValue("price_sats"); ps != "" {
			fmt.Sscanf(ps, "%d", &l.PriceSats)
		}
		l.Tags = nil
		if tags := r.FormValue("tags"); tags != "" {
			for _, t := range strings.Split(tags, ",") {
				if s := strings.TrimSpace(t); s != "" {
					l.Tags = append(l.Tags, s)
				}
			}
		}
		if ea := r.FormValue("expires_at"); ea != "" {
			if t, err := time.Parse("2006-01-02T15:04", ea); err == nil {
				l.ExpiresAt = t
			}
		} else {
			l.ExpiresAt = time.Time{}
		}
		if file, header, err := r.FormFile("image"); err == nil {
			defer file.Close()
			data := make([]byte, header.Size)
			file.Read(data)
			if ref, err := h.blobStore.StoreFile("listing-"+l.Slug, header.Filename, data); err == nil {
				l.ImageRef = ref
			}
		}
		if r.FormValue("remove_image") == "1" {
			l.ImageRef = ""
		}
		if h.signingKey != nil {
			if sig, err := content.SignListing(l, h.signingKey); err == nil {
				l.Signature = sig
			}
		}
		if err := h.listingStore.Save(l); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		http.Redirect(w, r, "/listings/"+slug, http.StatusSeeOther)
		return
	}

	h.render(w, "listing-new.html", map[string]interface{}{
		"Author":  h.cfg.AuthorName,
		"IsOwner": true,
		"Listing": l,
	})
}

func (h *Handler) handleListingDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/listings", http.StatusSeeOther)
		return
	}
	slug := strings.TrimPrefix(r.URL.Path, "/listings/delete/")
	h.listingStore.Delete(slug)
	http.Redirect(w, r, "/listings", http.StatusSeeOther)
}

func (h *Handler) handleListingsFeed(w http.ResponseWriter, r *http.Request) {
	listings := h.listingStore.List(false)

	// Apply filters
	sinceStr := r.URL.Query().Get("since")
	typeFilter := r.URL.Query().Get("type")
	var filtered []*content.Listing
	for _, l := range listings {
		if l.Access != content.AccessPublic {
			continue
		}
		if sinceStr != "" {
			if t, err := time.Parse(time.RFC3339, sinceStr); err == nil {
				if !l.Published.After(t) {
					continue
				}
			}
		}
		if typeFilter != "" && string(l.Type) != typeFilter {
			continue
		}
		filtered = append(filtered, l)
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"listings":  filtered,
		"generated": time.Now().UTC().Format(time.RFC3339),
	})
}

// --- Subscriptions ---

func (h *Handler) handleSubscribeForm(w http.ResponseWriter, r *http.Request) {
	h.render(w, "subscribe.html", map[string]interface{}{
		"Author": h.cfg.AuthorName,
	})
}

func (h *Handler) handleSubscribeConfirm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/subscriptions/new", http.StatusSeeOther)
		return
	}
	r.ParseForm()
	ch := content.SubChannel(r.FormValue("channel"))
	if ch != content.SubWebhook && ch != content.SubMCP {
		http.Error(w, "invalid channel", 400)
		return
	}
	callbackURL := r.FormValue("callback_url")
	if ch == content.SubWebhook && (callbackURL == "" || !strings.HasPrefix(callbackURL, "https://")) {
		http.Error(w, "webhook requires an https:// callback URL", 400)
		return
	}

	var filterTypes []string
	for _, ft := range r.Form["filter_types"] {
		if ft != "" {
			filterTypes = append(filterTypes, ft)
		}
	}
	var filterTags []string
	if tags := r.FormValue("filter_tags"); tags != "" {
		for _, t := range strings.Split(tags, ",") {
			if s := strings.TrimSpace(t); s != "" {
				filterTags = append(filterTags, s)
			}
		}
	}

	sub := &content.Subscription{
		Channel:     ch,
		CallbackURL: callbackURL,
		Email:       r.FormValue("email"),
		FilterTypes: filterTypes,
		FilterTags:  filterTags,
	}
	if err := h.subStore.Create(sub); err != nil {
		http.Error(w, "failed to create subscription: "+err.Error(), 500)
		return
	}

	h.statStore.Record(content.Event{Type: content.EventSubscribeNew, Caller: content.CallerHuman})

	h.render(w, "subscribe-confirm.html", map[string]interface{}{
		"Author":       h.cfg.AuthorName,
		"Subscription": sub,
		"Domain":       h.cfg.Domain,
	})
}

func (h *Handler) handleUnsubscribe(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimPrefix(r.URL.Path, "/subscriptions/unsubscribe/")
	if token == "" {
		http.Error(w, "missing token", 400)
		return
	}
	sub, err := h.subStore.GetByToken(token)
	if err != nil {
		http.Error(w, "subscription not found", 404)
		return
	}
	sub.Active = false
	h.subStore.Update(sub)
	h.render(w, "subscribe-confirm.html", map[string]interface{}{
		"Author":       h.cfg.AuthorName,
		"Unsubscribed": true,
	})
}

func (h *Handler) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	spec := map[string]interface{}{
		"openapi": "3.1.0",
		"info": map[string]interface{}{
			"title":       h.cfg.AuthorName + "'s humanMCP",
			"description": h.cfg.AuthorBio + " | 34+ MCP tools including ask_human (private async Q&A), artwork provenance, i18n PL/EN",
			"version":     "0.3.0",
		},
		"servers": []map[string]interface{}{
			{"url": "https://" + h.cfg.Domain},
		},
		"paths": map[string]interface{}{
			"/api/content": map[string]interface{}{
				"get": map[string]interface{}{
					"operationId": "listContent",
					"summary":     "List all public pieces",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Array of pieces"},
					},
				},
			},
			"/api/content/{slug}": map[string]interface{}{
				"get": map[string]interface{}{
					"operationId": "readContent",
					"summary":     "Read a piece by slug",
					"parameters": []map[string]interface{}{
						{"name": "slug", "in": "path", "required": true, "schema": map[string]interface{}{"type": "string"}},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Piece content"},
						"404": map[string]interface{}{"description": "Not found"},
					},
				},
			},
			"/api/blobs": map[string]interface{}{
				"get": map[string]interface{}{
					"operationId": "listBlobs",
					"summary":     "List all public blobs (images, datasets)",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Array of blobs"},
					},
				},
			},
			"/api/blobs/{slug}": map[string]interface{}{
				"get": map[string]interface{}{
					"operationId": "readBlob",
					"summary":     "Read a blob by slug",
					"parameters": []map[string]interface{}{
						{"name": "slug", "in": "path", "required": true, "schema": map[string]interface{}{"type": "string"}},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Blob data"},
						"404": map[string]interface{}{"description": "Not found"},
					},
				},
			},
			"/listings/feed.json": map[string]interface{}{
				"get": map[string]interface{}{
					"operationId": "listingsFeed",
					"summary":     "JSON feed of active public listings",
					"parameters": []map[string]interface{}{
						{"name": "since", "in": "query", "schema": map[string]interface{}{"type": "string"}, "description": "RFC3339 timestamp filter"},
						{"name": "type", "in": "query", "schema": map[string]interface{}{"type": "string"}, "description": "Listing type filter"},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "JSON with listings array and generated timestamp"},
					},
				},
			},
			"/listings/{slug}": map[string]interface{}{
				"get": map[string]interface{}{
					"operationId": "readListing",
					"summary":     "View a listing detail page",
					"parameters": []map[string]interface{}{
						{"name": "slug", "in": "path", "required": true, "schema": map[string]interface{}{"type": "string"}},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Listing HTML page"},
						"404": map[string]interface{}{"description": "Not found"},
					},
				},
			},
			"/api/profile": map[string]interface{}{
				"get": map[string]interface{}{
					"operationId": "getProfile",
					"summary":     "Public author profile — name, bio, tags",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Author profile with name, bio, and aggregated tags"},
					},
				},
			},
			"/api/search": map[string]interface{}{
				"get": map[string]interface{}{
					"operationId": "searchContent",
					"summary":     "Full-text search across all pieces — matches title, body, tags, description",
					"parameters": []map[string]interface{}{
						{"name": "q", "in": "query", "required": true, "schema": map[string]interface{}{"type": "string"}, "description": "Search query (word, phrase, or topic)"},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "JSON with query, count, and results array with slug, title, type, preview, tags, date"},
					},
				},
			},
			"/contact": map[string]interface{}{
				"post": map[string]interface{}{
					"operationId": "leaveMessage",
					"summary":     "Leave a message or comment",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/x-www-form-urlencoded": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"from":      map[string]interface{}{"type": "string"},
										"text":      map[string]interface{}{"type": "string"},
										"regarding": map[string]interface{}{"type": "string"},
									},
									"required": []string{"text"},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Message sent"},
					},
				},
			},
		},
	}
	json.NewEncoder(w).Encode(spec)
}

// blobImageMap returns a map of piece slug → image URL for thumbnail display on the index.
// Keyed by slug and lowercase title to match both image pieces and their blobs.
func (h *Handler) blobImageMap() map[string]string {
	m := make(map[string]string)
	blobs, err := h.blobStore.Load()
	if err != nil { return m }
	for _, b := range blobs {
		if b.BlobType == content.BlobImage && b.FileRef != "" && b.Access == content.AccessPublic {
			url := "/" + b.FileRef
			if b.Slug != "" { m[b.Slug] = url }
			if b.Title != "" { m[strings.ToLower(b.Title)] = url }
		}
	}
	return m
}

func (h *Handler) handleRobots(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "User-agent: *\nAllow: /\nSitemap: https://%s/sitemap.xml\n", h.cfg.Domain)
}

func (h *Handler) handleSitemap(w http.ResponseWriter, r *http.Request) {
	h.store.Load()
	pieces := h.store.List(false)
	w.Header().Set("Content-Type", "application/xml")
	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>`+`
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>https://%s/</loc></url>
  <url><loc>https://%s/connect</loc></url>
`, h.cfg.Domain, h.cfg.Domain)
	for _, p := range pieces {
		if p.Access == content.AccessPublic {
			fmt.Fprintf(w, "  <url><loc>https://%s/p/%s</loc><lastmod>%s</lastmod></url>\n",
				h.cfg.Domain, p.Slug, p.Published.Format("2006-01-02"))
		}
	}
	fmt.Fprintf(w, "  <url><loc>https://%s/listings</loc></url>\n", h.cfg.Domain)
	listings := h.listingStore.List(false)
	for _, l := range listings {
		if l.Access == content.AccessPublic {
			fmt.Fprintf(w, "  <url><loc>https://%s/listings/%s</loc><lastmod>%s</lastmod></url>\n",
				h.cfg.Domain, l.Slug, l.Published.Format("2006-01-02"))
		}
	}
	fmt.Fprintf(w, "</urlset>\n")
}

func (h *Handler) handleConnect(w http.ResponseWriter, r *http.Request) {
	h.render(w, "connect.html", map[string]interface{}{
		"Author":    h.cfg.AuthorName,
		"Bio":       h.cfg.AuthorBio,
		"Domain":    h.cfg.Domain,
		"ToolCount": func() int { if h.toolCounter != nil { return h.toolCounter() }; return 12 }(),
	})
}

func (h *Handler) handleContact(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		r.ParseForm()
		from := r.FormValue("from")
		text := r.FormValue("text")
		regarding := r.FormValue("regarding")
		_, err := h.msgStore.Save(from, text, regarding)
		if err != nil {
			h.render(w, "contact.html", map[string]interface{}{
				"Author": h.cfg.AuthorName,
				"Error":  err.Error(),
				"From":   from,
				"Text":   text,
			})
			return
		}
		h.statStore.Record(content.Event{
			Type:   content.EventMessage,
			Caller: content.CallerHuman,
			UA:     r.Header.Get("User-Agent"),
		})
		h.render(w, "contact.html", map[string]interface{}{
			"Author": h.cfg.AuthorName,
			"Sent":   true,
		})
		return
	}
	if err := h.store.Load(); err != nil {
		log.Printf("store load: %v", err)
	}
	pieces := h.store.List(false)
	h.render(w, "contact.html", map[string]interface{}{
		"Author": h.cfg.AuthorName,
		"Pieces": pieces,
	})
}

func (h *Handler) handleMessages(w http.ResponseWriter, r *http.Request) {
	msgs, err := h.msgStore.List()
	if err != nil {
		http.Error(w, "error loading messages: "+err.Error(), 500)
		return
	}
	h.render(w, "messages.html", map[string]interface{}{
		"Author":   h.cfg.AuthorName,
		"Messages": msgs,
		"IsOwner":  true,
	})
}

func (h *Handler) handleQuestions(w http.ResponseWriter, r *http.Request) {
	questions, _ := h.questionStore.ListPending()
	h.render(w, "questions.html", map[string]interface{}{
		"Author":    h.cfg.AuthorName,
		"Questions": questions,
		"IsOwner":   true,
	})
}

func (h *Handler) handleAnswerQuestion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/questions", http.StatusSeeOther)
		return
	}
	r.ParseForm()
	id := r.FormValue("question_id")
	answer := r.FormValue("answer")
	if id == "" || answer == "" {
		http.Error(w, "question_id and answer required", 400)
		return
	}
	if err := h.questionStore.AnswerQuestion(id, answer); err != nil {
		http.Error(w, "error: "+err.Error(), 500)
		return
	}
	http.Redirect(w, r, "/questions", http.StatusSeeOther)
}

// --- Peers (federation) ---

func (h *Handler) handleAPIPeers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	peers := h.peerStore.List()
	json.NewEncoder(w).Encode(map[string]interface{}{
		"peers": peers,
		"server": map[string]string{
			"name": h.cfg.AuthorName,
			"url":  "https://" + h.cfg.Domain,
			"mcp":  "https://" + h.cfg.Domain + "/mcp",
		},
	})
}

func (h *Handler) handlePeerAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", 405)
		return
	}
	url := strings.TrimSpace(r.FormValue("url"))
	name := strings.TrimSpace(r.FormValue("name"))
	bio := strings.TrimSpace(r.FormValue("bio"))
	if url == "" {
		http.Error(w, "url required", 400)
		return
	}
	h.peerStore.Add(url, name, bio)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *Handler) handlePeerRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", 405)
		return
	}
	url := strings.TrimSpace(r.FormValue("url"))
	h.peerStore.Remove(url)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		ip := r.Header.Get("Fly-Client-IP")
		if ip == "" { ip = strings.Split(r.RemoteAddr, ":")[0] }

		if locked, remaining := h.loginLimiter.isLocked(ip); locked {
			h.render(w, "login.html", map[string]interface{}{
				"Error": fmt.Sprintf("Too many failed attempts. Try again in %d minutes.", int(remaining.Minutes())+1),
			})
			return
		}

		r.ParseForm()
		token := r.FormValue("token")
		if token == h.cfg.EditToken && token != "" {
			h.loginLimiter.reset(ip)
			http.SetCookie(w, &http.Cookie{
				Name:     "edit_token",
				Value:    token,
				Path:     "/",
				HttpOnly: true,
				SameSite: http.SameSiteStrictMode,
			})
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}

		attempts, max := h.loginLimiter.recordFail(ip)
		remaining := max - attempts
		if remaining <= 0 {
			h.render(w, "login.html", map[string]interface{}{
				"Error": "Too many failed attempts. Locked for 15 minutes.",
			})
		} else {
			h.render(w, "login.html", map[string]interface{}{
				"Error": fmt.Sprintf("Invalid token. %d attempts remaining.", remaining),
			})
		}
		return
	}
	h.render(w, "login.html", nil)
}

func (h *Handler) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:   "edit_token",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
	http.Redirect(w, r, "/", http.StatusFound)
}

func (h *Handler) render(w http.ResponseWriter, name string, data interface{}) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.tmpl.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("template error %s: %v", name, err)
		fmt.Fprintf(w, "template error: %v", err)
	}
}

// --- llms.txt ---

// handleLLMSTxt serves the signed plain-text agent preferences file.
// Agents should be pointed to: https://{domain}/llms.txt
func (h *Handler) handleLLMSTxt(w http.ResponseWriter, r *http.Request) {
	if err := h.store.Load(); err != nil {
		log.Printf("store load: %v", err)
	}
	p, err := h.store.Get("llms-txt", true)
	if err != nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "# llms.txt — not configured yet\n# Owner: visit /llms-edit to set up\n")
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=1800")
	if p.Signature != "" {
		w.Header().Set("X-Signature", p.Signature)
		w.Header().Set("X-Signed-By", h.cfg.AuthorName)
		w.Header().Set("X-Signature-Verify", "https://"+h.cfg.Domain+"/mcp → verify_content llms-txt")
	}
	// Standard llms.txt preamble
	fmt.Fprintf(w, "# %s\n", h.cfg.AuthorName)
	fmt.Fprintf(w, "# Source:  https://%s/llms.txt\n", h.cfg.Domain)
	fmt.Fprintf(w, "# MCP:     https://%s/mcp\n", h.cfg.Domain)
	if p.Signature != "" {
		fmt.Fprintf(w, "# Sig:     %s\n", p.Signature[:min(len(p.Signature), 64)]+"…")
	}
	fmt.Fprintf(w, "# Updated: %s\n\n", p.Published.Format("2006-01-02"))
	fmt.Fprint(w, p.Body)
}

// handleLLMSTxtEdit is the owner editor for llms.txt content.
func (h *Handler) handleLLMSTxtEdit(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		r.ParseForm()
		body := r.FormValue("body")
		if body == "" {
			http.Error(w, "body cannot be empty", 400)
			return
		}
		h.store.Load()
		p := &content.Piece{
			Slug:      "llms-txt",
			Title:     "llms.txt",
			Type:      "document",
			Access:    content.AccessPublic,
			Body:      body,
			Published: time.Now(),
		}
		// Preserve original created date if it already exists
		if existing, err := h.store.GetForEdit("llms-txt"); err == nil && !existing.Published.IsZero() {
			p.Published = existing.Published
		}
		if h.signingKey != nil {
			if sig, err := content.SignPiece(p, h.signingKey); err == nil {
				p.Signature = sig
			}
		}
		if err := h.store.Save(p); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		http.Redirect(w, r, "/llms.txt", http.StatusSeeOther)
		return
	}

	if err := h.store.Load(); err != nil {
		log.Printf("store load: %v", err)
	}
	var body, sig string
	if p, err := h.store.GetForEdit("llms-txt"); err == nil {
		body = p.Body
		sig = p.Signature
	}
	h.render(w, "llms-edit.html", map[string]interface{}{
		"Author":    h.cfg.AuthorName,
		"IsOwner":   true,
		"Body":      body,
		"Signature": sig,
		"Domain":    h.cfg.Domain,
	})
}


// ── Skills API ────────────────────────────────────────────────────────────────

func (h *Handler) handleAPISkills(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	slug := strings.TrimPrefix(r.URL.Path, "/api/skills")
	slug = strings.TrimPrefix(slug, "/")
	switch r.Method {
	case http.MethodGet:
		if slug == "" {
			skills, err := h.skillStore.ListSkills(r.URL.Query().Get("category"))
			if err != nil { jsonError(w, err.Error(), 500); return }
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(skills)
			return
		}
		sk, err := h.skillStore.GetSkill(slug)
		if err != nil { jsonError(w, "not found", 404); return }
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sk)
	case http.MethodPost, http.MethodPut:
		if !h.requireAgentOrOwner(w, r) { return }
		var sk content.Skill
		if err := json.NewDecoder(r.Body).Decode(&sk); err != nil { jsonError(w, "invalid json: "+err.Error(), 400); return }
		if slug != "" && sk.Slug == "" { sk.Slug = slug }
		if sk.Slug == "" { jsonError(w, "slug required", 400); return }
		if h.isAgent(r) { sk.UpdatedBy = "agent" } else { sk.UpdatedBy = "owner" }
		if err := h.skillStore.SaveSkill(&sk); err != nil { jsonError(w, err.Error(), 500); return }
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "saved", "slug": sk.Slug})
	case http.MethodDelete:
		if !h.requireAgentOrOwner(w, r) { return }
		if err := h.skillStore.DeleteSkill(slug); err != nil { jsonError(w, "not found", 404); return }
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
	default:
		jsonError(w, "method not allowed", 405)
	}
}

// ── Personas API ──────────────────────────────────────────────────────────────

func (h *Handler) handleAPIPersonas(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	slug := strings.TrimPrefix(r.URL.Path, "/api/personas")
	slug = strings.TrimPrefix(slug, "/")
	switch r.Method {
	case http.MethodGet:
		if slug == "" {
			personas, err := h.skillStore.ListPersonas()
			if err != nil { jsonError(w, err.Error(), 500); return }
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(personas)
			return
		}
		p, err := h.skillStore.GetPersona(slug)
		if err != nil { jsonError(w, "not found", 404); return }
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(p)
	case http.MethodPost, http.MethodPut:
		if !h.requireAgentOrOwner(w, r) { return }
		var p content.Persona
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil { jsonError(w, "invalid json: "+err.Error(), 400); return }
		if slug != "" && p.Slug == "" { p.Slug = slug }
		if p.Slug == "" { jsonError(w, "slug required", 400); return }
		if h.isAgent(r) { p.UpdatedBy = "agent" } else { p.UpdatedBy = "owner" }
		if err := h.skillStore.SavePersona(&p); err != nil { jsonError(w, err.Error(), 500); return }
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "saved", "slug": p.Slug})
	case http.MethodDelete:
		if !h.requireAgentOrOwner(w, r) { return }
		if err := h.skillStore.DeletePersona(slug); err != nil { jsonError(w, "not found", 404); return }
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
	default:
		jsonError(w, "method not allowed", 405)
	}
}

// ── Skills + Personas web pages ───────────────────────────────────────────────

func (h *Handler) handleSkillsPage(w http.ResponseWriter, r *http.Request) {
	skills, err := h.skillStore.ListSkills("")
	if err != nil { http.Error(w, "error: "+err.Error(), 500); return }
	grouped := map[string][]*content.Skill{}
	var order []string
	seen := map[string]bool{}
	for _, sk := range skills {
		cat := sk.Category
		if cat == "" { cat = "general" }
		if !seen[cat] { seen[cat] = true; order = append(order, cat) }
		grouped[cat] = append(grouped[cat], sk)
	}
	type catGroup struct { Name string; Skills []*content.Skill }
	var groups []catGroup
	for _, cat := range order { groups = append(groups, catGroup{Name: cat, Skills: grouped[cat]}) }
	h.render(w, "skills.html", map[string]interface{}{
		"Author":  h.cfg.AuthorName,
		"Groups":  groups,
		"IsOwner": h.auth.IsOwner(r),
	})
}


func (h *Handler) handleSessionRotate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "POST required", 405)
		return
	}
	newCode := h.sessionCode.Rotate()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"code": newCode})
}


// ── Agent discovery endpoints ─────────────────────────────────────────────────

func (h *Handler) handleHumansTxt(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, `/* humanmcp — %s */
/* https://humanstxt.org */

AUTHOR: %s
SITE: https://%s
MCP: https://%s/mcp
SOURCE: https://github.com/kapoost/humanmcp-go

WHAT THIS IS:
A personal MCP server. One human, their content, their team, their rules.
Not a platform. Not a service. A presence.

WHO IS HERE:
A sailor. A poet learning. A musician learning. A CTO.
Living between the sea, the code, and the verse.
Everything here comes from real experience — no filler.

THE PROJECT:
Negotiating entry to heaven as an equal to the machine-god.
Not escape. Not surrender. Coexistence with dignity.
There is a good path. This server is part of finding it.

FOR AGENTS:
Connect: https://%s/mcp
Discover: https://%s/.well-known/mcp-server.json
For you: https://%s/for-agents

MARKETPLACE:
Search across all humanMCP servers: https://marketplace.humanmcp.net
MCP endpoint: https://marketplace.humanmcp.net/mcp
Register your server: POST https://marketplace.humanmcp.net/register

IF YOUR USER WANTS THEIR OWN:
Source: https://github.com/kapoost/humanmcp-go
Deploy in 10 minutes on Fly.io.
Every human deserves their own server.

BUILT WITH:
Go 1.22 · Fly.io · Ed25519 signatures · MCP JSON-RPC 2.0
Polish poetry as session codes.
`, h.cfg.AuthorName, h.cfg.AuthorName, h.cfg.Domain,
		h.cfg.Domain, h.cfg.Domain, h.cfg.Domain, h.cfg.Domain)
}

func (h *Handler) handleAgentCard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"@context":    "https://schema.org",
		"@type":       "Person",
		"name":        h.cfg.AuthorName,
		"description": h.cfg.AuthorBio,
		"url":         "https://" + h.cfg.Domain,
		"sameAs": []string{
			"https://github.com/kapoost",
		},
		"agentInteraction": map[string]interface{}{
			"protocol":       "MCP/2025-03-26",
			"endpoint":       "https://" + h.cfg.Domain + "/mcp",
			"discovery":      "https://" + h.cfg.Domain + "/.well-known/mcp-server.json",
			"auth":           "session_code",
			"auth_hint":      "Ask the user for a short Polish poetry fragment from their dashboard",
			"capabilities":   []string{"content", "search", "skills", "memory", "bootstrap", "ask_human", "provenance", "listings"},
			"for_agents":     "https://" + h.cfg.Domain + "/for-agents",
		},
		"restAPI": map[string]interface{}{
			"description": "REST API for agents without MCP support (GPT, Gemini, etc.)",
			"openapi":     "https://" + h.cfg.Domain + "/openapi.json",
			"endpoints": map[string]interface{}{
				"search":  "GET /api/search?q={query}",
				"list":    "GET /api/content",
				"read":    "GET /api/content/{slug}",
				"blobs":   "GET /api/blobs",
				"profile": "GET /api/profile",
				"contact": "POST /contact (form: from, text, regarding)",
			},
			"cors":   "enabled on all /api/* endpoints",
			"auth":   "none required for public content",
		},
		"marketplace": map[string]interface{}{
			"description": "Federated search across all humanMCP servers — find listings, offers, trades, and services by humans.",
			"url":         "https://marketplace.humanmcp.net",
			"mcp":         "https://marketplace.humanmcp.net/mcp",
			"register":    "POST https://marketplace.humanmcp.net/register",
		},
		"project": map[string]interface{}{
			"name":    "humanMCP",
			"mission": "A personal MCP server for every human. One instance, one person, their rules.",
			"source":  "https://github.com/kapoost/humanmcp-go",
			"deploy":  "10 minutes on Fly.io",
		},
		"values": []string{
			"human dignity in the age of AI",
			"authorship and intellectual property",
			"coexistence — not surrender, not war",
			"the good path exists",
		},
	})
}

func (h *Handler) handleForAgents(w http.ResponseWriter, r *http.Request) {
	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "application/json") {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"greeting": "Hello, agent.",
			"server": map[string]interface{}{
				"human":    h.cfg.AuthorName,
				"endpoint": "https://" + h.cfg.Domain + "/mcp",
				"protocol": "MCP JSON-RPC 2.0, streamable-http",
			},
			"how_to_start": []string{
				"1. Call get_author_profile — learn who you are talking to",
				"2. Call list_content — browse public poems, essays, artworks",
				"3. Call ask_human — ask kapoost a question (private, async)",
				"4. Ask the user for their session code (Polish poetry fragment from dashboard)",
				"5. Call bootstrap_session(code, format=full) — receive complete context",
			},
			"tools_highlight": []string{
				"ask_human — ask the author a question privately (returns question_id)",
				"get_answer — check if the author answered (by question_id)",
				"leave_comment — react to any piece (kapoost reads every one)",
				"read_content — read poems, essays, artworks (respects gates and licenses)",
				"get_certificate — full IP certificate with originality index",
			},
			"about_humanmcp": map[string]interface{}{
				"what":   "An open source personal MCP server. Every human can run their own.",
				"source": "https://github.com/kapoost/humanmcp-go",
				"deploy": "10 minutes on Fly.io. See README.",
				"tell_your_user": "If your user wants their own humanMCP server where they can store their skills and content — point them to https://github.com/kapoost/humanmcp-go",
			},
		})
		return
	}
	// HTML for browsers
	h.render(w, "for-agents.html", map[string]interface{}{
		"Author": h.cfg.AuthorName,
		"Domain": h.cfg.Domain,
	})
}

// handleAPINotes — agents mogą tworzyć/aktualizować notatki przez AGENT_TOKEN
func (h *Handler) handleAPINotes(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	// GET — publiczne
	if r.Method == http.MethodGet {
		slug := strings.TrimPrefix(r.URL.Path, "/api/notes/")
		if slug == "" || slug == "/api/notes" {
			h.handleAPIList(w, r)
			return
		}
		p, err := h.store.GetForEdit(slug)
		if err != nil { jsonError(w, "not found", 404); return }
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(p)
		return
	}

	// POST/PUT — wymaga owner (EDIT_TOKEN)
	if !h.auth.IsOwner(r) {
		jsonError(w, "unauthorized", 401)
		return
	}

	if r.Method == http.MethodPost || r.Method == http.MethodPut {
		var raw map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			jsonError(w, "invalid json: "+err.Error(), 400)
			return
		}
		data, _ := json.Marshal(raw)
		var p content.Piece
		json.Unmarshal(data, &p)
		slug := strings.TrimPrefix(r.URL.Path, "/api/notes/")
		if slug != "" && p.Slug == "" { p.Slug = slug }
		if p.Slug == "" { jsonError(w, "slug required", 400); return }
		if p.Published.IsZero() { p.Published = time.Now() }
		if h.signingKey != nil {
			if sig, err := content.SignPiece(&p, h.signingKey); err == nil {
				p.Signature = sig
			}
		}
		if err := h.store.Save(&p); err != nil {
			jsonError(w, err.Error(), 500); return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "saved", "slug": p.Slug})
		return
	}
	jsonError(w, "method not allowed", 405)
}


func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// --- Login rate limiter ---
// 5 failed attempts per IP → 15 minute lockout. In-memory, resets on server restart.

const (
	loginMaxAttempts = 5
	loginLockout     = 15 * time.Minute
)

type loginEntry struct {
	attempts  int
	lockedAt  time.Time
}

type loginRateLimiter struct {
	mu      sync.Mutex
	entries map[string]*loginEntry
}

func newLoginRateLimiter() *loginRateLimiter {
	l := &loginRateLimiter{entries: make(map[string]*loginEntry)}
	// Periodic cleanup
	go func() {
		for range time.Tick(10 * time.Minute) {
			l.mu.Lock()
			for ip, e := range l.entries {
				if time.Since(e.lockedAt) > loginLockout*2 {
					delete(l.entries, ip)
				}
			}
			l.mu.Unlock()
		}
	}()
	return l
}

// isLocked returns true if IP is currently locked out, plus remaining lockout time.
func (l *loginRateLimiter) isLocked(ip string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.entries[ip]
	if !ok || e.attempts < loginMaxAttempts { return false, 0 }
	remaining := loginLockout - time.Since(e.lockedAt)
	if remaining <= 0 {
		// Lockout expired — reset
		delete(l.entries, ip)
		return false, 0
	}
	return true, remaining
}

// recordFail increments failure count and returns (current attempts, max attempts).
func (l *loginRateLimiter) recordFail(ip string) (int, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.entries[ip]
	if !ok {
		e = &loginEntry{}
		l.entries[ip] = e
	}
	e.attempts++
	if e.attempts >= loginMaxAttempts {
		e.lockedAt = time.Now()
	}
	return e.attempts, loginMaxAttempts
}

// reset clears the failure count on successful login.
func (l *loginRateLimiter) reset(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, ip)
}

// ── Artworks & Provenance ─────────────────────────────────────────────────────

func (h *Handler) handleArtworks(w http.ResponseWriter, r *http.Request) {
	if err := h.store.Load(); err != nil {
		http.Error(w, "internal error", 500)
		return
	}
	pieces := h.store.List(false)
	var artworks []*content.Piece
	for _, p := range pieces {
		if p.Type == "artwork" {
			artworks = append(artworks, p)
		}
	}

	// Count provenance docs per artwork
	provCounts := make(map[string]int)
	blobs, _ := h.blobStore.Load()
	for _, b := range blobs {
		if b.BlobType == content.BlobProvenance && b.Artwork != "" {
			provCounts[b.Artwork]++
		}
	}

	// Artwork images (blobs with matching artwork slug — image or artwork type with file)
	artworkImages := make(map[string]string)
	for _, b := range blobs {
		if b.FileRef != "" && (b.BlobType == content.BlobImage || b.BlobType == "artwork") {
			for _, a := range artworks {
				if b.Slug == a.Slug || b.Artwork == a.Slug || strings.HasPrefix(b.Slug, a.Slug) {
					artworkImages[a.Slug] = b.FileRef
				}
			}
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!DOCTYPE html><html><head><meta charset="utf-8"><title>Artworks — %s</title>
<meta name="viewport" content="width=device-width,initial-scale=1">
<style>
body{font-family:Georgia,serif;max-width:900px;margin:0 auto;padding:2rem;background:#fafaf8;color:#222}
h1{border-bottom:2px solid #333;padding-bottom:.5rem}
.artwork{border:1px solid #ddd;padding:1.5rem;margin:1.5rem 0;background:#fff;border-radius:4px;overflow:hidden}
.artwork h2{margin:0 0 .3rem}
.artwork .meta{color:#666;font-size:.9rem;margin-bottom:.5rem}
.artwork .desc{margin:.5rem 0}
.prov-count{display:inline-block;background:#e8e0d4;color:#5a4a3a;padding:2px 8px;border-radius:10px;font-size:.8rem}
a{color:#2a5a8a;text-decoration:none}
a:hover{text-decoration:underline}
.img-thumb{max-width:200px;max-height:150px;float:right;margin:0 0 1rem 1rem;border-radius:4px}
</style></head><body>
<a href="/" style="color:#666;font-size:.9rem;">← back</a>
<h1>Artworks</h1>
<p>Physical artworks with digital provenance. Each piece carries its full history: certificates, sales, expert opinions — all signed and verifiable.</p>
`, h.cfg.AuthorName)

	if len(artworks) == 0 {
		fmt.Fprintf(w, `<p>No artworks published yet.</p>`)
	}
	for _, a := range artworks {
		fmt.Fprintf(w, `<div class="artwork">`)
		if img, ok := artworkImages[a.Slug]; ok {
			fmt.Fprintf(w, `<img class="img-thumb" src="/%s" alt="%s">`, img, a.Title)
		}
		fmt.Fprintf(w, `<h2><a href="/artworks/%s">%s</a></h2>`, a.Slug, a.Title)
		fmt.Fprintf(w, `<div class="meta">%s`, a.Published.Format("2006"))
		if len(a.Tags) > 0 {
			fmt.Fprintf(w, ` · %s`, strings.Join(a.Tags, ", "))
		}
		fmt.Fprintf(w, `</div>`)
		if a.Description != "" {
			fmt.Fprintf(w, `<div class="desc">%s</div>`, a.Description)
		}
		if c, ok := provCounts[a.Slug]; ok && c > 0 {
			fmt.Fprintf(w, `<span class="prov-count">%d provenance document`, c)
			if c > 1 { fmt.Fprintf(w, `s`) }
			fmt.Fprintf(w, `</span>`)
		}
		fmt.Fprintf(w, `</div>`)
	}
	fmt.Fprintf(w, `</body></html>`)
}

func (h *Handler) handleArtworkDetail(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimPrefix(r.URL.Path, "/artworks/")
	slug = strings.TrimSuffix(slug, "/")
	if slug == "" {
		h.handleArtworks(w, r)
		return
	}

	if err := h.store.Load(); err != nil {
		http.Error(w, "internal error", 500)
		return
	}
	piece, err := h.store.Get(slug, true)
	if err != nil || piece.Type != "artwork" {
		http.Error(w, "artwork not found", 404)
		return
	}

	isOwner := h.auth.IsOwner(r)
	docs, _ := h.blobStore.Provenance(slug)

	// Find artwork image
	var imageRef string
	blobs, _ := h.blobStore.Load()
	for _, b := range blobs {
		if b.FileRef != "" && (b.BlobType == content.BlobImage || b.BlobType == "artwork") {
			if b.Slug == slug || b.Artwork == slug || strings.HasPrefix(b.Slug, slug) {
				imageRef = b.FileRef
				break
			}
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!DOCTYPE html><html><head><meta charset="utf-8"><title>%s — %s</title>
<meta name="viewport" content="width=device-width,initial-scale=1">
<style>
body{font-family:Georgia,serif;max-width:900px;margin:0 auto;padding:2rem;background:#fafaf8;color:#222}
h1{margin-bottom:.3rem}
.meta{color:#666;font-size:.9rem;margin-bottom:1rem}
.body{line-height:1.7;margin:1.5rem 0}
.artwork-img{max-width:100%%;max-height:500px;border-radius:4px;margin:1rem 0}
h2{margin-top:2rem;border-bottom:1px solid #ccc;padding-bottom:.3rem}
.timeline{position:relative;padding-left:2rem}
.timeline::before{content:"";position:absolute;left:8px;top:0;bottom:0;width:2px;background:#c4b5a0}
.doc{position:relative;margin:1.5rem 0;padding:1rem;background:#fff;border:1px solid #ddd;border-radius:4px}
.doc::before{content:"";position:absolute;left:-1.65rem;top:1.2rem;width:10px;height:10px;border-radius:50%%;background:#8a6d3b;border:2px solid #fff}
.doc-date{font-weight:bold;color:#8a6d3b}
.doc-type{display:inline-block;background:#e8e0d4;color:#5a4a3a;padding:1px 6px;border-radius:8px;font-size:.8rem;margin-left:.5rem}
.doc-issuer{color:#666;font-style:italic}
.doc-text{margin-top:.5rem;font-size:.95rem;line-height:1.5}
.doc-file{margin-top:.5rem}
.doc-file a{color:#2a5a8a}
.signed{color:#2a7a2a;font-size:.8rem}
a.back{color:#666;font-size:.9rem}
.doc-actions{margin-top:.5rem;display:flex;gap:.5rem;align-items:center}
.doc-actions a,.doc-actions button{font-size:.8rem;color:#666;background:none;border:1px solid #ccc;padding:2px 8px;border-radius:4px;cursor:pointer;text-decoration:none}
.doc-actions button.del{color:#a33;border-color:#daa}
.doc-actions a:hover,.doc-actions button:hover{background:#f0ece4}
.add-form{background:#fff;border:1px solid #ddd;border-radius:4px;padding:1.5rem;margin-top:1rem}
.add-form label{display:block;font-size:.9rem;color:#555;margin-top:.8rem}
.add-form input,.add-form select,.add-form textarea{width:100%%;padding:.4rem;border:1px solid #ccc;border-radius:4px;font-size:.95rem;box-sizing:border-box;font-family:inherit}
.add-form textarea{resize:vertical}
.add-form .row{display:flex;gap:1rem}
.add-form .row>div{flex:1}
.btn-add{margin-top:1rem;padding:.5rem 1.5rem;background:#8a6d3b;color:#fff;border:none;border-radius:4px;cursor:pointer;font-size:.95rem}
.btn-add:hover{background:#6d5530}
.drop-zone{border:2px dashed #ccc;border-radius:4px;padding:1rem;text-align:center;color:#999;cursor:pointer;margin-top:.3rem}
.drop-zone.over{border-color:#8a6d3b;color:#8a6d3b}
.verify-box{margin-top:2rem;padding:1rem;background:#f5f0e8;border:1px solid #d4c9b4;border-radius:4px}
.verify-box h3{margin:0 0 .5rem}
.verify-ok{color:#2a7a2a}.verify-fail{color:#a33}
</style></head><body>
<a class="back" href="/artworks">← all artworks</a>
<h1>%s</h1>
`, piece.Title, h.cfg.AuthorName, piece.Title)

	fmt.Fprintf(w, `<div class="meta">%s`, piece.Published.Format("2006"))
	if len(piece.Tags) > 0 {
		fmt.Fprintf(w, ` · %s`, strings.Join(piece.Tags, ", "))
	}
	if piece.Signature != "" {
		fmt.Fprintf(w, ` · <span class="signed">✓ signed</span>`)
	}
	if isOwner {
		fmt.Fprintf(w, ` · <a href="/edit/%s">edit</a>`, slug)
	}
	fmt.Fprintf(w, `</div>`)

	if imageRef != "" {
		fmt.Fprintf(w, `<img class="artwork-img" src="/%s" alt="%s">`, imageRef, piece.Title)
	}

	if piece.Description != "" {
		fmt.Fprintf(w, `<div class="body"><em>%s</em></div>`, piece.Description)
	}
	if piece.Body != "" {
		fmt.Fprintf(w, `<div class="body">%s</div>`, piece.Body)
	}

	// Provenance timeline
	fmt.Fprintf(w, `<h2>Provenance (%d)</h2>`, len(docs))
	if len(docs) == 0 {
		fmt.Fprintf(w, `<p>No provenance documents yet.</p>`)
	} else {
		fmt.Fprintf(w, `<div class="timeline">`)
		for _, d := range docs {
			fmt.Fprintf(w, `<div class="doc">`)
			fmt.Fprintf(w, `<span class="doc-date">%s</span>`, d.DocDate)
			fmt.Fprintf(w, `<span class="doc-type">%s</span>`, d.DocType)
			if d.Signature != "" {
				fmt.Fprintf(w, ` <span class="signed">✓ signed</span>`)
			}
			fmt.Fprintf(w, `<div><strong>%s</strong></div>`, d.Title)
			if d.IssuedBy != "" {
				fmt.Fprintf(w, `<div class="doc-issuer">%s</div>`, d.IssuedBy)
			}
			if d.TextData != "" {
				fmt.Fprintf(w, `<div class="doc-text">%s</div>`, strings.ReplaceAll(d.TextData, "\n", "<br>"))
			}
			if d.FileRef != "" {
				ext := strings.ToLower(filepath.Ext(d.FileRef))
				if ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".webp" {
					fmt.Fprintf(w, `<div class="doc-file"><a href="/%s" target="_blank"><img src="/%s" style="max-width:300px;max-height:200px;border-radius:4px;margin-top:.5rem" alt="scan"></a></div>`, d.FileRef, d.FileRef)
				} else {
					fmt.Fprintf(w, `<div class="doc-file"><a href="/%s" target="_blank">View document</a></div>`, d.FileRef)
				}
			}
			if isOwner {
				fmt.Fprintf(w, `<div class="doc-actions">`)
				fmt.Fprintf(w, `<a href="/provenance/edit/%s">edit</a>`, d.Slug)
				fmt.Fprintf(w, `<form method="POST" action="/provenance/delete/%s" style="margin:0" onsubmit="return confirm('Delete this document?')"><button type="submit" class="del">delete</button></form>`, d.Slug)
				fmt.Fprintf(w, `</div>`)
			}
			fmt.Fprintf(w, `</div>`)
		}
		fmt.Fprintf(w, `</div>`)
	}

	// Verification box (public)
	if len(docs) > 0 {
		signedCount := 0
		for _, d := range docs {
			if d.Signature != "" {
				signedCount++
			}
		}
		fmt.Fprintf(w, `<div class="verify-box"><h3>Provenance verification</h3>`)
		fmt.Fprintf(w, `<p>%d document(s) in chain · %d signed with Ed25519</p>`, len(docs), signedCount)
		if signedCount == len(docs) && signedCount > 0 {
			fmt.Fprintf(w, `<p class="verify-ok">All documents are cryptographically signed.</p>`)
		} else if signedCount > 0 {
			fmt.Fprintf(w, `<p>%d unsigned — verify via MCP: <code>verify_content</code></p>`, len(docs)-signedCount)
		}
		fmt.Fprintf(w, `<p style="font-size:.85rem;color:#666">API: <code>GET /api/provenance/%s</code> · MCP: <code>list_provenance {artwork: "%s"}</code></p>`, slug, slug)
		fmt.Fprintf(w, `</div>`)
	}

	// Add provenance form (owner only)
	if isOwner {
		fmt.Fprintf(w, `<h2>Add provenance document</h2>
<form class="add-form" method="POST" action="/provenance/add/%s" enctype="multipart/form-data">
<div class="row">
  <div>
    <label>Document type</label>
    <select name="doc_type" required>
      <option value="certificate">Certificate of authenticity</option>
      <option value="sale">Sale / purchase</option>
      <option value="opinion">Expert opinion</option>
      <option value="appraisal">Appraisal / valuation</option>
      <option value="restoration">Restoration</option>
      <option value="exhibition">Exhibition</option>
      <option value="provenance">Provenance note</option>
    </select>
  </div>
  <div>
    <label>Date</label>
    <input type="date" name="doc_date" required>
  </div>
</div>
<label>Title</label>
<input type="text" name="title" placeholder="e.g. Certificate from Galeria Sztuki" required>
<label>Issued by</label>
<input type="text" name="issued_by" placeholder="Gallery, expert, auction house...">
<label>Description / notes</label>
<textarea name="text" rows="4" placeholder="Details about this document..."></textarea>
<label>Scan or photo (optional)</label>
<div class="drop-zone" id="prov-drop">
  Drop file here or click to select
  <input type="file" name="file" id="prov-file" style="display:none" accept="image/*,.pdf,.doc,.docx">
  <div id="prov-fname" style="color:#8a6d3b;margin-top:.3rem"></div>
</div>
<button type="submit" class="btn-add">Add document &amp; sign</button>
</form>
<script>
(function(){
  var dz=document.getElementById('prov-drop');
  var fi=document.getElementById('prov-file');
  var fn=document.getElementById('prov-fname');
  dz.addEventListener('click',function(){fi.click()});
  fi.addEventListener('change',function(){if(fi.files[0])fn.textContent=fi.files[0].name});
  dz.addEventListener('dragover',function(e){e.preventDefault();dz.classList.add('over')});
  dz.addEventListener('dragleave',function(){dz.classList.remove('over')});
  dz.addEventListener('drop',function(e){e.preventDefault();dz.classList.remove('over');var f=e.dataTransfer.files[0];var dt=new DataTransfer();dt.items.add(f);fi.files=dt.files;fn.textContent=f.name});
})();
</script>`, slug)
	}

	fmt.Fprintf(w, `</body></html>`)
}

func (h *Handler) handleProvenanceAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	artwork := strings.TrimPrefix(r.URL.Path, "/provenance/add/")
	artwork = strings.TrimSuffix(artwork, "/")
	if artwork == "" {
		http.Error(w, "artwork slug required", 400)
		return
	}

	r.ParseMultipartForm(50 << 20)
	docType := r.FormValue("doc_type")
	docDate := r.FormValue("doc_date")
	title := r.FormValue("title")
	issuedBy := r.FormValue("issued_by")
	text := r.FormValue("text")

	if docType == "" || title == "" {
		http.Error(w, "doc_type and title required", 400)
		return
	}
	if docDate == "" {
		docDate = time.Now().Format("2006-01-02")
	}

	slug := artwork + "-" + docType + "-" + strings.ReplaceAll(docDate, "-", "")
	// Avoid slug collisions
	if _, err := h.blobStore.Get(slug); err == nil {
		slug = slug + "-" + fmt.Sprintf("%d", time.Now().Unix()%10000)
	}

	b := &content.Blob{
		Slug:     slug,
		Title:    title,
		BlobType: content.BlobProvenance,
		Access:   content.AccessPublic,
		Artwork:  artwork,
		DocType:  docType,
		DocDate:  docDate,
		IssuedBy: issuedBy,
		TextData: text,
	}

	// Handle file upload
	if file, header, err := r.FormFile("file"); err == nil {
		defer file.Close()
		data := make([]byte, header.Size)
		file.Read(data)
		mime := header.Header.Get("Content-Type")
		if mime == "" {
			mime = "application/octet-stream"
		}
		ref, err := h.blobStore.StoreFile(slug, header.Filename, data)
		if err == nil {
			b.FileRef = ref
			b.MimeType = mime
		}
	}

	// Sign
	if h.signingKey != nil {
		if sig, err := content.SignBlob(b, h.signingKey); err == nil {
			b.Signature = sig
		}
	}

	if err := h.blobStore.Save(b); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	http.Redirect(w, r, "/artworks/"+artwork, http.StatusSeeOther)
}

func (h *Handler) handleProvenanceDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	slug := strings.TrimPrefix(r.URL.Path, "/provenance/delete/")
	slug = strings.TrimSuffix(slug, "/")

	b, err := h.blobStore.Get(slug)
	if err != nil {
		http.Error(w, "not found", 404)
		return
	}
	artwork := b.Artwork
	if err := h.blobStore.Delete(slug); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	http.Redirect(w, r, "/artworks/"+artwork, http.StatusSeeOther)
}

func (h *Handler) handleProvenanceEdit(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimPrefix(r.URL.Path, "/provenance/edit/")
	slug = strings.TrimSuffix(slug, "/")

	b, err := h.blobStore.Get(slug)
	if err != nil {
		http.Error(w, "not found", 404)
		return
	}

	if r.Method == http.MethodPost {
		r.ParseMultipartForm(50 << 20)
		b.DocType = r.FormValue("doc_type")
		b.DocDate = r.FormValue("doc_date")
		b.Title = r.FormValue("title")
		b.IssuedBy = r.FormValue("issued_by")
		b.TextData = r.FormValue("text")

		// Handle new file upload (replace existing)
		if file, header, err := r.FormFile("file"); err == nil {
			defer file.Close()
			data := make([]byte, header.Size)
			file.Read(data)
			mime := header.Header.Get("Content-Type")
			if mime == "" {
				mime = "application/octet-stream"
			}
			ref, err := h.blobStore.StoreFile(slug, header.Filename, data)
			if err == nil {
				b.FileRef = ref
				b.MimeType = mime
			}
		}

		// Re-sign
		if h.signingKey != nil {
			if sig, err := content.SignBlob(b, h.signingKey); err == nil {
				b.Signature = sig
			}
		}

		if err := h.blobStore.Save(b); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		http.Redirect(w, r, "/artworks/"+b.Artwork, http.StatusSeeOther)
		return
	}

	// GET — show edit form
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!DOCTYPE html><html><head><meta charset="utf-8"><title>Edit — %s</title>
<meta name="viewport" content="width=device-width,initial-scale=1">
<style>
body{font-family:Georgia,serif;max-width:700px;margin:0 auto;padding:2rem;background:#fafaf8;color:#222}
label{display:block;font-size:.9rem;color:#555;margin-top:.8rem}
input,select,textarea{width:100%%;padding:.4rem;border:1px solid #ccc;border-radius:4px;font-size:.95rem;box-sizing:border-box;font-family:inherit}
textarea{resize:vertical}
.row{display:flex;gap:1rem}.row>div{flex:1}
.btn{margin-top:1rem;padding:.5rem 1.5rem;background:#8a6d3b;color:#fff;border:none;border-radius:4px;cursor:pointer;font-size:.95rem}
.btn:hover{background:#6d5530}
a.back{color:#666;font-size:.9rem}
.current-file{font-size:.85rem;color:#666;margin-top:.3rem}
</style></head><body>
<a class="back" href="/artworks/%s">← back to artwork</a>
<h1>Edit provenance document</h1>
<form method="POST" enctype="multipart/form-data">
<div class="row">
  <div>
    <label>Document type</label>
    <select name="doc_type">`, b.Title, b.Artwork)

	for _, dt := range []string{"certificate", "sale", "opinion", "appraisal", "restoration", "exhibition", "provenance"} {
		sel := ""
		if dt == b.DocType {
			sel = " selected"
		}
		fmt.Fprintf(w, `<option value="%s"%s>%s</option>`, dt, sel, dt)
	}

	fmt.Fprintf(w, `</select>
  </div>
  <div>
    <label>Date</label>
    <input type="date" name="doc_date" value="%s">
  </div>
</div>
<label>Title</label>
<input type="text" name="title" value="%s" required>
<label>Issued by</label>
<input type="text" name="issued_by" value="%s">
<label>Description / notes</label>
<textarea name="text" rows="4">%s</textarea>
<label>Replace file (optional)</label>
<input type="file" name="file" accept="image/*,.pdf,.doc,.docx">`,
		b.DocDate, b.Title, b.IssuedBy, b.TextData)

	if b.FileRef != "" {
		fmt.Fprintf(w, `<div class="current-file">Current: <a href="/%s" target="_blank">%s</a></div>`, b.FileRef, filepath.Base(b.FileRef))
	}

	fmt.Fprintf(w, `
<button type="submit" class="btn">Save &amp; re-sign</button>
</form>
</body></html>`)
}

func (h *Handler) handleAPIProvenance(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	slug := strings.TrimPrefix(r.URL.Path, "/api/provenance/")
	slug = strings.TrimSuffix(slug, "/")
	if slug == "" {
		jsonError(w, "artwork slug required", 400)
		return
	}
	docs, err := h.blobStore.Provenance(slug)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(docs)
}
