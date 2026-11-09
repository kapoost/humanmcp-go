package content

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// --- Store tests ---

func TestStoreLoadAndList(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, dir, "hello.md", `---
slug: hello
title: Hello World
type: poem
access: public
tags: [test]
published: 2024-01-01
---

Line one.
Line two.`)

	s := NewStore(dir)
	if err := s.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	pieces := s.List(true)
	if len(pieces) != 1 {
		t.Fatalf("want 1 piece, got %d", len(pieces))
	}
	p := pieces[0]
	if p.Slug != "hello" { t.Errorf("slug: got %q", p.Slug) }
	if p.Title != "Hello World" { t.Errorf("title: got %q", p.Title) }
	if p.Type != "poem" { t.Errorf("type: got %q", p.Type) }
	if p.Access != AccessPublic { t.Errorf("access: got %q", p.Access) }
	if len(p.Tags) != 1 || p.Tags[0] != "test" { t.Errorf("tags: got %v", p.Tags) }
	if p.Body != "Line one.\nLine two." { t.Errorf("body: got %q", p.Body) }
}

func TestStoreGetLockedRedactsBody(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, dir, "secret.md", `---
slug: secret
title: Secret Poem
type: poem
access: locked
gate: challenge
challenge: What color is the sky?
answer: blue
---

The secret body.`)

	s := NewStore(dir)
	s.Load()

	// Without unlock — body should be empty
	p, err := s.Get("secret", false)
	if err != nil { t.Fatalf("Get: %v", err) }
	if p.Body != "" { t.Errorf("locked piece should have empty body, got %q", p.Body) }

	// With unlock — body should be present
	p2, _ := s.Get("secret", true)
	if p2.Body != "The secret body." { t.Errorf("unlocked body: got %q", p2.Body) }
}

func TestStoreCheckAnswer(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, dir, "q.md", `---
slug: q
title: Q
type: poem
access: locked
gate: challenge
challenge: What color is the sky?
answer: blue
---
body`)

	s := NewStore(dir)
	s.Load()

	if !s.CheckAnswer("q", "blue") { t.Error("exact answer should pass") }
	if !s.CheckAnswer("q", "Blue") { t.Error("case-insensitive should pass") }
	if !s.CheckAnswer("q", "  blue  ") { t.Error("trimmed whitespace should pass") }
	if s.CheckAnswer("q", "red") { t.Error("wrong answer should fail") }
	if s.CheckAnswer("q", "") { t.Error("empty answer should fail") }
	if s.CheckAnswer("nonexistent", "blue") { t.Error("nonexistent slug should fail") }
}

func TestStoreSaveAndDelete(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	s.Load()

	p := &Piece{
		Slug:      "new-poem",
		Title:     "New Poem",
		Type:      "poem",
		Access:    AccessPublic,
		Body:      "Hello world.",
		Tags:      []string{"test"},
		Published: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	if err := s.Save(p); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// File should exist
	if _, err := os.Stat(filepath.Join(dir, "new-poem.md")); err != nil {
		t.Errorf("file not created: %v", err)
	}

	// Should be loadable
	s2 := NewStore(dir)
	s2.Load()
	loaded, err := s2.Get("new-poem", false)
	if err != nil { t.Fatalf("Get after Save: %v", err) }
	if loaded.Title != "New Poem" { t.Errorf("title: got %q", loaded.Title) }
	if loaded.Body != "Hello world." { t.Errorf("body: got %q", loaded.Body) }

	// Delete
	if err := s.Delete("new-poem"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "new-poem.md")); !os.IsNotExist(err) {
		t.Error("file should be deleted")
	}
}

// --- Time gate tests ---

func TestTimeGateUnlocked(t *testing.T) {
	p := &Piece{
		Access:      AccessLocked,
		Gate:        GateTime,
		UnlockAfter: time.Now().Add(-1 * time.Hour), // past
	}
	if !p.IsUnlocked() {
		t.Error("piece with past unlock_after should be unlocked")
	}
}

func TestTimeGateLocked(t *testing.T) {
	p := &Piece{
		Access:      AccessLocked,
		Gate:        GateTime,
		UnlockAfter: time.Now().Add(24 * time.Hour), // future
	}
	if p.IsUnlocked() {
		t.Error("piece with future unlock_after should be locked")
	}
}

func TestPublicAlwaysUnlocked(t *testing.T) {
	p := &Piece{Access: AccessPublic}
	if !p.IsUnlocked() {
		t.Error("public piece should always be unlocked")
	}
}

// --- Frontmatter tests ---

func TestFrontmatterQuotedStrings(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, dir, "quoted.md", `---
slug: quoted
title: "A Title: With Colon"
description: "Short desc."
type: poem
access: public
---
body`)

	s := NewStore(dir)
	s.Load()
	p, err := s.Get("quoted", false)
	if err != nil { t.Fatalf("Get: %v", err) }
	if p.Title != "A Title: With Colon" { t.Errorf("title: got %q", p.Title) }
	if p.Description != "Short desc." { t.Errorf("desc: got %q", p.Description) }
}

func TestFrontmatterTagsBracketSyntax(t *testing.T) {
	dir := t.TempDir()
	writeMD(t, dir, "tags.md", `---
slug: tags
title: Tags
type: poem
access: public
tags: [sea, sailing, code]
---
body`)

	s := NewStore(dir)
	s.Load()
	p, _ := s.Get("tags", false)
	if len(p.Tags) != 3 { t.Fatalf("want 3 tags, got %d: %v", len(p.Tags), p.Tags) }
	if p.Tags[0] != "sea" || p.Tags[1] != "sailing" || p.Tags[2] != "code" {
		t.Errorf("tags: got %v", p.Tags)
	}
}

func TestFrontmatterRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	s.Load()

	original := &Piece{
		Slug:        "roundtrip",
		Title:       "Round Trip: Test",
		Type:        "poem",
		Access:      AccessLocked,
		Gate:        GateChallenge,
		Challenge:   "What is 2+2?",
		Answer:      "four",
		Description: "A test piece.",
		Tags:        []string{"a", "b"},
		Published:   time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		Body:        "Hello\nWorld",
	}

	s.Save(original)

	s2 := NewStore(dir)
	s2.Load()
	loaded, err := s2.GetForEdit("roundtrip")
	if err != nil { t.Fatalf("GetForEdit: %v", err) }

	if loaded.Title != original.Title { t.Errorf("title: %q != %q", loaded.Title, original.Title) }
	if loaded.Gate != original.Gate { t.Errorf("gate: %q != %q", loaded.Gate, original.Gate) }
	if loaded.Challenge != original.Challenge { t.Errorf("challenge: %q", loaded.Challenge) }
	if loaded.Answer != original.Answer { t.Errorf("answer: %q", loaded.Answer) }
	if loaded.Body != original.Body { t.Errorf("body: %q != %q", loaded.Body, original.Body) }
}

// --- helper ---
func writeMD(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatalf("writeMD: %v", err)
	}
}

// Store jest wspoldzielony przez handlery HTTP i MCP, a Go obsluguje kazde
// zadanie w osobnej goroutine. Bez tego testu wyscig na mapie `pieces` jest
// niewidoczny: reszta pakietu chodzi sekwencyjnie, wiec `-race` nigdy nie
// dotyka tej sciezki. Fix, ktorego nie da sie zademonstrowac, nie przezyje
// nastepnego refaktoru.
func TestStoreConcurrentReadWrite(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if err := s.Load(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if err := s.Save(&Piece{Slug: fmt.Sprintf("seed-%d", i), Title: "seed", Access: AccessPublic}); err != nil {
			t.Fatal(err)
		}
	}

	const workers, rounds = 8, 40
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < rounds; i++ {
				switch w % 4 {
				case 0:
					slug := fmt.Sprintf("w%d-%d", w, i)
					_ = s.Save(&Piece{Slug: slug, Title: "x", Access: AccessPublic})
					_ = s.Delete(slug)
				case 1:
					_ = s.List(false)
				case 2:
					_, _ = s.Get("seed-0", false)
					_, _ = s.GetForEdit("seed-1")
				case 3:
					_ = s.CheckAnswer("seed-2", "cokolwiek")
					_ = s.Load()
				}
			}
		}(w)
	}
	wg.Wait()

	// Ziarno musi przetrwac cala kanonade — Load() podmienia mape, wiec
	// to sprawdza tez, czy podmiana nie gubi zapisow z dysku.
	if got := len(s.List(false)); got < 5 {
		t.Errorf("po wspolbieznych operacjach zostalo %d kawalkow, spodziewam sie >= 5", got)
	}
}
