package cloud

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestPayoutPrepareFreezesOnceAndOffsetsDebt(t *testing.T) {
	s, u := testPolicyStore(t)
	ctx := context.Background()
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO cloud_payout_accounts(user_id,provider_id,country,charges_enabled,payouts_enabled)VALUES($1,$2,'US',true,true)`, u.ID, "acct_"+uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		amount int64
		age    string
	}{{7000, "31 days"}, {-1000, "1 day"}, {9000, "1 day"}} {
		if _, err := s.DB.ExecContext(ctx, `INSERT INTO cloud_earnings(id,user_id,reference,amount_cents,currency,status,description,created_at)VALUES($1,$2,$1,$3,'usd','pending','test',now()-$4::interval)`, uuid.NewString(), u.ID, item.amount, item.age); err != nil {
			t.Fatal(err)
		}
	}
	b, err := s.PreparePayout(ctx, u.ID, "test-operator")
	if err != nil {
		t.Fatal(err)
	}
	if b.AmountCents != 6000 {
		t.Fatalf("amount=%d", b.AmountCents)
	}
	again, err := s.PreparePayout(ctx, u.ID, "test-operator")
	if err != nil || again.ID != b.ID {
		t.Fatalf("duplicate prepare %+v %v", again, err)
	}
	server := NewServer(s, Config{})
	defer server.Close()
	if err = server.ExecutePayout(ctx, b.ID, "test-operator", true); err == nil {
		t.Fatal("cash disabled guard bypassed")
	}
}

func TestPayoutRetryUsesSameTransferAndDoesNotDoublePay(t *testing.T) {
	s, u := testPolicyStore(t)
	ctx := context.Background()
	account := "acct_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO cloud_payout_accounts(user_id,provider_id,country,charges_enabled,payouts_enabled)VALUES($1,$2,'US',true,true)`, u.ID, account); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO cloud_earnings(id,user_id,reference,amount_cents,currency,status,description,created_at)VALUES($1,$2,$1,6000,'usd','pending','test',now()-interval '31 days')`, uuid.NewString(), u.ID); err != nil {
		t.Fatal(err)
	}
	batch, err := s.PreparePayout(ctx, u.ID, "operator")
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(s, Config{CashPayoutsEnabled: true, StripeSecret: "test", SupportedPayoutCountries: "US"})
	defer server.Close()
	var calls atomic.Int32
	server.HTTP = &http.Client{Transport: adminTransport(func(r *http.Request) (*http.Response, error) {
		var body string
		switch {
		case strings.Contains(r.URL.Path, "/accounts/"):
			body = fmt.Sprintf(`{"id":%q,"country":"US","charges_enabled":true,"payouts_enabled":true,"details_submitted":true,"capabilities":{"transfers":"active"}}`, account)
		case strings.HasSuffix(r.URL.Path, "/balance"):
			body = `{"available":[{"currency":"usd","amount":100000}]}`
		case strings.HasSuffix(r.URL.Path, "/transfers"):
			if r.Header.Get("Idempotency-Key") != "payout:"+batch.ID {
				t.Error("unstable transfer key")
			}
			if calls.Add(1) == 1 {
				return nil, fmt.Errorf("timeout after provider accepted transfer")
			}
			body = fmt.Sprintf(`{"id":"tr_%s","amount":6000,"currency":"usd","destination":%q}`, batch.ID, account)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	if err = server.ExecutePayout(ctx, batch.ID, "operator", true); err == nil {
		t.Fatal("expected ambiguous timeout")
	}
	var group sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		group.Add(1)
		go func() { defer group.Done(); results <- server.ExecutePayout(ctx, batch.ID, "operator", true) }()
	}
	group.Wait()
	close(results)
	for e := range results {
		if e != nil {
			t.Fatal(e)
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("transfer calls=%d", calls.Load())
	}
	var status string
	if err = s.DB.QueryRowContext(ctx, `SELECT status FROM cloud_payout_batches WHERE id=$1`, batch.ID).Scan(&status); err != nil || status != "paid" {
		t.Fatalf("status %s %v", status, err)
	}
}

type adminTransport func(*http.Request) (*http.Response, error)

func (f adminTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestConcurrentProviderResolutionCannotRefundAttachedJob(t *testing.T) {
	s, u := testPolicyStore(t)
	ctx := context.Background()
	p, err := s.CreateProject(ctx, u, "Resolution race", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.GrantCredits(ctx, u.WorkspaceID, "resolution-race", 100, ""); err != nil {
		t.Fatal(err)
	}
	q, err := s.CreateQuote(ctx, u, QuoteInput{ProjectID: p.ID, Prompt: "test", Duration: 5, AspectRatio: "16:9"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	j, err := s.SubmitJob(ctx, u, q.ID, "resolution-race-key")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.ExecContext(ctx, `UPDATE cloud_jobs SET status='reconciliation_required' WHERE id=$1`, j.ID); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	go func() { <-start; results <- s.ResolveProviderJob(ctx, j.ID, "provider-known", "operator", false) }()
	go func() { <-start; results <- s.ResolveProviderJob(ctx, j.ID, "", "operator", true) }()
	close(start)
	successes := 0
	for i := 0; i < 2; i++ {
		e := <-results
		if e == nil {
			successes++
		} else if e != ErrConflict {
			t.Fatal(e)
		}
	}
	if successes != 1 {
		t.Fatalf("resolution winners=%d", successes)
	}
	got, err := s.Job(ctx, u, j.ID)
	if err != nil {
		t.Fatal(err)
	}
	balance, err := s.Credits(ctx, u.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status == "running" {
		if got.ProviderID != "provider-known" || balance.Reserved != 5 {
			t.Fatalf("attached job refunded %+v %+v", got, balance)
		}
	} else if got.Status == "failed" {
		if got.ProviderID != "" || balance.Reserved != 0 {
			t.Fatalf("failed job attached %+v %+v", got, balance)
		}
	} else {
		t.Fatalf("unexpected %+v", got)
	}
	var count int
	if err = s.DB.QueryRowContext(ctx, `SELECT count(*) FROM cloud_admin_audit WHERE resource_id=$1 AND action IN ('resolve_job_fail','resolve_job_provider')`, j.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("audit count=%d %v", count, err)
	}
}
