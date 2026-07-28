// Package notify delivers signed webhooks for Payment Intents that have
// reached a terminal state (Confirmed, Mismatched, Expired). Delivery is
// entirely downstream of intent state: this package only ever reads
// payment_intents, it never writes to them. See CONTEXT.md's "Delivery"
// entry.
//
// # Signature
//
// Each webhook body is signed HMAC-SHA256 with the merchant's WebhookSecret,
// Stripe-style, to prevent replay: the signed message is
//
//	<unix_timestamp> + "." + <json body>
//
// and both the timestamp and the resulting signature are sent in a single
// header:
//
//	X-Catraca-Signature: t=<unix_ts>,v1=<hex_hmac>
//
// A merchant verifies a delivery by re-computing the HMAC over
// "<t>.<body>" using their WebhookSecret and comparing it (constant-time)
// against v1.
package notify

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

// Sign computes the hex-encoded HMAC-SHA256 signature over
// "<timestamp>.<body>" using secret.
func Sign(secret string, timestamp int64, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(timestamp, 10)))
	mac.Write([]byte("."))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// SignatureHeader builds the value of the X-Catraca-Signature header for a
// delivery signed at timestamp.
func SignatureHeader(secret string, timestamp int64, body []byte) string {
	sig := Sign(secret, timestamp, body)
	return fmt.Sprintf("t=%d,v1=%s", timestamp, sig)
}

// SignatureHeaderName is the HTTP header carrying the timestamp and
// signature.
const SignatureHeaderName = "X-Catraca-Signature"

// VerifySignature parses a X-Catraca-Signature header value and reports
// whether it is a valid HMAC-SHA256 signature of body under secret. This is
// the verification a merchant's endpoint would perform; exported here so it
// can be exercised by tests without duplicating the wire format.
func VerifySignature(secret, header string, body []byte) bool {
	var timestamp int64
	var sig string
	for _, part := range strings.Split(header, ",") {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "t":
			ts, err := strconv.ParseInt(kv[1], 10, 64)
			if err != nil {
				return false
			}
			timestamp = ts
		case "v1":
			sig = kv[1]
		}
	}
	if sig == "" {
		return false
	}
	want := Sign(secret, timestamp, body)
	return hmac.Equal([]byte(want), []byte(sig))
}
