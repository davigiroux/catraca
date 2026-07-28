package watch

import (
	"context"
	"fmt"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/system"
	"github.com/gagliardetto/solana-go/rpc"
)

// RPCChainReader implements ChainReader against a real Solana JSON RPC
// endpoint. Phase 0/1 only decodes native SOL System Program transfers — SPL
// token transfers (non-empty Mint) are out of scope (see chainreader.go's
// Transaction.Mint doc).
//
// End-to-end behavior (a real devnet payment being found, commitment-tracked,
// and correctly time-stamped) can only be verified against a live RPC
// endpoint; that verification is a manual/follow-up step, not covered by the
// automated test suite. Automated tests here cover the pure decoding/mapping
// logic (commitment string mapping, unix-time conversion) using constructed
// fixtures.
type RPCChainReader struct {
	client *rpc.Client
}

// NewRPCChainReader returns an RPCChainReader talking to endpoint.
func NewRPCChainReader(endpoint string) *RPCChainReader {
	return &RPCChainReader{client: rpc.New(endpoint)}
}

// maxSupportedTransactionVersion is passed to GetTransaction so versioned
// transactions (v0) are not rejected by the RPC node.
var maxSupportedTransactionVersion = uint64(0)

// FindTransactionsByReference implements ChainReader. The reference pubkey
// is embedded as an account in the settling transfer's instruction per the
// Solana Pay convention (see CONTEXT.md's "Reference" entry), so
// signatures-for-address on it finds the settling transaction.
func (r *RPCChainReader) FindTransactionsByReference(ctx context.Context, reference string) ([]Transaction, error) {
	refKey, err := solana.PublicKeyFromBase58(reference)
	if err != nil {
		return nil, fmt.Errorf("watch: rpcchain: invalid reference %q: %w", reference, err)
	}

	sigs, err := r.client.GetSignaturesForAddress(ctx, refKey)
	if err != nil {
		return nil, fmt.Errorf("watch: rpcchain: get signatures for %q: %w", reference, err)
	}

	var out []Transaction
	for _, sigInfo := range sigs {
		if sigInfo.Err != nil {
			// Failed transaction: never a valid settlement.
			continue
		}

		txResult, err := r.client.GetTransaction(ctx, sigInfo.Signature, &rpc.GetTransactionOpts{
			Encoding:                       solana.EncodingBase64,
			MaxSupportedTransactionVersion: &maxSupportedTransactionVersion,
		})
		if err != nil {
			return nil, fmt.Errorf("watch: rpcchain: get transaction %s: %w", sigInfo.Signature, err)
		}
		if txResult == nil || txResult.Transaction == nil {
			continue
		}

		tx, err := txResult.Transaction.GetTransaction()
		if err != nil {
			return nil, fmt.Errorf("watch: rpcchain: decode transaction %s: %w", sigInfo.Signature, err)
		}

		transfer, ok := findSystemTransfer(tx)
		if !ok {
			continue
		}

		blockTime := unixTimeToTime(txResult.BlockTime)
		if blockTime.IsZero() {
			blockTime = unixTimeToTime(sigInfo.BlockTime)
		}

		out = append(out, Transaction{
			Signature: sigInfo.Signature.String(),
			Recipient: transfer.GetRecipientAccount().PublicKey.String(),
			Lamports:  *transfer.Lamports,
			Mint:      "", // native SOL only; SPL transfers are phase 2
			BlockTime: blockTime,
		})
	}
	return out, nil
}

// findSystemTransfer walks tx's instructions looking for a System Program
// transfer, returning the decoded instruction if found.
func findSystemTransfer(tx *solana.Transaction) (*system.Transfer, bool) {
	for _, ci := range tx.Message.Instructions {
		programID, err := tx.Message.Account(ci.ProgramIDIndex)
		if err != nil || !programID.Equals(system.ProgramID) {
			continue
		}

		accounts, err := ci.ResolveInstructionAccounts(&tx.Message)
		if err != nil {
			continue
		}

		inst, err := system.DecodeInstruction(accounts, ci.Data)
		if err != nil {
			continue
		}

		transfer, ok := inst.Impl.(*system.Transfer)
		if !ok || transfer.Lamports == nil {
			continue
		}
		return transfer, true
	}
	return nil, false
}

// GetCommitment implements ChainReader. A signature unknown to this RPC
// node (nil status) is reported as CommitmentProcessed, the least-final
// level — an unindexed-so-far transaction is not the same as a failed one.
func (r *RPCChainReader) GetCommitment(ctx context.Context, signature string) (Commitment, error) {
	sig, err := solana.SignatureFromBase58(signature)
	if err != nil {
		return CommitmentProcessed, fmt.Errorf("watch: rpcchain: invalid signature %q: %w", signature, err)
	}

	result, err := r.client.GetSignatureStatuses(ctx, true, sig)
	if err != nil {
		return CommitmentProcessed, fmt.Errorf("watch: rpcchain: get signature statuses %s: %w", signature, err)
	}
	if result == nil || len(result.Value) == 0 || result.Value[0] == nil {
		return CommitmentProcessed, nil
	}

	return confirmationStatusToCommitment(result.Value[0].ConfirmationStatus), nil
}

// confirmationStatusToCommitment maps an RPC ConfirmationStatusType to
// watch.Commitment. Unrecognized/empty statuses map to CommitmentProcessed,
// the safe (least final) default.
func confirmationStatusToCommitment(status rpc.ConfirmationStatusType) Commitment {
	switch status {
	case rpc.ConfirmationStatusFinalized:
		return CommitmentFinalized
	case rpc.ConfirmationStatusConfirmed:
		return CommitmentConfirmed
	default:
		return CommitmentProcessed
	}
}

// CurrentBlockTime implements ChainReader: the block time of the current
// finalized slot, used as the chain clock for expiry judgments (ADR 0002).
func (r *RPCChainReader) CurrentBlockTime(ctx context.Context) (time.Time, error) {
	slot, err := r.client.GetSlot(ctx, rpc.CommitmentFinalized)
	if err != nil {
		return time.Time{}, fmt.Errorf("watch: rpcchain: get slot: %w", err)
	}

	blockTime, err := r.client.GetBlockTime(ctx, slot)
	if err != nil {
		return time.Time{}, fmt.Errorf("watch: rpcchain: get block time for slot %d: %w", slot, err)
	}
	if blockTime == nil {
		return time.Time{}, fmt.Errorf("watch: rpcchain: no block time for slot %d", slot)
	}

	return time.Unix(int64(*blockTime), 0).UTC(), nil
}

// unixTimeToTime converts a possibly-nil *solana.UnixTimeSeconds to
// time.Time, returning the zero value if t is nil.
func unixTimeToTime(t *solana.UnixTimeSeconds) time.Time {
	if t == nil {
		return time.Time{}
	}
	return time.Unix(int64(*t), 0).UTC()
}
