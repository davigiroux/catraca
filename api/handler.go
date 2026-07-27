// Package api provides the HTTP handlers and merchant authentication for
// creating and fetching Payment Intents.
package api

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mr-tron/base58"

	"github.com/davigiroux/catraca/store"
)

// NewHandler builds the /payments HTTP surface backed by s and
// authenticated against merchants.
func NewHandler(s *store.Store, merchants Merchants) http.Handler {
	h := &handler{store: s, merchants: merchants}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /payments", h.createPayment)
	mux.HandleFunc("GET /payments/{id}", h.getPayment)
	return mux
}

type handler struct {
	store     *store.Store
	merchants Merchants
}

// authenticate extracts the bearer API key from the request and resolves it
// to a Merchant. It writes a 401 response and returns ok=false if the key is
// missing or unknown.
func (h *handler) authenticate(w http.ResponseWriter, r *http.Request) (Merchant, bool) {
	authHeader := r.Header.Get("Authorization")
	key, hasPrefix := strings.CutPrefix(authHeader, "Bearer ")
	if authHeader == "" || !hasPrefix || key == "" {
		writeError(w, http.StatusUnauthorized, "missing or malformed Authorization header")
		return Merchant{}, false
	}

	merchant, ok := h.merchants.ByAPIKey(key)
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid API key")
		return Merchant{}, false
	}
	return merchant, true
}

type createPaymentRequest struct {
	Amount   string `json:"amount"`
	Mint     string `json:"mint,omitempty"`
	Deadline string `json:"deadline"`
	Label    string `json:"label,omitempty"`
	Message  string `json:"message,omitempty"`
}

type paymentIntentResponse struct {
	ID         string `json:"id"`
	MerchantID string `json:"merchant_id"`
	Recipient  string `json:"recipient"`
	Amount     string `json:"amount"`
	Mint       string `json:"mint,omitempty"`
	Reference  string `json:"reference"`
	Deadline   string `json:"deadline"`
	State      string `json:"state"`
	CreatedAt  string `json:"created_at"`
	URL        string `json:"url"`
}

func (h *handler) createPayment(w http.ResponseWriter, r *http.Request) {
	merchant, ok := h.authenticate(w, r)
	if !ok {
		return
	}

	var req createPaymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Amount == "" {
		writeError(w, http.StatusBadRequest, "amount is required")
		return
	}
	if _, err := strconv.ParseFloat(req.Amount, 64); err != nil {
		writeError(w, http.StatusBadRequest, "amount must be numeric")
		return
	}
	if req.Deadline == "" {
		writeError(w, http.StatusBadRequest, "deadline is required")
		return
	}
	deadline, err := time.Parse(time.RFC3339, req.Deadline)
	if err != nil {
		writeError(w, http.StatusBadRequest, "deadline must be RFC3339")
		return
	}

	reference, err := newReference()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate reference")
		return
	}

	intent, err := h.store.CreateIntent(r.Context(), store.NewIntentParams{
		MerchantID: merchant.ID,
		Recipient:  merchant.Recipient,
		Amount:     req.Amount,
		Mint:       req.Mint,
		Reference:  reference,
		Deadline:   deadline,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create payment intent")
		return
	}

	writeJSON(w, http.StatusCreated, toResponse(intent, req.Label, req.Message))
}

func (h *handler) getPayment(w http.ResponseWriter, r *http.Request) {
	merchant, ok := h.authenticate(w, r)
	if !ok {
		return
	}

	id := r.PathValue("id")
	intent, err := h.store.GetIntent(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "payment intent not found")
		return
	}

	// Merchants may only see their own intents; treat a mismatch the same
	// as not-found rather than leaking existence.
	if intent.MerchantID != merchant.ID {
		writeError(w, http.StatusNotFound, "payment intent not found")
		return
	}

	writeJSON(w, http.StatusOK, toResponse(intent, "", ""))
}

func toResponse(intent store.Intent, label, message string) paymentIntentResponse {
	url := TransferRequest{
		Recipient: intent.Recipient,
		Amount:    intent.Amount,
		Mint:      intent.Mint,
		Reference: intent.Reference,
		Label:     label,
		Message:   message,
	}.URL()

	return paymentIntentResponse{
		ID:         intent.ID,
		MerchantID: intent.MerchantID,
		Recipient:  intent.Recipient,
		Amount:     intent.Amount,
		Mint:       intent.Mint,
		Reference:  intent.Reference,
		Deadline:   intent.Deadline.Format(time.RFC3339),
		State:      string(intent.State),
		CreatedAt:  intent.CreatedAt.Format(time.RFC3339),
		URL:        url,
	}
}

// newReference generates a fresh Solana Pay reference: a random 32-byte
// value, base58-encoded like a public key, unique per intent.
func newReference() (string, error) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", fmt.Errorf("api: generate reference: %w", err)
	}
	return base58.Encode(pub), nil
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
