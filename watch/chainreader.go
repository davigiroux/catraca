// Package watch scans the chain for transfers matching Pending Payment
// Intents (Pending → Detected) and confirms them once they land at the
// required commitment level (Detected → Confirmed). See CONTEXT.md and
// docs/adr/0001-finalized-by-default.md.
package watch

import (
	"context"
	"time"
)

// Commitment mirrors Solana RPC commitment levels, ordered from least to
// most final. Comparison with < / > reflects finality ordering (Processed <
// Confirmed < Finalized).
type Commitment int

const (
	// CommitmentProcessed: the transaction has landed in a bank but may
	// still be rolled back.
	CommitmentProcessed Commitment = iota
	// CommitmentConfirmed: the transaction has been voted on by a
	// supermajority of the cluster.
	CommitmentConfirmed
	// CommitmentFinalized: the transaction is in a block that has been
	// finalized (rollback is not possible).
	CommitmentFinalized
)

// String returns the RPC-style commitment name.
func (c Commitment) String() string {
	switch c {
	case CommitmentProcessed:
		return "processed"
	case CommitmentConfirmed:
		return "confirmed"
	case CommitmentFinalized:
		return "finalized"
	default:
		return "unknown"
	}
}

// Transaction is a landed on-chain transaction, reduced to what's needed to
// validate a SOL transfer against a Payment Intent.
type Transaction struct {
	// Signature identifies the transaction, for later commitment lookups.
	Signature string
	// Recipient is the destination account of the transfer.
	Recipient string
	// Lamports is the amount transferred, in lamports.
	Lamports uint64
	// Mint is the SPL token mint transferred, or empty for native SOL.
	// Phase 0 only validates native SOL transfers (empty Mint).
	Mint string
	// BlockTime is the chain (block producer) time at which this
	// transaction landed — never the watcher's wall clock. Per ADR 0002
	// and CONTEXT.md's "Deadline" entry, this is what a transfer's
	// eligibility is judged against, not when catraca happened to observe
	// it.
	BlockTime time.Time
}

// ChainReader is the production-shaped read interface the watcher needs
// from the chain. Implementations talk to a real Solana RPC endpoint (or,
// in tests, an in-process adapter) — this interface itself carries no
// RPC-client or test-harness types.
type ChainReader interface {
	// FindTransactionsByReference returns landed transactions whose account
	// list includes reference, per the Solana Pay reference convention (see
	// CONTEXT.md's "Reference" entry).
	FindTransactionsByReference(ctx context.Context, reference string) ([]Transaction, error)
	// GetCommitment returns the current commitment/finality status of the
	// transaction identified by signature.
	GetCommitment(ctx context.Context, signature string) (Commitment, error)
	// CurrentBlockTime returns the chain's current block time — the same
	// clock a transaction's BlockTime is drawn from. Expiry (ADR 0002)
	// must be judged against this, never time.Now(): a watcher that was
	// down for an hour must not treat that downtime as elapsed chain
	// time, and a slow-to-finalize transaction that landed before the
	// deadline must never be expired out from under it.
	CurrentBlockTime(ctx context.Context) (time.Time, error)
}
