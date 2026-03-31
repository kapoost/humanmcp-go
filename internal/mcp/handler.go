package mcp

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/kapoost/humanmcp-go/internal/auth"
	"github.com/kapoost/humanmcp-go/internal/config"
	"github.com/kapoost/humanmcp-go/internal/content"
)

// JSON-RPC types
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type Response struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// MCP tool definitions
type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"inputSchema"`
}

type ToolsListResult struct {
	Tools []Tool `json:"tools"`
}

type CallResult struct {
	Content []ContentBlock `json:"content"`
}

type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Handler wires up the MCP server
type Handler struct {
	cfg   *config.Config
	store *content.Store
	auth  *auth.Auth
}

func NewHandler(cfg *config.Config, store *content.Store, a *auth.Auth) *Handler {
	return &Handler{cfg: cfg, store: store, auth: a}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// SSE endpoint for streaming (future)
	if r.URL.Path == "/mcp/sse" {
		h.handleSSE(w, r)
		return
	}

	// Standard JSON-RPC POST
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, nil, -32700, "parse error")
		return
	}

	log.Printf("[MCP] method=%s id=%v", req.Method, req.ID)

	switch req.Method {
	case "initialize":
		h.handleInitialize(w, &req)
	case "tools/list":
		h.handleToolsList(w, &req)
	case "tools/call":
		h.handleToolsCall(w, r, &req)
	default:
		writeError(w, req.ID, -32601, "method not found: "+req.Method)
	}
}

func (h *Handler) handleInitialize(w http.ResponseWriter, req *Request) {
	writeResult(w, req.ID, map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities": map[string]interface{}{
			"tools": map[string]bool{"listChanged": false},
		},
		"serverInfo": map[string]string{
			"name":    "humanMCP/" + h.cfg.AuthorName,
			"version": "0.1.0",
		},
	})
}

func (h *Handler) handleToolsList(w http.ResponseWriter, req *Request) {
	tools := []Tool{
		{
			Name:        "get_author_profile",
			Description: "Returns the author's profile: name, bio, and server info.",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			Name:        "list_content",
			Description: "Lists all published content pieces. Returns title, slug, type, access level, description, and tags. Body is not included for locked/members content.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"type": map[string]interface{}{
						"type":        "string",
						"description": "Filter by content type: poem, essay, note, audio, image",
					},
					"tag": map[string]interface{}{
						"type":        "string",
						"description": "Filter by tag",
					},
				},
			},
		},
		{
			Name:        "read_content",
			Description: "Read the full body of a content piece by slug. Returns body if public. For locked content, returns the challenge question or payment info.",
			InputSchema: map[string]interface{}{
				"type":     "object",
				"required": []string{"slug"},
				"properties": map[string]interface{}{
					"slug": map[string]interface{}{
						"type":        "string",
						"description": "The slug of the content piece",
					},
				},
			},
		},
		{
			Name:        "request_access",
			Description: "Request access to a locked piece. Returns the gate type and challenge question (if applicable) or payment instructions.",
			InputSchema: map[string]interface{}{
				"type":     "object",
				"required": []string{"slug"},
				"properties": map[string]interface{}{
					"slug": map[string]interface{}{
						"type":        "string",
						"description": "The slug of the locked content piece",
					},
				},
			},
		},
		{
			Name:        "submit_answer",
			Description: "Submit an answer to a challenge gate. If correct, the full content is returned.",
			InputSchema: map[string]interface{}{
				"type":     "object",
				"required": []string{"slug", "answer"},
				"properties": map[string]interface{}{
					"slug": map[string]interface{}{
						"type":        "string",
						"description": "The slug of the content piece",
					},
					"answer": map[string]interface{}{
						"type":        "string",
						"description": "Your answer to the challenge question",
					},
				},
			},
		},
	}

	writeResult(w, req.ID, ToolsListResult{Tools: tools})
}

func (h *Handler) handleToolsCall(w http.ResponseWriter, r *http.Request, req *Request) {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeError(w, req.ID, -32602, "invalid params")
		return
	}

	switch params.Name {
	case "get_author_profile":
		h.toolAuthorProfile(w, req)
	case "list_content":
		h.toolListContent(w, req, params.Arguments)
	case "read_content":
		h.toolReadContent(w, req, params.Arguments)
	case "request_access":
		h.toolRequestAccess(w, req, params.Arguments)
	case "submit_answer":
		h.toolSubmitAnswer(w, req, params.Arguments)
	default:
		writeError(w, req.ID, -32602, "unknown tool: "+params.Name)
	}
}

func (h *Handler) toolAuthorProfile(w http.ResponseWriter, req *Request) {
	text := fmt.Sprintf("Author: %s\nBio: %s\nDomain: %s\nServer: humanMCP v0.1.0\n",
		h.cfg.AuthorName, h.cfg.AuthorBio, h.cfg.Domain)
	writeResult(w, req.ID, CallResult{Content: []ContentBlock{{Type: "text", Text: text}}})
}

func (h *Handler) toolListContent(w http.ResponseWriter, req *Request, args json.RawMessage) {
	var a struct {
		Type string `json:"type"`
		Tag  string `json:"tag"`
	}
	json.Unmarshal(args, &a)

	pieces := h.store.List(false)

	var filtered []*content.Piece
	for _, p := range pieces {
		if a.Type != "" && p.Type != a.Type {
			continue
		}
		if a.Tag != "" && !hasTag(p.Tags, a.Tag) {
			continue
		}
		filtered = append(filtered, p)
	}

	if len(filtered) == 0 {
		writeResult(w, req.ID, CallResult{Content: []ContentBlock{{Type: "text", Text: "No content found."}}})
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Content by %s (%d pieces):\n\n", h.cfg.AuthorName, len(filtered)))
	for _, p := range filtered {
		sb.WriteString(fmt.Sprintf("slug: %s\n", p.Slug))
		sb.WriteString(fmt.Sprintf("  title: %s\n", p.Title))
		sb.WriteString(fmt.Sprintf("  type: %s\n", p.Type))
		sb.WriteString(fmt.Sprintf("  access: %s\n", p.Access))
		if p.Description != "" {
			sb.WriteString(fmt.Sprintf("  description: %s\n", p.Description))
		}
		if len(p.Tags) > 0 {
			sb.WriteString(fmt.Sprintf("  tags: %s\n", strings.Join(p.Tags, ", ")))
		}
		sb.WriteString(fmt.Sprintf("  published: %s\n", p.Published.Format("2006-01-02")))
		sb.WriteString("\n")
	}

	writeResult(w, req.ID, CallResult{Content: []ContentBlock{{Type: "text", Text: sb.String()}}})
}

func (h *Handler) toolReadContent(w http.ResponseWriter, req *Request, args json.RawMessage) {
	var a struct {
		Slug string `json:"slug"`
	}
	json.Unmarshal(args, &a)
	if a.Slug == "" {
		writeError(w, req.ID, -32602, "slug is required")
		return
	}

	p, err := h.store.Get(a.Slug, false)
	if err != nil {
		writeError(w, req.ID, -32602, "not found: "+a.Slug)
		return
	}

	if p.Access == content.AccessPublic {
		text := fmt.Sprintf("# %s\ntype: %s | published: %s\n\n%s",
			p.Title, p.Type, p.Published.Format("2006-01-02"), p.Body)
		writeResult(w, req.ID, CallResult{Content: []ContentBlock{{Type: "text", Text: text}}})
		return
	}

	// Locked or members — return gate info
	text := fmt.Sprintf("# %s\n\nAccess: %s\n\nThis content requires access. Use request_access tool with slug=%q to get details.",
		p.Title, p.Access, a.Slug)
	writeResult(w, req.ID, CallResult{Content: []ContentBlock{{Type: "text", Text: text}}})
}

func (h *Handler) toolRequestAccess(w http.ResponseWriter, req *Request, args json.RawMessage) {
	var a struct {
		Slug string `json:"slug"`
	}
	json.Unmarshal(args, &a)
	if a.Slug == "" {
		writeError(w, req.ID, -32602, "slug is required")
		return
	}

	p, err := h.store.Get(a.Slug, false)
	if err != nil {
		writeError(w, req.ID, -32602, "not found: "+a.Slug)
		return
	}

	if p.Access == content.AccessPublic {
		writeResult(w, req.ID, CallResult{Content: []ContentBlock{{Type: "text", Text: "This content is public. Use read_content to read it."}}})
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Access gate for: %s\n\n", p.Title))
	sb.WriteString(fmt.Sprintf("Gate type: %s\n", p.Gate))

	switch p.Gate {
	case content.GateChallenge:
		sb.WriteString(fmt.Sprintf("\nChallenge question:\n  %s\n\n", p.Challenge))
		sb.WriteString("To unlock: use submit_answer tool with slug and your answer.\n")
	case content.GatePayment:
		sb.WriteString(fmt.Sprintf("\nPrice: %d sats (Lightning Network)\n", p.PriceSats))
		sb.WriteString("Payment support coming in a future release.\n")
	default:
		sb.WriteString("\nMembers-only content. Contact the author for access.\n")
	}

	writeResult(w, req.ID, CallResult{Content: []ContentBlock{{Type: "text", Text: sb.String()}}})
}

func (h *Handler) toolSubmitAnswer(w http.ResponseWriter, req *Request, args json.RawMessage) {
	var a struct {
		Slug   string `json:"slug"`
		Answer string `json:"answer"`
	}
	json.Unmarshal(args, &a)
	if a.Slug == "" || a.Answer == "" {
		writeError(w, req.ID, -32602, "slug and answer are required")
		return
	}

	if !h.store.CheckAnswer(a.Slug, a.Answer) {
		writeResult(w, req.ID, CallResult{Content: []ContentBlock{
			{Type: "text", Text: "Incorrect answer. Try again."},
		}})
		return
	}

	p, _ := h.store.Get(a.Slug, true)
	text := fmt.Sprintf("Correct! Here is the content:\n\n# %s\ntype: %s | published: %s\n\n%s",
		p.Title, p.Type, p.Published.Format("2006-01-02"), p.Body)
	writeResult(w, req.ID, CallResult{Content: []ContentBlock{{Type: "text", Text: text}}})
}

func (h *Handler) handleSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Send endpoint info
	fmt.Fprintf(w, "data: {\"type\":\"endpoint\",\"url\":\"https://%s/mcp\"}\n\n", h.cfg.Domain)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	// Keep alive
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			fmt.Fprintf(w, ": ping\n\n")
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
	}
}

// helpers

func writeResult(w http.ResponseWriter, id interface{}, result interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	})
}

func writeError(w http.ResponseWriter, id interface{}, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(Response{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &RPCError{Code: code, Message: msg},
	})
}

func hasTag(tags []string, tag string) bool {
	for _, t := range tags {
		if strings.EqualFold(t, tag) {
			return true
		}
	}
	return false
}
