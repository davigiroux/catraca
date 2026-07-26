package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return s
}

func TestCreateAndGetIntent_NativeSOL(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	deadline := time.Now().Add(time.Hour).Truncate(time.Second)
	in := NewIntentParams{
		MerchantID: "merchant-1",
		Recipient:  "mvines9iiHiQTysrwkJjGf2gb9Ex9jXJX8ns3qwf2kN",
		Amount:     "1.5",
		Mint:       "",
		Reference:  "8pM1DN3RiT8vbom5u1sNryaNT1nyL8CTTW3b5PwWXRBH",
		Deadline:   deadline,
	}

	got, err := s.CreateIntent(ctx, in)
	if err != nil {
		t.Fatalf("CreateIntent: %v", err)
	}

	if got.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if got.State != StatePending {
		t.Fatalf("expected state Pending, got %q", got.State)
	}
	if got.Mint != "" {
		t.Fatalf("expected empty mint for native SOL, got %q", got.Mint)
	}

	fetched, err := s.GetIntent(ctx, got.ID)
	if err != nil {
		t.Fatalf("GetIntent: %v", err)
	}
	if fetched != got {
		t.Fatalf("fetched intent %+v does not match created %+v", fetched, got)
	}
}

func TestCreateAndGetIntent_SPLToken(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	in := NewIntentParams{
		MerchantID: "merchant-1",
		Recipient:  "mvines9iiHiQTysrwkJjGf2gb9Ex9jXJX8ns3qwf2kN",
		Amount:     "10",
		Mint:       "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v",
		Reference:  "8pM1DN3RiT8vbom5u1sNryaNT1nyL8CTTW3b5PwWXRBH",
		Deadline:   time.Now().Add(time.Hour).Truncate(time.Second),
	}

	got, err := s.CreateIntent(ctx, in)
	if err != nil {
		t.Fatalf("CreateIntent: %v", err)
	}
	if got.Mint != in.Mint {
		t.Fatalf("expected mint %q, got %q", in.Mint, got.Mint)
	}

	fetched, err := s.GetIntent(ctx, got.ID)
	if err != nil {
		t.Fatalf("GetIntent: %v", err)
	}
	if fetched.Mint != in.Mint {
		t.Fatalf("expected fetched mint %q, got %q", in.Mint, fetched.Mint)
	}
}

func TestGetIntent_NotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.GetIntent(ctx, "does-not-exist")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
