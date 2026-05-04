package content

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Peer represents a known humanMCP server.
type Peer struct {
	URL       string    `json:"url"`        // e.g. "https://alice.humanmcp.net"
	Name      string    `json:"name"`       // human-readable name
	Bio       string    `json:"bio"`        // short description
	AddedAt   time.Time `json:"added_at"`
	LastSeen  time.Time `json:"last_seen,omitempty"`
}

// PeerStore manages known peers.
type PeerStore struct {
	dir   string
	mu    sync.RWMutex
	peers []Peer
}

func NewPeerStore(dir string) *PeerStore {
	ps := &PeerStore{dir: dir}
	ps.load()
	return ps
}

func (s *PeerStore) filePath() string {
	return filepath.Join(s.dir, "peers.json")
}

func (s *PeerStore) load() {
	data, err := os.ReadFile(s.filePath())
	if err != nil {
		return
	}
	json.Unmarshal(data, &s.peers)
}

func (s *PeerStore) save() error {
	os.MkdirAll(s.dir, 0755)
	data, err := json.MarshalIndent(s.peers, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath(), data, 0644)
}

// List returns all known peers.
func (s *PeerStore) List() []Peer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Peer, len(s.peers))
	copy(out, s.peers)
	return out
}

// Add registers a new peer. Returns false if already known.
func (s *PeerStore) Add(url, name, bio string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.peers {
		if p.URL == url {
			return false
		}
	}
	s.peers = append(s.peers, Peer{
		URL:     url,
		Name:    name,
		Bio:     bio,
		AddedAt: time.Now(),
	})
	s.save()
	return true
}

// Remove deletes a peer by URL.
func (s *PeerStore) Remove(url string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, p := range s.peers {
		if p.URL == url {
			s.peers = append(s.peers[:i], s.peers[i+1:]...)
			s.save()
			return true
		}
	}
	return false
}

// Touch updates last_seen for a peer.
func (s *PeerStore) Touch(url string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, p := range s.peers {
		if p.URL == url {
			s.peers[i].LastSeen = time.Now()
			s.save()
			return
		}
	}
}
