package watch

import "github.com/davigiroux/catraca/api"

// APIMerchants adapts api.Merchants (the operator-configured merchant
// lookup) to the watcher's MerchantLookup interface.
type APIMerchants struct {
	Merchants api.Merchants
}

// RequireConfirmedOnly implements MerchantLookup.
func (a APIMerchants) RequireConfirmedOnly(merchantID string) (confirmedOnly bool, ok bool) {
	m, ok := a.Merchants.ByID(merchantID)
	if !ok {
		return false, false
	}
	return m.RequireConfirmedOnly, true
}
