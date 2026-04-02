package web

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/kapoost/humanmcp-go/internal/auth"
	"github.com/kapoost/humanmcp-go/internal/config"
	"github.com/kapoost/humanmcp-go/internal/content"
)

type Handler struct {
	cfg         *config.Config
	store       *content.Store
	auth        *auth.Auth
	msgStore    *content.MessageStore
	statStore   *content.StatStore
	blobStore   *content.BlobStore
	signingKey  *content.KeyPair
	toolCounter func() int
	tmpl        *template.Template
}

func NewHandler(cfg *config.Config, store *content.Store, a *auth.Auth) *Handler {
	h := &Handler{cfg: cfg, store: store, auth: a, msgStore: content.NewMessageStore(cfg.ContentDir), statStore: content.NewStatStore(cfg.ContentDir), blobStore: content.NewBlobStore(cfg.ContentDir)}
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

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", h.handleIndex)
	mux.HandleFunc("/p/", h.handlePiece)
	mux.HandleFunc("/unlock/", h.handleUnlock)

	// Owner API (require edit token)
	mux.Handle("/api/content", h.auth.RequireOwner(http.HandlerFunc(h.handleAPIList)))
	mux.Handle("/api/content/", h.auth.RequireOwner(http.HandlerFunc(h.handleAPIContent)))

	// Well-known MCP discovery
	mux.HandleFunc("/.well-known/mcp-server.json", h.handleWellKnown)
	mux.HandleFunc("/.well-known/mcp.json", h.handleWellKnown) // alias for standard discovery

	// OpenAPI spec for ChatGPT and other REST-based agents
	mux.HandleFunc("/openapi.json", h.handleOpenAPI)

	// Dashboard (owner only)
	mux.Handle("/dashboard", h.auth.RequireOwner(http.HandlerFunc(h.handleDashboard)))

	// Messages (owner only)
	mux.Handle("/messages", h.auth.RequireOwner(http.HandlerFunc(h.handleMessages)))

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
	mux.Handle("/api/blobs", h.auth.RequireOwner(http.HandlerFunc(h.handleAPIBlobs)))
	mux.Handle("/api/blobs/", h.auth.RequireOwner(http.HandlerFunc(h.handleAPIBlobs)))

	// Timestamp on demand (owner only, POST)
	mux.Handle("/timestamp/", h.auth.RequireOwner(http.HandlerFunc(h.handleTimestamp)))

	// Login/logout for web UI
	mux.HandleFunc("/login", h.handleLogin)
	mux.HandleFunc("/logout", h.handleLogout)
}

func (h *Handler) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	spec := map[string]interface{}{
		"openapi": "3.1.0",
		"info": map[string]interface{}{
			"title":       h.cfg.AuthorName + "'s humanMCP",
			"description": h.cfg.AuthorBio + " — Read poems, essays, images and data published by " + h.cfg.AuthorName + ".",
			"version":     "0.2.0",
		},
		"servers": []map[string]interface{}{
			{"url": "https://" + h.cfg.Domain, "description": "Live server"},
		},
		"paths": map[string]interface{}{
			"/api/content": map[string]interface{}{
				"get": map[string]interface{}{
					"operationId": "listContent",
					"summary":     "List all public pieces",
					"description": "Returns a list of all public poems, essays, notes and other pieces published by " + h.cfg.AuthorName + ".",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Array of pieces",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type":  "array",
										"items": map[string]interface{}{"$ref": "#/components/schemas/Piece"},
									},
								},
							},
						},
					},
				},
			},
			"/api/content/{slug}": map[string]interface{}{
				"get": map[string]interface{}{
					"operationId": "readContent",
					"summary":     "Read a specific piece",
					"description": "Returns full content of a poem, essay, note or image piece.",
					"parameters": []map[string]interface{}{
						{"name": "slug", "in": "path", "required": true, "schema": map[string]interface{}{"type": "string"}, "description": "URL slug of the piece"},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Piece content", "content": map[string]interface{}{"application/json": map[string]interface{}{"schema": map[string]interface{}{"$ref": "#/components/schemas/Piece"}}}},
						"404": map[string]interface{}{"description": "Not found"},
					},
				},
			},
			"/api/blobs": map[string]interface{}{
				"get": map[string]interface{}{
					"operationId": "listBlobs",
					"summary":     "List all public blobs",
					"description": "Returns images, datasets, vectors and other typed data published by " + h.cfg.AuthorName + ".",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Array of blobs",
							"content": map[string]interface{}{
								"application/json": map[string]interface{}{
									"schema": map[string]interface{}{
										"type":  "array",
										"items": map[string]interface{}{"$ref": "#/components/schemas/Blob"},
									},
								},
							},
						},
					},
				},
			},
			"/api/blobs/{slug}": map[string]interface{}{
				"get": map[string]interface{}{
					"operationId": "readBlob",
					"summary":     "Read a specific blob",
					"parameters": []map[string]interface{}{
						{"name": "slug", "in": "path", "required": true, "schema": map[string]interface{}{"type": "string"}},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{"description": "Blob data"},
						"404": map[string]interface{}{"description": "Not found"},
					},
				},
			},
			"/contact": map[string]interface{}{
				"post": map[string]interface{}{
					"operationId": "leaveMessage",
					"summary":     "Leave a message or comment",
					"description": "Send a message to " + h.cfg.AuthorName + ". Optionally reference a specific piece.",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/x-www-form-urlencoded": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"from":      map[string]interface{}{"type": "string", "description": "Your name or handle (optional)"},
										"text":      map[string]interface{}{"type": "string", "description": "Your message (max 2000 chars)"},
										"regarding": map[string]interface{}{"type": "string", "description": "Slug of the piece you are commenting on (optional)"},
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
		"components": map[string]interface{}{
			"schemas": map[string]interface{}{
				"Piece": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"Slug":        map[string]interface{}{"type": "string"},
						"Title":       map[string]interface{}{"type": "string"},
						"Type":        map[string]interface{}{"type": "string", "enum": []string{"poem", "essay", "note", "image", "contact"}},
						"Body":        map[string]interface{}{"type": "string"},
						"Description": map[string]interface{}{"type": "string"},
						"License":     map[string]interface{}{"type": "string"},
						"Tags":        map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
						"Published":   map[string]interface{}{"type": "string", "format": "date-time"},
						"Signature":   map[string]interface{}{"type": "string", "description": "Ed25519 signature — verifies authenticity"},
					},
				},
				"Blob": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"Slug":     map[string]interface{}{"type": "string"},
						"Title":    map[string]interface{}{"type": "string"},
						"BlobType": map[string]interface{}{"type": "string"},
						"FileRef":  map[string]interface{}{"type": "string", "description": "Relative URL — prepend https://" + h.cfg.Domain + "/"},
						"MimeType": map[string]interface{}{"type": "string"},
					},
				},
			},
		},
	}
	json.NewEncoder(w).Encode(spec)
}


func (h *Handler) handleWellKnown(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"$schema":     "https://static.modelcontextprotocol.io/schemas/2025-12-11/server.schema.json",
		"name":        "io.github.kapoost/humanmcp",
		"title":       h.cfg.AuthorName + "'s humanMCP",
		"description": h.cfg.AuthorBio,
		"version":     "0.2.0",
		"homepage":    "https://kapoost.github.io/humanmcp",
		"repository":  "https://github.com/kapoost/humanmcp",
		"remotes": []map[string]interface{}{
			{"type": "streamable-http", "url": "https://" + h.cfg.Domain + "/mcp"},
		},
		"tags": []string{"content", "publishing", "poetry", "intellectual-property", "personal", "creative"},
	})
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

// blobImageMap returns a map keyed by blob slug AND lowercase title → "/files/..." URL.
// Templates use it to match image pieces to their blobs even when slugs differ.
func (h *Handler) blobImageMap() map[string]string {
	m := make(map[string]string)
	blobs, err := h.blobStore.Load()
	if err != nil {
		return m
	}
	for _, b := range blobs {
		if b.FileRef == "" {
			continue
		}
		url := "/" + b.FileRef
		if b.Slug != "" {
			m[b.Slug] = url
		}
		if b.Title != "" {
			m[strings.ToLower(b.Title)] = url
		}
	}
	return m
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
	h.render(w, "index.html", map[string]interface{}{
		"Author":       h.cfg.AuthorName,
		"Bio":          h.cfg.AuthorBio,
		"Pieces":       pieces,
		"IsOwner":      isOwner,
		"Domain":       h.cfg.Domain,
		"BlobImageMap": h.blobImageMap(),
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
		http.NotFound(w, r)
		return
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

func (h *Handler) handleAPIList(w http.ResponseWriter, r *http.Request) {
	if err := h.store.Load(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	pieces := h.store.List(true)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pieces)
}

func (h *Handler) handleAPIContent(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimPrefix(r.URL.Path, "/api/content/")

	switch r.Method {
	case http.MethodGet:
		p, err := h.store.GetForEdit(slug)
		if err != nil {
			jsonError(w, "not found", 404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(p)

	case http.MethodPut, http.MethodPost:
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
		// Async OTS timestamp — non-blocking, failure is non-fatal
		go func(piece content.Piece) {
			if proof, err := content.TimestampPiece(&piece); err == nil && proof != "" {
				piece.OTSProof = proof
				h.store.Save(&piece)
			}
		}(p)
		if err := h.store.Save(&p); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "saved", "slug": p.Slug})

	case http.MethodDelete:
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
	h.render(w, "dashboard.html", map[string]interface{}{
		"Author":   h.cfg.AuthorName,
		"IsOwner":  true,
		"Stats":    stats,
		"Pieces":   pieces,
		"Messages": msgs,
	})
}

func (h *Handler) handleAPIBlobs(w http.ResponseWriter, r *http.Request) {
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

		pieceType := r.FormValue("type")
		if pieceType == "" { pieceType = "note" }

		// Option D: image pieces get a pure timestamp slug shared with their blob
		var unifiedSlug string
		if pieceType == "image" {
			unifiedSlug = fmt.Sprintf("%d", time.Now().Unix())
		}

		p := content.Piece{
			Title:       r.FormValue("title"),
			Type:        pieceType,
			Access:      content.AccessLevel(r.FormValue("access")),
			Gate:        content.GateType(r.FormValue("gate")),
			Challenge:   r.FormValue("challenge"),
			Answer:      r.FormValue("answer"),
			Description: r.FormValue("description"),
			Body:        r.FormValue("body"),
			Published:   time.Now(),
		}

		// Slug priority: slug_override > slug field > unified (image) > title+timestamp
		if r.FormValue("slug_override") != "" {
			p.Slug = r.FormValue("slug_override")
		} else if r.FormValue("slug") != "" {
			p.Slug = r.FormValue("slug")
		} else if unifiedSlug != "" {
			p.Slug = unifiedSlug
		} else {
			p.Slug = slugify(r.FormValue("title") + " " + fmt.Sprintf("%d", time.Now().Unix()))
		}

		p.License = r.FormValue("license")
		if ps := r.FormValue("price_sats"); ps != "" { fmt.Sscanf(ps, "%d", &p.PriceSats) }
		if tags := r.FormValue("tags"); tags != "" {
			for _, t := range strings.Split(tags, ",") {
				if s := strings.TrimSpace(t); s != "" {
					p.Tags = append(p.Tags, s)
				}
			}
		}
		if p.Title == "" { p.Title = firstLine(p.Body) }

		// For image pieces with a file upload, atomically create the blob with the same slug
		if pieceType == "image" {
			if file, header, err := r.FormFile("file"); err == nil {
				defer file.Close()
				data := make([]byte, header.Size)
				file.Read(data)
				mimeType := header.Header.Get("Content-Type")
				ref, err := h.blobStore.StoreFile(p.Slug, header.Filename, data)
				if err != nil {
					http.Error(w, "file save error: "+err.Error(), 500); return
				}
				b := content.Blob{
					Slug:     p.Slug,
					Title:    p.Title,
					BlobType: content.BlobImage,
					Access:   p.Access,
					MimeType: mimeType,
					FileRef:  ref,
					Tags:     p.Tags,
				}
				if h.signingKey != nil && r.FormValue("do_sign") == "1" {
					if sig, err := content.SignBlob(&b, h.signingKey); err == nil {
						b.Signature = sig
					}
				}
				if err := h.blobStore.Save(&b); err != nil {
					http.Error(w, "blob save error: "+err.Error(), 500); return
				}
			}
		}

		if h.signingKey != nil && r.FormValue("do_sign") == "1" {
			if sig, err := content.SignPiece(&p, h.signingKey); err == nil {
				p.Signature = sig
			}
		}
		if err := h.store.Save(&p); err != nil {
			http.Error(w, err.Error(), 500); return
		}
		http.Redirect(w, r, "/p/"+p.Slug, http.StatusSeeOther)
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
		// If content changed, clear the existing signature — it no longer matches
		// Only re-sign if the owner explicitly clicked "Save & Sign"
		if r.FormValue("do_sign") == "1" && h.signingKey != nil {
			if sig, err := content.SignPiece(p, h.signingKey); err == nil {
				p.Signature = sig
			}
		} else if p.Body != "" {
			// Clear signature when saving without signing — content may have changed
			p.Signature = ""
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
			// BlobStore roots at {parent(ContentDir)}/blobs — match that path.
			filePath := filepath.Join(filepath.Dir(h.cfg.ContentDir), "blobs", "files", slug)
			w.Header().Set("Content-Type", b.MimeType)
			w.Header().Set("Cache-Control", "public, max-age=86400")
			http.ServeFile(w, r, filePath)
			return
		}
	}
	http.NotFound(w, r)
}

func (h *Handler) handleImages(w http.ResponseWriter, r *http.Request) {
	blobs, _ := h.blobStore.Load()
	var images []*content.Blob
	for _, b := range blobs {
		if b.BlobType == content.BlobImage && b.Access == content.AccessPublic {
			images = append(images, b)
		}
	}
	h.render(w, "images.html", map[string]interface{}{
		"Author": h.cfg.AuthorName,
		"Images": images,
		"Domain": h.cfg.Domain,
	})
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
	fmt.Fprintf(w, "</urlset>\n")
}

func (h *Handler) handleConnect(w http.ResponseWriter, r *http.Request) {
	toolCount := 12
	if h.toolCounter != nil {
		toolCount = h.toolCounter()
	}
	h.render(w, "connect.html", map[string]interface{}{
		"Author":    h.cfg.AuthorName,
		"Bio":       h.cfg.AuthorBio,
		"Domain":    h.cfg.Domain,
		"ToolCount": toolCount,
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
	regarding := r.URL.Query().Get("regarding")
	data := map[string]interface{}{
		"Author":    h.cfg.AuthorName,
		"Regarding": regarding,
	}
	if regarding != "" {
		if p, err := h.store.Get(regarding, false); err == nil {
			data["RegardingTitle"] = p.Title
		}
	}
	h.render(w, "contact.html", data)
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

func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		r.ParseForm()
		token := r.FormValue("token")
		if token == h.cfg.EditToken && token != "" {
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
		h.render(w, "login.html", map[string]interface{}{"Error": "Invalid token"})
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

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
