package content

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestNotifierDeliversWebhook(t *testing.T) {
	dir := t.TempDir() + "/content"
	ls := NewListingStore(dir)
	ss := NewSubscriptionStore(dir)
	statStore := NewStatStore(dir)

	var received atomic.Int32
	var lastBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		lastBody = buf[:n]
		w.WriteHeader(200)
	}))
	defer srv.Close()

	sub := &Subscription{
		Channel:     SubWebhook,
		CallbackURL: srv.URL,
	}
	ss.Create(sub)

	// Save a listing after sub was created
	time.Sleep(10 * time.Millisecond)
	ls.Save(&Listing{
		Slug: "test", Title: "Test", Type: ListingSell, Status: ListingOpen,
		Access: AccessPublic, Published: time.Now(),
	})

	n := NewNotifier(ls, ss, statStore, log.Default())
	n.tick()

	if received.Load() != 1 {
		t.Errorf("expected 1 webhook delivery, got %d", received.Load())
	}

	var payload struct {
		SubscriptionID string    `json:"subscription_id"`
		Listings       []Listing `json:"listings"`
	}
	json.Unmarshal(lastBody, &payload)
	if payload.SubscriptionID != sub.ID {
		t.Errorf("subscription_id mismatch: %s", payload.SubscriptionID)
	}
	if len(payload.Listings) != 1 || payload.Listings[0].Slug != "test" {
		t.Errorf("listings mismatch: %+v", payload.Listings)
	}
}

func TestNotifierLastSeenAdvances(t *testing.T) {
	dir := t.TempDir() + "/content"
	ls := NewListingStore(dir)
	ss := NewSubscriptionStore(dir)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	sub := &Subscription{Channel: SubWebhook, CallbackURL: srv.URL}
	ss.Create(sub)

	time.Sleep(10 * time.Millisecond)
	ls.Save(&Listing{
		Slug: "a", Title: "A", Type: ListingSell, Status: ListingOpen,
		Access: AccessPublic, Published: time.Now(),
	})

	n := NewNotifier(ls, ss, nil, log.Default())
	n.tick()

	updated, _ := ss.Get(sub.ID)
	if updated.LastSeen.IsZero() {
		t.Error("LastSeen should be set after delivery")
	}

	// Second tick should not deliver again
	var count atomic.Int32
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		w.WriteHeader(200)
	}))
	defer srv2.Close()

	updated.CallbackURL = srv2.URL
	ss.Update(updated)

	n2 := NewNotifier(ls, ss, nil, log.Default())
	n2.tick()

	if count.Load() != 0 {
		t.Errorf("should not redeliver, got %d", count.Load())
	}
}

func TestNotifierDeactivatesAfter10Failures(t *testing.T) {
	dir := t.TempDir() + "/content"
	ls := NewListingStore(dir)
	ss := NewSubscriptionStore(dir)

	// Server that always returns 500
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	sub := &Subscription{Channel: SubWebhook, CallbackURL: srv.URL}
	ss.Create(sub)

	time.Sleep(10 * time.Millisecond)
	ls.Save(&Listing{
		Slug: "fail", Title: "Fail", Type: ListingSell, Status: ListingOpen,
		Access: AccessPublic, Published: time.Now(),
	})

	n := NewNotifier(ls, ss, nil, log.Default())
	for i := 0; i < 10; i++ {
		n.tick()
	}

	updated, _ := ss.Get(sub.ID)
	if updated.Active {
		t.Error("subscription should be deactivated after 10 failures")
	}
	if updated.FailCount < 10 {
		t.Errorf("fail count should be >= 10, got %d", updated.FailCount)
	}
}

func TestNotifierRunContext(t *testing.T) {
	dir := t.TempDir() + "/content"
	ls := NewListingStore(dir)
	ss := NewSubscriptionStore(dir)

	n := NewNotifier(ls, ss, nil, log.Default())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		n.Run(ctx, 10*time.Millisecond)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Error("Run should stop when context is cancelled")
	}
}

func TestNotifierSkipsNonPublicListings(t *testing.T) {
	dir := t.TempDir() + "/content"
	ls := NewListingStore(dir)
	ss := NewSubscriptionStore(dir)

	var count atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	sub := &Subscription{Channel: SubWebhook, CallbackURL: srv.URL}
	ss.Create(sub)

	time.Sleep(10 * time.Millisecond)
	ls.Save(&Listing{
		Slug: "private", Title: "Private", Type: ListingSell, Status: ListingOpen,
		Access: AccessLocked, Published: time.Now(),
	})

	n := NewNotifier(ls, ss, nil, log.Default())
	n.tick()

	if count.Load() != 0 {
		t.Error("should not deliver non-public listings")
	}
}
