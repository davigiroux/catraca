# catraca

Self-hosted Solana payments backend: a merchant creates a payment request, catraca watches the chain for the matching transfer, and fires a signed webhook when it's confirmed.

Request + detect + notify — never custody, signing, swaps, or fiat. Funds go straight from the payer to the merchant's wallet; catraca only watches and reports.

## Matching a transfer to an intent

Each Payment Intent gets a fresh Reference pubkey, which the payer's transfer carries as an account per the [Solana Pay](https://docs.solanapay.com/spec) convention. `POST /payments` returns the `solana:` URL with it filled in. The watcher finds the settling transaction via signatures-for-address on the reference. The recipient wallet is shared across intents, so the reference — not the address or amount — is what identifies a payment.

## Lifecycle

```
pending ──> detected ──> confirmed
   │            └──────> mismatched
   └───────────────────> expired
```

Confirmed, mismatched, and expired are terminal; each produces a signed webhook. Intents promote at `finalized` commitment by default ([ADR-0001](docs/adr/0001-finalized-by-default.md)).

## Two decisions that are easy to get wrong

Both are in [ADR-0002](docs/adr/0002-one-tx-per-intent-chain-time-expiry.md).

**One transaction settles one intent.** A transfer carrying the reference that fails amount/mint validation lands the intent in terminal `mismatched`, never partially-paid.

**Expiry is judged by chain time.** The deadline is compared against the settling transaction's block time, never catraca's wall clock — otherwise payment outcomes would depend on the watcher's uptime. A `detected` intent is never expired while its transfer is in flight.

## Status

`catracad` serves the HTTP API and runs the watch/notify loop on SQLite. Merchants are configured by the Operator in `merchants.json`. The watcher decodes native SOL transfers only; SPL tokens are not yet supported. Vocabulary is in [CONTEXT.md](CONTEXT.md).

```
go run ./cmd/catracad -merchants merchants.json -rpc-endpoint <url>
```

## License

Apache-2.0. Usable as-is, including commercially.
