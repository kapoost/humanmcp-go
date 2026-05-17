package content

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Skill struct {
	Slug      string    `json:"slug"`
	Category  string    `json:"category"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Tags      []string  `json:"tags,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
	UpdatedBy string    `json:"updated_by,omitempty"`
}

type Persona struct {
	Slug      string         `json:"slug"`
	Name      string         `json:"name"`
	Role      string         `json:"role"`
	Prompt    string         `json:"prompt"`
	Tags      []string       `json:"tags,omitempty"`
	Level     int            `json:"level,omitempty"`
	XP        int            `json:"xp,omitempty"`
	MP        int            `json:"mp,omitempty"`
	Sessions  int            `json:"sessions,omitempty"`
	Stats     map[string]int `json:"stats,omitempty"`
	UpdatedAt time.Time      `json:"updated_at"`
	UpdatedBy string         `json:"updated_by,omitempty"`
}

// vaultPersonaResp matches mysloodsiewnia /persona/{id} JSON shape.
type vaultPersonaResp struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Role       string `json:"role"`
	Prompt     string `json:"prompt"`
	BasePrompt string `json:"base_prompt"`
}

type vaultPersonasResp struct {
	Personas []vaultPersonaResp `json:"personas"`
}

// vaultSkillResp matches mysloodsiewnia /skill/{slug} JSON shape.
type vaultSkillResp struct {
	Slug         string   `json:"slug"`
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	PersonaID    string   `json:"persona_id"`
	Tags         []string `json:"tags"`
	Instructions string   `json:"instructions"`
	UpdatedAt    string   `json:"updated_at"`
}

type vaultSkillsResp struct {
	Skills []vaultSkillResp `json:"skills"`
}

const cacheTTL = 5 * time.Minute

type cachedPersona struct {
	p        *Persona
	cachedAt time.Time
}

type cachedSkill struct {
	s        *Skill
	cachedAt time.Time
}

type SkillStore struct {
	mu       sync.RWMutex
	dir      string
	pdir     string
	vaultURL string
	client   *http.Client

	// cache (TTL via cachedAt)
	personaCache map[string]cachedPersona
	skillCache   map[string]cachedSkill
	listCacheAt  time.Time
	listPCache   []*Persona
	listSCache   []*Skill
}

// NewSkillStore creates a store backed by content dir, optionally fronted by vault HTTP.
// When vaultURL is set, GET methods fetch from vault with cache + dir fallback.
// When empty, behaves as pure dir-based store.
func NewSkillStore(contentDir, vaultURL string) *SkillStore {
	s := &SkillStore{
		dir:          filepath.Join(contentDir, "skills"),
		pdir:         filepath.Join(contentDir, "personas"),
		vaultURL:     strings.TrimRight(vaultURL, "/"),
		client:       &http.Client{Timeout: 4 * time.Second},
		personaCache: make(map[string]cachedPersona),
		skillCache:   make(map[string]cachedSkill),
	}
	os.MkdirAll(s.dir, 0755)
	os.MkdirAll(s.pdir, 0755)
	return s
}

// ── Personas ─────────────────────────────────────────────────────────────────

func (s *SkillStore) ListPersonas() ([]*Persona, error) {
	if s.vaultURL != "" {
		s.mu.RLock()
		fresh := time.Since(s.listCacheAt) < cacheTTL && s.listPCache != nil
		cached := s.listPCache
		s.mu.RUnlock()
		if fresh {
			return cached, nil
		}
		if list, err := s.fetchPersonasFromVault(); err == nil {
			s.mu.Lock()
			s.listPCache = list
			s.listCacheAt = time.Now()
			s.mu.Unlock()
			return list, nil
		}
		// fallthrough to disk
	}
	return s.listPersonasFromDisk()
}

func (s *SkillStore) GetPersona(slug string) (*Persona, error) {
	if s.vaultURL != "" {
		s.mu.RLock()
		c, ok := s.personaCache[slug]
		s.mu.RUnlock()
		if ok && time.Since(c.cachedAt) < cacheTTL {
			return c.p, nil
		}
		if p, err := s.fetchPersonaFromVault(slug); err == nil {
			s.mu.Lock()
			s.personaCache[slug] = cachedPersona{p: p, cachedAt: time.Now()}
			s.mu.Unlock()
			return p, nil
		}
		// fallthrough to disk
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.loadPersonaFromDisk(slug + ".json")
}

func (s *SkillStore) fetchPersonaFromVault(slug string) (*Persona, error) {
	url := s.vaultURL + "/persona/" + slug
	resp, err := s.client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("vault %d: %s", resp.StatusCode, string(body))
	}
	var v vaultPersonaResp
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return nil, err
	}
	return vaultToPersona(v), nil
}

func (s *SkillStore) fetchPersonasFromVault() ([]*Persona, error) {
	url := s.vaultURL + "/personas"
	resp, err := s.client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("vault list %d", resp.StatusCode)
	}
	var v vaultPersonasResp
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return nil, err
	}
	out := make([]*Persona, 0, len(v.Personas))
	for _, vp := range v.Personas {
		out = append(out, vaultToPersona(vp))
	}
	return out, nil
}

func vaultToPersona(v vaultPersonaResp) *Persona {
	return &Persona{
		Slug:   v.ID,
		Name:   v.Name,
		Role:   v.Role,
		Prompt: v.Prompt,
	}
}

// ListPersonasFromDisk returns only personas stored on the fly volume.
// This is the demo subset shown to anonymous callers (no session code, no OAuth).
func (s *SkillStore) ListPersonasFromDisk() ([]*Persona, error) {
	return s.listPersonasFromDisk()
}

// Disk fallback — read from /data/content/personas/*.json
func (s *SkillStore) listPersonasFromDisk() ([]*Persona, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries, err := os.ReadDir(s.pdir)
	if err != nil {
		return nil, err
	}
	var out []*Persona
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		p, err := s.loadPersonaFromDisk(e.Name())
		if err != nil {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

func (s *SkillStore) loadPersonaFromDisk(filename string) (*Persona, error) {
	data, err := os.ReadFile(filepath.Join(s.pdir, filename))
	if err != nil {
		return nil, err
	}
	var p Persona
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// SavePersona — disabled when vault is canonical. Edit in vault directly.
func (s *SkillStore) SavePersona(p *Persona) error {
	if s.vaultURL != "" {
		return fmt.Errorf("persona writes go to vault directly; SavePersona disabled when VAULT_URL set")
	}
	if p.Slug == "" {
		return fmt.Errorf("slug required")
	}
	p.UpdatedAt = time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.pdir, p.Slug+".json"), data, 0644)
}

func (s *SkillStore) DeletePersona(slug string) error {
	if s.vaultURL != "" {
		return fmt.Errorf("persona deletes go to vault directly; DeletePersona disabled when VAULT_URL set")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	path := filepath.Join(s.pdir, slug+".json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("not found")
	}
	return os.Remove(path)
}

// ── Skills ───────────────────────────────────────────────────────────────────

func (s *SkillStore) ListSkills(category string) ([]*Skill, error) {
	if s.vaultURL != "" {
		s.mu.RLock()
		fresh := time.Since(s.listCacheAt) < cacheTTL && s.listSCache != nil
		cached := s.listSCache
		s.mu.RUnlock()
		if fresh {
			return filterByCategory(cached, category), nil
		}
		if list, err := s.fetchSkillsFromVault(); err == nil {
			s.mu.Lock()
			s.listSCache = list
			// listCacheAt shared with personas — fine, both refresh together
			s.listCacheAt = time.Now()
			s.mu.Unlock()
			return filterByCategory(list, category), nil
		}
	}
	return s.listSkillsFromDisk(category)
}

func filterByCategory(skills []*Skill, category string) []*Skill {
	if category == "" {
		return skills
	}
	out := make([]*Skill, 0, len(skills))
	for _, sk := range skills {
		if sk.Category == category {
			out = append(out, sk)
		}
	}
	return out
}

func (s *SkillStore) GetSkill(slug string) (*Skill, error) {
	if s.vaultURL != "" {
		s.mu.RLock()
		c, ok := s.skillCache[slug]
		s.mu.RUnlock()
		if ok && time.Since(c.cachedAt) < cacheTTL {
			return c.s, nil
		}
		if sk, err := s.fetchSkillFromVault(slug); err == nil {
			s.mu.Lock()
			s.skillCache[slug] = cachedSkill{s: sk, cachedAt: time.Now()}
			s.mu.Unlock()
			return sk, nil
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.loadSkillFromDisk(slug + ".json")
}

func (s *SkillStore) fetchSkillFromVault(slug string) (*Skill, error) {
	url := s.vaultURL + "/skill/" + slug
	resp, err := s.client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("vault %d", resp.StatusCode)
	}
	var v vaultSkillResp
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return nil, err
	}
	return vaultToSkill(v), nil
}

func (s *SkillStore) fetchSkillsFromVault() ([]*Skill, error) {
	url := s.vaultURL + "/skills"
	resp, err := s.client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("vault list %d", resp.StatusCode)
	}
	var v vaultSkillsResp
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return nil, err
	}
	out := make([]*Skill, 0, len(v.Skills))
	for _, vs := range v.Skills {
		out = append(out, vaultToSkill(vs))
	}
	return out, nil
}

func vaultToSkill(v vaultSkillResp) *Skill {
	body := v.Description
	if v.Instructions != "" {
		body = v.Description + "\n\n" + v.Instructions
	}
	cat := v.PersonaID
	if cat == "" {
		cat = "general"
	}
	updated, _ := time.Parse(time.RFC3339, v.UpdatedAt)
	return &Skill{
		Slug:      v.Slug,
		Category:  cat,
		Title:     v.Title,
		Body:      body,
		Tags:      v.Tags,
		UpdatedAt: updated,
	}
}

// Disk fallback
func (s *SkillStore) listSkillsFromDisk(category string) ([]*Skill, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	var out []*Skill
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		sk, err := s.loadSkillFromDisk(e.Name())
		if err != nil {
			continue
		}
		if category != "" && sk.Category != category {
			continue
		}
		out = append(out, sk)
	}
	return out, nil
}

func (s *SkillStore) loadSkillFromDisk(filename string) (*Skill, error) {
	data, err := os.ReadFile(filepath.Join(s.dir, filename))
	if err != nil {
		return nil, err
	}
	var sk Skill
	if err := json.Unmarshal(data, &sk); err != nil {
		return nil, err
	}
	return &sk, nil
}

// SaveSkill — disabled when vault is canonical.
func (s *SkillStore) SaveSkill(sk *Skill) error {
	if s.vaultURL != "" {
		return fmt.Errorf("skill writes go to vault directly; SaveSkill disabled when VAULT_URL set")
	}
	if sk.Slug == "" {
		return fmt.Errorf("slug required")
	}
	sk.UpdatedAt = time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := json.MarshalIndent(sk, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.dir, sk.Slug+".json"), data, 0644)
}

func (s *SkillStore) DeleteSkill(slug string) error {
	if s.vaultURL != "" {
		return fmt.Errorf("skill deletes go to vault directly; DeleteSkill disabled when VAULT_URL set")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	path := filepath.Join(s.dir, slug+".json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("not found")
	}
	return os.Remove(path)
}

func (s *SkillStore) Categories() ([]string, error) {
	skills, err := s.ListSkills("")
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var cats []string
	for _, sk := range skills {
		if !seen[sk.Category] {
			seen[sk.Category] = true
			cats = append(cats, sk.Category)
		}
	}
	return cats, nil
}

// InvalidateCache — call after upserting to vault to force refresh.
func (s *SkillStore) InvalidateCache() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.personaCache = make(map[string]cachedPersona)
	s.skillCache = make(map[string]cachedSkill)
	s.listPCache = nil
	s.listSCache = nil
	s.listCacheAt = time.Time{}
}
