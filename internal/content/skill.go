package content

import (
	"encoding/json"
	"fmt"
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
	Slug      string    `json:"slug"`
	Name      string    `json:"name"`
	Role      string    `json:"role"`
	Prompt    string    `json:"prompt"`
	Tags      []string  `json:"tags,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
	UpdatedBy string    `json:"updated_by,omitempty"`
}

type SkillStore struct {
	mu   sync.RWMutex
	dir  string
	pdir string
}

func NewSkillStore(contentDir string) *SkillStore {
	s := &SkillStore{
		dir:  filepath.Join(contentDir, "skills"),
		pdir: filepath.Join(contentDir, "personas"),
	}
	os.MkdirAll(s.dir, 0755)
	os.MkdirAll(s.pdir, 0755)
	return s
}

func (s *SkillStore) ListSkills(category string) ([]*Skill, error) {
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
		sk, err := s.loadSkill(e.Name())
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

func (s *SkillStore) GetSkill(slug string) (*Skill, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.loadSkill(slug + ".json")
}

func (s *SkillStore) SaveSkill(sk *Skill) error {
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

func (s *SkillStore) loadSkill(filename string) (*Skill, error) {
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

func (s *SkillStore) ListPersonas() ([]*Persona, error) {
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
		p, err := s.loadPersona(e.Name())
		if err != nil {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

func (s *SkillStore) GetPersona(slug string) (*Persona, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.loadPersona(slug + ".json")
}

func (s *SkillStore) SavePersona(p *Persona) error {
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
	s.mu.Lock()
	defer s.mu.Unlock()
	path := filepath.Join(s.pdir, slug+".json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("not found")
	}
	return os.Remove(path)
}

func (s *SkillStore) loadPersona(filename string) (*Persona, error) {
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
