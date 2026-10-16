package cloud

import (
	"context"
	"github.com/google/uuid"
	"net/http"
	"testing"
	"time"
)

// Explicit offline attestation for legacy-focused payout fixtures. Production
// never fabricates historical receipts or grants backing implicitly.
func backLegacyTestEarnings(t *testing.T, s *Store, user string) {
	t.Helper()
	ctx := context.Background()
	rows, e := s.DB.QueryContext(ctx, `SELECT id,amount_cents FROM cloud_earnings WHERE user_id=$1 AND amount_cents>0 AND currency='usd' AND status<>'paid' AND NOT cloud_reward_earning_backed(id)`, user)
	if e != nil {
		t.Fatal(e)
	}
	ids := []string{}
	var total int64
	for rows.Next() {
		var id string
		var n int64
		if e = rows.Scan(&id, &n); e != nil {
			t.Fatal(e)
		}
		ids = append(ids, id)
		total += n
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		t.Fatal(e)
	}
	if total == 0 {
		return
	}
	source := "fixture-legacy-" + uuid.NewString()
	if e = s.RecordRewardFunding(ctx, RewardFunding{ID: source, Reference: source, Kind: "generation", Cents: total, EvidenceReference: "offline fixture: explicitly attested fresh backing", ProposedBy: "fixture-finance-a", ReconciledBy: "fixture-finance-b"}); e != nil {
		t.Fatal(e)
	}
	for _, id := range ids {
		proposal, e := s.ProposeRewardLegacyBacking(ctx, RewardLegacyBackingProposal{EarningID: id, SourceID: source, Reason: "explicit offline legacy fixture attestation", EvidenceReference: "test-only funded clearance"}, "fixture-finance-a")
		if e != nil {
			t.Fatal(e)
		}
		if e = s.ApproveRewardLegacyBacking(ctx, proposal, "fixture-finance-b", "independent offline fixture review"); e != nil {
			t.Fatal(e)
		}
	}
}
func TestLegacyRewardRequiresIndependentFreshBacking(t *testing.T) {
	s, u := revenueTestStore(t)
	ctx := context.Background()
	must := func(e error) {
		t.Helper()
		if e != nil {
			t.Fatal(e)
		}
	}
	_, e := s.DB.Exec(`INSERT INTO cloud_payout_accounts(user_id,provider_id,country,charges_enabled,payouts_enabled)VALUES($1,$2,'US',true,true)`, u.ID, "acct_"+uuid.NewString())
	must(e)
	earning := uuid.NewString()
	_, e = s.DB.Exec(`INSERT INTO cloud_earnings(id,user_id,reference,amount_cents,currency,status,description)VALUES($1,$2,$1,6000,'usd','available','legacy exception fixture')`, earning, u.ID)
	must(e)
	balance, e := s.RewardBalance(ctx, u.ID, time.Now())
	must(e)
	if balance.AvailableCents != 0 || balance.LegacyUnbackedCents != 6000 {
		t.Fatalf("unbacked spendable %+v", balance)
	}
	if _, e = s.PreparePayout(ctx, u.ID, "finance"); e == nil {
		t.Fatal("unbacked legacy prepared")
	}
	source := "legacy-source-" + uuid.NewString()
	must(s.RecordRewardFunding(ctx, RewardFunding{ID: source, Reference: source, Kind: "generation", Cents: 6000, EvidenceReference: "new actual funding statement", ProposedBy: "finance-a", ReconciledBy: "finance-b"}))
	proposal, e := s.ProposeRewardLegacyBacking(ctx, RewardLegacyBackingProposal{EarningID: earning, SourceID: source, Reason: "independent funded clearance", EvidenceReference: "fresh cash statement"}, "finance-a")
	must(e)
	if s.ApproveRewardLegacyBacking(ctx, proposal, "finance-a", "self") == nil {
		t.Fatal("self approval accepted")
	}
	must(s.ApproveRewardLegacyBacking(ctx, proposal, "finance-b", "verified new source coverage"))
	must(s.ApproveRewardLegacyBacking(ctx, proposal, "finance-b", "idempotent retry"))
	var reserved int64
	must(s.DB.QueryRow(`SELECT reserved_cents FROM cloud_reward_sources WHERE id=$1`, source).Scan(&reserved))
	if reserved != 6000 {
		t.Fatal("backing not reserved exactly once", reserved)
	}
	balance, e = s.RewardBalance(ctx, u.ID, time.Now())
	must(e)
	if balance.AvailableCents != 6000 || balance.LegacyUnbackedCents != 0 {
		t.Fatalf("backed balance %+v", balance)
	}
	batch, e := s.PreparePayout(ctx, u.ID, "finance")
	must(e)
	if batch.AmountCents != 6000 {
		t.Fatal("incorrect legacy payout", batch)
	}
	_, e = s.DB.Exec(`INSERT INTO cloud_earnings(id,user_id,reference,amount_cents,currency,status,description)VALUES($1,$2,$1,1000,'usd','available','unbacked positive cannot cover debt'),($3,$2,$3,-1000,'usd','pending','late debt')`, uuid.NewString(), u.ID, uuid.NewString())
	must(e)
	app := NewServer(s, Config{CashPayoutsEnabled: true, StripeSecret: "offline-only", SupportedPayoutCountries: "US"})
	defer app.Close()
	app.HTTP = &http.Client{Transport: adminTransport(func(*http.Request) (*http.Response, error) {
		t.Fatal("unbacked money covered debt and reached provider")
		return nil, ErrUnavailable
	})}
	if e = app.ExecutePayout(ctx, batch.ID, "finance", true); e != ErrConflict {
		t.Fatalf("unbacked positive offset payout debt: %v", e)
	}
	_, e = s.DB.Exec(`INSERT INTO cloud_reward_revenue_source_holds(source_id,reason)VALUES($1,'fresh backing source reversed')`, source)
	must(e)
	var held bool
	must(s.DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM cloud_reward_holds WHERE user_id=$1)`, u.ID).Scan(&held))
	if !held {
		t.Fatal("legacy source reversal did not hold account")
	}
	if _, e = s.DB.Exec(`DELETE FROM cloud_reward_holds WHERE user_id=$1`, u.ID); e == nil {
		t.Fatal("unrecovered legacy source hold released")
	}
	if _, e = s.DB.Exec(`UPDATE cloud_reward_legacy_backing SET cents=1 WHERE earning_id=$1`, earning); e == nil {
		t.Fatal("legacy coverage overwritten")
	}
}
