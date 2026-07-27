package watch

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/davigiroux/catraca/store"
)

// ScanForExpiry transitions Pending and Detected intents to Expired once
// the chain has been observed past their Deadline — and only then. Per ADR
// 0002 and CONTEXT.md's "Deadline"/"Expired" entries, expiry is judged
// against the chain's current block time (w.Chain.CurrentBlockTime), never
// the watcher's wall clock: this is what makes payment outcomes
// independent of catraca's own uptime, and it's what makes crash-recovery
// safe — a freshly-restarted watcher re-derives "is it past deadline?" from
// the chain itself before it may emit Expired, rather than trusting
// anything about elapsed wall-clock time it can't have observed while
// down.
//
// An intent is only expired if no transaction carrying its reference has a
// BlockTime at or before the Deadline. This is why a Detected intent is
// never expired while its transfer is in flight: the transfer that put it
// in Detected already carries a BlockTime, and if that BlockTime is ≤
// Deadline the intent stays eligible to reach Confirmed via ScanDetected
// no matter how late finality itself arrives.
func (w *Watcher) ScanForExpiry(ctx context.Context) error {
	now, err := w.Chain.CurrentBlockTime(ctx)
	if err != nil {
		return fmt.Errorf("watch: scan for expiry: current block time: %w", err)
	}

	var errs []error
	for _, state := range []store.State{store.StatePending, store.StateDetected} {
		intents, err := w.Store.ListByState(ctx, state)
		if err != nil {
			errs = append(errs, fmt.Errorf("watch: scan for expiry: list %s: %w", state, err))
			continue
		}
		for _, intent := range intents {
			if err := w.scanOneForExpiry(ctx, intent, state, now); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

func (w *Watcher) scanOneForExpiry(ctx context.Context, intent store.Intent, from store.State, now time.Time) error {
	if !now.After(intent.Deadline) {
		// Chain time hasn't reached the deadline yet: not eligible for
		// expiry, no matter how long the watcher has been running or how
		// long it was down.
		return nil
	}

	txs, err := w.Chain.FindTransactionsByReference(ctx, intent.Reference)
	if err != nil {
		return fmt.Errorf("watch: scan for expiry: intent %s: %w", intent.ID, err)
	}
	for _, tx := range txs {
		if !tx.BlockTime.After(intent.Deadline) {
			// A transfer carrying the reference landed with a block time
			// at or before the deadline — it counts (CONTEXT.md
			// "Deadline"), even if it hasn't finalized yet. Leave it for
			// ScanPending/ScanDetected to carry to Confirmed; never
			// expire out from under it.
			return nil
		}
	}

	if err := w.Store.UpdateState(ctx, intent.ID, from, store.StateExpired); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			// Already transitioned by a concurrent scan (e.g. ScanDetected
			// confirmed it in the meantime), or gone.
			return nil
		}
		return fmt.Errorf("watch: scan for expiry: intent %s: %w", intent.ID, err)
	}
	return nil
}
