package notify

import "github.com/davigiroux/catraca/api"

// APIMerchants adapts api.Merchants (the operator-configured merchant
// lookup) to the Notifier's MerchantLookup interface. Mirrors
// watch.APIMerchants.
type APIMerchants struct {
	Merchants api.Merchants
}

// WebhookEndpoint implements MerchantLookup.
func (a APIMerchants) WebhookEndpoint(merchantID string) (Endpoint, bool) {
	m, ok := a.Merchants.ByID(merchantID)
	if !ok {
		return Endpoint{}, false
	}
	return Endpoint{URL: m.WebhookURL, Secret: m.WebhookSecret}, true
}
