package cloud

import (
	"math/big"
	"time"
)

// VestedCents computes floor(principal * elapsed / duration), without overflow.
// It is a pure maturity calculation; confirmation must reserve the whole liability.
func VestedCents(principal int64, confirmedAt, at time.Time, duration time.Duration) (int64, error) {
	if principal < 0 || duration <= 0 {
		return 0, ErrValidation
	}
	if !at.After(confirmedAt) {
		return 0, nil
	}
	elapsed := at.Sub(confirmedAt)
	if elapsed >= duration {
		return principal, nil
	}
	n := new(big.Int).Mul(big.NewInt(principal), big.NewInt(int64(elapsed)))
	return n.Quo(n, big.NewInt(int64(duration))).Int64(), nil
}
