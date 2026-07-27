package api

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMerchants(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "merchants.json")
	body := `[
		{
			"id": "acme",
			"api_key": "sk_acme_test",
			"recipient": "mvines9iiHiQTysrwkJjGf2gb9Ex9jXJX8ns3qwf2kN",
			"webhook_url": "https://acme.example/webhooks/catraca",
			"webhook_secret": "whsec_acme"
		}
	]`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	merchants, err := LoadMerchants(path)
	if err != nil {
		t.Fatalf("LoadMerchants: %v", err)
	}

	m, ok := merchants.ByAPIKey("sk_acme_test")
	if !ok {
		t.Fatal("expected merchant to be found by API key")
	}
	if m.ID != "acme" {
		t.Fatalf("expected ID acme, got %q", m.ID)
	}
	if m.Recipient != "mvines9iiHiQTysrwkJjGf2gb9Ex9jXJX8ns3qwf2kN" {
		t.Fatalf("unexpected recipient %q", m.Recipient)
	}

	if _, ok := merchants.ByAPIKey("does-not-exist"); ok {
		t.Fatal("expected unknown API key lookup to fail")
	}
}
