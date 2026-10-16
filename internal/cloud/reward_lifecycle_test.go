package cloud

import (
	"context"
	"github.com/google/uuid"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRewardFundingConfirmationAndRelease(t *testing.T) {
	s, u := testPlatform(t)
	ctx := context.Background()
	_, err := s.DB.Exec(`UPDATE cloud_reward_policies SET active=true WHERE version='pilot-v1'`)
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Exec(`UPDATE cloud_reward_policies SET active=false WHERE version='pilot-v1'`)
	if err = s.EnrollRewards(ctx, u.ID, "pilot-v1"); err != nil {
		t.Fatal(err)
	}
	video := "reward-video-" + uuid.NewString()
	_, err = s.DB.Exec(`INSERT INTO cloud_videos(id,channel_id,title,status,visibility,audience,published_at)VALUES($1,$2,'Reward fixture','published','public','free',now()-interval '2 days')`, video, u.ChannelID)
	if err != nil {
		t.Fatal(err)
	}
	source := RewardFunding{ID: uuid.NewString(), Reference: uuid.NewString(), Kind: "generation", Cents: 101, EvidenceReference: "reconciliation:test", ProposedBy: "finance-a", ReconciledBy: "finance-b"}
	bad := source
	bad.ReconciledBy = bad.ProposedBy
	if err = s.RecordRewardFunding(ctx, bad); err == nil {
		t.Fatal("self reconciliation allowed")
	}
	if err = s.RecordRewardFunding(ctx, source); err != nil {
		t.Fatal(err)
	}
	if err = s.RecordRewardFunding(ctx, source); err != nil {
		t.Fatal("retry", err)
	}
	day := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
	draft := RewardBatchDraft{ID: uuid.NewString(), Day: day, SourceID: source.ID, PolicyVersion: "pilot-v1", EvidenceReference: "qualified:test", ProposedBy: "finance-a", BudgetCents: 101, Weights: map[string]int64{video: 1}}
	if err = s.DraftRewardBatch(ctx, draft); err != nil {
		t.Fatal(err)
	}
	var snapshot []byte
	if err = s.DB.QueryRow(`SELECT input_snapshot FROM cloud_reward_batches WHERE id=$1`, draft.ID).Scan(&snapshot); err != nil || !strings.Contains(string(snapshot), video) {
		t.Fatalf("missing immutable input %s %v", snapshot, err)
	}
	if _, err = s.DB.Exec(`UPDATE cloud_reward_batches SET input_snapshot='{}' WHERE id=$1`, draft.ID); err == nil {
		t.Fatal("mutable inputs")
	}
	if err = s.ConfirmRewardBatch(ctx, draft.ID, "finance-a"); err == nil {
		t.Fatal("self approval allowed")
	}
	if err = s.ConfirmRewardBatch(ctx, draft.ID, "finance-b"); err != nil {
		t.Fatal(err)
	}
	if err = s.ConfirmRewardBatch(ctx, draft.ID, "finance-b"); err != nil {
		t.Fatal("retry", err)
	}
	var confirmed time.Time
	s.DB.QueryRow(`SELECT confirmed_at FROM cloud_reward_batches WHERE id=$1`, draft.ID).Scan(&confirmed)
	for i := 0; i < 2; i++ {
		if err = s.ReleaseRewardVesting(ctx, u.ID, confirmed.Add(15*24*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	var amount int64
	if err = s.DB.QueryRow(`SELECT coalesce(sum(amount_cents),0) FROM cloud_earnings WHERE user_id=$1 AND reference LIKE 'vesting:%'`, u.ID).Scan(&amount); err != nil || amount != 50 {
		t.Fatalf("half release %d %v", amount, err)
	}
	if err = s.ReleaseRewardVesting(ctx, u.ID, confirmed.Add(30*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	s.DB.QueryRow(`SELECT coalesce(sum(amount_cents),0) FROM cloud_earnings WHERE user_id=$1 AND reference LIKE 'vesting:%'`, u.ID).Scan(&amount)
	if amount != 101 {
		t.Fatalf("full release %d", amount)
	}
	if _, err = s.DB.Exec(`UPDATE cloud_reward_allocations SET cents=102 WHERE batch_id=$1`, draft.ID); err == nil {
		t.Fatal("mutable principal")
	}
}

func TestRewardConcurrentFundingCannotOverspend(t *testing.T) {
	s, u := testPlatform(t)
	ctx := context.Background()
	s.DB.Exec(`UPDATE cloud_reward_policies SET active=true WHERE version='pilot-v1'`)
	defer s.DB.Exec(`UPDATE cloud_reward_policies SET active=false WHERE version='pilot-v1'`)
	if err := s.EnrollRewards(ctx, u.ID, "pilot-v1"); err != nil {
		t.Fatal(err)
	}
	video := "reward-video-" + uuid.NewString()
	if _, err := s.DB.Exec(`INSERT INTO cloud_videos(id,channel_id,title,status,visibility,audience,published_at)VALUES($1,$2,'fixture','published','public','free',now()-interval '3 days')`, video, u.ChannelID); err != nil {
		t.Fatal(err)
	}
	source := RewardFunding{ID: uuid.NewString(), Reference: uuid.NewString(), Kind: "generation", Cents: 100, EvidenceReference: "test", ProposedBy: "a", ReconciledBy: "b"}
	if err := s.RecordRewardFunding(ctx, source); err != nil {
		t.Fatal(err)
	}
	ids := []string{uuid.NewString(), uuid.NewString()}
	for i, id := range ids {
		if err := s.DraftRewardBatch(ctx, RewardBatchDraft{ID: id, Day: time.Now().UTC().AddDate(0, 0, -1-i).Format("2006-01-02"), SourceID: source.ID, PolicyVersion: "pilot-v1", EvidenceReference: "test", ProposedBy: "a", BudgetCents: 100, Weights: map[string]int64{video: 1}}); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, id := range ids {
		wg.Add(1)
		go func(id string) { defer wg.Done(); errs <- s.ConfirmRewardBatch(ctx, id, "b") }(id)
	}
	wg.Wait()
	close(errs)
	success := 0
	for err := range errs {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("successful confirmations %d", success)
	}
}

func TestRewardWindowDoesNotRestartAndBalancesStaySeparate(t *testing.T) {
	s, u := testPlatform(t)
	ctx := context.Background()
	id := "reward-window-" + uuid.NewString()
	first := time.Now().UTC().Add(-31 * 24 * time.Hour)
	if _, err := s.DB.Exec(`INSERT INTO cloud_videos(id,channel_id,title,status,visibility,audience,published_at)VALUES($1,$2,'Window fixture','published','public','free',$3)`, id, u.ChannelID, first); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`UPDATE cloud_videos SET status='draft',first_published_at=now() WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`UPDATE cloud_videos SET status='published',published_at=now() WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	var saved time.Time
	if err := s.DB.QueryRow(`SELECT first_published_at FROM cloud_videos WHERE id=$1`, id).Scan(&saved); err != nil || saved.Sub(first) > time.Millisecond {
		t.Fatalf("clock changed %v %v", saved, err)
	}
	videos, err := s.RewardVideos(ctx, u.ID, 25, 0)
	if err != nil || len(videos) != 1 || videos[0].Status != "ended" {
		t.Fatalf("eligibility %v %v", videos, err)
	}
	balance, err := s.RewardBalance(ctx, u.ID, time.Now())
	if err != nil || balance.Estimate != nil || balance.EstimateStatus != "insufficient_data" || balance.AvailableCents != 0 {
		t.Fatalf("balance %+v %v", balance, err)
	}
	if err := s.EnrollRewards(ctx, u.ID, "unapproved-policy"); err == nil {
		t.Fatal("invalid policy accepted")
	}
}

func TestRewardAvailabilityDeductsPendingRefunds(t *testing.T) {
	s, u := testPlatform(t)
	for _, entry := range []struct {
		amount int64
		status string
	}{{10000, "available"}, {-8000, "pending"}, {5000, "pending"}} {
		id := uuid.NewString()
		if _, err := s.DB.Exec(`INSERT INTO cloud_earnings(id,user_id,reference,amount_cents,currency,status,description)VALUES($1,$2,$1,$3,'usd',$4,'fixture')`, id, u.ID, entry.amount, entry.status); err != nil {
			t.Fatal(err)
		}
	}
	backLegacyTestEarnings(t, s, u.ID)
	balance, err := s.RewardBalance(context.Background(), u.ID, time.Now())
	if err != nil || balance.AvailableCents != 2000 || balance.PendingLegacyCents != 5000 {
		t.Fatalf("balance %+v err %v", balance, err)
	}
}
