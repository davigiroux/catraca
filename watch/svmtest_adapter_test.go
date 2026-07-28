package watch

import (
	"context"
	"sync"
	"time"

	solana "github.com/gagliardetto/solana-go"

	"github.com/davigiroux/catraca/svmtest"
)

// svmChain is a test-only adapter implementing ChainReader on top of an
// svmtest.Harness. It never leaks litesvm-go types across the ChainReader
// boundary — production code will get a real RPC-backed implementation in
// a later ticket.
//
// litesvm-go's TxOutcome doesn't expose the submitted transaction's account
// keys/amounts for us to parse back out, so this adapter records what it
// sent at send time (it's the one building the transfer, via
// SendTransferWithReference) rather than reconstructing it from the
// outcome.
type svmChain struct {
	h *svmtest.Harness

	mu          sync.Mutex
	byReference map[string][]Transaction
	commitments map[string]Commitment
	finalizedAt map[string]uint64 // signature -> slot at which it finalizes
}

func newSVMChain(h *svmtest.Harness) *svmChain {
	return &svmChain{
		h:           h,
		byReference: map[string][]Transaction{},
		commitments: map[string]Commitment{},
		finalizedAt: map[string]uint64{},
	}
}

// SendTransferWithReference lands a SOL transfer via the harness and
// records it for later ChainReader lookups. finalizesAtSlot is the slot at
// or after which GetCommitment reports CommitmentFinalized for this
// transaction; before that it reports CommitmentConfirmed.
func (c *svmChain) SendTransferWithReference(
	payer solana.PrivateKey,
	recipient solana.PublicKey,
	reference solana.PublicKey,
	lamports uint64,
	finalizesAtSlot uint64,
) *Transaction {
	out := c.h.SendTransferWithReference(payer, recipient, reference, lamports)

	sig := out.Signature().String()
	tx := Transaction{
		Signature: sig,
		Recipient: recipient.String(),
		Lamports:  lamports,
		Mint:      "", // native SOL
		BlockTime: time.Unix(c.h.Clock().UnixTimestamp, 0).UTC(),
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	ref := reference.String()
	c.byReference[ref] = append(c.byReference[ref], tx)
	c.commitments[sig] = CommitmentConfirmed
	c.finalizedAt[sig] = finalizesAtSlot

	return &tx
}

func (c *svmChain) FindTransactionsByReference(ctx context.Context, reference string) ([]Transaction, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Transaction(nil), c.byReference[reference]...), nil
}

func (c *svmChain) GetCommitment(ctx context.Context, signature string) (Commitment, error) {
	c.mu.Lock()
	finalizesAt := c.finalizedAt[signature]
	c.mu.Unlock()

	slot := c.h.Clock().Slot
	if slot >= finalizesAt {
		return CommitmentFinalized, nil
	}
	return CommitmentConfirmed, nil
}

// CurrentBlockTime returns the harness's simulated chain clock, the same
// clock landed transactions' BlockTime is drawn from — never wall-clock
// time (ADR 0002).
func (c *svmChain) CurrentBlockTime(ctx context.Context) (time.Time, error) {
	return time.Unix(c.h.Clock().UnixTimestamp, 0).UTC(), nil
}
