package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/davigiroux/catraca/store"
)

// TerminalStates are the intent states that trigger a webhook delivery
// attempt. Pending and Detected intents never produce a delivery.
var TerminalStates = []store.State{
	store.StateConfirmed,
	store.StateMismatched,
	store.StateExpired,
}

// IntentLister is the subset of *store.Store the Notifier needs to read
// intents. It is read-only: the Notifier never writes to payment_intents.
type IntentLister interface {
	ListByState(ctx context.Context, state store.State) ([]store.Intent, error)
}

// DeliveryStore is the subset of *notify.Store the Notifier needs to track
// delivery attempts.
type DeliveryStore interface {
	Get(ctx context.Context, intentID string) (Delivery, error)
	Save(ctx context.Context, d Delivery) error
}

// Endpoint is a merchant's webhook configuration.
type Endpoint struct {
	URL    string
	Secret string
}

// MerchantLookup resolves a merchant's webhook endpoint. Kept minimal (like
// watch.MerchantLookup) so the Notifier doesn't need the full api.Merchant
// shape.
type MerchantLookup interface {
	WebhookEndpoint(merchantID string) (Endpoint, bool)
}

// Notifier scans terminal-state intents and delivers signed webhooks,
// tracking attempts in a DeliveryStore separate from intent state.
type Notifier struct {
	Intents    IntentLister
	Deliveries DeliveryStore
	Merchants  MerchantLookup

	// HTTPClient is used for delivery attempts. Its Timeout bounds a single
	// attempt so one hung endpoint can't stall an entire scan pass. If nil,
	// a client with AttemptTimeout (or a 10s default) is used.
	HTTPClient *http.Client

	// AttemptTimeout bounds a single delivery attempt when HTTPClient is
	// nil. Defaults to 10s.
	AttemptTimeout time.Duration

	// Now returns the current time; overridable in tests.
	Now func() time.Time
}

func (n *Notifier) now() time.Time {
	if n.Now != nil {
		return n.Now()
	}
	return time.Now().UTC()
}

func (n *Notifier) client() *http.Client {
	if n.HTTPClient != nil {
		return n.HTTPClient
	}
	timeout := n.AttemptTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &http.Client{Timeout: timeout}
}

// payload is the JSON body sent to a merchant's webhook endpoint.
type payload struct {
	IntentID   string `json:"intent_id"`
	MerchantID string `json:"merchant_id"`
	State      string `json:"state"`
	Recipient  string `json:"recipient"`
	Amount     string `json:"amount"`
	Mint       string `json:"mint,omitempty"`
	Reference  string `json:"reference"`
}

// Scan lists intents across all terminal states and attempts delivery for
// each one that is due (new, or past its next-retry-at, and not delivered
// or dead-lettered). Each intent's delivery attempt runs independently and
// concurrently, so one merchant's slow or down endpoint never blocks or
// delays delivery to any other merchant. Errors from individual intents are
// collected and returned together.
func (n *Notifier) Scan(ctx context.Context) error {
	var intents []store.Intent
	for _, state := range TerminalStates {
		batch, err := n.Intents.ListByState(ctx, state)
		if err != nil {
			return fmt.Errorf("notify: scan: list %s: %w", state, err)
		}
		intents = append(intents, batch...)
	}

	var (
		mu   sync.Mutex
		errs []error
		wg   sync.WaitGroup
	)
	for _, intent := range intents {
		wg.Add(1)
		go func(intent store.Intent) {
			defer wg.Done()
			if err := n.deliverOne(ctx, intent); err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		}(intent)
	}
	wg.Wait()

	return errors.Join(errs...)
}

func (n *Notifier) deliverOne(ctx context.Context, intent store.Intent) error {
	d, err := n.Deliveries.Get(ctx, intent.ID)
	if err != nil {
		return fmt.Errorf("notify: intent %s: %w", intent.ID, err)
	}
	if d.Delivered || d.DeadLettered {
		return nil
	}
	now := n.now()
	if d.Attempts > 0 && now.Before(d.NextRetryAt) {
		return nil // not due yet
	}

	endpoint, ok := n.Merchants.WebhookEndpoint(intent.MerchantID)
	if !ok {
		return fmt.Errorf("notify: intent %s: unknown merchant %s", intent.ID, intent.MerchantID)
	}

	body, err := json.Marshal(payload{
		IntentID:   intent.ID,
		MerchantID: intent.MerchantID,
		State:      string(intent.State),
		Recipient:  intent.Recipient,
		Amount:     intent.Amount,
		Mint:       intent.Mint,
		Reference:  intent.Reference,
	})
	if err != nil {
		return fmt.Errorf("notify: intent %s: marshal payload: %w", intent.ID, err)
	}

	sendErr := n.send(ctx, endpoint, body, now.Unix())

	d.Attempts++
	d.UpdatedAt = now
	if sendErr == nil {
		d.Delivered = true
		d.LastError = ""
	} else {
		d.LastError = sendErr.Error()
		if delay, ok := NextDelay(d.Attempts); ok {
			d.NextRetryAt = now.Add(delay)
		} else {
			d.DeadLettered = true
		}
	}

	if err := n.Deliveries.Save(ctx, d); err != nil {
		return fmt.Errorf("notify: intent %s: save delivery: %w", intent.ID, err)
	}
	return nil
}

func (n *Notifier) send(ctx context.Context, endpoint Endpoint, body []byte, timestamp int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(SignatureHeaderName, SignatureHeader(endpoint.Secret, timestamp, body))

	resp, err := n.client().Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("endpoint returned status %d", resp.StatusCode)
	}
	return nil
}
