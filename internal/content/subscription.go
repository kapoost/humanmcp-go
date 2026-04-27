package content

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type SubChannel string

const (
	SubWebhook SubChannel = "webhook"
	SubMCP     SubChannel = "mcp"   // pull-based; subscriber polls list_listings(since=...)
	SubEmail   SubChannel = "email" // reserved — not delivered yet
)

type Subscription struct {
	ID          string     `json:"id"`
	Token       string     `json:"token"`
	Channel     SubChannel `json:"channel"`
	CallbackURL string     `json:"callback_url,omitempty"`
	Email       string     `json:"email,omitempty"`
	FilterTypes []string   `json:"filter_types,omitempty"`
	FilterTags  []string   `json:"filter_tags,omitempty"`
	Created     time.Time  `json:"created"`
	LastSeen    time.Time  `json:"last_seen,omitempty"`
	Active      bool       `json:"active"`
	FailCount   int        `json:"fail_count,omitempty"`
}

// Matches returns true if the listing matches this subscription's filters.
// Type filter: must match if set. Tag filter: OR-match (any tag overlap).
func (s *Subscription) Matches(l *Listing) bool {
	if len(s.FilterTypes) > 0 {
		found := false
		for _, ft := range s.FilterTypes {
			if ft == string(l.Type) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if len(s.FilterTags) > 0 {
		overlap := false
		for _, ft := range s.FilterTags {
			for _, lt := range l.Tags {
				if ft == lt {
					overlap = true
					break
				}
			}
			if overlap {
				break
			}
		}
		if !overlap {
			return false
		}
	}
	return true
}

type SubscriptionStore struct {
	path string
	mu   sync.Mutex
}

func NewSubscriptionStore(contentDir string) *SubscriptionStore {
	return &SubscriptionStore{
		path: filepath.Join(filepath.Dir(contentDir), "subscriptions.json"),
	}
}

func (ss *SubscriptionStore) Load() ([]*Subscription, error) {
	data, err := os.ReadFile(ss.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var subs []*Subscription
	if err := json.Unmarshal(data, &subs); err != nil {
		return nil, err
	}
	return subs, nil
}

func (ss *SubscriptionStore) Create(sub *Subscription) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	sub.ID = randomHex(8)
	sub.Token = randomHex(16)
	sub.Created = time.Now().UTC()
	sub.Active = true

	subs, _ := ss.load()
	subs = append(subs, sub)
	return ss.writeAll(subs)
}

func (ss *SubscriptionStore) Get(id string) (*Subscription, error) {
	subs, err := ss.Load()
	if err != nil {
		return nil, err
	}
	for _, s := range subs {
		if s.ID == id {
			return s, nil
		}
	}
	return nil, fmt.Errorf("subscription not found: %s", id)
}

func (ss *SubscriptionStore) GetByToken(tok string) (*Subscription, error) {
	subs, err := ss.Load()
	if err != nil {
		return nil, err
	}
	for _, s := range subs {
		if s.Token == tok {
			return s, nil
		}
	}
	return nil, fmt.Errorf("subscription not found")
}

func (ss *SubscriptionStore) List() []*Subscription {
	subs, _ := ss.Load()
	return subs
}

func (ss *SubscriptionStore) Update(sub *Subscription) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	subs, _ := ss.load()
	for i, s := range subs {
		if s.ID == sub.ID {
			subs[i] = sub
			return ss.writeAll(subs)
		}
	}
	return fmt.Errorf("subscription not found: %s", sub.ID)
}

func (ss *SubscriptionStore) Delete(id string) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	subs, _ := ss.load()
	var out []*Subscription
	for _, s := range subs {
		if s.ID != id {
			out = append(out, s)
		}
	}
	if len(out) == len(subs) {
		return fmt.Errorf("subscription not found: %s", id)
	}
	return ss.writeAll(out)
}

func (ss *SubscriptionStore) ActiveCount() int {
	subs, _ := ss.Load()
	n := 0
	for _, s := range subs {
		if s.Active {
			n++
		}
	}
	return n
}

func (ss *SubscriptionStore) load() ([]*Subscription, error) {
	data, err := os.ReadFile(ss.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var subs []*Subscription
	json.Unmarshal(data, &subs)
	return subs, nil
}

func (ss *SubscriptionStore) writeAll(subs []*Subscription) error {
	dir := filepath.Dir(ss.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(subs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(ss.path, data, 0644)
}

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}
