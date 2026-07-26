// Package svmtest is catraca's shared in-process SVM test harness. It wraps
// litesvm-go (an in-process, no-validator Solana VM) with the primitives
// other test suites need: a funded payer, a SOL-transfer-with-Reference
// transaction builder following the Solana Pay convention (a unique pubkey
// appended to the transfer instruction's account list — see CONTEXT.md's
// "Reference" entry), and time-travel helpers for exercising Deadline /
// expiry logic (ADR 0002) without waiting on wall-clock time.
package svmtest

import (
	"testing"

	litesvm "github.com/LiteSVM/litesvm-go"
	solana "github.com/gagliardetto/solana-go"
)

// DefaultAirdropLamports is the balance a FundedPayer receives unless the
// caller asks for a different amount.
const DefaultAirdropLamports = 10_000_000_000 // 10 SOL

// Harness owns a single in-process LiteSVM instance for the lifetime of a
// test. Create one with New; it registers its own cleanup, so callers never
// need to close it explicitly.
type Harness struct {
	tb  testing.TB
	svm *litesvm.LiteSVM
}

// New spins up a fresh LiteSVM instance and arranges for it to be closed
// when the test finishes.
func New(tb testing.TB) *Harness {
	tb.Helper()

	svm, err := litesvm.New()
	if err != nil {
		tb.Fatalf("svmtest: litesvm.New: %v", err)
	}
	tb.Cleanup(svm.Close)

	return &Harness{tb: tb, svm: svm}
}

// SVM returns the underlying litesvm-go instance for callers that need
// lower-level access than the harness exposes.
func (h *Harness) SVM() *litesvm.LiteSVM {
	return h.svm
}

// FundedPayer creates a new keypair and airdrops it DefaultAirdropLamports,
// leaving it ready to act as a transaction fee payer / transfer source.
func (h *Harness) FundedPayer() solana.PrivateKey {
	h.tb.Helper()
	return h.FundedPayerWithLamports(DefaultAirdropLamports)
}

// FundedPayerWithLamports creates a new keypair and airdrops it the given
// lamport amount.
func (h *Harness) FundedPayerWithLamports(lamports uint64) solana.PrivateKey {
	h.tb.Helper()

	priv, err := solana.NewRandomPrivateKey()
	if err != nil {
		h.tb.Fatalf("svmtest: NewRandomPrivateKey: %v", err)
	}

	if err := h.svm.Airdrop(priv.PublicKey(), lamports); err != nil {
		h.tb.Fatalf("svmtest: Airdrop: %v", err)
	}

	return priv
}

// Balance returns the lamport balance of the given account.
func (h *Harness) Balance(pubkey solana.PublicKey) uint64 {
	h.tb.Helper()

	lamports, _, err := h.svm.Balance(pubkey)
	if err != nil {
		h.tb.Fatalf("svmtest: Balance: %v", err)
	}
	return lamports
}
