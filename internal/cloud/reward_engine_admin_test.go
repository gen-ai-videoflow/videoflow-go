package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRewardQualityReviewRevisionUpdates(t *testing.T) {
	s, u := revenueTestStore(t)
	video := uuid.NewString()
	if _, e := s.DB.Exec(`INSERT INTO cloud_videos(id,channel_id,title,status,visibility,audience)VALUES($1,$2,'Review','published','public','free')`, video, u.ChannelID); e != nil {
		t.Fatal(e)
	}
	app := NewServer(s, Config{})
	defer app.Close()
	call := func(revision int, quality int) int {
		route := chi.NewRouteContext()
		route.URLParams.Add("id", video)
		body := fmt.Sprintf(`{"originality_bps":%d,"trust_bps":10000,"eligible":true,"revision":%d,"reason":"review actual evidence"}`, quality, revision)
		r := httptest.NewRequest("PUT", "/", strings.NewReader(body))
		r = r.WithContext(context.WithValue(context.WithValue(r.Context(), userContext{}, u), chi.RouteCtxKey, route))
		w := httptest.NewRecorder()
		app.adminRewardQuality(w, r)
		return w.Code
	}
	if code := call(0, 8000); code != 200 {
		t.Fatalf("create %d", code)
	}
	if code := call(1, 9000); code != 200 {
		t.Fatalf("update %d", code)
	}
	if code := call(1, 10000); code != 409 {
		t.Fatalf("stale %d", code)
	}
	var count int
	if e := s.DB.QueryRow(`SELECT count(*) FROM cloud_reward_quality_history WHERE video_id=$1`, video).Scan(&count); e != nil || count != 2 {
		t.Fatalf("history %d %v", count, e)
	}
}
func TestRewardShadowRunsWhileMoneyPaused(t *testing.T) {
	s, _ := revenueTestStore(t)
	ctx := context.Background()
	run, e := s.QueueRewardCalculation(ctx, time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02"), true, "reviewer", "readiness shadow")
	if e != nil {
		t.Fatal(e)
	}
	if e = s.RunRewardMaintenance(ctx, false, false); e != nil {
		t.Fatal(e)
	}
	var status string
	if e = s.DB.QueryRow(`SELECT status FROM cloud_reward_calculation_runs WHERE id=$1`, run.RunID).Scan(&status); e != nil || status != "succeeded" {
		t.Fatalf("%s %v", status, e)
	}
}
func TestRewardPayoutRunRetriesTransientFailureAndHonorsSmallThreshold(t *testing.T) {
	s, u := revenueTestStore(t)
	ctx := context.Background()
	p := DefaultRewardEnginePolicy()
	p.PayoutsEnabled = true
	p.PayoutThresholdCents = 100
	raw, _ := json.Marshal(p)
	if _, e := s.DB.Exec(`INSERT INTO cloud_reward_engine_policy(id,revision,policy_version,policy,updated_by)VALUES(true,1,'pilot-v1',$1,'fixture')`, raw); e != nil {
		t.Fatal(e)
	}
	if _, e := s.DB.Exec(`INSERT INTO cloud_payout_accounts(user_id,provider_id,country,charges_enabled,payouts_enabled)VALUES($1,$2,'US',true,true)`, u.ID, "acct_"+uuid.NewString()); e != nil {
		t.Fatal(e)
	}
	if _, e := s.DB.Exec(`INSERT INTO cloud_earnings(id,user_id,reference,amount_cents,currency,status,description)VALUES($1,$2,$1,250,'usd','available','fixture')`, uuid.NewString(), u.ID); e != nil {
		t.Fatal(e)
	}
	backLegacyTestEarnings(t, s, u.ID)
	if _, e := s.DB.Exec(`CREATE FUNCTION test_payout_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'transient fixture'; END $$; CREATE TRIGGER test_payout_failure BEFORE INSERT ON cloud_payout_batches FOR EACH ROW EXECUTE FUNCTION test_payout_failure()`); e != nil {
		t.Fatal(e)
	}
	backLegacyTestEarnings(t, s, u.ID)
	first, e := s.PrepareRewardPayoutRun(ctx, "finance", "monthly test")
	if e != nil || first.Failures != 1 || first.Batches != 0 {
		t.Fatalf("failure %+v %v", first, e)
	}
	if _, e = s.DB.Exec(`DROP TRIGGER test_payout_failure ON cloud_payout_batches`); e != nil {
		t.Fatal(e)
	}
	next, e := s.PrepareRewardPayoutRun(ctx, "finance", "retry review")
	if e != nil || next.ID != first.ID || next.Failures != 0 || next.Batches != 1 {
		t.Fatalf("retry %+v %v", next, e)
	}
	again, e := s.PrepareRewardPayoutRun(ctx, "finance", "idempotent retry")
	if e != nil || again.ID != next.ID || again.Batches != 1 {
		t.Fatalf("dedup %+v %v", again, e)
	}
}

func TestRewardCalculationRejectsReclaimedWorker(t *testing.T) {
	s, _ := revenueTestStore(t)
	ctx := context.Background()
	day := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
	run, e := s.QueueRewardCalculation(ctx, day, true, "reviewer", "fencing test")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.DB.Exec(`UPDATE cloud_reward_calculation_runs SET status='running',attempt=2,lease_until=now()+interval '5 minutes' WHERE id=$1`, run.RunID); e != nil {
		t.Fatal(e)
	}
	if e = s.calculateRewardRun(ctx, run.RunID, day, "pilot-v1", "reviewer", true, DefaultRewardEnginePolicy(), 1); e != ErrConflict {
		t.Fatalf("stale worker: %v", e)
	}
	var status string
	var attempt int
	if e = s.DB.QueryRow(`SELECT status,attempt FROM cloud_reward_calculation_runs WHERE id=$1`, run.RunID).Scan(&status, &attempt); e != nil || status != "running" || attempt != 2 {
		t.Fatalf("state %s %d %v", status, attempt, e)
	}
}
