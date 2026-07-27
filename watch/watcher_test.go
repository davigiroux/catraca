package watch

import (
	"context"
	"testing"
	"time"

	"github.com/davigiroux/catraca/store"
)

// fakeChain is a test-only ChainReader with no chain/RPC/litesvm
// dependency at all — the interface boundary itself.
type fakeChain struct {
	byReference map[string][]Transaction
	commitments map[string]Commitment
	// now is the fake chain's current block time. Tests advance it
	// directly (never via time.Now()) to simulate chain time passing.
	now time.Time
}

func newFakeChain() *fakeChain {
	return &fakeChain{
		byReference: map[string][]Transaction{},
		commitments: map[string]Commitment{},
		now:         time.Now(),
	}
}

func (f *fakeChain) FindTransactionsByReference(ctx context.Context, reference string) ([]Transaction, error) {
	return f.byReference[reference], nil
}

func (f *fakeChain) GetCommitment(ctx context.Context, signature string) (Commitment, error) {
	return f.commitments[signature], nil
}

func (f *fakeChain) CurrentBlockTime(ctx context.Context) (time.Time, error) {
	return f.now, nil
}

type fakeMerchants struct {
	confirmedOnly map[string]bool
}

func (f fakeMerchants) RequireConfirmedOnly(merchantID string) (bool, bool) {
	v, ok := f.confirmedOnly[merchantID]
	return v, ok
}

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return s
}

func mustCreateIntent(t *testing.T, s *store.Store, p store.NewIntentParams) store.Intent {
	t.Helper()
	if p.MerchantID == "" {
		p.MerchantID = "merchant-1"
	}
	if p.Recipient == "" {
		p.Recipient = "mvines9iiHiQTysrwkJjGf2gb9Ex9jXJX8ns3qwf2kN"
	}
	if p.Deadline.IsZero() {
		p.Deadline = time.Now().Add(time.Hour)
	}
	got, err := s.CreateIntent(context.Background(), p)
	if err != nil {
		t.Fatalf("CreateIntent: %v", err)
	}
	return got
}

func TestScanPending_TransitionsToDetectedWhenLanded(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	intent := mustCreateIntent(t, s, store.NewIntentParams{Amount: "1.5", Reference: "ref-1"})

	chain := newFakeChain()
	chain.byReference["ref-1"] = []Transaction{{Signature: "sig-1", Recipient: intent.Recipient, Lamports: 1_500_000_000}}

	w := &Watcher{Store: s, Chain: chain, DefaultCommitment: CommitmentFinalized}
	if err := w.ScanPending(ctx); err != nil {
		t.Fatalf("ScanPending: %v", err)
	}

	got, err := s.GetIntent(ctx, intent.ID)
	if err != nil {
		t.Fatalf("GetIntent: %v", err)
	}
	if got.State != store.StateDetected {
		t.Fatalf("expected Detected, got %q", got.State)
	}
}

func TestScanPending_LeavesUntouchedWhenNotLanded(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	intent := mustCreateIntent(t, s, store.NewIntentParams{Amount: "1.5", Reference: "ref-1"})

	chain := newFakeChain() // no transactions for any reference

	w := &Watcher{Store: s, Chain: chain, DefaultCommitment: CommitmentFinalized}
	if err := w.ScanPending(ctx); err != nil {
		t.Fatalf("ScanPending: %v", err)
	}

	got, err := s.GetIntent(ctx, intent.ID)
	if err != nil {
		t.Fatalf("GetIntent: %v", err)
	}
	if got.State != store.StatePending {
		t.Fatalf("expected Pending, got %q", got.State)
	}
}

func TestScanDetected_ConfirmsWhenFinalizedAndAmountMatches(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	intent := mustCreateIntent(t, s, store.NewIntentParams{Amount: "1.5", Reference: "ref-1"})
	if err := s.UpdateState(ctx, intent.ID, store.StatePending, store.StateDetected); err != nil {
		t.Fatalf("UpdateState: %v", err)
	}

	chain := newFakeChain()
	chain.byReference["ref-1"] = []Transaction{{Signature: "sig-1", Recipient: intent.Recipient, Lamports: 1_500_000_000}}
	chain.commitments["sig-1"] = CommitmentFinalized

	w := &Watcher{Store: s, Chain: chain, DefaultCommitment: CommitmentFinalized}
	if err := w.ScanDetected(ctx); err != nil {
		t.Fatalf("ScanDetected: %v", err)
	}

	got, err := s.GetIntent(ctx, intent.ID)
	if err != nil {
		t.Fatalf("GetIntent: %v", err)
	}
	if got.State != store.StateConfirmed {
		t.Fatalf("expected Confirmed, got %q", got.State)
	}
}

func TestScanDetected_WaitsForRequiredCommitment(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	intent := mustCreateIntent(t, s, store.NewIntentParams{Amount: "1.5", Reference: "ref-1"})
	if err := s.UpdateState(ctx, intent.ID, store.StatePending, store.StateDetected); err != nil {
		t.Fatalf("UpdateState: %v", err)
	}

	chain := newFakeChain()
	chain.byReference["ref-1"] = []Transaction{{Signature: "sig-1", Recipient: intent.Recipient, Lamports: 1_500_000_000}}
	chain.commitments["sig-1"] = CommitmentConfirmed // not finalized

	w := &Watcher{Store: s, Chain: chain, DefaultCommitment: CommitmentFinalized}
	if err := w.ScanDetected(ctx); err != nil {
		t.Fatalf("ScanDetected: %v", err)
	}

	got, err := s.GetIntent(ctx, intent.ID)
	if err != nil {
		t.Fatalf("GetIntent: %v", err)
	}
	if got.State != store.StateDetected {
		t.Fatalf("expected still Detected, got %q", got.State)
	}
}

func TestScanDetected_MerchantOptedDownToConfirmed(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	intent := mustCreateIntent(t, s, store.NewIntentParams{MerchantID: "merchant-opted-down", Amount: "1.5", Reference: "ref-1"})
	if err := s.UpdateState(ctx, intent.ID, store.StatePending, store.StateDetected); err != nil {
		t.Fatalf("UpdateState: %v", err)
	}

	chain := newFakeChain()
	chain.byReference["ref-1"] = []Transaction{{Signature: "sig-1", Recipient: intent.Recipient, Lamports: 1_500_000_000}}
	chain.commitments["sig-1"] = CommitmentConfirmed // finalized not required for this merchant

	merchants := fakeMerchants{confirmedOnly: map[string]bool{"merchant-opted-down": true}}

	w := &Watcher{Store: s, Chain: chain, Merchants: merchants, DefaultCommitment: CommitmentFinalized}
	if err := w.ScanDetected(ctx); err != nil {
		t.Fatalf("ScanDetected: %v", err)
	}

	got, err := s.GetIntent(ctx, intent.ID)
	if err != nil {
		t.Fatalf("GetIntent: %v", err)
	}
	if got.State != store.StateConfirmed {
		t.Fatalf("expected Confirmed, got %q", got.State)
	}
}

func TestScanDetected_AmountMismatchStaysDetected(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	intent := mustCreateIntent(t, s, store.NewIntentParams{Amount: "1.5", Reference: "ref-1"})
	if err := s.UpdateState(ctx, intent.ID, store.StatePending, store.StateDetected); err != nil {
		t.Fatalf("UpdateState: %v", err)
	}

	chain := newFakeChain()
	// Landed transfer is for the wrong amount.
	chain.byReference["ref-1"] = []Transaction{{Signature: "sig-1", Recipient: intent.Recipient, Lamports: 1_000_000_000}}
	chain.commitments["sig-1"] = CommitmentFinalized

	w := &Watcher{Store: s, Chain: chain, DefaultCommitment: CommitmentFinalized}
	if err := w.ScanDetected(ctx); err != nil {
		t.Fatalf("ScanDetected: %v", err)
	}

	got, err := s.GetIntent(ctx, intent.ID)
	if err != nil {
		t.Fatalf("GetIntent: %v", err)
	}
	if got.State != store.StateDetected {
		t.Fatalf("expected to remain Detected on amount mismatch (Mismatched is #8), got %q", got.State)
	}
}

func TestScanDetected_MintMismatchStaysDetected(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	intent := mustCreateIntent(t, s, store.NewIntentParams{Amount: "1.5", Reference: "ref-1"}) // native SOL intent
	if err := s.UpdateState(ctx, intent.ID, store.StatePending, store.StateDetected); err != nil {
		t.Fatalf("UpdateState: %v", err)
	}

	chain := newFakeChain()
	chain.byReference["ref-1"] = []Transaction{{
		Signature: "sig-1", Recipient: intent.Recipient, Lamports: 1_500_000_000,
		Mint: "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v", // SPL, not native
	}}
	chain.commitments["sig-1"] = CommitmentFinalized

	w := &Watcher{Store: s, Chain: chain, DefaultCommitment: CommitmentFinalized}
	if err := w.ScanDetected(ctx); err != nil {
		t.Fatalf("ScanDetected: %v", err)
	}

	got, err := s.GetIntent(ctx, intent.ID)
	if err != nil {
		t.Fatalf("GetIntent: %v", err)
	}
	if got.State != store.StateDetected {
		t.Fatalf("expected to remain Detected on mint mismatch, got %q", got.State)
	}
}
