package cloud

import (
	"context"
	"github.com/google/uuid"
	"sync"
	"testing"
	"time"
)

func correctionFixture(t *testing.T) (*Store, User, string, string) {
	t.Helper()
	s, u := testPlatform(t)
	ctx := context.Background()
	// Other engine tests intentionally leave a paused current policy. This
	// fixture explicitly enables new confirmations and restores that exact state.
	var previousPolicy []byte
	if e := s.DB.QueryRow(`SELECT policy FROM cloud_reward_engine_policy WHERE id=true`).Scan(&previousPolicy); e == nil {
		if _, e = s.DB.Exec(`UPDATE cloud_reward_engine_policy SET policy=jsonb_set(jsonb_set(policy,'{enabled}','true'),'{confirmations_enabled}','true') WHERE id=true`); e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() { s.DB.Exec(`UPDATE cloud_reward_engine_policy SET policy=$1 WHERE id=true`, previousPolicy) })
	}
	if _, e := s.DB.Exec(`UPDATE cloud_reward_policies SET active=true WHERE version='pilot-v1'`); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.DB.Exec(`UPDATE cloud_reward_policies SET active=false WHERE version='pilot-v1'`) })
	if e := s.EnrollRewards(ctx, u.ID, "pilot-v1"); e != nil {
		t.Fatal(e)
	}
	video := uuid.NewString()
	if _, e := s.DB.Exec(`INSERT INTO cloud_videos(id,channel_id,title,status,visibility,audience,published_at)VALUES($1,$2,'Original','published','public','free',now()-interval '2 days')`, video, u.ChannelID); e != nil {
		t.Fatal(e)
	}
	source := func() string {
		f := RewardFunding{ID: uuid.NewString(), Reference: uuid.NewString(), Kind: "generation", Cents: 100, EvidenceReference: "settled:correction-fixture", ProposedBy: "a", ReconciledBy: "b"}
		if e := s.RecordRewardFunding(ctx, f); e != nil {
			t.Fatal(e)
		}
		return f.ID
	}
	original, fresh := source(), source()
	batch := uuid.NewString()
	if e := s.DraftRewardBatch(ctx, RewardBatchDraft{ID: batch, Day: time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02"), SourceID: original, PolicyVersion: "pilot-v1", EvidenceReference: "qualified:original", ProposedBy: "a", BudgetCents: 100, Weights: map[string]int64{video: 1}}); e != nil {
		t.Fatal(e)
	}
	if e := s.ConfirmRewardBatch(ctx, batch, "b"); e != nil {
		t.Fatal(e)
	}
	// Late evidence is permitted after original content ends eligibility; no
	// original batch day/publication timestamp is rewritten to sneak it through.
	if _, e := s.DB.Exec(`UPDATE cloud_videos SET visibility='private' WHERE id=$1`, video); e != nil {
		t.Fatal(e)
	}
	return s, u, batch, fresh
}
func TestCorrectionPositiveFundingMaturityAndImmutableHistory(t *testing.T) {
	s, u, batch, fresh := correctionFixture(t)
	ctx := context.Background()
	c := RewardCorrection{ID: uuid.NewString(), OriginalBatchID: batch, UserID: u.ID, Cents: 51, SourceID: fresh, Reason: "Accepted delayed original evidence", EvidenceReference: "review:late-1"}
	if e := s.ProposeRewardCorrection(ctx, c, "a"); e != nil {
		t.Fatal(e)
	}
	if e := s.ProposeRewardCorrection(ctx, c, "a"); e != nil {
		t.Fatal(e)
	}
	if e := s.ApproveRewardCorrection(ctx, c.ID, "a", "self"); e == nil {
		t.Fatal("self approval")
	}
	if e := s.ApproveRewardCorrection(ctx, c.ID, "b", "independently accepted"); e != nil {
		t.Fatal(e)
	}
	if e := s.ApproveRewardCorrection(ctx, c.ID, "b", "retry"); e != nil {
		t.Fatal(e)
	}
	var at time.Time
	if e := s.DB.QueryRow(`SELECT approved_at FROM cloud_reward_corrections WHERE id=$1`, c.ID).Scan(&at); e != nil {
		t.Fatal(e)
	}
	var reserved int64
	s.DB.QueryRow(`SELECT reserved_cents FROM cloud_reward_sources WHERE id=$1`, fresh).Scan(&reserved)
	if reserved != 51 {
		t.Fatal("unfunded correction", reserved)
	}
	before, e := s.RewardBalance(ctx, u.ID, at)
	if e != nil {
		t.Fatal(e)
	}
	if before.ConfirmedCents != 151 {
		t.Fatal("summary omitted correction", before)
	}
	if e = s.ReleaseRewardVesting(ctx, u.ID, at.Add(15*24*time.Hour)); e != nil {
		t.Fatal(e)
	}
	var released int64
	s.DB.QueryRow(`SELECT released_cents FROM cloud_reward_correction_releases WHERE correction_id=$1`, c.ID).Scan(&released)
	if released != 25 {
		t.Fatal("half maturity", released)
	}
	if e = s.ReleaseRewardVesting(ctx, u.ID, at.Add(30*24*time.Hour)); e != nil {
		t.Fatal(e)
	}
	if e = s.ReleaseRewardVesting(ctx, u.ID, at.Add(30*24*time.Hour)); e != nil {
		t.Fatal(e)
	}
	balance, e := s.RewardBalance(ctx, u.ID, at.Add(30*24*time.Hour))
	if e != nil {
		t.Fatal(e)
	}
	if balance.AvailableCents != 151 || balance.UnvestedCents != 0 {
		t.Fatalf("correction double count %+v", balance)
	}
	var principal int64
	s.DB.QueryRow(`SELECT sum(cents) FROM cloud_reward_allocations WHERE batch_id=$1`, batch).Scan(&principal)
	if principal != 100 {
		t.Fatal("rewrote principal", principal)
	}
	if _, e = s.DB.Exec(`UPDATE cloud_reward_corrections SET cents=52 WHERE id=$1`, c.ID); e == nil {
		t.Fatal("mutable correction")
	}
	if _, e = s.DB.Exec(`UPDATE cloud_earnings SET amount_cents=999 WHERE reference LIKE $1`, "correction:release:"+c.ID+":%"); e == nil {
		t.Fatal("mutable correction earnings")
	}
}
func TestCorrectionConcurrentFundingAndSourceIsolation(t *testing.T) {
	s, u, batch, fresh := correctionFixture(t)
	ctx := context.Background()
	ids := []string{uuid.NewString(), uuid.NewString()}
	for _, id := range ids {
		if e := s.ProposeRewardCorrection(ctx, RewardCorrection{ID: id, OriginalBatchID: batch, UserID: u.ID, Cents: 75, SourceID: fresh, Reason: "Late accepted evidence", EvidenceReference: "review:" + id}, "a"); e != nil {
			t.Fatal(e)
		}
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, id := range ids {
		wg.Add(1)
		go func(id string) { defer wg.Done(); errs <- s.ApproveRewardCorrection(ctx, id, "b", "independent") }(id)
	}
	wg.Wait()
	close(errs)
	success := 0
	for e := range errs {
		if e == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatal("overspent source", success)
	}
	var original string
	s.DB.QueryRow(`SELECT source_id FROM cloud_reward_batches WHERE id=$1`, batch).Scan(&original)
	c := RewardCorrection{ID: uuid.NewString(), OriginalBatchID: batch, UserID: u.ID, Cents: 1, SourceID: original, Reason: "More evidence", EvidenceReference: "review:source"}
	if e := s.ProposeRewardCorrection(ctx, c, "a"); e != nil {
		t.Fatal(e)
	}
	if e := s.ApproveRewardCorrection(ctx, c.ID, "b", "independent"); e == nil {
		t.Fatal("original source reused")
	}
	if _, e := s.DB.Exec(`INSERT INTO cloud_reward_revenue_source_holds(source_id,reason)VALUES($1,'shortfall')`, fresh); e != nil {
		t.Fatal(e)
	}
	c.ID = uuid.NewString()
	c.SourceID = fresh
	if e := s.ProposeRewardCorrection(ctx, c, "a"); e != nil {
		t.Fatal(e)
	}
	if e := s.ApproveRewardCorrection(ctx, c.ID, "b", "independent"); e == nil {
		t.Fatal("held source used")
	}
	campaignSource := RewardFunding{ID: uuid.NewString(), Reference: uuid.NewString(), Kind: "advertising", Cents: 100, EvidenceReference: "settled:campaign", ProposedBy: "a", ReconciledBy: "b"}
	if e := s.RecordRewardFunding(ctx, campaignSource); e != nil {
		t.Fatal(e)
	}
	var video string
	s.DB.QueryRow(`SELECT video_id FROM cloud_reward_allocations WHERE batch_id=$1 LIMIT 1`, batch).Scan(&video)
	if _, e := s.DB.Exec(`INSERT INTO cloud_sponsor_campaigns(id,title,brief,terms_version,video_id,source_id,budget_cents,starts_at,ends_at,created_by)VALUES($1,'Sponsor','Sponsor agreement','v1',$2,$3,100,now(),now()+interval '1 day',$4)`, uuid.NewString(), video, campaignSource.ID, u.ID); e != nil {
		t.Fatal(e)
	}
	c.ID = uuid.NewString()
	c.SourceID = campaignSource.ID
	if e := s.ProposeRewardCorrection(ctx, c, "a"); e != nil {
		t.Fatal(e)
	}
	if e := s.ApproveRewardCorrection(ctx, c.ID, "b", "independent"); e == nil {
		t.Fatal("campaign source used for cash correction")
	}
	var held bool
	s.DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM cloud_reward_holds WHERE user_id=$1)`, u.ID).Scan(&held)
	if !held {
		t.Fatal("reversed correction backing did not hold creator")
	}
}
func TestCorrectionNegativeDebtHoldAndBoundedClawback(t *testing.T) {
	s, u, batch, _ := correctionFixture(t)
	ctx := context.Background()
	c := RewardCorrection{ID: uuid.NewString(), OriginalBatchID: batch, UserID: u.ID, Cents: -60, Reason: "Invalid traffic review accepted", EvidenceReference: "review:invalid-traffic"}
	if e := s.ProposeRewardCorrection(ctx, c, "a"); e != nil {
		t.Fatal(e)
	}
	if e := s.ApproveRewardCorrection(ctx, c.ID, "b", "independent evidence review"); e != nil {
		t.Fatal(e)
	}
	if e := s.ApproveRewardCorrection(ctx, c.ID, "b", "retry"); e != nil {
		t.Fatal(e)
	}
	balance, e := s.RewardBalance(ctx, u.ID, time.Now().UTC())
	if e != nil {
		t.Fatal(e)
	}
	if !balance.Held || balance.AdjustmentCents != -60 || balance.AvailableCents != -60 {
		t.Fatalf("debt summary %+v", balance)
	}
	if _, e = s.DB.Exec(`DELETE FROM cloud_reward_holds WHERE user_id=$1`, u.ID); e == nil {
		t.Fatal("outstanding debt hold removed")
	}
	c.ID = uuid.NewString()
	c.Cents = -41
	if e = s.ProposeRewardCorrection(ctx, c, "a"); e != nil {
		t.Fatal(e)
	}
	if e = s.ApproveRewardCorrection(ctx, c.ID, "b", "independent"); e == nil {
		t.Fatal("clawback exceeds original obligation")
	}
	var confirmed time.Time
	s.DB.QueryRow(`SELECT confirmed_at FROM cloud_reward_batches WHERE id=$1`, batch).Scan(&confirmed)
	if e = s.ReleaseRewardVesting(ctx, u.ID, confirmed.Add(30*24*time.Hour)); e != nil {
		t.Fatal(e)
	}
	if _, e = s.DB.Exec(`DELETE FROM cloud_reward_holds WHERE user_id=$1`, u.ID); e != nil {
		t.Fatal("settled debt still blocks manual release", e)
	}
	balance, e = s.RewardBalance(ctx, u.ID, confirmed.Add(30*24*time.Hour))
	if e != nil || balance.AvailableCents != 40 || balance.ConfirmedCents != 100 {
		t.Fatalf("net corrected balance %+v %v", balance, e)
	}
	var count int
	s.DB.QueryRow(`SELECT count(*) FROM cloud_reward_notifications WHERE user_id=$1 AND dedup_key=$2`, u.ID, "correction:"+c.ID).Scan(&count)
	if count != 0 {
		t.Fatal("failed approval notified")
	}
}
