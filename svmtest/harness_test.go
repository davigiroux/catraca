package svmtest_test

import (
	"testing"
	"time"

	solana "github.com/gagliardetto/solana-go"

	"github.com/davigiroux/catraca/svmtest"
)

func TestFundedPayer_HasBalance(t *testing.T) {
	h := svmtest.New(t)

	payer := h.FundedPayer()

	if got := h.Balance(payer.PublicKey()); got != svmtest.DefaultAirdropLamports {
		t.Fatalf("payer balance = %d, want %d", got, svmtest.DefaultAirdropLamports)
	}
}

func TestFundedPayerWithLamports_CustomAmount(t *testing.T) {
	h := svmtest.New(t)

	payer := h.FundedPayerWithLamports(1_234_000)

	if got := h.Balance(payer.PublicKey()); got != 1_234_000 {
		t.Fatalf("payer balance = %d, want 1234000", got)
	}
}

func TestSendTransferWithReference_LandsAndMovesLamports(t *testing.T) {
	h := svmtest.New(t)

	payer := h.FundedPayer()
	recipient := solana.NewWallet().PublicKey()
	reference := solana.NewWallet().PublicKey()

	const amount = 1_000_000_000

	out := h.SendTransferWithReference(payer, recipient, reference, amount)

	if !out.IsOk() {
		t.Fatalf("transfer failed: %s (logs: %v)", out.Error(), out.Logs())
	}

	if got := h.Balance(recipient); got != amount {
		t.Fatalf("recipient balance = %d, want %d", got, amount)
	}

	wantPayer := uint64(svmtest.DefaultAirdropLamports) - amount - out.Fee()
	if got := h.Balance(payer.PublicKey()); got != wantPayer {
		t.Fatalf("payer balance = %d, want %d", got, wantPayer)
	}
}

func TestBuildTransferWithReference_IncludesReferenceAccount(t *testing.T) {
	h := svmtest.New(t)

	payer := h.FundedPayer()
	recipient := solana.NewWallet().PublicKey()
	reference := solana.NewWallet().PublicKey()

	txBytes := svmtest.BuildTransferWithReference(h, payer, recipient, reference, 1_000_000)

	decoded, err := solana.TransactionFromBytes(txBytes)
	if err != nil {
		t.Fatalf("TransactionFromBytes: %v", err)
	}

	if len(decoded.Message.Instructions) != 1 {
		t.Fatalf("instructions = %d, want 1", len(decoded.Message.Instructions))
	}

	found := false
	for _, acc := range decoded.Message.AccountKeys {
		if acc.Equals(reference) {
			found = true
		}
	}
	if !found {
		t.Fatalf("reference pubkey %s not present in transaction account keys", reference)
	}
}

func TestWarpToSlot_AdvancesClockSlot(t *testing.T) {
	h := svmtest.New(t)

	h.WarpToSlot(500)

	if got := h.Clock().Slot; got != 500 {
		t.Fatalf("clock slot = %d, want 500", got)
	}
}

func TestAdvanceClockBy_MovesUnixTimestampForward(t *testing.T) {
	h := svmtest.New(t)

	before := h.Clock().UnixTimestamp

	h.AdvanceClockBy(24 * time.Hour)

	after := h.Clock().UnixTimestamp
	if after-before != int64((24 * time.Hour).Seconds()) {
		t.Fatalf("unix timestamp advanced by %d, want %d", after-before, int64((24 * time.Hour).Seconds()))
	}
}

func TestSetClock_ReplacesClockWholesale(t *testing.T) {
	h := svmtest.New(t)

	c := h.Clock()
	c.Epoch = 7
	c.UnixTimestamp = 1_700_000_000
	h.SetClock(c)

	got := h.Clock()
	if got.Epoch != 7 || got.UnixTimestamp != 1_700_000_000 {
		t.Fatalf("clock after SetClock = %+v, want Epoch=7 UnixTimestamp=1700000000", got)
	}
}
