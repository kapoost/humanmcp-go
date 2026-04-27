package content

import (
	"testing"
)

func TestSubscriptionCreateAndGet(t *testing.T) {
	dir := t.TempDir() + "/content"
	store := NewSubscriptionStore(dir)

	sub := &Subscription{
		Channel:     SubWebhook,
		CallbackURL: "https://example.com/hook",
		FilterTypes: []string{"sell"},
	}
	if err := store.Create(sub); err != nil {
		t.Fatalf("create: %v", err)
	}

	if sub.ID == "" || sub.Token == "" {
		t.Error("ID and Token should be set after Create")
	}
	if !sub.Active {
		t.Error("subscription should be active")
	}

	got, err := store.Get(sub.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.CallbackURL != "https://example.com/hook" {
		t.Errorf("callback: %s", got.CallbackURL)
	}
}

func TestSubscriptionGetByToken(t *testing.T) {
	dir := t.TempDir() + "/content"
	store := NewSubscriptionStore(dir)

	sub := &Subscription{Channel: SubMCP}
	store.Create(sub)

	got, err := store.GetByToken(sub.Token)
	if err != nil {
		t.Fatalf("get by token: %v", err)
	}
	if got.ID != sub.ID {
		t.Errorf("id mismatch")
	}
}

func TestSubscriptionTokenUniqueness(t *testing.T) {
	dir := t.TempDir() + "/content"
	store := NewSubscriptionStore(dir)

	sub1 := &Subscription{Channel: SubMCP}
	sub2 := &Subscription{Channel: SubMCP}
	store.Create(sub1)
	store.Create(sub2)

	if sub1.Token == sub2.Token {
		t.Error("tokens should be unique")
	}
	if sub1.ID == sub2.ID {
		t.Error("IDs should be unique")
	}
}

func TestSubscriptionMatches(t *testing.T) {
	tests := []struct {
		name    string
		sub     Subscription
		listing Listing
		want    bool
	}{
		{
			name:    "no filters matches everything",
			sub:     Subscription{},
			listing: Listing{Type: ListingSell, Tags: []string{"x"}},
			want:    true,
		},
		{
			name:    "type filter match",
			sub:     Subscription{FilterTypes: []string{"sell"}},
			listing: Listing{Type: ListingSell},
			want:    true,
		},
		{
			name:    "type filter no match",
			sub:     Subscription{FilterTypes: []string{"buy"}},
			listing: Listing{Type: ListingSell},
			want:    false,
		},
		{
			name:    "tag filter match (OR)",
			sub:     Subscription{FilterTags: []string{"a", "b"}},
			listing: Listing{Type: ListingSell, Tags: []string{"b", "c"}},
			want:    true,
		},
		{
			name:    "tag filter no match",
			sub:     Subscription{FilterTags: []string{"a", "b"}},
			listing: Listing{Type: ListingSell, Tags: []string{"c"}},
			want:    false,
		},
		{
			name:    "type AND tag both must pass",
			sub:     Subscription{FilterTypes: []string{"sell"}, FilterTags: []string{"a"}},
			listing: Listing{Type: ListingBuy, Tags: []string{"a"}},
			want:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.sub.Matches(&tc.listing); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSubscriptionUpdateAndDelete(t *testing.T) {
	dir := t.TempDir() + "/content"
	store := NewSubscriptionStore(dir)

	sub := &Subscription{Channel: SubWebhook, CallbackURL: "https://a.com"}
	store.Create(sub)

	sub.Active = false
	if err := store.Update(sub); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, _ := store.Get(sub.ID)
	if got.Active {
		t.Error("should be inactive after update")
	}

	if err := store.Delete(sub.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	list := store.List()
	if len(list) != 0 {
		t.Errorf("expected 0 after delete, got %d", len(list))
	}
}

func TestActiveCount(t *testing.T) {
	dir := t.TempDir() + "/content"
	store := NewSubscriptionStore(dir)

	store.Create(&Subscription{Channel: SubMCP})
	store.Create(&Subscription{Channel: SubMCP})

	if store.ActiveCount() != 2 {
		t.Errorf("expected 2, got %d", store.ActiveCount())
	}

	subs := store.List()
	subs[0].Active = false
	store.Update(subs[0])

	if store.ActiveCount() != 1 {
		t.Errorf("expected 1, got %d", store.ActiveCount())
	}
}
