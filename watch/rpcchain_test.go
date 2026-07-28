package watch

import (
	"testing"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

// These tests cover the pure decoding/mapping logic that doesn't require a
// live RPC connection: commitment string -> Commitment mapping, and
// unix-time conversion. End-to-end behavior against a real Solana RPC
// endpoint (finding a real transaction, decoding a real System Program
// transfer instruction) is not covered by automated tests here — see the
// RPCChainReader doc comment and the PR description for why.

func TestConfirmationStatusToCommitment(t *testing.T) {
	cases := []struct {
		name   string
		status rpc.ConfirmationStatusType
		want   Commitment
	}{
		{"finalized", rpc.ConfirmationStatusFinalized, CommitmentFinalized},
		{"confirmed", rpc.ConfirmationStatusConfirmed, CommitmentConfirmed},
		{"processed", rpc.ConfirmationStatusProcessed, CommitmentProcessed},
		{"unknown/empty", rpc.ConfirmationStatusType(""), CommitmentProcessed},
		{"garbage", rpc.ConfirmationStatusType("bogus"), CommitmentProcessed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := confirmationStatusToCommitment(tc.status)
			if got != tc.want {
				t.Errorf("confirmationStatusToCommitment(%q) = %v, want %v", tc.status, got, tc.want)
			}
		})
	}
}

func TestUnixTimeToTime(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		got := unixTimeToTime(nil)
		if !got.IsZero() {
			t.Errorf("unixTimeToTime(nil) = %v, want zero", got)
		}
	})

	t.Run("converts unix seconds to UTC", func(t *testing.T) {
		ut := solana.UnixTimeSeconds(1700000000)
		got := unixTimeToTime(&ut)
		want := time.Unix(1700000000, 0).UTC()
		if !got.Equal(want) {
			t.Errorf("unixTimeToTime(%d) = %v, want %v", ut, got, want)
		}
		if got.Location() != time.UTC {
			t.Errorf("unixTimeToTime result location = %v, want UTC", got.Location())
		}
	})
}
