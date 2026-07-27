package notify

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/davigiroux/catraca/store"
)

// fakeIntents implements IntentLister over an in-memory list, grouped by
// state, without touching a real database.
type fakeIntents struct {
	byState map[store.State][]store.Intent
}

func (f *fakeIntents) ListByState(_ context.Context, state store.State) ([]store.Intent, error) {
	return f.byState[state], nil
}

// fakeMerchants implements MerchantLookup.
type fakeMerchants map[string]Endpoint

func (f fakeMerchants) WebhookEndpoint(merchantID string) (Endpoint, bool) {
	e, ok := f[merchantID]
	return e, ok
}

func newDeliveryStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open delivery store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestNotifier_OnlyTerminalStatesProduceDeliveries(t *testing.T) {
	var received atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	intents := &fakeIntents{byState: map[store.State][]store.Intent{
		store.StatePending:  {{ID: "p1", MerchantID: "m1"}},
		store.StateDetected: {{ID: "d1", MerchantID: "m1"}},
	}}
	merchants := fakeMerchants{"m1": {URL: srv.URL, Secret: "s"}}
	deliveries := newDeliveryStore(t)

	n := &Notifier{Intents: intents, Deliveries: deliveries, Merchants: merchants}
	if err := n.Scan(context.Background()); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	if got := received.Load(); got != 0 {
		t.Fatalf("expected 0 webhook deliveries for non-terminal states, got %d", got)
	}
}

func TestNotifier_DeliversSignedWebhookForTerminalStates(t *testing.T) {
	secret := "whsec_abc"
	var gotHeader string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get(SignatureHeaderName)
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		gotBody = buf
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	intent := store.Intent{ID: "i1", MerchantID: "m1", State: store.StateConfirmed, Recipient: "r", Amount: "1.5", Reference: "ref"}
	intents := &fakeIntents{byState: map[store.State][]store.Intent{
		store.StateConfirmed: {intent},
	}}
	merchants := fakeMerchants{"m1": {URL: srv.URL, Secret: secret}}
	deliveries := newDeliveryStore(t)

	n := &Notifier{Intents: intents, Deliveries: deliveries, Merchants: merchants}
	if err := n.Scan(context.Background()); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	if gotHeader == "" {
		t.Fatal("expected a signature header on the delivered webhook")
	}
	if !VerifySignature(secret, gotHeader, gotBody) {
		t.Fatalf("merchant could not verify signature %q over body %q", gotHeader, gotBody)
	}

	d, err := deliveries.Get(context.Background(), intent.ID)
	if err != nil {
		t.Fatalf("Get delivery: %v", err)
	}
	if !d.Delivered {
		t.Fatal("expected delivery to be marked Delivered")
	}
	if d.Attempts != 1 {
		t.Fatalf("expected 1 attempt, got %d", d.Attempts)
	}
}

func TestNotifier_RetriesWithBackoffThenDeadLetters(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	intent := store.Intent{ID: "i1", MerchantID: "m1", State: store.StateExpired}
	intents := &fakeIntents{byState: map[store.State][]store.Intent{
		store.StateExpired: {intent},
	}}
	merchants := fakeMerchants{"m1": {URL: srv.URL, Secret: "s"}}
	deliveries := newDeliveryStore(t)

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	n := &Notifier{
		Intents: intents, Deliveries: deliveries, Merchants: merchants,
		Now: func() time.Time { return now },
	}

	// Fail through the whole schedule.
	for i := 0; i < len(Schedule); i++ {
		if err := n.Scan(context.Background()); err != nil {
			t.Fatalf("Scan attempt %d: %v", i+1, err)
		}
		d, err := deliveries.Get(context.Background(), intent.ID)
		if err != nil {
			t.Fatalf("Get delivery: %v", err)
		}
		if d.DeadLettered {
			t.Fatalf("dead-lettered too early, after attempt %d", i+1)
		}
		wantNext := now.Add(Schedule[i])
		if !d.NextRetryAt.Equal(wantNext) {
			t.Fatalf("attempt %d: next retry = %v, want %v", i+1, d.NextRetryAt, wantNext)
		}
		// Advance past the retry time so the next Scan is due.
		now = wantNext.Add(time.Second)
	}

	// One more failed attempt beyond the schedule: dead-letter.
	if err := n.Scan(context.Background()); err != nil {
		t.Fatalf("final scan: %v", err)
	}
	d, err := deliveries.Get(context.Background(), intent.ID)
	if err != nil {
		t.Fatalf("Get delivery: %v", err)
	}
	if !d.DeadLettered {
		t.Fatal("expected delivery to be dead-lettered after exhausting the schedule")
	}
	if d.Attempts != len(Schedule)+1 {
		t.Fatalf("expected %d attempts, got %d", len(Schedule)+1, d.Attempts)
	}
}

func TestNotifier_RetryNotDueYetIsSkipped(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	intent := store.Intent{ID: "i1", MerchantID: "m1", State: store.StateConfirmed}
	intents := &fakeIntents{byState: map[store.State][]store.Intent{
		store.StateConfirmed: {intent},
	}}
	merchants := fakeMerchants{"m1": {URL: srv.URL, Secret: "s"}}
	deliveries := newDeliveryStore(t)

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	n := &Notifier{
		Intents: intents, Deliveries: deliveries, Merchants: merchants,
		Now: func() time.Time { return now },
	}

	if err := n.Scan(context.Background()); err != nil {
		t.Fatalf("Scan 1: %v", err)
	}
	if got := attempts.Load(); got != 1 {
		t.Fatalf("expected 1 attempt, got %d", got)
	}

	// Same "now": the retry isn't due yet, so a second scan should not
	// call the endpoint again.
	if err := n.Scan(context.Background()); err != nil {
		t.Fatalf("Scan 2: %v", err)
	}
	if got := attempts.Load(); got != 1 {
		t.Fatalf("expected still 1 attempt (retry not due), got %d", got)
	}
}

func TestNotifier_FailingEndpointNeverTouchesIntentStore(t *testing.T) {
	// The IntentLister here has no UpdateState method at all, so if the
	// Notifier ever tried to write intent state, this test would fail to
	// compile — this is a compile-time guarantee, not just runtime.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	intent := store.Intent{ID: "i1", MerchantID: "m1", State: store.StateMismatched}
	intents := &fakeIntents{byState: map[store.State][]store.Intent{
		store.StateMismatched: {intent},
	}}
	merchants := fakeMerchants{"m1": {URL: srv.URL, Secret: "s"}}
	deliveries := newDeliveryStore(t)

	n := &Notifier{Intents: intents, Deliveries: deliveries, Merchants: merchants}
	if err := n.Scan(context.Background()); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	// intents.byState is untouched (fakeIntents has no mutation path), and
	// the intent's State field is never read back or asserted here — the
	// point is that Notifier has no way to mutate it.
}

func TestNotifier_SlowEndpointDoesNotBlockOtherMerchants(t *testing.T) {
	release := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		w.WriteHeader(http.StatusOK)
	}))
	defer slow.Close()
	defer close(release)

	var fastDelivered atomic.Bool
	fast := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fastDelivered.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	defer fast.Close()

	intents := &fakeIntents{byState: map[store.State][]store.Intent{
		store.StateConfirmed: {
			{ID: "slow-intent", MerchantID: "slow-merchant", State: store.StateConfirmed},
			{ID: "fast-intent", MerchantID: "fast-merchant", State: store.StateConfirmed},
		},
	}}
	merchants := fakeMerchants{
		"slow-merchant": {URL: slow.URL, Secret: "s"},
		"fast-merchant": {URL: fast.URL, Secret: "s"},
	}
	deliveries := newDeliveryStore(t)

	n := &Notifier{Intents: intents, Deliveries: deliveries, Merchants: merchants}

	done := make(chan struct{})
	go func() {
		_ = n.Scan(context.Background())
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("Scan returned before the slow endpoint was released")
	case <-time.After(200 * time.Millisecond):
		// Expected: Scan is still blocked on the slow endpoint.
	}

	if !fastDelivered.Load() {
		t.Fatal("fast merchant's webhook was not delivered while the slow one was still in flight")
	}

	release <- struct{}{}
	<-done
}
