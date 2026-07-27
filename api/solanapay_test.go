package api

import "testing"

// From the Solana Pay spec example:
// solana:mvines9iiHiQTysrwkJjGf2gb9Ex9jXJX8ns3qwf2kN?amount=1&label=Michael&message=Thanks%20for%20all%20the%20fish&memo=OrderId12345
func TestTransferRequestURL_SpecExample(t *testing.T) {
	req := TransferRequest{
		Recipient: "mvines9iiHiQTysrwkJjGf2gb9Ex9jXJX8ns3qwf2kN",
		Amount:    "1",
		Label:     "Michael",
		Message:   "Thanks for all the fish",
	}

	got := req.URL()
	want := "solana:mvines9iiHiQTysrwkJjGf2gb9Ex9jXJX8ns3qwf2kN?amount=1&label=Michael&message=Thanks%20for%20all%20the%20fish"
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestTransferRequestURL_NativeSOLWithReference(t *testing.T) {
	req := TransferRequest{
		Recipient: "mvines9iiHiQTysrwkJjGf2gb9Ex9jXJX8ns3qwf2kN",
		Amount:    "1.5",
		Reference: "8pM1DN3RiT8vbom5u1sNryaNT1nyL8CTTW3b5PwWXRBH",
	}

	got := req.URL()
	want := "solana:mvines9iiHiQTysrwkJjGf2gb9Ex9jXJX8ns3qwf2kN?amount=1.5&reference=8pM1DN3RiT8vbom5u1sNryaNT1nyL8CTTW3b5PwWXRBH"
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestTransferRequestURL_SPLToken(t *testing.T) {
	req := TransferRequest{
		Recipient: "mvines9iiHiQTysrwkJjGf2gb9Ex9jXJX8ns3qwf2kN",
		Amount:    "10",
		Mint:      "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v",
		Reference: "8pM1DN3RiT8vbom5u1sNryaNT1nyL8CTTW3b5PwWXRBH",
	}

	got := req.URL()
	want := "solana:mvines9iiHiQTysrwkJjGf2gb9Ex9jXJX8ns3qwf2kN?amount=10&spl-token=EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v&reference=8pM1DN3RiT8vbom5u1sNryaNT1nyL8CTTW3b5PwWXRBH"
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}
