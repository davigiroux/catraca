package watch

import (
	"context"
	"errors"
	"fmt"

	"github.com/davigiroux/catraca/store"
)

// MerchantLookup resolves a merchant's required commitment level. Kept
// minimal so the watcher doesn't need the full api.Merchant shape.
type MerchantLookup interface {
	// RequireConfirmedOnly reports whether the merchant has opted down from
	// the platform default (finalized) to confirmed commitment (ADR 0001).
	// ok is false if the merchant is unknown.
	RequireConfirmedOnly(merchantID string) (confirmedOnly bool, ok bool)
}

// IntentStore is the subset of *store.Store the watcher needs. Defined here
// so tests can substitute a fake without a real SQLite database.
type IntentStore interface {
	ListByState(ctx context.Context, state store.State) ([]store.Intent, error)
	UpdateState(ctx context.Context, id string, from, to store.State) error
}

// Watcher scans Pending intents for landed transfers (→ Detected) and
// Detected intents for sufficient commitment + valid amount/mint (→
// Confirmed).
type Watcher struct {
	Store     IntentStore
	Chain     ChainReader
	Merchants MerchantLookup

	// DefaultCommitment is the commitment required for merchants that
	// haven't opted down. Per ADR 0001 this should be CommitmentFinalized.
	DefaultCommitment Commitment
}

// ScanPending looks for a landed transaction referencing each Pending
// intent's Reference and, if found, transitions it to Detected. Errors from
// individual intents are collected and returned together; scanning
// continues past a single intent's failure.
func (w *Watcher) ScanPending(ctx context.Context) error {
	intents, err := w.Store.ListByState(ctx, store.StatePending)
	if err != nil {
		return fmt.Errorf("watch: scan pending: list: %w", err)
	}

	var errs []error
	for _, intent := range intents {
		txs, err := w.Chain.FindTransactionsByReference(ctx, intent.Reference)
		if err != nil {
			errs = append(errs, fmt.Errorf("watch: scan pending: intent %s: %w", intent.ID, err))
			continue
		}
		if len(txs) == 0 {
			continue
		}

		if err := w.Store.UpdateState(ctx, intent.ID, store.StatePending, store.StateDetected); err != nil {
			// Already transitioned by a concurrent scan, or gone: not an
			// error worth surfacing.
			if !errors.Is(err, store.ErrNotFound) {
				errs = append(errs, fmt.Errorf("watch: scan pending: intent %s: %w", intent.ID, err))
			}
		}
	}
	return errors.Join(errs...)
}

// ScanDetected checks each Detected intent's landed transaction against the
// merchant's required commitment level and against the intent's expected
// amount/mint. Intents that pass both checks transition to Confirmed.
//
// Amount mismatches are left in Detected rather than transitioned to a
// Mismatched state — that state and its handling is ticket #8.
func (w *Watcher) ScanDetected(ctx context.Context) error {
	intents, err := w.Store.ListByState(ctx, store.StateDetected)
	if err != nil {
		return fmt.Errorf("watch: scan detected: list: %w", err)
	}

	var errs []error
	for _, intent := range intents {
		if err := w.scanOneDetected(ctx, intent); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (w *Watcher) scanOneDetected(ctx context.Context, intent store.Intent) error {
	txs, err := w.Chain.FindTransactionsByReference(ctx, intent.Reference)
	if err != nil {
		return fmt.Errorf("watch: scan detected: intent %s: %w", intent.ID, err)
	}
	if len(txs) == 0 {
		// Landed previously (that's how it became Detected) but the
		// ChainReader no longer reports it; nothing to do this pass.
		return nil
	}
	tx := txs[0]

	commitment, err := w.Chain.GetCommitment(ctx, tx.Signature)
	if err != nil {
		return fmt.Errorf("watch: scan detected: intent %s: %w", intent.ID, err)
	}
	if commitment < w.requiredCommitment(intent.MerchantID) {
		return nil // not final enough yet
	}

	// Phase 0 is SOL-only: both the intent and the landed transfer must be
	// native SOL (empty mint).
	if intent.Mint != "" || tx.Mint != "" {
		// TODO(#8): SPL/mint mismatches should transition to Mismatched.
		return nil
	}

	wantLamports, err := lamportsFromSOL(intent.Amount)
	if err != nil {
		return fmt.Errorf("watch: scan detected: intent %s: %w", intent.ID, err)
	}
	if tx.Lamports != wantLamports {
		// TODO(#8): amount mismatches should transition to Mismatched
		// rather than being silently left in Detected.
		return nil
	}

	if err := w.Store.UpdateState(ctx, intent.ID, store.StateDetected, store.StateConfirmed); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		return fmt.Errorf("watch: scan detected: intent %s: %w", intent.ID, err)
	}
	return nil
}

func (w *Watcher) requiredCommitment(merchantID string) Commitment {
	if w.Merchants != nil {
		if confirmedOnly, ok := w.Merchants.RequireConfirmedOnly(merchantID); ok && confirmedOnly {
			return CommitmentConfirmed
		}
	}
	return w.DefaultCommitment
}
