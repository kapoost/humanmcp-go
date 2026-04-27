package content

import (
	"encoding/json"
	"testing"
	"time"
)

func TestListingSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir() + "/content"
	store := NewListingStore(dir)

	l := &Listing{
		Slug:    "test-item",
		Type:    ListingSell,
		Title:   "Test Item",
		Body:    "A test listing body.",
		Tags:    []string{"test", "go"},
		Price:   "100 PLN",
		Status:  ListingOpen,
		Access:  AccessPublic,
	}
	if err := store.Save(l); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 listing, got %d", len(loaded))
	}
	if loaded[0].Slug != "test-item" {
		t.Errorf("slug mismatch: %s", loaded[0].Slug)
	}
	if loaded[0].Title != "Test Item" {
		t.Errorf("title mismatch: %s", loaded[0].Title)
	}
	if loaded[0].Price != "100 PLN" {
		t.Errorf("price mismatch: %s", loaded[0].Price)
	}
}

func TestListingIsExpired(t *testing.T) {
	l := &Listing{Status: ListingOpen}
	if l.IsExpired() {
		t.Error("no expiry should not be expired")
	}

	l.ExpiresAt = time.Now().Add(-time.Hour)
	if !l.IsExpired() {
		t.Error("past expiry should be expired")
	}

	l.ExpiresAt = time.Now().Add(time.Hour)
	if l.IsExpired() {
		t.Error("future expiry should not be expired")
	}
}

func TestListingIsActive(t *testing.T) {
	l := &Listing{Status: ListingOpen}
	if !l.IsActive() {
		t.Error("open listing should be active")
	}

	l.Status = ListingPaused
	if l.IsActive() {
		t.Error("paused listing should not be active")
	}

	l.Status = ListingOpen
	l.ExpiresAt = time.Now().Add(-time.Hour)
	if l.IsActive() {
		t.Error("expired listing should not be active")
	}
}

func TestListingStoreListSince(t *testing.T) {
	dir := t.TempDir() + "/content"
	store := NewListingStore(dir)

	old := &Listing{
		Slug: "old", Type: ListingSell, Title: "Old", Status: ListingOpen, Access: AccessPublic,
		Published: time.Now().Add(-2 * time.Hour),
	}
	recent := &Listing{
		Slug: "recent", Type: ListingBuy, Title: "Recent", Status: ListingOpen, Access: AccessPublic,
		Published: time.Now().Add(-30 * time.Minute),
	}
	store.Save(old)
	store.Save(recent)

	since := time.Now().Add(-time.Hour)
	result := store.ListSince(since)
	if len(result) != 1 || result[0].Slug != "recent" {
		t.Errorf("expected only 'recent', got %d items", len(result))
	}
}

func TestListingStoreGetAndDelete(t *testing.T) {
	dir := t.TempDir() + "/content"
	store := NewListingStore(dir)

	store.Save(&Listing{Slug: "x", Title: "X", Status: ListingOpen, Access: AccessPublic})

	l, err := store.Get("x")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if l.Title != "X" {
		t.Errorf("title: %s", l.Title)
	}

	if err := store.Delete("x"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err = store.Get("x")
	if err == nil {
		t.Error("should not find deleted listing")
	}
}

func TestListingJSONShape(t *testing.T) {
	l := &Listing{
		Slug:      "test",
		Type:      ListingOffer,
		Title:     "Test",
		Body:      "Body",
		Tags:      []string{"a"},
		Price:     "free",
		PriceSats: 1000,
		Status:    ListingOpen,
		Access:    AccessPublic,
		Published: time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC),
	}
	data, err := json.Marshal(l)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]interface{}
	json.Unmarshal(data, &m)

	for _, key := range []string{"slug", "type", "title", "body", "tags", "price", "price_sats", "status", "access", "published"} {
		if _, ok := m[key]; !ok {
			t.Errorf("missing JSON key: %s", key)
		}
	}
}

func TestListingStoreUpdate(t *testing.T) {
	dir := t.TempDir() + "/content"
	store := NewListingStore(dir)

	store.Save(&Listing{Slug: "u", Title: "Original", Status: ListingOpen})
	store.Save(&Listing{Slug: "u", Title: "Updated", Status: ListingPaused})

	l, _ := store.Get("u")
	if l.Title != "Updated" {
		t.Errorf("expected Updated, got %s", l.Title)
	}
	if l.Status != ListingPaused {
		t.Errorf("expected paused, got %s", l.Status)
	}

	all, _ := store.Load()
	if len(all) != 1 {
		t.Errorf("expected 1 listing after update, got %d", len(all))
	}
}
