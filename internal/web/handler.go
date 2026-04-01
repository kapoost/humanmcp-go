package web

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/kapoost/humanmcp-go/internal/auth"
	"github.com/kapoost/humanmcp-go/internal/config"
	"github.com/kapoost/humanmcp-go/internal/content"
)

type ToolCounter interface {
	ToolCount() int
}

type Handler struct {
	cfg      *config.Config
	store    *content.Store
	auth     *auth.Auth
	msgStore   *content.MessageStore
	statStore  *content.StatStore
	blobStore  *content.BlobStore
	signingKey *content.KeyPair // parsed once at startup
	toolCounter ToolCounter
	tmpl  *template.Template
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
		"excerpt": func(s string, n int) string {
			// strip blank lines, return first n chars cut at word boundary
			lines := strings.Split(strings.TrimSpace(s), "\n")
			var out []string
			for _, l := range lines {
				if t := strings.TrimSpace(l); t != "" { out = append(out, t) }
				if len(out) >= 3 { break }
			}
			result := strings.Join(out, " ")
			if len(result) <= n { return result }
			// cut at last space before n
			cut := result[:n]
			if idx := strings.LastIndex(cut, " "); idx > n/2 { cut = cut[:idx] }
			return cut + "…"
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
	}).Parse(allTemplates))
	return h
}

func (h *Handler) SetToolCounter(tc ToolCounter) { h.toolCounter = tc }

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", h.handleIndex)
	mux.HandleFunc("/p/", h.handlePiece)
	mux.HandleFunc("/unlock/", h.handleUnlock)

	// Owner API (require edit token)
	mux.Handle("/api/content", h.auth.RequireOwner(http.HandlerFunc(h.handleAPIList)))
	mux.Handle("/api/content/", h.auth.RequireOwner(http.HandlerFunc(h.handleAPIContent)))

	// Well-known MCP discovery
	mux.HandleFunc("/.well-known/mcp-server.json", h.handleWellKnown)

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


	// Blob uploader UI (owner only)
	mux.Handle("/upload", h.auth.RequireOwner(http.HandlerFunc(h.handleUploadPage)))

	// Blob upload (owner only)
	mux.Handle("/api/blobs", h.auth.RequireOwner(http.HandlerFunc(h.handleAPIBlobs)))
	mux.Handle("/api/blobs/", h.auth.RequireOwner(http.HandlerFunc(h.handleAPIBlobs)))

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
		"version":     "0.1.0",
		"homepage":    "https://kapoost.github.io/humanmcp",
		"repository":  map[string]string{"url": "https://github.com/kapoost/humanmcp", "source": "github"},
		"remotes": []map[string]interface{}{
			{"type": "streamable-http", "url": "https://" + h.cfg.Domain + "/mcp"},
		},
		"tags": []string{"content", "publishing", "poetry", "intellectual-property", "personal", "creative"},
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
	blobs, _ := h.blobStore.Load()
	blobsBySlug := make(map[string]*content.Blob)
	for _, b := range blobs { blobsBySlug[b.Slug] = b }
	h.render(w, "index.html", map[string]interface{}{
		"Author":      h.cfg.AuthorName,
		"Bio":         h.cfg.AuthorBio,
		"Pieces":      pieces,
		"IsOwner":     isOwner,
		"Domain":      h.cfg.Domain,
		"BlobsBySlug": blobsBySlug,
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
	var imageBlob *content.Blob
	if p.Type == "image" {
		if b, err := h.blobStore.Get(slug); err == nil { imageBlob = b }
	}
	h.render(w, "piece.html", map[string]interface{}{
		"Author":     h.cfg.AuthorName,
		"Piece":      p,
		"IsLocked":   isLocked,
		"IsOwner":    isOwner,
		"UnlockDate": unlockDate,
		"ImageBlob":  imageBlob,
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

func (h *Handler) handleUploadPage(w http.ResponseWriter, r *http.Request) {
	h.render(w, "blob-uploader.html", map[string]interface{}{
		"Author":  h.cfg.AuthorName,
		"IsOwner": true,
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

		// If a file is attached — save as image blob and redirect to /images
		if file, header, err := r.FormFile("file"); err == nil {
			defer file.Close()
			data := make([]byte, header.Size)
			file.Read(data)
			slug := r.FormValue("slug")
			if slug == "" {
				if r.FormValue("title") != "" {
					slug = slugify(r.FormValue("title"))
				} else {
					slug = slugify(header.Filename)
				}
			}
			ref, err := h.blobStore.StoreFile(slug, header.Filename, data)
			if err != nil { http.Error(w, "file save error: "+err.Error(), 500); return }
			mimeType := header.Header.Get("Content-Type")
			if mimeType == "" { mimeType = "image/jpeg" }
			b := &content.Blob{
				Slug:        slug,
				Title:       r.FormValue("title"),
				BlobType:    content.BlobImage,
				Description: r.FormValue("description"),
				Access:      content.AccessPublic,
				MimeType:    mimeType,
				FileRef:     ref,
				Published:   time.Now(),
			}
			if tags := r.FormValue("tags"); tags != "" {
				for _, t := range strings.Split(tags, ",") {
					if s := strings.TrimSpace(t); s != "" { b.Tags = append(b.Tags, s) }
				}
			}
			if h.signingKey != nil {
				if sig, err := content.SignBlob(b, h.signingKey); err == nil { b.Signature = sig }
			}
			if err := h.blobStore.Save(b); err != nil { http.Error(w, err.Error(), 500); return }
			http.Redirect(w, r, "/images", http.StatusSeeOther)
			return
		}

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
		if ps := r.FormValue("price_sats"); ps != "" { fmt.Sscanf(ps, "%d", &p.PriceSats) }
		if tags := r.FormValue("tags"); tags != "" {
			for _, t := range strings.Split(tags, ",") {
				if s := strings.TrimSpace(t); s != "" {
					p.Tags = append(p.Tags, s)
				}
			}
		}
		if p.Title == "" { p.Title = firstLine(p.Body) }
		if h.signingKey != nil {
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
			data, err := h.blobStore.ReadFile(b.FileRef)
			if err != nil { http.NotFound(w, r); return }
			w.Header().Set("Content-Type", b.MimeType)
			w.Header().Set("Cache-Control", "public, max-age=86400")
			w.Write(data)
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

func (h *Handler) getToolCount() int {
	if h.toolCounter != nil { return h.toolCounter.ToolCount() }
	return 0
}

func (h *Handler) handleConnect(w http.ResponseWriter, r *http.Request) {
	h.render(w, "connect.html", map[string]interface{}{
		"Author":    h.cfg.AuthorName,
		"Bio":       h.cfg.AuthorBio,
		"Domain":    h.cfg.Domain,
		"ToolCount": h.getToolCount(),
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
