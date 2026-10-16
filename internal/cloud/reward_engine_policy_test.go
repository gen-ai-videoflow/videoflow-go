package cloud

import (
	"context"
	"testing"
)

func TestRewardEnginePolicyValidation(t *testing.T) {
	p := DefaultRewardEnginePolicy()
	if err := validateRewardEnginePolicy(p); err != nil {
		t.Fatal(err)
	}
	p.QualityWeights.Watch++
	if validateRewardEnginePolicy(p) == nil {
		t.Fatal("unbalanced weights accepted")
	}
	p = DefaultRewardEnginePolicy()
	p.EarningWindowDays = 0
	if validateRewardEnginePolicy(p) == nil {
		t.Fatal("zero window")
	}
}
func TestRewardEngineIndependentPolicyApproval(t *testing.T) {
	s, _ := testPlatform(t)
	ctx := context.Background()
	record, err := s.RewardEnginePolicy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	p := record.Policy
	p.FundingBPS = 1000
	id, err := s.ProposeRewardEnginePolicy(ctx, p, record.Revision, "review policy", "finance-a")
	if err != nil {
		t.Fatal(err)
	}
	if s.ApproveRewardEnginePolicy(ctx, id, "finance-a") == nil {
		t.Fatal("self approval")
	}
	if err = s.ApproveRewardEnginePolicy(ctx, id, "finance-b"); err != nil {
		t.Fatal(err)
	}
	got, err := s.RewardEnginePolicy(ctx)
	if err != nil || got.Revision != record.Revision+1 || got.Policy.FundingBPS != 1000 {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err = s.ProposeRewardEnginePolicy(ctx, p, record.Revision, "stale edit", "finance-a"); err == nil {
		t.Fatal("stale revision accepted")
	}
	t.Cleanup(func() {
		r, e := s.RewardEnginePolicy(ctx)
		if e != nil {
			return
		}
		id, e := s.ProposeRewardEnginePolicy(ctx, DefaultRewardEnginePolicy(), r.Revision, "restore fixture defaults", "fixture-a")
		if e == nil {
			_ = s.ApproveRewardEnginePolicy(ctx, id, "fixture-b")
		}
	})
}
