package watch

import (
	"fmt"
	"math/big"
)

// lamportsPerSOL is the fixed-point scale of native SOL (9 decimals).
var lamportsPerSOL = big.NewRat(1_000_000_000, 1)

// lamportsFromSOL converts a decimal SOL amount string (as stored on
// Intent.Amount, e.g. "1.5") to an exact lamport count. It errors if the
// string isn't a valid decimal or doesn't represent a whole number of
// lamports.
func lamportsFromSOL(amount string) (uint64, error) {
	r, ok := new(big.Rat).SetString(amount)
	if !ok {
		return 0, fmt.Errorf("watch: invalid amount %q", amount)
	}
	r.Mul(r, lamportsPerSOL)
	if !r.IsInt() {
		return 0, fmt.Errorf("watch: amount %q is not a whole number of lamports", amount)
	}
	return r.Num().Uint64(), nil
}
