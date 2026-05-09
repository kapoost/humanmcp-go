package content

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
)

type Message struct {
	ID        string    `json:"id"`
	From      string    `json:"from"`       // optional handle, max 32 chars
	Text      string    `json:"text"`       // max 2000 chars
	Regarding string    `json:"regarding"`  // optional slug it refers to
	Kind      string    `json:"kind"`       // message, comment, question, license
	At        time.Time `json:"at"`
}

// MessageStore saves messages as simple .txt files under /data/messages/
type MessageStore struct {
	dir string
}

func NewMessageStore(contentDir string) *MessageStore {
	return &MessageStore{dir: filepath.Join(filepath.Dir(contentDir), "messages")}
}

// Save validates and persists a message. Returns the message ID or an error.
func (ms *MessageStore) Save(from, text, regarding string) (*Message, error) {
	return ms.SaveKind(from, text, regarding, "message")
}

// SaveKind saves with explicit kind (message, comment, question, license).
func (ms *MessageStore) SaveKind(from, text, regarding, kind string) (*Message, error) {
	// --- strict sanitisation ---

	from = sanitiseField(from, 64)
	text = sanitiseField(text, 2000)
	regarding = sanitiseField(regarding, 64)

	if text == "" {
		return nil, fmt.Errorf("message text is required")
	}

	// URLs are welcome in messages and comments
	lower := strings.ToLower(text)

	// No HTML/script
	for _, bad := range []string{"<", ">", "script", "onclick", "onerror"} {
		if strings.Contains(lower, bad) {
			return nil, fmt.Errorf("invalid content in message")
		}
	}

	if err := os.MkdirAll(ms.dir, 0755); err != nil {
		return nil, fmt.Errorf("cannot create messages dir: %w", err)
	}

	if kind == "" {
		kind = "message"
	}

	id := fmt.Sprintf("%d", time.Now().UnixNano())
	m := &Message{
		ID:        id,
		From:      from,
		Text:      text,
		Regarding: regarding,
		Kind:      kind,
		At:        time.Now().UTC(),
	}

	// Write as plain text — owner reads these as files
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("id:        %s\n", m.ID))
	sb.WriteString(fmt.Sprintf("kind:      %s\n", m.Kind))
	sb.WriteString(fmt.Sprintf("at:        %s\n", m.At.Format("2006-01-02 15:04 UTC")))
	if m.From != "" {
		sb.WriteString(fmt.Sprintf("from:      %s\n", m.From))
	}
	if m.Regarding != "" {
		sb.WriteString(fmt.Sprintf("regarding: %s\n", m.Regarding))
	}
	sb.WriteString(fmt.Sprintf("\n%s\n", m.Text))

	path := filepath.Join(ms.dir, id+".txt")
	if err := os.WriteFile(path, []byte(sb.String()), 0644); err != nil {
		return nil, fmt.Errorf("cannot save message: %w", err)
	}

	return m, nil
}

// List returns all messages sorted newest-first (owner use only).
func (ms *MessageStore) List() ([]*Message, error) {
	entries, err := os.ReadDir(ms.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var msgs []*Message
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".txt") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(ms.dir, e.Name()))
		if err != nil {
			continue
		}
		m := parseMessageFile(string(data))
		if m != nil {
			msgs = append(msgs, m)
		}
	}

	sort.Slice(msgs, func(i, j int) bool {
		return msgs[i].At.After(msgs[j].At)
	})
	return msgs, nil
}

func parseMessageFile(content string) *Message {
	m := &Message{}
	lines := strings.Split(content, "\n")
	bodyStart := -1
	for i, line := range lines {
		if line == "" && bodyStart == -1 {
			bodyStart = i + 1
			continue
		}
		if bodyStart != -1 {
			continue
		}
		if strings.HasPrefix(line, "id:") {
			m.ID = strings.TrimSpace(strings.TrimPrefix(line, "id:"))
		} else if strings.HasPrefix(line, "kind:") {
			m.Kind = strings.TrimSpace(strings.TrimPrefix(line, "kind:"))
		} else if strings.HasPrefix(line, "from:") {
			m.From = strings.TrimSpace(strings.TrimPrefix(line, "from:"))
		} else if strings.HasPrefix(line, "regarding:") {
			m.Regarding = strings.TrimSpace(strings.TrimPrefix(line, "regarding:"))
		} else if strings.HasPrefix(line, "at:") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "at:"))
			t, _ := time.Parse("2006-01-02 15:04 UTC", val)
			m.At = t
		}
	}
	if bodyStart >= 0 && bodyStart < len(lines) {
		m.Text = strings.TrimSpace(strings.Join(lines[bodyStart:], "\n"))
	}
	return m
}

// sanitiseField strips control chars, trims whitespace, truncates to maxLen
func sanitiseField(s string, maxLen int) string {
	// Keep only printable Unicode, no control chars
	var b strings.Builder
	for _, r := range s {
		if unicode.IsPrint(r) || r == ' ' {
			b.WriteRune(r)
		}
	}
	s = strings.TrimSpace(b.String())
	// Collapse multiple spaces
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	runes := []rune(s)
	if len(runes) > maxLen {
		runes = runes[:maxLen]
	}
	return string(runes)
}
