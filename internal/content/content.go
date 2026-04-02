package content

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type AccessLevel string

const (
	AccessPublic  AccessLevel = "public"
	AccessMembers AccessLevel = "members"
	AccessLocked  AccessLevel = "locked"
)

type GateType string

const (
	GatePayment   GateType = "payment"
	GateChallenge GateType = "challenge"
	GateManual    GateType = "manual"   // owner reviews + approves
	GateTime      GateType = "time"     // unlocks on a date
	GateTrade     GateType = "trade"    // future: peer exchange
)

type Piece struct {
	Slug        string      `json:"Slug"`
	Title       string      `json:"Title"`
	Type        string      `json:"Type"`
	Access      AccessLevel `json:"Access"`
	Gate        GateType    `json:"Gate"`
	Challenge   string      `json:"Challenge"`
	Answer      string      `json:"Answer"`
	License     string      `json:"License"`    // LicenseType string
	PriceSats   int         `json:"PriceSats"`
	UnlockAfter time.Time   `json:"UnlockAfter"` // for time gate
	Tags        []string    `json:"Tags"`
	Published   time.Time   `json:"Published"`
	Description string      `json:"Description"`
	Body        string      `json:"Body"`
	Signature   string      `json:"Signature"`  // Ed25519 signature (base64)
	OTSProof    string      `json:"OTSProof"`   // OpenTimestamps proof (base64) — Bitcoin-anchored timestamp
	FilePath    string      `json:"-"`
}

// IsUnlocked returns true if the piece is accessible without a gate
func (p *Piece) IsUnlocked() bool {
	if p.Access == AccessPublic {
		return true
	}
	if p.Gate == GateTime && !p.UnlockAfter.IsZero() && time.Now().After(p.UnlockAfter) {
		return true
	}
	return false
}

type Store struct {
	dir    string
	pieces map[string]*Piece
	cache  *Cache[[]*Piece]  // TTL cache for List()
}

func NewStore(dir string) *Store {
	return &Store{
		dir:    dir,
		pieces: make(map[string]*Piece),
		cache:  NewCache[[]*Piece](5 * time.Second),
	}
}

func (s *Store) Load() error {
	s.pieces = make(map[string]*Piece)
	return filepath.WalkDir(s.dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		p, err := parsePiece(path)
		if err != nil {
			return nil
		}
		if p.Slug == "" {
			p.Slug = strings.TrimSuffix(filepath.Base(path), ".md")
		}
		if p.Type == "" { p.Type = "poem" }
		if p.Access == "" { p.Access = AccessPublic }
		if p.Published.IsZero() { p.Published = time.Now() }
		s.pieces[p.Slug] = p
		return nil
	})
}

func (s *Store) List(includeBody bool) []*Piece {
	out := make([]*Piece, 0, len(s.pieces))
	for _, p := range s.pieces {
		cp := *p
		if !includeBody && !cp.IsUnlocked() {
			cp.Body = ""
		}
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Published.After(out[j].Published)
	})
	return out
}

func (s *Store) Get(slug string, unlocked bool) (*Piece, error) {
	p, ok := s.pieces[slug]
	if !ok {
		return nil, fmt.Errorf("not found: %s", slug)
	}
	if p.IsUnlocked() || unlocked {
		return p, nil
	}
	cp := *p
	cp.Body = ""
	return &cp, nil
}

func (s *Store) GetForEdit(slug string) (*Piece, error) {
	p, ok := s.pieces[slug]
	if !ok {
		return nil, fmt.Errorf("not found: %s", slug)
	}
	return p, nil
}

func (s *Store) CheckAnswer(slug, answer string) bool {
	p, ok := s.pieces[slug]
	if !ok || p.Gate != GateChallenge {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(p.Answer), strings.TrimSpace(answer))
}

func (s *Store) Save(p *Piece) error {
	if err := os.MkdirAll(s.dir, 0755); err != nil {
		return err
	}
	if p.Published.IsZero() {
		p.Published = time.Now()
	}
	path := filepath.Join(s.dir, p.Slug+".md")
	p.FilePath = path

	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.WriteString(marshalFrontmatter(p))
	buf.WriteString("---\n\n")
	buf.WriteString(p.Body)

	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		return err
	}
	s.pieces[p.Slug] = p
	s.cache.Invalidate()
	return nil
}

func (s *Store) Delete(slug string) error {
	p, ok := s.pieces[slug]
	if !ok {
		return fmt.Errorf("not found: %s", slug)
	}
	if err := os.Remove(p.FilePath); err != nil {
		return err
	}
	delete(s.pieces, slug)
	s.cache.Invalidate()
	return nil
}

func parsePiece(path string) (*Piece, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	p := &Piece{FilePath: path}
	var fmLines []string
	var bodyLines []string
	inFM := false
	fmDone := false
	lineNum := 0

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		lineNum++
		if lineNum == 1 && line == "---" { inFM = true; continue }
		if inFM && line == "---" { inFM = false; fmDone = true; continue }
		if inFM { fmLines = append(fmLines, line) } else if fmDone { bodyLines = append(bodyLines, line) }
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	parseFrontmatter(fmLines, p)
	body := strings.Join(bodyLines, "\n")
	p.Body = strings.TrimPrefix(body, "\n")
	return p, nil
}
