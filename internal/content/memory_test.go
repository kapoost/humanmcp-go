package content

import (
	"strings"
	"testing"
	"time"
)

func TestMemorySave(t *testing.T) {
	ms := NewMemoryStore(t.TempDir())
	m, err := ms.Save("Łukasz woli krótkie odpowiedzi", "test-session", []string{"preferences"})
	if err != nil {
		t.Fatalf("save failed: %v", err)
	}
	if m.ID == "" {
		t.Error("ID should not be empty")
	}
	if m.Body != "Łukasz woli krótkie odpowiedzi" {
		t.Error("body mismatch")
	}
	if m.AgentHint != "test-session" {
		t.Error("agent_hint mismatch")
	}
}

func TestMemorySaveEmptyBody(t *testing.T) {
	ms := NewMemoryStore(t.TempDir())
	_, err := ms.Save("", "hint", nil)
	if err == nil {
		t.Error("empty body should return error")
	}
	_, err = ms.Save("   ", "hint", nil)
	if err == nil {
		t.Error("whitespace-only body should return error")
	}
}

func TestMemorySaveTruncatesLongBody(t *testing.T) {
	ms := NewMemoryStore(t.TempDir())
	long := strings.Repeat("a", 3000)
	m, err := ms.Save(long, "", nil)
	if err != nil {
		t.Fatalf("save failed: %v", err)
	}
	if len(m.Body) > 2000 {
		t.Errorf("body should be truncated to 2000, got %d", len(m.Body))
	}
}

func TestMemoryList(t *testing.T) {
	ms := NewMemoryStore(t.TempDir())
	ms.Save("pierwsza", "s1", []string{"tech"})
	time.Sleep(time.Millisecond)
	ms.Save("druga", "s2", []string{"personal"})
	time.Sleep(time.Millisecond)
	ms.Save("trzecia", "s3", []string{"tech"})

	all, err := ms.List("", 0)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("expected 3 memories, got %d", len(all))
	}
	// Najnowsze najpierw
	if all[0].Body != "trzecia" {
		t.Errorf("first should be newest, got: %s", all[0].Body)
	}
}

func TestMemoryListFilterByTag(t *testing.T) {
	ms := NewMemoryStore(t.TempDir())
	ms.Save("tech obs", "", []string{"tech"})
	ms.Save("personal obs", "", []string{"personal"})
	ms.Save("tech obs 2", "", []string{"tech", "preferences"})

	tech, _ := ms.List("tech", 0)
	if len(tech) != 2 {
		t.Errorf("expected 2 tech memories, got %d", len(tech))
	}
}

func TestMemoryListLimit(t *testing.T) {
	ms := NewMemoryStore(t.TempDir())
	for i := 0; i < 5; i++ {
		ms.Save("obs", "", nil)
		time.Sleep(time.Millisecond)
	}
	limited, _ := ms.List("", 3)
	if len(limited) != 3 {
		t.Errorf("expected 3 (limit), got %d", len(limited))
	}
}

func TestMemoryDelete(t *testing.T) {
	ms := NewMemoryStore(t.TempDir())
	m, _ := ms.Save("do usuniecia", "", nil)

	err := ms.Delete(m.ID)
	if err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	all, _ := ms.List("", 0)
	if len(all) != 0 {
		t.Error("memory should be deleted")
	}
}

func TestMemoryDeleteNotFound(t *testing.T) {
	ms := NewMemoryStore(t.TempDir())
	err := ms.Delete("nieistniejace-id")
	if err == nil {
		t.Error("deleting nonexistent memory should return error")
	}
}
