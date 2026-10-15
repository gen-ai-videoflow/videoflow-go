package cloud

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestDisputeHoldsAreIndependentIdempotentAndRecoverable(t *testing.T) {
	store, u := testPlatform(t)
	ctx := context.Background()
	if err := store.GrantCredits(ctx, u.WorkspaceID, "dispute-seed:"+u.ID, 1000, "test"); err != nil {
		t.Fatal(err)
	}
	_, err := store.DB.ExecContext(ctx, `INSERT INTO cloud_purchases(id,workspace_id,pack_id,credits,price_cents,currency,idempotency_key,status,payment_intent)VALUES($1,$2,'test',1000,1000,'usd',$1,'paid',$3)`, "dispute-"+u.ID, u.WorkspaceID, "pi_"+u.ID)
	if err != nil {
		t.Fatal(err)
	}
	status := "needs_response"
	server := NewServer(store, Config{StripeSecret: "sk_test"})
	server.HTTP = &http.Client{Transport: commerceRoundTrip(func(r *http.Request) (*http.Response, error) {
		body := `{"id":"dp_` + u.ID + `","payment_intent":"pi_` + u.ID + `","status":"` + status + `","amount":1000,"currency":"usd"}`
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	raw := json.RawMessage(`{"id":"dp_` + u.ID + `"}`)
	apply := func(event string) {
		t.Helper()
		if e := server.paymentDispute(ctx, event+u.ID, raw); e != nil {
			t.Fatal(e)
		}
	}
	apply("opened")
	apply("opened")
	b, err := store.Credits(ctx, u.WorkspaceID)
	if err != nil || b.Available != 0 || !b.Restricted {
		t.Fatalf("hold %+v %v", b, err)
	}
	// Winning must not clear a separately applied operator restriction.
	_, err = store.DB.ExecContext(ctx, `UPDATE cloud_credit_accounts SET restricted=true WHERE workspace_id=$1`, u.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	status = "won"
	apply("closed")
	b, err = store.Credits(ctx, u.WorkspaceID)
	if err != nil || !b.Restricted {
		t.Fatalf("operator restriction cleared %+v %v", b, err)
	}
	_, err = store.DB.ExecContext(ctx, `UPDATE cloud_credit_accounts SET restricted=false WHERE workspace_id=$1`, u.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	b, err = store.Credits(ctx, u.WorkspaceID)
	if err != nil || b.Available != 1000 || b.Restricted {
		t.Fatalf("won %+v %v", b, err)
	}
	// A delayed opened event reads the current provider outcome instead of stale payload.
	apply("delayed-open")
	b, err = store.Credits(ctx, u.WorkspaceID)
	if err != nil || b.Available != 1000 {
		t.Fatalf("delayed %+v %v", b, err)
	}
	status = "lost"
	apply("lost")
	b, err = store.Credits(ctx, u.WorkspaceID)
	if err != nil || b.Available != 0 {
		t.Fatalf("lost %+v %v", b, err)
	}
	var count int
	if err = store.DB.QueryRowContext(ctx, `SELECT count(*) FROM cloud_admin_audit WHERE resource_id=$1 AND action='payment_dispute'`, "dp_"+u.ID).Scan(&count); err != nil || count != 4 {
		t.Fatalf("audit %d %v", count, err)
	}
}

func TestCreatorDisputeBlocksAlreadyPreparedPayout(t *testing.T) {
	store, u := testPlatform(t)
	ctx := context.Background()
	_, err := store.DB.ExecContext(ctx, `INSERT INTO cloud_payout_accounts(user_id,provider_id,country,charges_enabled,payouts_enabled)VALUES($1,$2,'US',true,true)`, u.ID, "acct_"+u.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.DB.ExecContext(ctx, `INSERT INTO cloud_earnings(id,user_id,reference,amount_cents,currency,status,description,created_at)VALUES($1,$2,$1,6000,'usd','pending','test',now()-interval '31 days')`, "earning-"+u.ID, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	batch, err := store.PreparePayout(ctx, u.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.DB.ExecContext(ctx, `INSERT INTO cloud_payment_disputes(id,payment_intent,status,amount_cents,currency,creator_id)VALUES($1,$2,'under_review',1000,'usd',$3)`, "dp_"+u.ID, "pi_"+u.ID, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.PreparePayout(ctx, u.ID, "test"); err != ErrConflict {
		t.Fatalf("prepared hold %v", err)
	}
	server := NewServer(store, Config{StripeSecret: "sk_test", CashPayoutsEnabled: true, SupportedPayoutCountries: "US"})
	defer server.Close()
	server.HTTP = &http.Client{Transport: commerceRoundTrip(func(r *http.Request) (*http.Response, error) {
		t.Error("held payout contacted Stripe")
		return nil, ErrUnavailable
	})}
	if err = server.ExecutePayout(ctx, batch.ID, "test", true); err != ErrConflict {
		t.Fatalf("execute hold %v", err)
	}
}

func TestRefundAfterPayoutPreparationBlocksFirstTransfer(t *testing.T) {
	store, u := testPlatform(t)
	ctx := context.Background()
	_, err := store.DB.ExecContext(ctx, `INSERT INTO cloud_payout_accounts(user_id,provider_id,country,charges_enabled,payouts_enabled)VALUES($1,$2,'US',true,true)`, u.ID, "acct_"+u.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.DB.ExecContext(ctx, `INSERT INTO cloud_earnings(id,user_id,reference,amount_cents,currency,status,description,created_at)VALUES($1,$2,$1,6000,'usd','pending','test',now()-interval '31 days')`, "earning-"+u.ID, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	batch, err := store.PreparePayout(ctx, u.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.DB.ExecContext(ctx, `INSERT INTO cloud_earnings(id,user_id,reference,amount_cents,currency,status,description)VALUES($1,$2,$1,-1000,'usd','pending','refund')`, "refund-"+u.ID, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(store, Config{StripeSecret: "sk_test", CashPayoutsEnabled: true, SupportedPayoutCountries: "US"})
	defer server.Close()
	server.HTTP = &http.Client{Transport: commerceRoundTrip(func(r *http.Request) (*http.Response, error) {
		t.Error("insufficient matured balance contacted Stripe")
		return nil, ErrUnavailable
	})}
	if err = server.ExecutePayout(ctx, batch.ID, "test", true); err != ErrConflict {
		t.Fatalf("refund after preparation %v", err)
	}
}
