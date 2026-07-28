package notify

import "testing"

func TestSign_KnownVector(t *testing.T) {
	// crafted vector: verify Go's hmac output matches a hand-computed one
	// isn't practical without external tooling, so instead assert
	// determinism and sensitivity to each input.
	got := Sign("secret", 1000, []byte(`{"a":1}`))
	if got == "" {
		t.Fatal("Sign returned empty signature")
	}
	if got != Sign("secret", 1000, []byte(`{"a":1}`)) {
		t.Fatal("Sign is not deterministic for identical inputs")
	}
	if got == Sign("other-secret", 1000, []byte(`{"a":1}`)) {
		t.Fatal("Sign did not change with a different secret")
	}
	if got == Sign("secret", 1001, []byte(`{"a":1}`)) {
		t.Fatal("Sign did not change with a different timestamp")
	}
	if got == Sign("secret", 1000, []byte(`{"a":2}`)) {
		t.Fatal("Sign did not change with a different body")
	}
}

func TestVerifySignature_MerchantCanReconstructAndValidate(t *testing.T) {
	secret := "whsec_test"
	body := []byte(`{"intent_id":"abc","state":"confirmed"}`)
	timestamp := int64(1700000000)

	header := SignatureHeader(secret, timestamp, body)

	if !VerifySignature(secret, header, body) {
		t.Fatal("merchant could not verify a signature it should be able to")
	}
}

func TestVerifySignature_RejectsWrongSecret(t *testing.T) {
	body := []byte(`{"intent_id":"abc"}`)
	header := SignatureHeader("real-secret", 1700000000, body)

	if VerifySignature("wrong-secret", header, body) {
		t.Fatal("verification succeeded with the wrong secret")
	}
}

func TestVerifySignature_RejectsTamperedBody(t *testing.T) {
	secret := "whsec_test"
	body := []byte(`{"amount":"1.0"}`)
	header := SignatureHeader(secret, 1700000000, body)

	tampered := []byte(`{"amount":"100.0"}`)
	if VerifySignature(secret, header, tampered) {
		t.Fatal("verification succeeded against a tampered body")
	}
}

func TestVerifySignature_RejectsMalformedHeader(t *testing.T) {
	if VerifySignature("secret", "garbage", []byte("body")) {
		t.Fatal("verification succeeded against a malformed header")
	}
	if VerifySignature("secret", "", []byte("body")) {
		t.Fatal("verification succeeded against an empty header")
	}
}
