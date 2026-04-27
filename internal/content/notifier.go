package content

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"
)

type Notifier struct {
	listings  *ListingStore
	subs      *SubscriptionStore
	statStore *StatStore
	client    *http.Client
	logger    *log.Logger
}

func NewNotifier(listings *ListingStore, subs *SubscriptionStore, statStore *StatStore, logger *log.Logger) *Notifier {
	return &Notifier{
		listings:  listings,
		subs:      subs,
		statStore: statStore,
		client:    &http.Client{Timeout: 5 * time.Second},
		logger:    logger,
	}
}

func (n *Notifier) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n.tick()
		}
	}
}

func (n *Notifier) tick() {
	subs := n.subs.List()
	for _, sub := range subs {
		if !sub.Active || sub.Channel != SubWebhook || sub.CallbackURL == "" {
			continue
		}
		n.deliver(sub)
	}
}

func (n *Notifier) deliver(sub *Subscription) {
	since := sub.LastSeen
	if since.IsZero() {
		since = sub.Created
	}
	listings := n.listings.ListSince(since)

	var matching []*Listing
	for _, l := range listings {
		if l.Access == AccessPublic && sub.Matches(l) {
			matching = append(matching, l)
		}
	}
	if len(matching) == 0 {
		return
	}

	payload := struct {
		SubscriptionID string     `json:"subscription_id"`
		Listings       []*Listing `json:"listings"`
	}{
		SubscriptionID: sub.ID,
		Listings:       matching,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return
	}

	resp, err := n.client.Post(sub.CallbackURL, "application/json", bytes.NewReader(body))
	if err != nil {
		n.handleFailure(sub)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		sub.LastSeen = time.Now().UTC()
		sub.FailCount = 0
		n.subs.Update(sub)
		// Record match events
		if n.statStore != nil {
			for _, l := range matching {
				n.statStore.Record(Event{
					Type: EventSubscribeMatch,
					Slug: l.Slug,
				})
			}
		}
	} else {
		n.handleFailure(sub)
	}
}

func (n *Notifier) handleFailure(sub *Subscription) {
	sub.FailCount++
	if sub.FailCount >= 10 {
		sub.Active = false
		n.logger.Printf("[notifier] subscription %s deactivated after %d failures", sub.ID, sub.FailCount)
	}
	n.subs.Update(sub)
}
