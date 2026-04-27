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

type EventType string

const (
	EventRead       EventType = "read"
	EventList       EventType = "list"
	EventUnlock     EventType = "unlock"
	EventUnlockFail EventType = "unlock_fail"
	EventMessage    EventType = "message"
	EventComment    EventType = "comment"
	EventProfile        EventType = "profile"
	EventAccess         EventType = "access"
	EventListingView    EventType = "listing_view"
	EventListingResponse EventType = "listing_response"
	EventSubscribeNew   EventType = "subscribe"
	EventSubscribeMatch EventType = "subscribe_match"
)

type CallerType string

const (
	CallerAgent   CallerType = "agent"
	CallerHuman   CallerType = "human"
	CallerUnknown CallerType = "unknown"
)

type Event struct {
	At      time.Time  `json:"at"`
	Type    EventType  `json:"type"`
	Caller  CallerType `json:"caller"`
	Slug    string     `json:"slug,omitempty"`
	UA      string     `json:"ua,omitempty"`
	From    string     `json:"from,omitempty"`
	Ref     string     `json:"ref,omitempty"`    // HTTP referrer
	Country string     `json:"country,omitempty"` // from Fly-Client-IP geo header
	VisitorHash string `json:"vh,omitempty"`     // hashed(ip+date) — never raw IP
}

type HourBucket struct {
	Hour  int `json:"hour"`
	Count int `json:"count"`
}

type Stats struct {
	// Counters
	TotalReads    int `json:"total_reads"`
	TotalMessages int `json:"total_messages"`
	TotalComments int `json:"total_comments"`
	TotalUnlocks  int `json:"total_unlocks"`
	TotalInterest int `json:"total_interest"`
	AgentCalls    int `json:"agent_calls"`
	HumanVisits   int `json:"human_visits"`
	UniqueVisitors int `json:"unique_visitors"`
	TotalListings  int `json:"total_listings"`
	TotalSubscribers int `json:"total_subscribers"`
	ListingViews   int `json:"listing_views"`

	// Breakdowns
	ReadsBySlug    map[string]int `json:"reads_by_slug"`
	InterestBySlug map[string]int `json:"interest_by_slug"`
	TagReads       map[string]int `json:"tag_reads"`
	TopAgents      map[string]int `json:"top_agents"`
	TopReferrers   map[string]int `json:"top_referrers"`
	Countries       map[string]int `json:"countries"`
	ListingReadsBySlug map[string]int `json:"listing_reads_by_slug"`

	// Challenge funnel per slug: [checked, attempted, succeeded]
	ChallengeFunnel map[string][3]int `json:"challenge_funnel"`

	// Hour-of-day distribution (0-23)
	HourlyReads [24]int `json:"hourly_reads"`

	// Recent events
	RecentEvents []Event `json:"recent_events"`
}

type StatStore struct {
	path    string
	tagPath string
	mu      sync.Mutex
	cache   *Cache[*Stats] // 10s TTL — dashboard doesn't need live data
}

func NewStatStore(contentDir string) *StatStore {
	base := filepath.Dir(contentDir)
	return &StatStore{
		path:    filepath.Join(base, "stats.ndjson"),
		tagPath: filepath.Join(base, "slug-tags.json"),
		cache:   NewCache[*Stats](10 * time.Second),
	}
}

func (ss *StatStore) Record(e Event) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	e.At = time.Now().UTC()
	ss.cache.Invalidate()
	data, err := json.Marshal(e)
	if err != nil {
		return
	}
	f, err := os.OpenFile(ss.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s\n", data)
}

// UpdateSlugTags keeps a slug→tags index so stats can show tag breakdowns
func (ss *StatStore) UpdateSlugTags(slugTags map[string][]string) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	data, _ := json.Marshal(slugTags)
	os.WriteFile(ss.tagPath, data, 0644)
}

func (ss *StatStore) loadSlugTags() map[string][]string {
	data, err := os.ReadFile(ss.tagPath)
	if err != nil {
		return nil
	}
	var m map[string][]string
	json.Unmarshal(data, &m)
	return m
}

func (ss *StatStore) Compute() (*Stats, error) {
	if cached, ok := ss.cache.Get(); ok {
		return cached, nil
	}
	ss.mu.Lock()
	defer ss.mu.Unlock()

	s := &Stats{
		ReadsBySlug:     make(map[string]int),
		InterestBySlug:  make(map[string]int),
		TagReads:        make(map[string]int),
		TopAgents:       make(map[string]int),
		TopReferrers:    make(map[string]int),
		Countries:       make(map[string]int),
		ChallengeFunnel:    make(map[string][3]int),
		ListingReadsBySlug: make(map[string]int),
	}

	data, err := os.ReadFile(ss.path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}

	slugTags := ss.loadSlugTags()
	uniqueVH := make(map[string]bool)

	var all []Event
	for _, line := range splitLines(string(data)) {
		var e Event
		if json.Unmarshal([]byte(line), &e) == nil {
			all = append(all, e)
		}
	}

	for _, e := range all {
		// caller counts
		switch e.Caller {
		case CallerAgent:
			s.AgentCalls++
		case CallerHuman:
			s.HumanVisits++
		}

		// unique visitors (hashed)
		if e.VisitorHash != "" {
			uniqueVH[e.VisitorHash] = true
		}

		// agent identity
		if e.From != "" {
			s.TopAgents[e.From]++
		}

		// country
		if e.Country != "" {
			s.Countries[e.Country]++
		}

		// referrer — strip query strings, keep domain only
		if e.Ref != "" {
			domain := cleanReferrer(e.Ref)
			if domain != "" {
				s.TopReferrers[domain]++
			}
		}

		// event type
		switch e.Type {
		case EventRead:
			s.TotalReads++
			if e.Slug != "" {
				s.ReadsBySlug[e.Slug]++
				// hour of day
				s.HourlyReads[e.At.Hour()]++
				// tag analytics
				if slugTags != nil {
					for _, tag := range slugTags[e.Slug] {
						s.TagReads[tag]++
					}
				}
			}
		case EventMessage:
			s.TotalMessages++
		case EventComment:
			s.TotalComments++
		case EventUnlock:
			s.TotalUnlocks++
			if e.Slug != "" {
				f := s.ChallengeFunnel[e.Slug]
				f[2]++
				s.ChallengeFunnel[e.Slug] = f
			}
		case EventUnlockFail:
			if e.Slug != "" {
				f := s.ChallengeFunnel[e.Slug]
				f[1]++
				s.ChallengeFunnel[e.Slug] = f
			}
		case EventAccess:
			s.TotalInterest++
			if e.Slug != "" {
				s.InterestBySlug[e.Slug]++
				// count as funnel entry
				f := s.ChallengeFunnel[e.Slug]
				f[0]++
				s.ChallengeFunnel[e.Slug] = f
			}
		case EventListingView:
			s.ListingViews++
			if e.Slug != "" {
				s.ListingReadsBySlug[e.Slug]++
			}
		}
	}

	s.UniqueVisitors = len(uniqueVH)

	// Last 30 events, newest first
	for i := len(all) - 1; i >= 0 && len(s.RecentEvents) < 30; i-- {
		s.RecentEvents = append(s.RecentEvents, all[i])
	}

	ss.cache.Set(s)
	return s, nil
}

// TopN returns the top N entries from a map by value
func TopN(m map[string]int, n int) []struct{ Key string; Val int } {
	type kv struct{ Key string; Val int }
	var sorted []kv
	for k, v := range m {
		sorted = append(sorted, kv{k, v})
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Val > sorted[j].Val })
	if len(sorted) > n {
		sorted = sorted[:n]
	}
	result := make([]struct{ Key string; Val int }, len(sorted))
	for i, kv := range sorted {
		result[i] = struct{ Key string; Val int }{kv.Key, kv.Val}
	}
	return result
}

func cleanReferrer(ref string) string {
	ref = strings.TrimPrefix(ref, "https://")
	ref = strings.TrimPrefix(ref, "http://")
	if idx := strings.Index(ref, "/"); idx > 0 {
		ref = ref[:idx]
	}
	if idx := strings.Index(ref, "?"); idx > 0 {
		ref = ref[:idx]
	}
	// Skip self-referrals
	if strings.Contains(ref, "fly.dev") || strings.Contains(ref, "localhost") {
		return ""
	}
	return ref
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			if i > start {
				out = append(out, s[start:i])
			}
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

func CallerFromUA(ua string) CallerType {
	if ua == "" {
		return CallerUnknown
	}
	lower := strings.ToLower(ua[:min(len(ua), 120)])
	for _, kw := range []string{"claude", "gpt", "openai", "anthropic", "llm", "agent", "bot", "curl", "python", "go-http", "okhttp", "axios", "mcp", "langchain"} {
		if strings.Contains(lower, kw) {
			return CallerAgent
		}
	}
	for _, kw := range []string{"mozilla", "chrome", "safari", "firefox", "webkit"} {
		if strings.Contains(lower, kw) {
			return CallerHuman
		}
	}
	return CallerUnknown
}

// VisitorHash creates a non-reversible daily visitor token from IP
func VisitorHash(ip, date string) string {
	// Simple hash — not storing IP, just a daily unique token
	h := 0
	for _, c := range ip + "|" + date {
		h = h*31 + int(c)
	}
	if h < 0 { h = -h }
	return fmt.Sprintf("%x", h)
}
