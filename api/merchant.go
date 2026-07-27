package api

import (
	"encoding/json"
	"fmt"
	"os"
)

// Merchant is the owner of Payment Intents: holds an API key, a recipient
// wallet, and a webhook endpoint. Created by the Operator via config; no
// self-serve signup in v1.
type Merchant struct {
	ID            string `json:"id"`
	APIKey        string `json:"api_key"`
	Recipient     string `json:"recipient"`
	WebhookURL    string `json:"webhook_url"`
	WebhookSecret string `json:"webhook_secret"`
}

// Merchants is an operator-configured lookup of merchants by API key.
type Merchants struct {
	byAPIKey map[string]Merchant
}

// ByAPIKey returns the merchant owning the given API key, if any.
func (m Merchants) ByAPIKey(key string) (Merchant, bool) {
	merchant, ok := m.byAPIKey[key]
	return merchant, ok
}

// LoadMerchants reads the operator-configured merchant list from a JSON
// file: an array of Merchant objects.
func LoadMerchants(path string) (Merchants, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Merchants{}, fmt.Errorf("api: load merchants: %w", err)
	}

	var list []Merchant
	if err := json.Unmarshal(data, &list); err != nil {
		return Merchants{}, fmt.Errorf("api: load merchants: %w", err)
	}

	byAPIKey := make(map[string]Merchant, len(list))
	for _, m := range list {
		if m.APIKey == "" {
			return Merchants{}, fmt.Errorf("api: load merchants: merchant %q has no api_key", m.ID)
		}
		byAPIKey[m.APIKey] = m
	}

	return Merchants{byAPIKey: byAPIKey}, nil
}
