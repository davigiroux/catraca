# catraca

Self-hosted Solana payments backend: a merchant creates a payment request, solgate watches the chain for the matching transfer, and fires a signed webhook when it's confirmed. Request + detect + notify — never custody, signing, swaps, or fiat.

## Language

**Payment Intent**:
A merchant's expectation of one specific on-chain transfer: recipient, amount, mint, reference, and deadline. One transaction settles one intent.
_Avoid_: payment request, invoice, order, charge

**Merchant**:
The owner of payment intents within a deployment: holds an API key, a recipient wallet, and a webhook endpoint. First-class from day one; created by the Operator (no self-serve signup in v1).
_Avoid_: tenant, account, user

**Operator**:
The person running a solgate deployment. May be the only Merchant (solo store) or manage many (SaaS platform).
_Avoid_: admin, host

**Reference**:
A unique pubkey issued per intent and included in the transfer's account list (Solana Pay convention), identifying which intent a transfer settles.
_Avoid_: reference key, memo, correlation id

**Pending**:
Intent state: created and being watched; no transfer carrying its reference seen yet.

**Detected**:
Intent state: a transfer carrying the reference has been seen on-chain but is not yet final.

**Confirmed**:
Intent state (terminal): the transfer is finalized and passed amount/mint validation. The only state that triggers a webhook.
_Avoid_: paid, settled, completed

**Deadline**:
The latest chain block time at which a settling transfer counts. Judged against the transfer's block time, never solgate's wall clock. Merchant-set per intent.
_Avoid_: timeout, TTL, expiry window

**Expired**:
Intent state (terminal): the chain has been observed past the deadline and no transfer with block time ≤ deadline carries the reference. A Detected intent is never Expired while its transfer is in flight.

**Mismatched**:
Intent state (terminal): a transfer carrying the reference finalized but failed validation (wrong amount or mint). Flagged to the merchant, never silently ignored.
_Avoid_: failed, rejected, partial

**Delivery**:
An attempt to notify a merchant endpoint of a confirmed (or mismatched/expired) intent via signed webhook. Delivery state is separate from intent state — an intent is Confirmed whether or not its webhook has landed.
_Avoid_: notification, event
