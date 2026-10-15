package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestCloudHTTPRejectsCrossOriginAndLegacyPaths(t *testing.T) {
	store, u := testPlatform(t)
	app := NewServer(store, Config{PublicOrigin: "http://localhost:5173", DevMode: true})
	defer app.Close()
	_, token, e := store.Login(context.Background(), u.Email, "Test-password-123!")
	if e != nil {
		t.Fatal(e)
	}
	for _, path := range []string{"/settings/providers", "/storage/private.mp4", "/projects/active"} {
		req := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		app.Router().ServeHTTP(w, req)
		if w.Code != 404 {
			t.Fatalf("legacy route %s exposed", path)
		}
	}
	req := httptest.NewRequest("POST", "/v1/projects", bytes.NewBufferString(`{"name":"Attack"}`))
	req.Header.Set("Origin", "https://attacker.example")
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "videoflow_session", Value: token})
	w := httptest.NewRecorder()
	app.Router().ServeHTTP(w, req)
	if w.Code != 401 {
		t.Fatalf("csrf accepted %d", w.Code)
	}
	req = httptest.NewRequest("GET", "/api/v1/me", nil)
	req.AddCookie(&http.Cookie{Name: "videoflow_session", Value: token})
	w = httptest.NewRecorder()
	app.Router().ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("proxy alias: %d %s", w.Code, w.Body.String())
	}
}
func TestConcurrentReservationsNeverExceedCredits(t *testing.T) {
	store, u := testPlatform(t)
	ctx := context.Background()
	if e := store.GrantCredits(ctx, u.WorkspaceID, "initial", 100, ""); e != nil {
		t.Fatal(e)
	}
	p, e := store.CreateProject(ctx, u, "Concurrent", "")
	if e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	results := make(chan error, 12)
	for i := 0; i < 12; i++ {
		q, e := store.CreateQuote(ctx, u, QuoteInput{ProjectID: p.ID, Prompt: "a scene", Duration: 5, AspectRatio: "16:9"}, 10)
		if e != nil {
			t.Fatal(e)
		}
		wg.Add(1)
		go func(i int, q Quote) {
			defer wg.Done()
			_, err := store.SubmitJob(ctx, u, q.ID, fmt.Sprintf("parallel-key-%d", i))
			results <- err
		}(i, q)
	}
	wg.Wait()
	close(results)
	success := 0
	for e := range results {
		if e == nil {
			success++
		} else if e != ErrInsufficientCredits {
			t.Fatal(e)
		}
	}
	if success != 2 {
		t.Fatalf("successful reservations=%d", success)
	}
	b, e := store.Credits(ctx, u.WorkspaceID)
	if e != nil || b.Available != 0 || b.Reserved != 100 {
		t.Fatalf("%+v %v", b, e)
	}
}
func TestPaymentWebhookIsVerifiedAndIdempotent(t *testing.T) {
	store, u := testPlatform(t)
	ctx := context.Background()
	app := NewServer(store, Config{PublicOrigin: "http://localhost:5173", DevMode: true, StripeWebhookSecret: "test-secret"})
	defer app.Close()
	id := fmt.Sprintf("purchase%d", time.Now().UnixNano())
	_, e := store.DB.ExecContext(ctx, `INSERT INTO cloud_purchases(id,workspace_id,pack_id,credits,price_cents,currency,stripe_session,idempotency_key)VALUES($1,$2,'starter',1000,1000,'usd',$3,$1)`, id, u.WorkspaceID, "cs_"+id)
	if e != nil {
		t.Fatal(e)
	}
	object := map[string]any{"id": "cs_" + id, "client_reference_id": id, "payment_status": "paid", "amount_total": 1000, "currency": "usd"}
	event := map[string]any{"id": "evt_" + id, "type": "checkout.session.completed", "data": map[string]any{"object": object}}
	body, _ := json.Marshal(event)
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("POST", "/v1/webhooks/stripe", bytes.NewReader(body))
		req.Header.Set("Stripe-Signature", stripeSignature(body, "test-secret", time.Now().Unix()))
		w := httptest.NewRecorder()
		app.Router().ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("webhook %d: %s", w.Code, w.Body.String())
		}
	}
	b, e := store.Credits(ctx, u.WorkspaceID)
	if e != nil || b.Available != 1000 {
		t.Fatalf("duplicate grant %+v %v", b, e)
	}
	req := httptest.NewRequest("POST", "/v1/webhooks/stripe", bytes.NewBufferString(`{}`))
	w := httptest.NewRecorder()
	app.Router().ServeHTTP(w, req)
	if w.Code != 401 {
		t.Fatal("unsigned event accepted")
	}
}

func TestTrustedProxyOnlyUsesGeneratedCloudFrontViewerAddress(t *testing.T) {
	app := NewServer(nil, Config{TrustedProxyCIDRs: "10.0.0.0/16"})
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.1.12:1234"
	req.Header.Set("CloudFront-Viewer-Address", "203.0.113.17:443")
	req.Header.Set("X-Forwarded-For", "attacker, 203.0.113.17, 198.51.100.1")
	if app.peer(req) != "203.0.113.17" {
		t.Fatal("trusted edge not resolved")
	}
	req.RemoteAddr = "198.51.100.22:1234"
	if app.peer(req) != "198.51.100.22" {
		t.Fatal("untrusted forged edge header accepted")
	}
}
func TestRedisOutageFailsClosedForSensitiveAdmission(t *testing.T) {
	app := NewServer(nil, Config{PublicOrigin: "https://videoflow.example"})
	defer app.Close()
	req := httptest.NewRequest("POST", "/v1/auth/login", bytes.NewBufferString(`{"email":"a@example.com","password":"Password123456"}`))
	req.Header.Set("Origin", "https://videoflow.example")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	app.Router().ServeHTTP(w, req)
	if w.Code != 503 {
		t.Fatalf("sensitive rate fallback allowed: %d", w.Code)
	}
}
func TestCreditRefundAfterSpendCreatesDebtAndDoesNotDuplicate(t *testing.T) {
	store, u := testPlatform(t)
	ctx := context.Background()
	app := NewServer(store, Config{})
	id := "refund-" + u.ID
	store.DB.ExecContext(ctx, `INSERT INTO cloud_purchases(id,workspace_id,pack_id,credits,price_cents,currency,status,payment_intent,idempotency_key)VALUES($1,$2,'starter',100,1000,'usd','paid',$3,$1)`, id, u.WorkspaceID, "pi_"+id)
	if e := store.GrantCredits(ctx, u.WorkspaceID, "purchase:"+id, 100, "Purchase"); e != nil {
		t.Fatal(e)
	}
	p, _ := store.CreateProject(ctx, u, "Spend", "")
	q, _ := store.CreateQuote(ctx, u, QuoteInput{ProjectID: p.ID, Prompt: "scene", Duration: 5, AspectRatio: "16:9"}, 10)
	j, e := store.SubmitJob(ctx, u, q.ID, "refund-spend-key")
	if e != nil {
		t.Fatal(e)
	}
	if e = store.FinishJob(ctx, j.ID, "succeeded", "workspaces/test/output.mp4", ""); e != nil {
		t.Fatal(e)
	}
	raw := json.RawMessage(fmt.Sprintf(`{"payment_intent":%q,"amount":1000,"amount_refunded":1000}`, "pi_"+id))
	for i := 0; i < 2; i++ {
		handled, e := app.creditRefund(ctx, fmt.Sprintf("evt-%s-%d", id, i), raw)
		if e != nil || !handled {
			t.Fatalf("refund %v %v", handled, e)
		}
	}
	b, e := store.Credits(ctx, u.WorkspaceID)
	var debt int64
	store.DB.QueryRowContext(ctx, `SELECT debt FROM cloud_credit_accounts WHERE workspace_id=$1`, u.WorkspaceID).Scan(&debt)
	if e != nil || b.Available != 0 || debt != 50 {
		t.Fatalf("balance %+v debt%d %v", b, debt, e)
	}
	if e = store.GrantCredits(ctx, u.WorkspaceID, "repay", 100, "New purchase"); e != nil {
		t.Fatal(e)
	}
	b, _ = store.Credits(ctx, u.WorkspaceID)
	if b.Available != 50 {
		t.Fatalf("debt not repaid: %+v", b)
	}
	var unbalanced int
	store.DB.QueryRowContext(ctx, `SELECT count(*) FROM (SELECT entry_id FROM cloud_credit_postings GROUP BY entry_id HAVING sum(amount)<>0)x`).Scan(&unbalanced)
	if unbalanced != 0 {
		t.Fatal("unbalanced ledger")
	}
}
