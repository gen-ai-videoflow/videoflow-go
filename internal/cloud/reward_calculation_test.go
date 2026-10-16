package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"testing"
	"time"
)

func TestRewardCalculationFundedEvidenceLifecycle(t *testing.T) {
	s, u := revenueTestStore(t)
	ctx := context.Background()
	p := DefaultRewardEnginePolicy()
	p.Enabled = true
	p.EstimatesEnabled = true
	p.ConfirmationsEnabled = true
	p.FundingBPS = 6000
	p.DailyCapCents = 101
	p.MinimumQualifiedViews = 1
	version := "fixture-" + uuid.NewString()
	raw, _ := json.Marshal(p)
	_, e := s.DB.Exec(`INSERT INTO cloud_reward_policies(version,terms_version,active,window_seconds,vesting_seconds,created_by,created_at)VALUES($1,$1,true,2592000,2592000,'fixture',now()-interval '4 days')`, version)
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.DB.Exec(`INSERT INTO cloud_reward_engine_policy(id,revision,policy_version,policy,updated_by)VALUES(true,1,$1,$2,'fixture')`, version, raw)
	if e != nil {
		t.Fatal(e)
	}
	current, _ := s.RewardEnginePolicy(ctx)
	if e = s.EnrollRewards(ctx, u.ID, current.PolicyVersion); e != nil {
		t.Fatal(e)
	}
	if _, e = s.DB.Exec(`UPDATE cloud_reward_enrollments SET accepted_at=now()-interval '3 days' WHERE user_id=$1 AND policy_version=$2`, u.ID, current.PolicyVersion); e != nil {
		t.Fatal(e)
	}
	video := uuid.NewString()
	_, e = s.DB.Exec(`INSERT INTO cloud_videos(id,channel_id,title,status,visibility,audience,published_at)VALUES($1,$2,'Calculation fixture','published','public','free',now()-interval '2 days')`, video, u.ChannelID)
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.DB.Exec(`INSERT INTO cloud_reward_quality_reviews(video_id,originality_bps,trust_bps,eligible,reason,actor)VALUES($1,10000,10000,true,'verified test originality','reviewer')`, video)
	if e != nil {
		t.Fatal(e)
	}
	viewer, e := s.Register(ctx, fmt.Sprintf("viewer-%s@example.com", uuid.NewString()), "Test-password-123!", "Viewer", "viewer"+uuid.NewString()[:8])
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.DB.Exec(`UPDATE cloud_users SET verified=true WHERE id=$1`, viewer.ID); e != nil {
		t.Fatal(e)
	}
	session := uuid.NewString()
	_, e = s.DB.Exec(`INSERT INTO cloud_watch_sessions(id,video_id,user_id,viewer_hash,traffic_source)VALUES($1,$2,$3,$3,'organic')`, session, video, viewer.ID)
	if e != nil {
		t.Fatal(e)
	}
	day := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
	_, e = s.DB.Exec(`INSERT INTO cloud_watch_events(id,user_id,video_id,seconds,session_id,created_at)VALUES($1,$2,$3,10,$4,$5::date+interval '1 hour')`, uuid.NewString(), viewer.ID, video, session, day)
	if e != nil {
		t.Fatal(e)
	}
	// An excluded event and a creator's own activity must never increase weight.
	event := uuid.NewString()
	_, e = s.DB.Exec(`INSERT INTO cloud_watch_events(id,user_id,video_id,seconds,session_id,created_at)VALUES($1,$2,$3,15,$4,$5::date+interval '2 hours')`, event, u.ID, video, session, day)
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.DB.Exec(`INSERT INTO cloud_watch_exclusions(event_id,actor,reason)VALUES($1,'fixture','invalid traffic')`, event)
	if e != nil {
		t.Fatal(e)
	}
	// Historical depleted lots must not hide newer available funding behind LIMIT.
	if _, e = s.DB.Exec(`INSERT INTO cloud_reward_sources(id,reference,kind,cents,reserved_cents,evidence_reference,proposed_by,reconciled_by,reconciled_at) SELECT 'depleted-'||g,'depleted-'||g,'generation',1,1,'fixture','a','b',now()-interval '10 days' FROM generate_series(1,101) g`); e != nil {
		t.Fatal(e)
	}
	f := RewardFunding{ID: uuid.NewString(), Reference: uuid.NewString(), Kind: "generation", Cents: 101, EvidenceReference: "actual:test", ProposedBy: "finance-a", ReconciledBy: "finance-b"}
	if e = s.RecordRewardFunding(ctx, f); e != nil {
		t.Fatal(e)
	}
	shadow, e := s.QueueRewardCalculation(ctx, day, true, "scheduler", "shadow validation")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.ProcessRewardCalculation(ctx); e != nil {
		t.Fatal(e)
	}
	var count int
	if e = s.DB.QueryRow(`SELECT count(*) FROM cloud_reward_batches WHERE evidence_reference=$1`, "calculation:"+shadow.RunID).Scan(&count); e != nil || count != 0 {
		t.Fatalf("shadow created money: %d %v", count, e)
	}
	run, e := s.QueueRewardCalculation(ctx, day, false, "scheduler", "funded daily estimate")
	if e != nil {
		t.Fatal(e)
	}
	again, e := s.QueueRewardCalculation(ctx, day, false, "scheduler", "retry")
	if e != nil || again.RunID != run.RunID {
		t.Fatalf("queue dedup: %+v %v", again, e)
	}
	if _, e = s.ProcessRewardCalculation(ctx); e != nil {
		t.Fatal(e)
	}
	var views int64
	if e = s.DB.QueryRow(`SELECT views FROM cloud_reward_evidence_snapshots WHERE run_id=$1 AND video_id=$2`, run.RunID, video).Scan(&views); e != nil || views != 1 {
		t.Fatalf("evidence %d %v", views, e)
	}
	balance, e := s.RewardBalance(ctx, u.ID, time.Now())
	if e != nil || balance.Estimate == nil || *balance.Estimate != 101 || balance.ConfirmedCents != 0 {
		t.Fatalf("provisional: %+v %v", balance, e)
	}
	var batch string
	if e = s.DB.QueryRow(`SELECT id FROM cloud_reward_batches WHERE evidence_reference=$1`, "calculation:"+run.RunID).Scan(&batch); e != nil {
		t.Fatal(e)
	}
	if e = s.ConfirmRewardBatch(ctx, batch, "finance-independent"); e != nil {
		t.Fatal(e)
	}
	balance, e = s.RewardBalance(ctx, u.ID, time.Now())
	if e != nil || balance.Estimate != nil || balance.ConfirmedCents != 101 {
		t.Fatalf("confirmed: %+v %v", balance, e)
	}
	second := RewardFunding{ID: uuid.NewString(), Reference: uuid.NewString(), Kind: "generation", Cents: 101, EvidenceReference: "independent fixture", ProposedBy: "a", ReconciledBy: "b"}
	if e = s.RecordRewardFunding(ctx, second); e != nil {
		t.Fatal(e)
	}
	manual := RewardBatchDraft{ID: uuid.NewString(), Day: day, SourceID: second.ID, PolicyVersion: current.PolicyVersion, EvidenceReference: "reviewed:duplicate-slice", ProposedBy: "a", BudgetCents: 101, Weights: map[string]int64{video: 1}}
	if e = s.DraftRewardBatch(ctx, manual); e != nil {
		t.Fatal(e)
	}
	if e = s.ConfirmRewardBatch(ctx, manual.ID, "b"); e != ErrConflict {
		t.Fatalf("duplicate day confirmation: %v", e)
	}
	// A later policy with a larger cap cannot reward the same historical slice again.
	p.DailyCapCents = 202
	policyID, e := s.ProposeRewardEnginePolicy(ctx, p, current.Revision, "new prospective cap", "finance-a")
	if e != nil {
		t.Fatal(e)
	}
	if e = s.ApproveRewardEnginePolicy(ctx, policyID, "finance-b"); e != nil {
		t.Fatal(e)
	}
	next, e := s.QueueRewardCalculation(ctx, day, false, "scheduler", "do not reuse old evidence")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.ProcessRewardCalculation(ctx); e != nil {
		t.Fatal(e)
	}
	var estimated int64
	if e = s.DB.QueryRow(`SELECT estimated_cents FROM cloud_reward_calculation_runs WHERE id=$1`, next.RunID).Scan(&estimated); e != nil || estimated != 0 {
		t.Fatalf("retroactive duplicate %d %v", estimated, e)
	}
	if processed, e := s.ProcessRewardCalculation(ctx); e != nil || processed {
		t.Fatalf("repeat processed %v %v", processed, e)
	}
}
