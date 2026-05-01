package content

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type ListingType string

const (
	ListingSell    ListingType = "sell"
	ListingBuy     ListingType = "buy"
	ListingOffer   ListingType = "offer"   // I can do X for you
	ListingRequest ListingType = "request" // I need someone to do X
	ListingTrade   ListingType = "trade"
)

type ListingStatus string

const (
	ListingOpen      ListingStatus = "open"
	ListingPaused    ListingStatus = "paused"
	ListingClosed    ListingStatus = "closed"
	ListingFulfilled ListingStatus = "fulfilled"
)

type Listing struct {
	Slug      string        `json:"slug"`
	Type      ListingType   `json:"type"`
	Title     string        `json:"title"`
	Body      string        `json:"body"`
	Tags      []string      `json:"tags,omitempty"`
	Price     string        `json:"price,omitempty"`      // free-form e.g. "200 PLN", "0.001 BTC"
	PriceSats int64         `json:"price_sats,omitempty"` // optional structured price
	Status    ListingStatus `json:"status"`
	Access    AccessLevel   `json:"access"`
	Published time.Time     `json:"published"`
	ExpiresAt time.Time     `json:"expires_at,omitempty"`
	ImageRef  string        `json:"image_ref,omitempty"` // path to uploaded image (e.g. files/listing-slug.jpg)
	Signature string        `json:"signature,omitempty"`
}

func (l *Listing) IsExpired() bool {
	return !l.ExpiresAt.IsZero() && time.Now().After(l.ExpiresAt)
}

func (l *Listing) IsActive() bool {
	return l.Status == ListingOpen && !l.IsExpired()
}

// ListingStore manages listings persisted as a single JSON file.
type ListingStore struct {
	path  string
	mu    sync.RWMutex
	cache *Cache[[]*Listing]
}

func NewListingStore(contentDir string) *ListingStore {
	return &ListingStore{
		path:  filepath.Join(filepath.Dir(contentDir), "listings.json"),
		cache: NewCache[[]*Listing](5 * time.Second),
	}
}

func (s *ListingStore) Load() ([]*Listing, error) {
	if cached, ok := s.cache.Get(); ok {
		return cached, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var listings []*Listing
	if err := json.Unmarshal(data, &listings); err != nil {
		return nil, err
	}
	sort.Slice(listings, func(i, j int) bool {
		return listings[i].Published.After(listings[j].Published)
	})
	s.cache.Set(listings)
	return listings, nil
}

// List returns listings sorted by Published desc. If includeInactive is false,
// only active (open + not expired) listings are returned.
func (s *ListingStore) List(includeInactive bool) []*Listing {
	all, _ := s.Load()
	if includeInactive {
		return all
	}
	var out []*Listing
	for _, l := range all {
		if l.IsActive() {
			out = append(out, l)
		}
	}
	return out
}

// ListSince returns active listings published after t, sorted by Published desc.
func (s *ListingStore) ListSince(t time.Time) []*Listing {
	all, _ := s.Load()
	var out []*Listing
	for _, l := range all {
		if l.IsActive() && l.Published.After(t) {
			out = append(out, l)
		}
	}
	return out
}

func (s *ListingStore) Get(slug string) (*Listing, error) {
	all, err := s.Load()
	if err != nil {
		return nil, err
	}
	for _, l := range all {
		if l.Slug == slug {
			return l, nil
		}
	}
	return nil, fmt.Errorf("listing not found: %s", slug)
}

func (s *ListingStore) Save(l *Listing) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache.Invalidate()

	if l.Published.IsZero() {
		l.Published = time.Now()
	}

	var listings []*Listing
	data, err := os.ReadFile(s.path)
	if err == nil {
		json.Unmarshal(data, &listings)
	}

	// Update or append
	found := false
	for i, existing := range listings {
		if existing.Slug == l.Slug {
			listings[i] = l
			found = true
			break
		}
	}
	if !found {
		listings = append(listings, l)
	}

	return s.writeAll(listings)
}

func (s *ListingStore) Delete(slug string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache.Invalidate()

	data, err := os.ReadFile(s.path)
	if err != nil {
		return fmt.Errorf("listing not found: %s", slug)
	}
	var listings []*Listing
	json.Unmarshal(data, &listings)

	var out []*Listing
	for _, l := range listings {
		if l.Slug != slug {
			out = append(out, l)
		}
	}
	if len(out) == len(listings) {
		return fmt.Errorf("listing not found: %s", slug)
	}
	return s.writeAll(out)
}

func (s *ListingStore) writeAll(listings []*Listing) error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(listings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0644)
}
