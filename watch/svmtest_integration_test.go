package watch

import (
	"context"
	"testing"

	solana "github.com/gagliardetto/solana-go"

	"github.com/davigiroux/catraca/store"
	"github.com/davigiroux/catraca/svmtest"
)

// TestWatcher_PendingToDetectedToConfirmed_ViaSVM exercises the full
// happy path against an in-process litesvm-go instance: fund a payer, send
// a SOL transfer carrying the intent's reference, scan Pending → Detected,
// warp the clock forward, scan Detected → Confirmed. Zero network.
func TestWatcher_PendingToDetectedToConfirmed_ViaSVM(t *testing.T) {
	ctx := context.Background()
	h := svmtest.New(t)
	s := newTestStore(t)

	recipientKey, err := solana.NewRandomPrivateKey()
	if err != nil {
		t.Fatalf("NewRandomPrivateKey: %v", err)
	}
	recipient := recipientKey.PublicKey()

	referenceKey, err := solana.NewRandomPrivateKey()
	if err != nil {
		t.Fatalf("NewRandomPrivateKey: %v", err)
	}
	reference := referenceKey.PublicKey()

	payer := h.FundedPayer()

	const lamports = 1_500_000_000 // 1.5 SOL
	intent := mustCreateIntent(t, s, store.NewIntentParams{
		Recipient: recipient.String(),
		Amount:    "1.5",
		Reference: reference.String(),
	})

	chain := newSVMChain(h)
	// Not finalized until slot 100.
	chain.SendTransferWithReference(payer, recipient, reference, lamports, 100)

	w := &Watcher{Store: s, Chain: chain, DefaultCommitment: CommitmentFinalized}

	// Pending -> Detected: the transfer has landed.
	if err := w.ScanPending(ctx); err != nil {
		t.Fatalf("ScanPending: %v", err)
	}
	got, err := s.GetIntent(ctx, intent.ID)
	if err != nil {
		t.Fatalf("GetIntent: %v", err)
	}
	if got.State != store.StateDetected {
		t.Fatalf("expected Detected after ScanPending, got %q", got.State)
	}

	// Detected should stay Detected before finality.
	if err := w.ScanDetected(ctx); err != nil {
		t.Fatalf("ScanDetected: %v", err)
	}
	got, err = s.GetIntent(ctx, intent.ID)
	if err != nil {
		t.Fatalf("GetIntent: %v", err)
	}
	if got.State != store.StateDetected {
		t.Fatalf("expected still Detected before finality, got %q", got.State)
	}

	// Warp past finality and rescan.
	h.WarpToSlot(100)
	if err := w.ScanDetected(ctx); err != nil {
		t.Fatalf("ScanDetected: %v", err)
	}
	got, err = s.GetIntent(ctx, intent.ID)
	if err != nil {
		t.Fatalf("GetIntent: %v", err)
	}
	if got.State != store.StateConfirmed {
		t.Fatalf("expected Confirmed after finality, got %q", got.State)
	}
}
