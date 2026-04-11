package web

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kapoost/humanmcp-go/internal/auth"
	"github.com/kapoost/humanmcp-go/internal/config"
	"github.com/kapoost/humanmcp-go/internal/content"
)

// ── helpers ──────────────────────────────────────────────────────────────────


func testStores(t *testing.T) (*content.SessionCode, *content.MemoryStore, *content.SkillStore) {
	t.Helper()
	sc := content.NewSessionCode(24 * time.Hour)
	ms := content.NewMemoryStore(t.TempDir())
	ss := content.NewSkillStore(t.TempDir())
	return sc, ms, ss
}

func newTestHandler(t *testing.T) (*Handler, string) {
	sc, ms, ss := testStores(t)
	t.Helper()
	dir := t.TempDir()
	cfg := &config.Config{
		AuthorName: "testuser",
		Domain:     "localhost:8080",
		ContentDir: dir,
		EditToken:  "secret",
	}
	store := content.NewStore(dir)
	a := auth.New("secret")
	h := NewHandler(cfg, store, a, sc, ms, ss)
	return h, dir
}

func serve(h *Handler, r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	mux.ServeHTTP(w, r)
	return w
}

func ownerGet(path string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, path, nil)
	r.AddCookie(&http.Cookie{Name: "edit_token", Value: "secret"})
	return r
}

func ownerPost(path string, fields map[string]string) *http.Request {
	form := url.Values{}
	for k, v := range fields {
		form.Set(k, v)
	}
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(&http.Cookie{Name: "edit_token", Value: "secret"})
	return r
}

func seedPiece(t *testing.T, dir string, slug, body, access string) {
	t.Helper()
	md := fmt.Sprintf("---\nslug: %s\ntitle: Test\naccess: %s\ntype: note\n---\n%s", slug, access, body)
	os.WriteFile(filepath.Join(dir, slug+".md"), []byte(md), 0644)
}

// ── template integrity ────────────────────────────────────────────────────────

func TestAllTemplatesDefined(t *testing.T) {
	h, _ := newTestHandler(t)
	for _, name := range []string{
		"index.html", "piece.html", "login.html", "dashboard.html",
		"contact.html", "connect.html", "new.html", "images.html",
		"messages.html",
		"css", "header", "header-simple", "footer",
	} {
		if h.tmpl.Lookup(name) == nil {
			t.Errorf("template %q not defined", name)
		}
	}
}

// ── public pages ──────────────────────────────────────────────────────────────

func TestIndexRenders(t *testing.T) {
	h, _ := newTestHandler(t)
	w := serve(h, httptest.NewRequest("GET", "/", nil))
	if w.Code != 200 { t.Fatalf("index: got %d", w.Code) }
	if strings.Contains(w.Body.String(), "template error") {
		t.Error("index has template error")
	}
}

func TestImagesPageRenders(t *testing.T) {
	h, _ := newTestHandler(t)
	w := serve(h, httptest.NewRequest("GET", "/images", nil))
	if w.Code != 200 { t.Fatalf("images: got %d", w.Code) }
	if !strings.Contains(w.Body.String(), "Images") {
		t.Error("images page missing heading")
	}
}

func TestConnectPageShowsDynamicToolCount(t *testing.T) {
	h, _ := newTestHandler(t)
	counter := &mockCounter{n: 7}
	h.SetToolCounter(counter)
	w := serve(h, httptest.NewRequest("GET", "/connect", nil))
	if w.Code != 200 { t.Fatalf("connect: got %d", w.Code) }
	if !strings.Contains(w.Body.String(), "7") {
		t.Error("connect page should show dynamic tool count 7")
	}
}

type mockCounter struct{ n int }
func (m *mockCounter) ToolCount() int { return m.n }

func TestPublicPieceRenders(t *testing.T) {
	h, dir := newTestHandler(t)
	seedPiece(t, dir, "hello", "some content", "public")
	w := serve(h, httptest.NewRequest("GET", "/p/hello", nil))
	if w.Code != 200 { t.Fatalf("piece: got %d", w.Code) }
	if !strings.Contains(w.Body.String(), "some content") {
		t.Error("piece body missing")
	}
}

func TestLockedPieceHidesBody(t *testing.T) {
	h, dir := newTestHandler(t)
	seedPiece(t, dir, "secret", "TOP SECRET CONTENT", "locked")
	w := serve(h, httptest.NewRequest("GET", "/p/secret", nil))
	if w.Code != 200 { t.Fatalf("got %d", w.Code) }
	if strings.Contains(w.Body.String(), "TOP SECRET CONTENT") {
		t.Error("locked piece body leaked to public")
	}
}

func TestLockedPieceVisibleToOwner(t *testing.T) {
	h, dir := newTestHandler(t)
	seedPiece(t, dir, "secret", "TOP SECRET CONTENT", "locked")
	w := serve(h, ownerGet("/p/secret"))
	if w.Code != 200 { t.Fatalf("got %d", w.Code) }
	if !strings.Contains(w.Body.String(), "TOP SECRET CONTENT") {
		t.Error("owner should see locked piece body")
	}
}

func TestUnknownPiece404(t *testing.T) {
	h, _ := newTestHandler(t)
	w := serve(h, httptest.NewRequest("GET", "/p/doesnotexist", nil))
	if w.Code != 404 { t.Errorf("expected 404, got %d", w.Code) }
}

// ── auth protection ───────────────────────────────────────────────────────────

func TestNewPageRequiresAuth(t *testing.T) {
	h, _ := newTestHandler(t)
	w := serve(h, httptest.NewRequest("GET", "/new", nil))
	if w.Code != 302 { t.Errorf("expected redirect, got %d", w.Code) }
}

func TestEditPageRequiresAuth(t *testing.T) {
	h, dir := newTestHandler(t)
	seedPiece(t, dir, "hello", "body", "public")
	w := serve(h, httptest.NewRequest("GET", "/edit/hello", nil))
	if w.Code != 302 { t.Errorf("expected redirect, got %d", w.Code) }
}

func TestDeleteRequiresAuth(t *testing.T) {
	h, dir := newTestHandler(t)
	seedPiece(t, dir, "hello", "body", "public")
	r := httptest.NewRequest(http.MethodPost, "/delete/hello", nil)
	w := serve(h, r)
	if w.Code != 302 { t.Errorf("expected redirect, got %d", w.Code) }
	// piece must still exist
	w2 := serve(h, httptest.NewRequest("GET", "/p/hello", nil))
	if w2.Code != 200 { t.Error("piece should still exist after unauth delete attempt") }
}

func TestDashboardRequiresAuth(t *testing.T) {
	h, _ := newTestHandler(t)
	w := serve(h, httptest.NewRequest("GET", "/dashboard", nil))
	if w.Code != 302 { t.Errorf("expected redirect, got %d", w.Code) }
}

func TestWrongTokenRejected(t *testing.T) {
	h, _ := newTestHandler(t)
	r := httptest.NewRequest("GET", "/new", nil)
	r.AddCookie(&http.Cookie{Name: "edit_token", Value: "wrongtoken"})
	w := serve(h, r)
	if w.Code != 302 { t.Errorf("expected redirect, got %d", w.Code) }
}

// ── post creation ─────────────────────────────────────────────────────────────

func TestOwnerCanCreatePost(t *testing.T) {
	h, _ := newTestHandler(t)
	w := serve(h, ownerPost("/new", map[string]string{
		"title": "My Poem",
		"body":  "roses are red",
		"type":  "poem",
		"access": "public",
	}))
	if w.Code != 303 { t.Fatalf("expected redirect after create, got %d", w.Code) }
}

func TestOwnerCanDeletePost(t *testing.T) {
	h, dir := newTestHandler(t)
	seedPiece(t, dir, "todelete", "bye", "public")
	h.store.Load()
	w := serve(h, ownerPost("/delete/todelete", nil))
	if w.Code != 303 { t.Fatalf("delete: got %d", w.Code) }
	w2 := serve(h, httptest.NewRequest("GET", "/p/todelete", nil))
	if w2.Code != 404 { t.Error("piece should be gone after delete") }
}

func TestDeleteNonexistentReturns404(t *testing.T) {
	h, _ := newTestHandler(t)
	w := serve(h, ownerPost("/delete/ghost", nil))
	if w.Code != 404 { t.Errorf("expected 404, got %d", w.Code) }
}

func TestDeleteRequiresPost(t *testing.T) {
	h, dir := newTestHandler(t)
	seedPiece(t, dir, "nodelete", "body", "public")
	w := serve(h, ownerGet("/delete/nodelete"))
	if w.Code == 200 { t.Error("GET to /delete/ should not delete") }
	w2 := serve(h, httptest.NewRequest("GET", "/p/nodelete", nil))
	if w2.Code != 200 { t.Error("piece should still exist after GET to /delete/") }
}

// ── user input abuse ──────────────────────────────────────────────────────────

func TestXSSInContactFormBlocked(t *testing.T) {
	h, _ := newTestHandler(t)
	w := serve(h, ownerPost("/contact", map[string]string{
		"text": "<script>alert('xss')</script>",
		"from": "hacker",
	}))
	body := w.Body.String()
	if strings.Contains(body, "<script>") {
		t.Error("XSS not blocked in contact form")
	}
}

func TestOversizedContactMessageTruncated(t *testing.T) {
	h, _ := newTestHandler(t)
	big := strings.Repeat("a", 10000)
	w := serve(h, ownerPost("/contact", map[string]string{
		"text": big,
		"from": "flood",
	}))
	if w.Code == 500 { t.Error("oversized message caused 500") }
}

func TestHTMLInjectionInPostBodyEscaped(t *testing.T) {
	h, dir := newTestHandler(t)
	seedPiece(t, dir, "xss", "<script>alert(1)</script>", "public")
	w := serve(h, httptest.NewRequest("GET", "/p/xss", nil))
	if strings.Contains(w.Body.String(), "<script>alert(1)") {
		t.Error("HTML in piece body not escaped")
	}
}

func TestPathTraversalBlocked(t *testing.T) {
	h, _ := newTestHandler(t)
	w := serve(h, httptest.NewRequest("GET", "/p/../../../etc/passwd", nil))
	if w.Code == 200 && strings.Contains(w.Body.String(), "root:") {
		t.Error("path traversal succeeded")
	}
}

func TestEmptyContactMessageRejected(t *testing.T) {
	h, _ := newTestHandler(t)
	w := serve(h, ownerPost("/contact", map[string]string{
		"text": "",
		"from": "someone",
	}))
	if w.Code == 303 { t.Error("empty message should not be accepted") }
}

// ── file serving ──────────────────────────────────────────────────────────────

func TestFileServingFindsCorrectPath(t *testing.T) {
	h, dir := newTestHandler(t)
	// blobStore uses filepath.Dir(contentDir)/blobs — match that
	blobsDir := filepath.Join(filepath.Dir(dir), "blobs", "files")
	os.MkdirAll(blobsDir, 0755)
	os.WriteFile(filepath.Join(blobsDir, "test-img.jpg"), []byte("JPEG_DATA"), 0644)
	b := &content.Blob{
		Slug: "test-img", Title: "Test",
		BlobType: content.BlobImage,
		Access:   content.AccessPublic,
		MimeType: "image/jpeg",
		FileRef:  "files/test-img.jpg",
	}
	h.blobStore.Save(b)
	w := serve(h, httptest.NewRequest("GET", "/files/test-img.jpg", nil))
	if w.Code != 200 { t.Errorf("file serving: expected 200, got %d", w.Code) }
	if !strings.Contains(w.Body.String(), "JPEG_DATA") {
		t.Error("file content not served")
	}
}

func TestLockedFileBlocked(t *testing.T) {
	h, dir := newTestHandler(t)
	blobsDir := filepath.Join(filepath.Dir(dir), "blobs", "files")
	os.MkdirAll(blobsDir, 0755)
	os.WriteFile(filepath.Join(blobsDir, "private.jpg"), []byte("SECRET"), 0644)
	b := &content.Blob{
		Slug: "private", Title: "Private",
		BlobType: content.BlobImage,
		Access:   content.AccessLevel("locked"),
		MimeType: "image/jpeg",
		FileRef:  "files/private.jpg",
	}
	h.blobStore.Save(b)
	w := serve(h, httptest.NewRequest("GET", "/files/private.jpg", nil))
	if w.Code == 200 { t.Error("locked file should not be served publicly") }
}

func TestUnknownFile404(t *testing.T) {
	h, _ := newTestHandler(t)
	w := serve(h, httptest.NewRequest("GET", "/files/ghost.jpg", nil))
	if w.Code != 404 { t.Errorf("expected 404, got %d", w.Code) }
}

// ── SEO / well-known ──────────────────────────────────────────────────────────

func TestRobotsServed(t *testing.T) {
	h, _ := newTestHandler(t)
	w := serve(h, httptest.NewRequest("GET", "/robots.txt", nil))
	if w.Code != 200 { t.Fatalf("robots.txt: got %d", w.Code) }
	if !strings.Contains(w.Body.String(), "Sitemap") {
		t.Error("robots.txt missing Sitemap")
	}
}

func TestSitemapContainsPublicPieces(t *testing.T) {
	h, dir := newTestHandler(t)
	seedPiece(t, dir, "public-poem", "body", "public")
	seedPiece(t, dir, "private-poem", "body", "locked")
	w := serve(h, httptest.NewRequest("GET", "/sitemap.xml", nil))
	if w.Code != 200 { t.Fatalf("sitemap: got %d", w.Code) }
	body := w.Body.String()
	if !strings.Contains(body, "public-poem") { t.Error("sitemap missing public piece") }
	if strings.Contains(body, "private-poem") { t.Error("sitemap should not contain locked piece") }
}

func TestWellKnownMCPServer(t *testing.T) {
	h, _ := newTestHandler(t)
	w := serve(h, httptest.NewRequest("GET", "/.well-known/mcp-server.json", nil))
	if w.Code != 200 { t.Fatalf("well-known: got %d", w.Code) }
	if !strings.Contains(w.Body.String(), "io.github.kapoost") {
		t.Error("well-known missing registry name")
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("well-known missing CORS header")
	}
}

// ── two-column index layout ───────────────────────────────────────────────────

func TestIndexTwoColumnLayout(t *testing.T) {
	h, dir := newTestHandler(t)

	// Text piece — should show excerpt
	seedPiece(t, dir, "poem", "The sea is wide and the wind is cold.", "public")

	// Image piece with a blob
	seedPiece(t, dir, "photo", "", "public")
	blobDir := filepath.Join(filepath.Dir(dir), "blobs", "files")
	os.MkdirAll(blobDir, 0755)
	os.WriteFile(filepath.Join(blobDir, "photo.jpg"), []byte("JPEG"), 0644)
	h.blobStore.Save(&content.Blob{
		Slug: "photo", Title: "Photo",
		BlobType: content.BlobImage,
		Access:   content.AccessPublic,
		MimeType: "image/jpeg",
		FileRef:  "files/photo.jpg",
	})
	h.store.Load()

	w := serve(h, httptest.NewRequest("GET", "/", nil))
	if w.Code != 200 {
		t.Fatalf("index: got %d", w.Code)
	}
	body := w.Body.String()

	for _, want := range []string{
		"piece-row", "piece-left", "piece-right", "piece-thumb",
		"piece-excerpt", "files/photo.jpg",
		".piece-row{", ".piece-left{", ".piece-right{", ".piece-thumb{",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("index missing %q — two-column layout broken", want)
		}
	}
}
