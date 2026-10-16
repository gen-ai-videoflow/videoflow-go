package cloud

import "math/big"

type RewardScoreInput struct{ Views, WatchBPS, RetentionBPS, OriginalityBPS, TrustBPS int64 }

// ScoreRewardEvidence uses only accepted, normalized evidence. Purchasing volume
// and loss history are deliberately absent. Units are private policy basis points.
func ScoreRewardEvidence(e RewardScoreInput, p RewardEnginePolicy) (int64, int64, error) {
	if validateRewardEnginePolicy(p) != nil || e.Views < 0 {
		return 0, 0, ErrValidation
	}
	for _, n := range []int64{e.WatchBPS, e.RetentionBPS, e.OriginalityBPS, e.TrustBPS} {
		if n < 0 || n > 10000 {
			return 0, 0, ErrValidation
		}
	}
	if e.Views < p.MinimumQualifiedViews {
		return 0, 0, nil
	}
	w := p.QualityWeights
	score := (e.WatchBPS*w.Watch + e.RetentionBPS*w.Retention + e.OriginalityBPS*w.Originality + e.TrustBPS*w.Trust) / 10000
	weight := new(big.Int).Mul(big.NewInt(e.Views), big.NewInt(score))
	if !weight.IsInt64() {
		return 0, 0, ErrValidation
	}
	return score, weight.Int64(), nil
}
