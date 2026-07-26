package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/davigiroux/catraca/store"
)

func newTestHandler(t *testing.T) (http.Handler, Merchants) {
	t.Helper()
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("store.Close: %v", err)
		}
	})

	merchants := Merchants{byAPIKey: map[string]Merchant{
		"sk_acme": {
			ID:        "acme",
			APIKey:    "sk_acme",
			Recipient: "mvines9iiHiQTysrwkJjGf2gb9Ex9jXJX8ns3qwf2kN",
		},
		"sk_other": {
			ID:        "other",
			APIKey:    "sk_other",
			Recipient: "6ZS7hXaCJTKmZjJqoosHNe9hoTcqZ9dyewwzDkC5Mnzz",
		},
	}}

	return NewHandler(s, merchants), merchants
}

func doRequest(t *testing.T, h http.Handler, method, path, apiKey string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestPostPayments_RequiresAPIKey(t *testing.T) {
	h, _ := newTestHandler(t)
	rec := doRequest(t, h, http.MethodPost, "/payments", "", map[string]any{"amount": "1"})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPostPayments_RejectsUnknownAPIKey(t *testing.T) {
	h, _ := newTestHandler(t)
	rec := doRequest(t, h, http.MethodPost, "/payments", "sk_bogus", map[string]any{"amount": "1"})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPostPayments_RequiresAmountAndDeadline(t *testing.T) {
	h, _ := newTestHandler(t)
	rec := doRequest(t, h, http.MethodPost, "/payments", "sk_acme", map[string]any{})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPostPayments_CreatesPendingIntentWithSolanaPayURL(t *testing.T) {
	h, _ := newTestHandler(t)
	deadline := time.Now().Add(time.Hour).UTC().Truncate(time.Second)

	rec := doRequest(t, h, http.MethodPost, "/payments", "sk_acme", map[string]any{
		"amount":   "1.5",
		"deadline": deadline.Format(time.RFC3339),
		"label":    "Acme Store",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp paymentIntentResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if resp.ID == "" {
		t.Fatal("expected non-empty id")
	}
	if resp.State != "pending" {
		t.Fatalf("expected state pending, got %q", resp.State)
	}
	if resp.Recipient != "mvines9iiHiQTysrwkJjGf2gb9Ex9jXJX8ns3qwf2kN" {
		t.Fatalf("unexpected recipient %q", resp.Recipient)
	}
	if resp.Reference == "" {
		t.Fatal("expected non-empty reference")
	}
	if resp.Mint != "" {
		t.Fatalf("expected empty mint for native SOL, got %q", resp.Mint)
	}
	wantURL := "solana:mvines9iiHiQTysrwkJjGf2gb9Ex9jXJX8ns3qwf2kN?amount=1.5&reference=" + resp.Reference + "&label=Acme%20Store"
	if resp.URL != wantURL {
		t.Fatalf("got  %s\nwant %s", resp.URL, wantURL)
	}
}

func TestGetPayments_ReturnsCreatedIntent(t *testing.T) {
	h, _ := newTestHandler(t)
	deadline := time.Now().Add(time.Hour).UTC().Truncate(time.Second)

	createRec := doRequest(t, h, http.MethodPost, "/payments", "sk_acme", map[string]any{
		"amount":   "2",
		"deadline": deadline.Format(time.RFC3339),
	})
	var created paymentIntentResponse
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal create response: %v", err)
	}

	getRec := doRequest(t, h, http.MethodGet, "/payments/"+created.ID, "sk_acme", nil)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", getRec.Code, getRec.Body.String())
	}

	var fetched paymentIntentResponse
	if err := json.Unmarshal(getRec.Body.Bytes(), &fetched); err != nil {
		t.Fatalf("unmarshal get response: %v", err)
	}
	if fetched != created {
		t.Fatalf("fetched %+v does not match created %+v", fetched, created)
	}
}

func TestGetPayments_UnknownID_Returns404(t *testing.T) {
	h, _ := newTestHandler(t)
	rec := doRequest(t, h, http.MethodGet, "/payments/does-not-exist", "sk_acme", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetPayments_BelongingToAnotherMerchant_Returns404(t *testing.T) {
	h, _ := newTestHandler(t)
	deadline := time.Now().Add(time.Hour).UTC().Truncate(time.Second)

	createRec := doRequest(t, h, http.MethodPost, "/payments", "sk_acme", map[string]any{
		"amount":   "1",
		"deadline": deadline.Format(time.RFC3339),
	})
	var created paymentIntentResponse
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal create response: %v", err)
	}

	rec := doRequest(t, h, http.MethodGet, "/payments/"+created.ID, "sk_other", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for cross-merchant access, got %d: %s", rec.Code, rec.Body.String())
	}
}
