package svmtest

import (
	litesvm "github.com/LiteSVM/litesvm-go"
	solana "github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/system"
)

// BuildTransferWithReference builds a signed legacy transaction carrying a
// single System Program transfer from payer to recipient, with reference
// appended (read-only, non-signer) to the transfer instruction's account
// list per the Solana Pay convention — see CONTEXT.md's "Reference" entry.
// It does not submit the transaction; use SendTransferWithReference, or
// pass the returned bytes to the harness's SVM directly, to land it.
func BuildTransferWithReference(
	h *Harness,
	payer solana.PrivateKey,
	recipient solana.PublicKey,
	reference solana.PublicKey,
	lamports uint64,
) []byte {
	h.tb.Helper()

	blockhash, err := h.svm.LatestBlockhash()
	if err != nil {
		h.tb.Fatalf("svmtest: LatestBlockhash: %v", err)
	}

	builder := system.NewTransferInstructionBuilder().
		SetLamports(lamports).
		SetFundingAccount(payer.PublicKey()).
		SetRecipientAccount(recipient)
	builder.Append(solana.Meta(reference))
	ix := builder.Build()

	tx, err := solana.NewTransaction(
		[]solana.Instruction{ix},
		blockhash,
		solana.TransactionPayer(payer.PublicKey()),
	)
	if err != nil {
		h.tb.Fatalf("svmtest: NewTransaction: %v", err)
	}

	if _, err := tx.Sign(func(k solana.PublicKey) *solana.PrivateKey {
		if k.Equals(payer.PublicKey()) {
			return &payer
		}
		return nil
	}); err != nil {
		h.tb.Fatalf("svmtest: Sign: %v", err)
	}

	txBytes, err := tx.MarshalBinary()
	if err != nil {
		h.tb.Fatalf("svmtest: MarshalBinary: %v", err)
	}

	return txBytes
}

// SendTransferWithReference builds a SOL transfer carrying reference (per
// BuildTransferWithReference) and lands it in-process via litesvm-go. The
// caller owns the returned outcome and must call Close on it.
func (h *Harness) SendTransferWithReference(
	payer solana.PrivateKey,
	recipient solana.PublicKey,
	reference solana.PublicKey,
	lamports uint64,
) *litesvm.TxOutcome {
	h.tb.Helper()

	txBytes := BuildTransferWithReference(h, payer, recipient, reference, lamports)

	out, err := h.svm.SendLegacyTransaction(txBytes)
	if err != nil {
		h.tb.Fatalf("svmtest: SendLegacyTransaction: %v", err)
	}
	h.tb.Cleanup(out.Close)

	return out
}
