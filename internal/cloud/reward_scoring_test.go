package cloud

import "testing"

func TestRewardScoreRequiresQualifiedEvidence(t *testing.T) {
	p := DefaultRewardEnginePolicy()
	evidence := RewardScoreInput{Views: 10, WatchBPS: 10000, RetentionBPS: 5000, OriginalityBPS: 10000, TrustBPS: 10000}
	score, weight, err := ScoreRewardEvidence(evidence, p)
	if err != nil || score != 8500 || weight != 85000 {
		t.Fatalf("%d %d %v", score, weight, err)
	}
	evidence.Views = 0
	if score, weight, err = ScoreRewardEvidence(evidence, p); err != nil || score != 0 || weight != 0 {
		t.Fatalf("zero evidence %d %d %v", score, weight, err)
	}
	evidence.Views = 10
	evidence.RetentionBPS = 10001
	if _, _, err = ScoreRewardEvidence(evidence, p); err == nil {
		t.Fatal("invalid factor accepted")
	}
}
