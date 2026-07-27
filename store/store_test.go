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

func mustCreateIntent(t *testing.T, s *Store, ref string) Intent {
	t.Helper()
	got, err := s.CreateIntent(context.Background(), NewIntentParams{
		MerchantID: "merchant-1",
		Recipient:  "mvines9iiHiQTysrwkJjGf2gb9Ex9jXJX8ns3qwf2kN",
		Amount:     "1.5",
		Reference:  ref,
		Deadline:   time.Now().Add(time.Hour).Truncate(time.Second),
	})
	if err != nil {
		t.Fatalf("CreateIntent: %v", err)
	}
	return got
}

func TestUpdateState_Success(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	intent := mustCreateIntent(t, s, "ref-1")

	if err := s.UpdateState(ctx, intent.ID, StatePending, StateDetected); err != nil {
		t.Fatalf("UpdateState: %v", err)
	}

	fetched, err := s.GetIntent(ctx, intent.ID)
	if err != nil {
		t.Fatalf("GetIntent: %v", err)
	}
	if fetched.State != StateDetected {
		t.Fatalf("expected state Detected, got %q", fetched.State)
	}
}

func TestUpdateState_NoMatch(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	intent := mustCreateIntent(t, s, "ref-2")

	// Wrong `from` state: intent is Pending, not Detected.
	err := s.UpdateState(ctx, intent.ID, StateDetected, StateConfirmed)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for mismatched from-state, got %v", err)
	}

	// Unknown ID.
	err = s.UpdateState(ctx, "does-not-exist", StatePending, StateDetected)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for unknown id, got %v", err)
	}

	fetched, err := s.GetIntent(ctx, intent.ID)
	if err != nil {
		t.Fatalf("GetIntent: %v", err)
	}
	if fetched.State != StatePending {
		t.Fatalf("expected state to remain Pending, got %q", fetched.State)
	}
}

func TestListByState(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	pending1 := mustCreateIntent(t, s, "ref-a")
	pending2 := mustCreateIntent(t, s, "ref-b")
	detected := mustCreateIntent(t, s, "ref-c")
	if err := s.UpdateState(ctx, detected.ID, StatePending, StateDetected); err != nil {
		t.Fatalf("UpdateState: %v", err)
	}

	pendings, err := s.ListByState(ctx, StatePending)
	if err != nil {
		t.Fatalf("ListByState: %v", err)
	}
	if len(pendings) != 2 {
		t.Fatalf("expected 2 pending intents, got %d", len(pendings))
	}
	ids := map[string]bool{pendings[0].ID: true, pendings[1].ID: true}
	if !ids[pending1.ID] || !ids[pending2.ID] {
		t.Fatalf("expected pending1/pending2 in result, got %+v", pendings)
	}

	detecteds, err := s.ListByState(ctx, StateDetected)
	if err != nil {
		t.Fatalf("ListByState: %v", err)
	}
	if len(detecteds) != 1 || detecteds[0].ID != detected.ID {
		t.Fatalf("expected only detected intent, got %+v", detecteds)
	}

	empty, err := s.ListByState(ctx, StateConfirmed)
	if err != nil {
		t.Fatalf("ListByState: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected no confirmed intents, got %+v", empty)
	}
}
