package api

import "strings"

// TransferRequest builds a Solana Pay "transfer request" URL per the spec:
// https://docs.solanapay.com/spec
type TransferRequest struct {
	// Recipient is the base58-encoded public key that should receive the
	// transfer. Required.
	Recipient string
	// Amount is a non-negative integer or decimal string, in "user" units.
	Amount string
	// Mint is the base58-encoded SPL Token mint public key. Empty means a
	// native SOL transfer.
	Mint string
	// Reference is a base58-encoded 32-byte public key used to identify the
	// transaction that settles this request.
	Reference string
	// Label describes the source of the transfer request.
	Label string
	// Message describes the nature of the transfer request.
	Message string
}

// URL renders the transfer request as a "solana:" URL.
func (r TransferRequest) URL() string {
	var b strings.Builder
	b.WriteString("solana:")
	b.WriteString(r.Recipient)

	params := make([][2]string, 0, 5)
	if r.Amount != "" {
		params = append(params, [2]string{"amount", r.Amount})
	}
	if r.Mint != "" {
		params = append(params, [2]string{"spl-token", r.Mint})
	}
	if r.Reference != "" {
		params = append(params, [2]string{"reference", r.Reference})
	}
	if r.Label != "" {
		params = append(params, [2]string{"label", r.Label})
	}
	if r.Message != "" {
		params = append(params, [2]string{"message", r.Message})
	}

	for i, p := range params {
		if i == 0 {
			b.WriteByte('?')
		} else {
			b.WriteByte('&')
		}
		b.WriteString(p[0])
		b.WriteByte('=')
		b.WriteString(encodeURIComponent(p[1]))
	}

	return b.String()
}

// encodeURIComponent mimics JavaScript's encodeURIComponent, which the
// Solana Pay spec mandates for label/message/memo (and is a strict superset
// of what's needed for base58 recipient/mint/reference values, which never
// contain characters requiring escaping). Go's url.QueryEscape encodes
// spaces as "+" instead of "%20" and does not match, so build on it and
// patch the one difference.
func encodeURIComponent(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case strings.ContainsRune("-_.!~*'()", r):
			b.WriteRune(r)
		default:
			for _, c := range []byte(string(r)) {
				b.WriteString("%")
				const hex = "0123456789ABCDEF"
				b.WriteByte(hex[c>>4])
				b.WriteByte(hex[c&0xF])
			}
		}
	}
	return b.String()
}
