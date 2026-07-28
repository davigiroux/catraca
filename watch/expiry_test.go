package watch

import (
	"context"
	"testing"
	"time"

	solana "github.com/gagliardetto/solana-go"

	"github.com/davigiroux/catraca/store"
	"github.com/davigiroux/catraca/svmtest"
)

// TestScanForExpiry_PendingWithNoTransferExpires: chain time past deadline,
// nothing ever landed -> Expired.
func TestScanForExpiry_PendingWithNoTransferExpires(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	deadline := time.Now().Add(time.Hour)
	intent := mustCreateIntent(t, s, store.NewIntentParams{
		Amount:    "1.5",
		Reference: "ref-1",
		Deadline:  deadline,
	})

	chain := newFakeChain()
	chain.now = deadline.Add(time.Second) // chain has observed past the deadline

	w := &Watcher{Store: s, Chain: chain, DefaultCommitment: CommitmentFinalized}
	if err := w.ScanForExpiry(ctx); err != nil {
		t.Fatalf("ScanForExpiry: %v", err)
	}

	got, err := s.GetIntent(ctx, intent.ID)
	if err != nil {
		t.Fatalf("GetIntent: %v", err)
	}
	if got.State != store.StateExpired {
		t.Fatalf("expected Expired, got %q", got.State)
	}
}

// TestScanForExpiry_PendingBeforeChainDeadlineStaysPending: chain hasn't
// reached the deadline yet -> must not expire, regardless of how long the
// scan has been running.
func TestScanForExpiry_PendingBeforeChainDeadlineStaysPending(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	deadline := time.Now().Add(time.Hour)
	intent := mustCreateIntent(t, s, store.NewIntentParams{
		Amount:    "1.5",
		Reference: "ref-1",
		Deadline:  deadline,
	})

	chain := newFakeChain()
	chain.now = deadline.Add(-time.Minute) // chain time not yet past deadline

	w := &Watcher{Store: s, Chain: chain, DefaultCommitment: CommitmentFinalized}
	if err := w.ScanForExpiry(ctx); err != nil {
		t.Fatalf("ScanForExpiry: %v", err)
	}

	got, err := s.GetIntent(ctx, intent.ID)
	if err != nil {
		t.Fatalf("GetIntent: %v", err)
	}
	if got.State != store.StatePending {
		t.Fatalf("expected still Pending, got %q", got.State)
	}
}

// TestScanForExpiry_DetectedNeverExpiresWhileTransferInFlight: a Detected
// intent's landed transfer has a BlockTime at or before the deadline, but
// chain time is already past the deadline (finality/observation is late).
// The intent must never be expired — it stays Detected, eligible to reach
// Confirmed via ScanDetected.
func TestScanForExpiry_DetectedNeverExpiresWhileTransferInFlight(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	deadline := time.Now().Add(time.Hour)
	intent := mustCreateIntent(t, s, store.NewIntentParams{
		Amount:    "1.5",
		Reference: "ref-1",
		Deadline:  deadline,
	})
	if err := s.UpdateState(ctx, intent.ID, store.StatePending, store.StateDetected); err != nil {
		t.Fatalf("UpdateState: %v", err)
	}

	chain := newFakeChain()
	chain.byReference["ref-1"] = []Transaction{{
		Signature: "sig-1",
		Recipient: intent.Recipient,
		Lamports:  1_500_000_000,
		BlockTime: deadline.Add(-time.Minute), // landed before the deadline
	}}
	chain.commitments["sig-1"] = CommitmentProcessed // not yet final
	chain.now = deadline.Add(24 * time.Hour)         // chain time is way past deadline

	w := &Watcher{Store: s, Chain: chain, DefaultCommitment: CommitmentFinalized}

	// Run several rounds, as a crash-recovery / repeated-scan simulation
	// would: Expired must never become reachable.
	for i := 0; i < 3; i++ {
		if err := w.ScanForExpiry(ctx); err != nil {
			t.Fatalf("ScanForExpiry: %v", err)
		}
		got, err := s.GetIntent(ctx, intent.ID)
		if err != nil {
			t.Fatalf("GetIntent: %v", err)
		}
		if got.State != store.StateDetected {
			t.Fatalf("round %d: expected still Detected, got %q", i, got.State)
		}
	}

	// Finality now arrives late; ScanDetected must still be able to
	// confirm it.
	chain.commitments["sig-1"] = CommitmentFinalized
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

// TestScanForExpiry_DetectedWithNoEligibleTransferExpires: a Detected
// intent whose only landed transfer has a BlockTime after the deadline
// (shouldn't normally happen, but exercises the boundary) expires once
// chain time passes the deadline.
func TestScanForExpiry_DetectedWithNoEligibleTransferExpires(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	deadline := time.Now().Add(time.Hour)
	intent := mustCreateIntent(t, s, store.NewIntentParams{
		Amount:    "1.5",
		Reference: "ref-1",
		Deadline:  deadline,
	})
	if err := s.UpdateState(ctx, intent.ID, store.StatePending, store.StateDetected); err != nil {
		t.Fatalf("UpdateState: %v", err)
	}

	chain := newFakeChain()
	chain.byReference["ref-1"] = []Transaction{{
		Signature: "sig-1",
		Recipient: intent.Recipient,
		Lamports:  1_500_000_000,
		BlockTime: deadline.Add(time.Minute), // landed after the deadline
	}}
	chain.commitments["sig-1"] = CommitmentFinalized
	chain.now = deadline.Add(2 * time.Minute)

	w := &Watcher{Store: s, Chain: chain, DefaultCommitment: CommitmentFinalized}
	if err := w.ScanForExpiry(ctx); err != nil {
		t.Fatalf("ScanForExpiry: %v", err)
	}

	got, err := s.GetIntent(ctx, intent.ID)
	if err != nil {
		t.Fatalf("GetIntent: %v", err)
	}
	if got.State != store.StateExpired {
		t.Fatalf("expected Expired, got %q", got.State)
	}
}

// TestScanForExpiry_RestartRescanBeforeExpiring simulates a crash/restart:
// a brand new Watcher (fresh, no in-memory state) built against a chain
// that has NOT yet observed past the deadline must not expire, and only
// after a subsequent scan observes chain time past the deadline (having
// established that fact itself) does it expire. This proves Expired is
// only reachable via a chain-time check performed by the scan itself, not
// by trusting any prior in-process state.
func TestScanForExpiry_RestartRescanBeforeExpiring(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	deadline := time.Now().Add(time.Hour)
	intent := mustCreateIntent(t, s, store.NewIntentParams{
		Amount:    "1.5",
		Reference: "ref-1",
		Deadline:  deadline,
	})

	chain := newFakeChain()
	chain.now = deadline.Add(-time.Second) // "crash" happens before deadline

	// First Watcher instance ("pre-crash process"): scans, deadline not
	// yet reached chain-side, nothing expires.
	w1 := &Watcher{Store: s, Chain: chain, DefaultCommitment: CommitmentFinalized}
	if err := w1.ScanForExpiry(ctx); err != nil {
		t.Fatalf("ScanForExpiry (pre-crash): %v", err)
	}
	got, err := s.GetIntent(ctx, intent.ID)
	if err != nil {
		t.Fatalf("GetIntent: %v", err)
	}
	if got.State != store.StatePending {
		t.Fatalf("expected still Pending pre-crash, got %q", got.State)
	}

	// "Restart": a brand new Watcher value, sharing only the durable
	// store and chain — no carried-over in-process state. Chain time has
	// now moved past the deadline (the chain itself, not the watcher's
	// wall clock, establishes this).
	chain.now = deadline.Add(time.Minute)
	w2 := &Watcher{Store: s, Chain: chain, DefaultCommitment: CommitmentFinalized}
	if err := w2.ScanForExpiry(ctx); err != nil {
		t.Fatalf("ScanForExpiry (post-restart): %v", err)
	}
	got, err = s.GetIntent(ctx, intent.ID)
	if err != nil {
		t.Fatalf("GetIntent: %v", err)
	}
	if got.State != store.StateExpired {
		t.Fatalf("expected Expired post-restart re-scan, got %q", got.State)
	}
}

// TestScanForExpiry_ViaSVM exercises the same "transfer landed before
// deadline, chain observes past deadline late" scenario against the real
// in-process litesvm-go harness, warping its Clock sysvar rather than
// sleeping wall-clock time.
func TestScanForExpiry_ViaSVM(t *testing.T) {
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

	chainNow := time.Unix(h.Clock().UnixTimestamp, 0).UTC()
	deadline := chainNow.Add(time.Minute)

	const lamports = 1_500_000_000
	intent := mustCreateIntent(t, s, store.NewIntentParams{
		Recipient: recipient.String(),
		Amount:    "1.5",
		Reference: reference.String(),
		Deadline:  deadline,
	})

	chain := newSVMChain(h)
	// Lands well before the deadline, but won't finalize until slot 100.
	chain.SendTransferWithReference(payer, recipient, reference, lamports, 100)

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

	// Move chain time well past the deadline without warping slots (so
	// the transfer still hasn't finalized) — simulates finality arriving
	// late.
	h.AdvanceClockBy(24 * time.Hour)

	if err := w.ScanForExpiry(ctx); err != nil {
		t.Fatalf("ScanForExpiry: %v", err)
	}
	got, err = s.GetIntent(ctx, intent.ID)
	if err != nil {
		t.Fatalf("GetIntent: %v", err)
	}
	if got.State != store.StateDetected {
		t.Fatalf("expected still Detected (transfer in flight), got %q", got.State)
	}

	// Finality catches up.
	h.WarpToSlot(100)
	if err := w.ScanDetected(ctx); err != nil {
		t.Fatalf("ScanDetected: %v", err)
	}
	got, err = s.GetIntent(ctx, intent.ID)
	if err != nil {
		t.Fatalf("GetIntent: %v", err)
	}
	if got.State != store.StateConfirmed {
		t.Fatalf("expected Confirmed, got %q", got.State)
	}
}
