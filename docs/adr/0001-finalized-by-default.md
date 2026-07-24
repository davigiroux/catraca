# Payments confirm at `finalized` commitment by default

Most Solana apps promote at `confirmed` commitment (~1–2s); catraca defaults to `finalized` (~13s), with a per-merchant opt-down to `confirmed`. At `finalized` a rollback is impossible, so the system needs no reorg-handling machinery and the "nothing lost or double-delivered" guarantee is structural rather than probabilistic. We traded ~10s of default latency for eliminating the hardest edge-case class in the product; merchants selling reversible goods can opt down.
