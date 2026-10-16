package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func revenueTestStore(t *testing.T) (*Store, User) {
	t.Helper()
	dsn := os.Getenv("CLOUD_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set CLOUD_TEST_DATABASE_URL for PostgreSQL integration tests")
	}
	ctx := context.Background()
	base, err := OpenStore(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "revenue_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = base.DB.ExecContext(ctx, `CREATE SCHEMA `+schema); err != nil {
		base.Close()
		t.Fatal(err)
	}
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := parsed.Query()
	q.Set("search_path", schema)
	parsed.RawQuery = q.Encode()
	s, err := OpenStore(ctx, parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		s.Close()
		base.DB.ExecContext(context.Background(), `DROP SCHEMA `+schema+` CASCADE`)
		base.Close()
	})
	if err = s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	u, err := s.Register(ctx, schema+"@example.com", "Test-password-123!", "Revenue test", schema)
	if err != nil {
		t.Fatal(err)
	}
	return s, u
}

func revInt(n int64) *int64 { return &n }
func TestRewardRevenueEvidenceValidation(t *testing.T) {
	e := RewardRevenueEvidence{Kind: "cost", ResourceID: "job", Reason: "statement", EvidenceReference: "invoice"}
	if validRevenueEvidence(e) {
		t.Fatal("missing costs silently became zero")
	}
	e.ProviderCostCents = revInt(0)
	e.MediaCostCents = revInt(0)
	if !validRevenueEvidence(e) {
		t.Fatal("explicit zero cost rejected")
	}
	if revenuePortion(100, 1, 3)+revenuePortion(100, 2, 3)-revenuePortion(100, 1, 3) != 66 {
		t.Fatal("integer allocation")
	}
}
func TestRewardRevenueFIFOAndRefundSafety(t *testing.T) {
	s, u := revenueTestStore(t)
	ctx := context.Background()
	suffix := uuid.NewString()
	day := time.Now().UTC().AddDate(0, 0, -2).Format("2006-01-02")
	at, _ := time.Parse("2006-01-02", day)
	receiptAt := at.Add(-time.Hour)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	p := DefaultRewardEnginePolicy()
	p.FundingBPS = 5000
	p.ReserveBPS = 2000
	p.DailyCapCents = 100000
	raw, _ := json.Marshal(p)
	_, err := s.DB.ExecContext(ctx, `INSERT INTO cloud_reward_engine_policy(id,revision,policy_version,policy,updated_by)VALUES(true,1,'pilot-v1',$1,'test') ON CONFLICT(id)DO UPDATE SET policy=$1,revision=cloud_reward_engine_policy.revision+1`, raw)
	must(err)
	receipt := "purchase-" + suffix
	_, err = s.DB.ExecContext(ctx, `INSERT INTO cloud_purchases(id,workspace_id,pack_id,credits,price_cents,currency,status,stripe_session,idempotency_key,payment_intent)VALUES($1,$2,'test',100,1000,'usd','paid',$3,$1,$4)`, receipt, u.WorkspaceID, "cs_"+suffix, "pi_"+suffix)
	must(err)
	must(s.GrantCredits(ctx, u.WorkspaceID, "purchase:"+receipt, 100, "paid receipt"))
	_, err = s.DB.ExecContext(ctx, `UPDATE cloud_credit_entries SET created_at=$2 WHERE reference=$1`, "purchase:"+receipt, receiptAt)
	must(err)
	project, err := s.CreateProject(ctx, u, "Revenue", "Revenue")
	must(err)
	q, err := s.CreateQuote(ctx, u, QuoteInput{ProjectID: project.ID, Prompt: "A bird flying over the sea", Duration: 5, AspectRatio: "16:9"}, 10)
	must(err)
	j, err := s.SubmitJob(ctx, u, q.ID, "revenue-"+suffix)
	must(err)
	_, err = s.DB.ExecContext(ctx, `UPDATE cloud_jobs SET status='succeeded',credits=60,promotional_credits=10,updated_at=$2 WHERE id=$1`, j.ID, at.Add(time.Hour))
	must(err)
	result, err := s.ReconcileRewardRevenue(ctx, day, "finance-runner")
	must(err)
	if result.Status != "pending_reconciliation" {
		t.Fatalf("missing evidence funded: %+v", result)
	}
	evidence := RewardRevenueEvidence{Kind: "receipt", ResourceID: receipt, Provider: "stripe", GrossCents: revInt(1000), FeeCents: revInt(100), TaxCents: revInt(100), RefundCents: revInt(0), SettledAt: receiptAt.Format(time.RFC3339), EvidenceReference: "settled USD statement", Reason: "actual receipts"}
	id, err := s.ProposeRewardRevenueEvidence(ctx, evidence, "finance-a")
	must(err)
	if s.ApproveRewardRevenueEvidence(ctx, id, "finance-a", "self") == nil {
		t.Fatal("self approval accepted")
	}
	must(s.ApproveRewardRevenueEvidence(ctx, id, "finance-b", "independent statement review"))
	evidence = RewardRevenueEvidence{Kind: "cost", ResourceID: j.ID, ProviderCostCents: revInt(100), MediaCostCents: revInt(50), EvidenceReference: "provider and storage invoices", Reason: "actual costs"}
	id, err = s.ProposeRewardRevenueEvidence(ctx, evidence, "finance-a")
	must(err)
	must(s.ApproveRewardRevenueEvidence(ctx, id, "finance-b", "verified both cost statements"))
	result, err = s.ReconcileRewardRevenue(ctx, day, "finance-runner")
	must(err)
	if result.Status != "reconciled" || result.NetCents != 250 || result.FundedCents != 100 {
		t.Fatalf("recognition %+v; expected 50 purchased / 100 credits, net250 funding100", result)
	}
	again, err := s.ReconcileRewardRevenue(ctx, day, "finance-runner")
	must(err)
	if again.Status != "already_reconciled" || again.FundedCents != 100 {
		t.Fatalf("duplicate reconciliation %+v", again)
	}
	var used int64
	must(s.DB.QueryRowContext(ctx, `SELECT sum(credits) FROM cloud_reward_revenue_consumption WHERE job_id=$1`, j.ID).Scan(&used))
	if used != 50 {
		t.Fatal("promotional credits recognized", used)
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE cloud_reward_sources SET reserved_cents=50 WHERE id=$1`, result.SourceID)
	must(err)
	var channel string
	must(s.DB.QueryRowContext(ctx, `SELECT id FROM cloud_channels WHERE user_id=$1`, u.ID).Scan(&channel))
	video := "rv-" + suffix
	batch := "rb-" + suffix
	_, err = s.DB.ExecContext(ctx, `INSERT INTO cloud_videos(id,channel_id,job_id,title,description,visibility,audience)VALUES($1,$2,$3,'Revenue video','','public','free')`, video, channel, j.ID)
	must(err)
	_, err = s.DB.ExecContext(ctx, `INSERT INTO cloud_reward_batches(id,day,source_id,policy_version,input_hash,evidence_reference,total_cents,proposed_by)VALUES($1,$2,$3,'pilot-v1','test','statement',50,'finance-a')`, batch, day, result.SourceID)
	must(err)
	_, err = s.DB.ExecContext(ctx, `INSERT INTO cloud_reward_allocations(id,batch_id,video_id,user_id,cents)VALUES($1,$2,$3,$4,50)`, "ra-"+suffix, batch, video, u.ID)
	must(err)
	_, err = s.DB.ExecContext(ctx, `UPDATE cloud_reward_batches SET status='confirmed',confirmed_by='finance-b',confirmed_at=now() WHERE id=$1`, batch)
	must(err)
	_, err = s.DB.ExecContext(ctx, `UPDATE cloud_purchases SET refunded_cents=100 WHERE id=$1`, receipt)
	must(err)
	var holdReason string
	must(s.DB.QueryRowContext(ctx, `SELECT reason FROM cloud_reward_holds WHERE user_id=$1`, u.ID).Scan(&holdReason))
	if _, e := s.DB.ExecContext(ctx, `DELETE FROM cloud_reward_holds WHERE user_id=$1`, u.ID); e == nil {
		t.Fatal("coverage hold released without funded recovery")
	}
	if _, err = s.DB.ExecContext(ctx, `UPDATE cloud_reward_sources SET reserved_cents=60 WHERE id=$1`, result.SourceID); err == nil {
		t.Fatal("refund failed to block new reservation")
	}
	var reserved int64
	must(s.DB.QueryRowContext(ctx, `SELECT reserved_cents FROM cloud_reward_sources WHERE id=$1`, result.SourceID).Scan(&reserved))
	if reserved != 50 {
		t.Fatal("confirmed liability rewritten")
	}
	if _, err = s.DB.ExecContext(ctx, `UPDATE cloud_reward_revenue_lots SET gross_cents=2 WHERE receipt_id=$1`, receipt); err == nil {
		t.Fatal("immutable lot changed")
	}
	replacement := "fresh-" + suffix
	must(s.RecordRewardFunding(ctx, RewardFunding{ID: replacement, Reference: replacement, Kind: "generation", Cents: 100, EvidenceReference: "independently reconciled fresh real funds", ProposedBy: "finance-a", ReconciledBy: "finance-b"}))
	recovery := RewardRevenueEvidence{Kind: "recovery", ResourceID: result.SourceID, ReplacementSourceID: replacement, EvidenceReference: "fresh funding and source coverage review", Reason: "replace reversed source backing"}
	recoveryID, e := s.ProposeRewardRevenueEvidence(ctx, recovery, "finance-a")
	must(e)
	if s.ApproveRewardRevenueEvidence(ctx, recoveryID, "finance-a", "self") == nil {
		t.Fatal("self approved coverage recovery")
	}
	must(s.ApproveRewardRevenueEvidence(ctx, recoveryID, "finance-b", "verified fresh backing"))
	must(s.DB.QueryRowContext(ctx, `SELECT reserved_cents FROM cloud_reward_sources WHERE id=$1`, replacement).Scan(&reserved))
	if reserved != 50 {
		t.Fatal("fresh source money not reserved", reserved)
	}
	_, e = s.DB.ExecContext(ctx, `DELETE FROM cloud_reward_holds WHERE user_id=$1`, u.ID)
	must(e)
	_, e = s.DB.ExecContext(ctx, `INSERT INTO cloud_reward_revenue_source_holds(source_id,reason)VALUES($1,'replacement subsequently reversed')`, replacement)
	must(e)
	if _, e = s.DB.ExecContext(ctx, `DELETE FROM cloud_reward_holds WHERE user_id=$1`, u.ID); e == nil {
		t.Fatal("replacement reversal failed to restore creator coverage hold")
	}
	mockBody := fmt.Sprintf(`{"currency":"usd","payment_status":"paid","amount_total":1000,"total_details":{"amount_tax":100},"payment_intent":{"latest_charge":{"paid":true,"amount_refunded":100,"balance_transaction":{"id":"txn_test","amount":1000,"fee":100,"net":900,"currency":"usd","status":"available","available_on":%d}}}}`, time.Now().Add(-time.Hour).Unix())
	statementServer := NewServer(s, Config{StripeSecret: "offline-test-only"})
	statementServer.HTTP = &http.Client{Transport: adminTransport(func(req *http.Request) (*http.Response, error) {
		if req.Method != "GET" || req.URL.Query().Get("expand[]") != "payment_intent.latest_charge.balance_transaction" {
			t.Fatal("incorrect Stripe statement request")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(mockBody)), Header: make(http.Header)}, nil
	})}
	statement, e := statementServer.RewardStripeStatement(ctx, receipt)
	must(e)
	if statement.FeeCents == nil || *statement.FeeCents != 100 || statement.TaxCents == nil {
		t.Fatal("statement fields lost")
	}
	mockBody = strings.Replace(mockBody, `"amount_tax":100`, `"unavailable_tax":100`, 1)
	if _, e = statementServer.RewardStripeStatement(ctx, receipt); e == nil {
		t.Fatal("absent tax became zero")
	}
	server := NewServer(s, Config{})
	rec := httptest.NewRecorder()
	server.adminRewardRevenueOverview(rec, httptest.NewRequest("GET", "/admin/rewards/revenue", nil))
	if rec.Code != 200 {
		t.Fatalf("revenue overview %d: %s", rec.Code, rec.Body.String())
	}
	t.Log(fmt.Sprintf("source %s backed by consumed purchased credits only", result.SourceID))
}
