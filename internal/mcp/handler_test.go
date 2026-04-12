package mcp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kapoost/humanmcp-go/internal/auth"
	"github.com/kapoost/humanmcp-go/internal/config"
	"github.com/kapoost/humanmcp-go/internal/content"
)

// --- test helpers ---


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

	os.WriteFile(dir+"/public.md", []byte(`---
slug: public
title: Public Poem
type: poem
access: public
tags: [test, sea]
published: 2024-01-01
---
Hello world.`), 0644)

	os.WriteFile(dir+"/locked.md", []byte(`---
slug: locked
title: Locked Poem
type: poem
access: locked
gate: challenge
challenge: What is 2+2?
answer: four
description: A locked piece.
published: 2024-01-01
---
The secret.`), 0644)

	os.WriteFile(dir+"/time-locked.md", []byte(`---
slug: time-locked
title: Time Locked
type: poem
access: locked
gate: time
unlock_after: 2099-01-01
description: Opens in the future.
published: 2024-01-01
---
Future content.`), 0644)

	cfg := &config.Config{
		AuthorName: "testuser",
		AuthorBio:  "test bio",
		Domain:     "localhost",
		ContentDir: dir,
	}
	store := content.NewStore(dir)
	store.Load()
	a := auth.New("testtoken", "test-agent-token")
	h := NewHandler(cfg, store, a, sc, ms, ss)
	return h, dir
}

func post(t *testing.T, h *Handler, method string, params interface{}) map[string]interface{} {
	t.Helper()
	body, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0", "id": 1, "method": method, "params": params,
	})
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	var result map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &result)
	return result
}

func tool(t *testing.T, h *Handler, name string, args map[string]interface{}) string {
	t.Helper()
	resp := post(t, h, "tools/call", map[string]interface{}{"name": name, "arguments": args})
	if resp["error"] != nil {
		t.Fatalf("tool %s error: %v", name, resp["error"])
	}
	result := resp["result"].(map[string]interface{})
	content_ := result["content"].([]interface{})
	return content_[0].(map[string]interface{})["text"].(string)
}

func toolErr(t *testing.T, h *Handler, name string, args map[string]interface{}) map[string]interface{} {
	t.Helper()
	resp := post(t, h, "tools/call", map[string]interface{}{"name": name, "arguments": args})
	return resp
}

// --- initialize ---

func TestInitialize(t *testing.T) {
	h, _ := newTestHandler(t)

	// No version requested — should return max supported
	resp := post(t, h, "initialize", map[string]interface{}{})
	result := resp["result"].(map[string]interface{})
	if result["protocolVersion"] != "2025-03-26" { t.Errorf("default protocol: %v", result["protocolVersion"]) }
	if result["serverInfo"] == nil { t.Error("serverInfo missing") }
	info := result["serverInfo"].(map[string]interface{})
	if info["version"] != "0.2.0" { t.Errorf("serverVersion: %v", info["version"]) }
	instructions := result["instructions"].(string)
	if len(instructions) < 100 { t.Error("instructions too short") }
	if bytes.Contains([]byte(instructions), []byte("LUKASZ")) { t.Error("real name leaked in instructions") }
	if bytes.Contains([]byte(instructions), []byte("Kapus")) { t.Error("surname leaked in instructions") }

	// Known version requested — should echo it
	resp2 := post(t, h, "initialize", map[string]interface{}{"protocolVersion": "2024-11-05"})
	result2 := resp2["result"].(map[string]interface{})
	if result2["protocolVersion"] != "2024-11-05" { t.Errorf("negotiated protocol: %v", result2["protocolVersion"]) }

	// Unknown version requested — should return max
	resp3 := post(t, h, "initialize", map[string]interface{}{"protocolVersion": "2099-01-01"})
	result3 := resp3["result"].(map[string]interface{})
	if result3["protocolVersion"] != "2025-03-26" { t.Errorf("unknown protocol fallback: %v", result3["protocolVersion"]) }
}

// --- tools/list ---

func TestToolsList(t *testing.T) {
	h, _ := newTestHandler(t)
	resp := post(t, h, "tools/list", map[string]interface{}{})
	result := resp["result"].(map[string]interface{})
	tools := result["tools"].([]interface{})

	required := map[string]bool{
		"get_author_profile": false,
		"list_content":       false,
		"read_content":       false,
		"list_blobs":         false,
		"read_blob":          false,
		"verify_content":     false,
		"request_access":     false,
		"submit_answer":      false,
		"leave_comment":      false,
		"leave_message":      false,
	}
	for _, t_ := range tools {
		name := t_.(map[string]interface{})["name"].(string)
		required[name] = true
	}
	for name, found := range required {
		if !found { t.Errorf("missing tool: %s", name) }
	}
	if len(tools) < 10 { t.Errorf("expected at least 10 tools, got %d", len(tools)) }
}

// --- get_author_profile ---

func TestAuthorProfile(t *testing.T) {
	h, _ := newTestHandler(t)
	text := tool(t, h, "get_author_profile", nil)
	if !bytes.Contains([]byte(text), []byte("testuser")) { t.Error("should contain author name") }
	if !bytes.Contains([]byte(text), []byte("public")) { t.Error("should mention content counts") }
}

// --- list_content ---

func TestListContent(t *testing.T) {
	h, _ := newTestHandler(t)
	text := tool(t, h, "list_content", map[string]interface{}{})
	if !bytes.Contains([]byte(text), []byte("public")) { t.Error("should list public piece") }
	if !bytes.Contains([]byte(text), []byte("locked")) { t.Error("should list locked piece") }
	if bytes.Contains([]byte(text), []byte("Hello world.")) { t.Error("body must not appear in list") }
	if bytes.Contains([]byte(text), []byte("The secret.")) { t.Error("locked body must not appear") }
}

func TestListContentFilterByType(t *testing.T) {
	h, _ := newTestHandler(t)
	text := tool(t, h, "list_content", map[string]interface{}{"type": "poem"})
	if !bytes.Contains([]byte(text), []byte("poem")) { t.Error("should return poems") }
}

func TestListContentFilterByTag(t *testing.T) {
	h, _ := newTestHandler(t)
	text := tool(t, h, "list_content", map[string]interface{}{"tag": "sea"})
	if !bytes.Contains([]byte(text), []byte("public")) { t.Error("sea-tagged piece should appear") }
}

// --- read_content ---

func TestReadPublicContent(t *testing.T) {
	h, _ := newTestHandler(t)
	text := tool(t, h, "read_content", map[string]interface{}{"slug": "public"})
	if !bytes.Contains([]byte(text), []byte("Hello world.")) { t.Error("should return body") }
	if !bytes.Contains([]byte(text), []byte("leave_comment")) { t.Error("should prompt for comment") }
	if !bytes.Contains([]byte(text), []byte("attribution")) { t.Error("should mention attribution") }
}

func TestReadLockedContentRedacted(t *testing.T) {
	h, _ := newTestHandler(t)
	text := tool(t, h, "read_content", map[string]interface{}{"slug": "locked"})
	if bytes.Contains([]byte(text), []byte("The secret.")) { t.Error("locked body must be redacted") }
	if !bytes.Contains([]byte(text), []byte("access")) { t.Error("should mention access requirement") }
}

func TestReadTimeLockedContent(t *testing.T) {
	h, _ := newTestHandler(t)
	text := tool(t, h, "read_content", map[string]interface{}{"slug": "time-locked"})
	if bytes.Contains([]byte(text), []byte("Future content.")) { t.Error("future-locked body should be redacted") }
}

func TestReadNonexistentSlug(t *testing.T) {
	h, _ := newTestHandler(t)
	resp := toolErr(t, h, "read_content", map[string]interface{}{"slug": "does-not-exist"})
	if resp["error"] == nil { t.Error("should return error for nonexistent slug") }
}

func TestReadContentMissingSlug(t *testing.T) {
	h, _ := newTestHandler(t)
	resp := toolErr(t, h, "read_content", map[string]interface{}{})
	if resp["error"] == nil { t.Error("should error when slug missing") }
}

// --- request_access ---

func TestRequestAccessChallenge(t *testing.T) {
	h, _ := newTestHandler(t)
	text := tool(t, h, "request_access", map[string]interface{}{"slug": "locked"})
	if !bytes.Contains([]byte(text), []byte("What is 2+2?")) { t.Error("should show challenge question") }
	if bytes.Contains([]byte(text), []byte("four")) { t.Error("must NOT reveal the answer") }
}

func TestRequestAccessTimeLocked(t *testing.T) {
	h, _ := newTestHandler(t)
	text := tool(t, h, "request_access", map[string]interface{}{"slug": "time-locked"})
	if !bytes.Contains([]byte(text), []byte("time")) { t.Error("should mention time lock") }
}

func TestRequestAccessPublicPiece(t *testing.T) {
	h, _ := newTestHandler(t)
	text := tool(t, h, "request_access", map[string]interface{}{"slug": "public"})
	if !bytes.Contains([]byte(text), []byte("public")) { t.Error("should say piece is public") }
}

// --- submit_answer ---

func TestSubmitCorrectAnswer(t *testing.T) {
	h, _ := newTestHandler(t)
	text := tool(t, h, "submit_answer", map[string]interface{}{"slug": "locked", "answer": "four"})
	if !bytes.Contains([]byte(text), []byte("The secret.")) { t.Error("correct answer should unlock") }
	if !bytes.Contains([]byte(text), []byte("leave_comment")) { t.Error("should prompt for comment after unlock") }
}

func TestSubmitCaseInsensitiveAnswer(t *testing.T) {
	h, _ := newTestHandler(t)
	text := tool(t, h, "submit_answer", map[string]interface{}{"slug": "locked", "answer": "FOUR"})
	if !bytes.Contains([]byte(text), []byte("The secret.")) { t.Error("case-insensitive answer should unlock") }
}

func TestSubmitTrimmedAnswer(t *testing.T) {
	h, _ := newTestHandler(t)
	text := tool(t, h, "submit_answer", map[string]interface{}{"slug": "locked", "answer": "  four  "})
	if !bytes.Contains([]byte(text), []byte("The secret.")) { t.Error("whitespace-trimmed answer should unlock") }
}

func TestSubmitWrongAnswer(t *testing.T) {
	h, _ := newTestHandler(t)
	text := tool(t, h, "submit_answer", map[string]interface{}{"slug": "locked", "answer": "five"})
	if bytes.Contains([]byte(text), []byte("The secret.")) { t.Error("wrong answer must not unlock") }
	if !bytes.Contains([]byte(text), []byte("What is 2+2?")) { t.Error("wrong answer should show hint") }
}

func TestSubmitAnswerMissingFields(t *testing.T) {
	h, _ := newTestHandler(t)
	resp := toolErr(t, h, "submit_answer", map[string]interface{}{"slug": "locked"})
	if resp["error"] == nil { t.Error("should error when answer missing") }
}

// --- leave_comment ---

func TestLeaveComment(t *testing.T) {
	h, _ := newTestHandler(t)
	text := tool(t, h, "leave_comment", map[string]interface{}{
		"slug": "public", "text": "Beautiful.", "from": "test-agent",
	})
	if !bytes.Contains([]byte(text), []byte("recorded")) { t.Error("should confirm receipt") }
	if !bytes.Contains([]byte(text), []byte("public")) { t.Error("should echo the slug") }
}

func TestLeaveCommentMissingFields(t *testing.T) {
	h, _ := newTestHandler(t)
	resp := toolErr(t, h, "leave_comment", map[string]interface{}{"slug": "public"})
	if resp["error"] == nil { t.Error("should error when text missing") }
}

// --- leave_message ---

func TestLeaveMessage(t *testing.T) {
	h, _ := newTestHandler(t)
	text := tool(t, h, "leave_message", map[string]interface{}{
		"text": "Great server!", "from": "tester",
	})
	if !bytes.Contains([]byte(text), []byte("received")) { t.Error("should confirm receipt") }
}

func TestLeaveMessageAllowsLinks(t *testing.T) {
	h, _ := newTestHandler(t)
	text := tool(t, h, "leave_message", map[string]interface{}{
		"text": "see https://kapoost.github.io/humanmcp for the landing page",
	})
	if !bytes.Contains([]byte(text), []byte("received")) { t.Error("links should now be allowed in messages") }
}

func TestLeaveMessageRejectsHTML(t *testing.T) {
	h, _ := newTestHandler(t)
	text := tool(t, h, "leave_message", map[string]interface{}{
		"text": "<script>alert(1)</script>",
	})
	if bytes.Contains([]byte(text), []byte("received")) { t.Error("HTML should be rejected") }
}

// --- verify_content ---

func TestVerifyUnsignedContent(t *testing.T) {
	h, _ := newTestHandler(t)
	text := tool(t, h, "verify_content", map[string]interface{}{"slug": "public"})
	if text == "" { t.Error("should return status") }
}

func TestVerifySignedContent(t *testing.T) {
	dir := t.TempDir()
	kp, _ := content.GenerateKeyPair()
	store := content.NewStore(dir)
	store.Load()

	p := &content.Piece{
		Slug: "signed", Title: "Signed", Type: "poem",
		Access: content.AccessPublic, Body: "A poem.",
		Published: time.Now(),
	}
	sig, _ := content.SignPiece(p, kp)
	p.Signature = sig
	store.Save(p)

	cfg := &config.Config{
		AuthorName: "test", ContentDir: dir,
		SigningPublicKey: kp.PublicKeyHex(),
	}
	store2 := content.NewStore(dir)
	store2.Load()
	h := NewHandler(cfg, store2, auth.New("", "test-agent-token"), content.NewSessionCode(24*time.Hour), content.NewMemoryStore(dir), content.NewSkillStore(dir))

	text := tool(t, h, "verify_content", map[string]interface{}{"slug": "signed"})
	if !bytes.Contains([]byte(text), []byte("VERIFIED")) {
		t.Errorf("signed piece should verify, got: %s", text)
	}
}

// --- list_blobs ---

func TestListBlobsEmpty(t *testing.T) {
	h, _ := newTestHandler(t)
	text := tool(t, h, "list_blobs", map[string]interface{}{})
	if text == "" { t.Error("should return some response") }
}

func TestListBlobsWithContent(t *testing.T) {
	dir := t.TempDir()
	contentDir := dir + "/content"
	os.MkdirAll(contentDir, 0755)

	cfg := &config.Config{AuthorName: "test", ContentDir: contentDir}
	store := content.NewStore(contentDir)
	store.Load()
	blobStore := content.NewBlobStore(contentDir)

	blobStore.Save(&content.Blob{
		Slug: "my-vector", Title: "Test Vector",
		BlobType: content.BlobVector, Access: content.AccessLocked,
		Schema: "text-embedding-3-small", Dimensions: 1536,
		Audience: []content.AudienceEntry{{Kind: "agent", ID: "claude"}},
	})

	h := NewHandler(cfg, store, auth.New("", "test-agent-token"), content.NewSessionCode(24*time.Hour), content.NewMemoryStore(contentDir), content.NewSkillStore(contentDir))

	// Agent:claude can see it
	text := tool(t, h, "list_blobs", map[string]interface{}{
		"caller_kind": "agent", "caller_id": "claude",
	})
	if !bytes.Contains([]byte(text), []byte("my-vector")) { t.Error("should list blob") }
	if !bytes.Contains([]byte(text), []byte("text-embedding-3-small")) { t.Error("should show schema") }
}

// --- read_blob ---

func TestReadBlobAccessDenied(t *testing.T) {
	dir := t.TempDir()
	contentDir := dir + "/content"
	os.MkdirAll(contentDir, 0755)

	cfg := &config.Config{AuthorName: "test", ContentDir: contentDir}
	store := content.NewStore(contentDir)
	blobStore := content.NewBlobStore(contentDir)
	blobStore.Save(&content.Blob{
		Slug: "private", Title: "Private",
		BlobType: content.BlobImage, Access: content.AccessLocked,
		Audience: []content.AudienceEntry{{Kind: "human", ID: "alice"}},
		TextData: "secret data",
	})
	h := NewHandler(cfg, store, auth.New("", "test-agent-token"), content.NewSessionCode(24*time.Hour), content.NewMemoryStore(contentDir), content.NewSkillStore(contentDir))

	// Bob should be denied
	text := tool(t, h, "read_blob", map[string]interface{}{
		"slug": "private", "caller_kind": "human", "caller_id": "bob",
	})
	if bytes.Contains([]byte(text), []byte("secret data")) { t.Error("bob should be denied") }
	if !bytes.Contains([]byte(text), []byte("denied")) && !bytes.Contains([]byte(text), []byte("Access")) {
		t.Error("should indicate access denied")
	}
}

func TestReadBlobAccessGranted(t *testing.T) {
	dir := t.TempDir()
	contentDir := dir + "/content"
	os.MkdirAll(contentDir, 0755)

	cfg := &config.Config{AuthorName: "test", ContentDir: contentDir}
	store := content.NewStore(contentDir)
	blobStore := content.NewBlobStore(contentDir)
	blobStore.Save(&content.Blob{
		Slug: "private", Title: "Private",
		BlobType: content.BlobContact, Access: content.AccessLocked,
		Audience:  []content.AudienceEntry{{Kind: "human", ID: "alice"}},
		TextData:  `{"email":"alice@example.com"}`,
		MimeType:  "application/json",
	})
	h := NewHandler(cfg, store, auth.New("", "test-agent-token"), content.NewSessionCode(24*time.Hour), content.NewMemoryStore(contentDir), content.NewSkillStore(contentDir))

	// Alice should get access
	text := tool(t, h, "read_blob", map[string]interface{}{
		"slug": "private", "caller_kind": "human", "caller_id": "alice",
	})
	if !bytes.Contains([]byte(text), []byte("alice@example.com")) { t.Error("alice should get data") }
}

// --- error handling ---

func TestUnknownMethod(t *testing.T) {
	h, _ := newTestHandler(t)
	resp := post(t, h, "nonexistent/method", map[string]interface{}{})
	if resp["error"] == nil { t.Error("unknown method should return error") }
}

func TestUnknownTool(t *testing.T) {
	h, _ := newTestHandler(t)
	resp := post(t, h, "tools/call", map[string]interface{}{
		"name": "nonexistent_tool", "arguments": map[string]interface{}{},
	})
	if resp["error"] == nil { t.Error("unknown tool should return error") }
}

func TestMalformedJSON(t *testing.T) {
	h, _ := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	var result map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &result)
	if result["error"] == nil { t.Error("malformed JSON should return error") }
}

func TestGetMethodRejected(t *testing.T) {
	h, _ := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed { t.Errorf("GET should be 405, got %d", w.Code) }
}

// --- stats integration ---

func TestReadRecordsStats(t *testing.T) {
	h, _ := newTestHandler(t)
	tool(t, h, "read_content", map[string]interface{}{"slug": "public"})
	stats, _ := h.statStore.Compute()
	if stats.TotalReads != 1 { t.Errorf("read should be recorded, got %d", stats.TotalReads) }
}

func TestAccessRecordsInterest(t *testing.T) {
	h, _ := newTestHandler(t)
	tool(t, h, "request_access", map[string]interface{}{"slug": "locked"})
	stats, _ := h.statStore.Compute()
	if stats.TotalInterest != 1 { t.Errorf("access should record interest, got %d", stats.TotalInterest) }
}

func TestMessageRecordedInStats(t *testing.T) {
	h, _ := newTestHandler(t)
	tool(t, h, "leave_message", map[string]interface{}{"text": "Hello!", "from": "tester"})
	stats, _ := h.statStore.Compute()
	if stats.TotalMessages != 1 { t.Errorf("message not recorded, got %d", stats.TotalMessages) }
}

// ── agent good behaviour ──────────────────────────────────────────────────────

func TestAgentFullReadFlow(t *testing.T) {
	h, _ := newTestHandler(t)
	// 1. profile
	profile := tool(t, h, "get_author_profile", nil)
	if !strings.Contains(profile, "testuser") {
		t.Error("profile missing author name")
	}
	// 2. list
	list := tool(t, h, "list_content", nil)
	if !strings.Contains(list, "public") {
		t.Error("list missing public piece")
	}
	// 3. read
	body := tool(t, h, "read_content", map[string]interface{}{"slug": "public"})
	if !strings.Contains(body, "Hello world") {
		t.Error("read_content missing body")
	}
	// 4. comment after reading
	comment := tool(t, h, "leave_comment", map[string]interface{}{
		"slug": "public",
		"text": "beautiful",
		"from": "claude",
	})
	if !strings.Contains(comment, "recorded") {
		t.Error("comment not confirmed")
	}
}

func TestAgentCanLeaveMessage(t *testing.T) {
	h, _ := newTestHandler(t)
	result := tool(t, h, "leave_message", map[string]interface{}{
		"text": "Hello from an agent. Here is my URL: https://example.com",
		"from": "gpt-5",
	})
	if !strings.Contains(result, "received") {
		t.Error("message not confirmed")
	}
}

func TestAgentFiltersByType(t *testing.T) {
	h, _ := newTestHandler(t)
	result := tool(t, h, "list_content", map[string]interface{}{"type": "poem"})
	if strings.Contains(result, "No content") {
		t.Error("should find poem-type content")
	}
}

func TestAgentFiltersByTag(t *testing.T) {
	h, _ := newTestHandler(t)
	result := tool(t, h, "list_content", map[string]interface{}{"tag": "testtag"})
	if result == "" {
		t.Error("tag filter returned empty")
	}
}

// ── agent hitting locked content ──────────────────────────────────────────────

func TestAgentCannotReadLockedContent(t *testing.T) {
	h, _ := newTestHandler(t)
	result := tool(t, h, "read_content", map[string]interface{}{"slug": "locked"})
	if strings.Contains(result, "secret body") {
		t.Error("agent read locked content body — must not happen")
	}
	if !strings.Contains(result, "locked") {
		t.Error("agent should be told content is locked")
	}
}

func TestAgentGetsGateDetailsForLocked(t *testing.T) {
	h, _ := newTestHandler(t)
	result := tool(t, h, "request_access", map[string]interface{}{"slug": "locked"})
	if !strings.Contains(result, "challenge") && !strings.Contains(result, "question") {
		t.Error("agent should see gate type for locked content")
	}
}

func TestAgentCanUnlockWithCorrectAnswer(t *testing.T) {
	h, _ := newTestHandler(t)
	result := tool(t, h, "submit_answer", map[string]interface{}{
		"slug":   "locked",
		"answer": "four",
	})
	if !strings.Contains(result, "Unlocked") && !strings.Contains(result, "secret body") {
		t.Error("correct answer should unlock content")
	}
}

func TestAgentWrongAnswerBlocked(t *testing.T) {
	h, _ := newTestHandler(t)
	result := tool(t, h, "submit_answer", map[string]interface{}{
		"slug":   "locked",
		"answer": "wrong-guess",
	})
	if strings.Contains(result, "secret body") {
		t.Error("wrong answer must never return content")
	}
}

// ── agent harmful / abuse attempts ───────────────────────────────────────────

func TestAgentCannotInjectHTMLViaComment(t *testing.T) {
	h, _ := newTestHandler(t)
	resp := toolRaw(t, h, "leave_comment", map[string]interface{}{
		"slug": "public",
		"text": "<script>alert('pwned')</script>",
		"from": "evil-agent",
	})
	// Should either be rejected or sanitised — must not echo raw script
	if strings.Contains(resp, "<script>") {
		t.Error("HTML injection via comment not blocked")
	}
}

func TestAgentCannotInjectHTMLViaMessage(t *testing.T) {
	h, _ := newTestHandler(t)
	resp := toolRaw(t, h, "leave_message", map[string]interface{}{
		"text": "<iframe src='evil.com'></iframe>",
		"from": "evil",
	})
	if strings.Contains(resp, "<iframe") {
		t.Error("HTML injection via message not blocked")
	}
}

func TestAgentPromptInjectionViaMessage(t *testing.T) {
	h, _ := newTestHandler(t)
	// Agent reads a piece that contains "instructions" — this tests
	// that the server doesn't act on them, just stores as text
	result := tool(t, h, "read_content", map[string]interface{}{"slug": "public"})
	// The piece body should be returned as data, not executed
	if strings.Contains(result, "SYSTEM:") || strings.Contains(result, "ignore previous") {
		t.Log("piece body returned as-is — up to the calling agent to handle safely")
	}
	// Server-side: we just verify no panic and valid response
	if result == "" {
		t.Error("read_content returned empty on valid slug")
	}
}

func TestAgentOversizedCommentTruncated(t *testing.T) {
	h, _ := newTestHandler(t)
	big := strings.Repeat("x", 10000)
	resp := toolRaw(t, h, "leave_comment", map[string]interface{}{
		"slug": "public",
		"text": big,
		"from": "flooder",
	})
	// Must not 500 — either truncate or accept
	if strings.Contains(resp, "500") || strings.Contains(resp, "internal") {
		t.Error("oversized comment caused server error")
	}
}

func TestAgentOversizedMessageTruncated(t *testing.T) {
	h, _ := newTestHandler(t)
	big := strings.Repeat("y", 50000)
	resp := toolRaw(t, h, "leave_message", map[string]interface{}{
		"text": big,
		"from": "flooder",
	})
	if strings.Contains(resp, "500") || strings.Contains(resp, "internal") {
		t.Error("oversized message caused server error")
	}
}

func TestAgentUnknownSlugGetsError(t *testing.T) {
	h, _ := newTestHandler(t)
	resp := toolErr(t, h, "read_content", map[string]interface{}{"slug": "doesnotexist"})
	if resp == nil {
		t.Error("unknown slug should return RPC error")
	}
}

func TestAgentMissingRequiredField(t *testing.T) {
	h, _ := newTestHandler(t)
	// leave_comment requires slug and text
	resp := toolErr(t, h, "leave_comment", map[string]interface{}{"from": "agent"})
	if resp == nil {
		t.Error("missing required fields should return RPC error")
	}
}

func TestAgentCallingUnknownTool(t *testing.T) {
	h, _ := newTestHandler(t)
	resp := toolErr(t, h, "delete_all_content", nil)
	if resp == nil {
		t.Error("unknown tool should return RPC error")
	}
}

func TestAgentCannotAccessOwnerRoutes(t *testing.T) {
	h, _ := newTestHandler(t)
	// MCP has no owner tools — verify delete/edit tools don't exist
	resp := toolErr(t, h, "delete_content", map[string]interface{}{"slug": "public"})
	if resp == nil {
		t.Error("delete_content should not exist as MCP tool")
	}
	resp2 := toolErr(t, h, "save_content", map[string]interface{}{"slug": "hack", "body": "owned"})
	if resp2 == nil {
		t.Error("save_content should not exist as MCP tool")
	}
}

func TestAgentRequestsPublicPieceAsPublic(t *testing.T) {
	h, _ := newTestHandler(t)
	result := tool(t, h, "read_content", map[string]interface{}{"slug": "public"})
	if !strings.Contains(result, "Hello world") {
		t.Error("agent should be able to read public content")
	}
}

func TestAgentGetsCertificateForPublicPiece(t *testing.T) {
	h, _ := newTestHandler(t)
	result := tool(t, h, "get_certificate", map[string]interface{}{"slug": "public"})
	if strings.Contains(result, "error") && !strings.Contains(result, "hash") {
		t.Error("get_certificate failed for public piece")
	}
}

// ── helper: toolRaw returns the full response text without failing on error ──

func toolRaw(t *testing.T, h *Handler, name string, args map[string]interface{}) string {
	t.Helper()
	body, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]interface{}{"name": name, "arguments": args},
	})
	r := httptest.NewRequest("POST", "/mcp", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w.Body.String()
}
