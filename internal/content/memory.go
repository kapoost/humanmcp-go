package content

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Memory to obserwacja agenta o właścicielu serwera.
// Zapisywana po sesji, pobierana przed nową.
type Memory struct {
	ID        string    `json:"id"`
	Body      string    `json:"body"`       // obserwacja w plain text
	Tags      []string  `json:"tags,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	AgentHint string    `json:"agent_hint,omitempty"` // skąd pochodzi obserwacja
}

type MemoryStore struct {
	mu  sync.RWMutex
	dir string
}

func NewMemoryStore(contentDir string) *MemoryStore {
	ms := &MemoryStore{dir: filepath.Join(contentDir, "memory")}
	os.MkdirAll(ms.dir, 0755)
	return ms
}

// Save zapisuje nową obserwację.
func (ms *MemoryStore) Save(body, agentHint string, tags []string) (*Memory, error) {
	if strings.TrimSpace(body) == "" {
		return nil, fmt.Errorf("body cannot be empty")
	}
	if len(body) > 2000 {
		body = body[:2000]
	}
	m := &Memory{
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		Body:      body,
		Tags:      tags,
		AgentHint: agentHint,
		CreatedAt: time.Now(),
	}
	ms.mu.Lock()
	defer ms.mu.Unlock()
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	return m, os.WriteFile(filepath.Join(ms.dir, m.ID+".json"), data, 0644)
}

// List zwraca obserwacje — najnowsze najpierw, opcjonalnie filtrowane tagiem.
func (ms *MemoryStore) List(tag string, limit int) ([]*Memory, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	entries, err := os.ReadDir(ms.dir)
	if err != nil {
		return nil, err
	}

	var memories []*Memory
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(ms.dir, e.Name()))
		if err != nil {
			continue
		}
		var m Memory
		if err := json.Unmarshal(data, &m); err != nil {
			continue
		}
		if tag != "" && !hasTagMem(m.Tags, tag) {
			continue
		}
		memories = append(memories, &m)
	}

	// Najnowsze najpierw
	sort.Slice(memories, func(i, j int) bool {
		return memories[i].CreatedAt.After(memories[j].CreatedAt)
	})

	if limit > 0 && len(memories) > limit {
		memories = memories[:limit]
	}
	return memories, nil
}

// Delete usuwa obserwację po ID.
func (ms *MemoryStore) Delete(id string) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	path := filepath.Join(ms.dir, id+".json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("not found")
	}
	return os.Remove(path)
}

func hasTagMem(tags []string, tag string) bool {
	for _, t := range tags {
		if strings.EqualFold(t, tag) {
			return true
		}
	}
	return false
}
