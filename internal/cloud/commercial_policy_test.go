package cloud

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func commercialBase() Config {
	return Config{DevMode: true, CreditsPerSecond: 10, StoryCredits: 2, AssemblyCredits: 2, ProviderMaxCostMicros: 25000, StoryMaxCostMicros: 5000, ProviderDailyBudgetMicros: 100000, PremiumPriceCents: 900}
}
func TestCommercialMarginBounds(t *testing.T) {
	runtime := runtimePolicyFromConfig(commercialBase())
	p := DefaultCommercialPolicy()
	if e := validateCommercialMargin(runtime, p); e != nil {
		t.Fatal(e)
	}
	p.Packs = []CreditPack{{ID: "cheap", Name: "Cheap", Credits: 100000000, PriceCents: 100, Currency: "usd"}}
	if e := validateCommercialMargin(runtime, p); e == nil {
		t.Fatal("insufficient margin accepted")
	}
	p = DefaultCommercialPolicy()
	p.FeeReserveBPS = 5000
	p.MediaReserveBPS = 5000
	if validateCommercialPolicy(p) == nil {
		t.Fatal("unbounded reserves")
	}
	p = DefaultCommercialPolicy()
	p.Packs = append(p.Packs, p.Packs[0])
	if validateCommercialPolicy(p) == nil {
		t.Fatal("duplicate pack")
	}
}
func TestCommercialPendingPurchaseSettlementUsesOriginalSnapshot(t *testing.T) {
	s, u := revenueTestStore(t)
	u.Verified = true
	ctx := context.Background()
	base := commercialBase()
	base.BillingEnabled = true
	base.StripeSecret = "test"
	base.StripeWebhookSecret = "test-webhook"
	// Each policy change is prospective. Restore the bootstrap catalog through
	// another audited approval so later fixtures can still buy launch pack IDs.
	restore := func() {
		r, e := s.CommercialPolicy(ctx)
		if e != nil {
			t.Error(e)
			return
		}
		id, e := s.ProposeCommercialPolicy(ctx, DefaultCommercialPolicy(), r.Revision, "restore reviewed launch catalog", "restore-a")
		if e == nil {
			e = s.ApproveCommercialPolicy(ctx, id, "restore-b", "independent restore", base)
		}
		if e != nil {
			t.Error(e)
		}
	}
	defer restore()
	initial, e := s.CommercialPolicy(ctx)
	if e != nil {
		t.Fatal(e)
	}
	app := NewServer(s, base)
	defer app.Close()
	app.HTTP = &http.Client{Transport: commerceRoundTrip(func(r *http.Request) (*http.Response, error) {
		if !strings.HasSuffix(r.URL.Path, "/checkout/sessions") {
			t.Fatalf("unexpected provider request %s", r.URL.Path)
		}
		id := r.Header.Get("Idempotency-Key")
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"id":"cs_` + id + `","url":"https://checkout.stripe.com/pay/` + id + `"}`))}, nil
	})}
	url, e := app.checkout(ctx, u, "starter", "old-purchase-key")
	if e != nil {
		t.Fatal(e)
	}
	var purchase, session string
	if e = s.DB.QueryRow(`SELECT id,stripe_session FROM cloud_purchases WHERE workspace_id=$1 AND idempotency_key='old-purchase-key'`, u.WorkspaceID).Scan(&purchase, &session); e != nil {
		t.Fatal(e)
	}
	// A pending crypto invoice is also a frozen financial contract.
	cryptoID := uuid.NewString()
	if _, e = s.DB.Exec(`INSERT INTO cloud_crypto_invoices(id,user_id,workspace_id,purpose,pack_id,credits,price_cents,idempotency_key,reference,network,vm_type,token,token_address,treasury_address,amount,consolidate_amount,amount_net,fee_amount,address,status)VALUES($1,$2,$3,'credits','starter',1000,1000,'old-crypto-key',$1,'bsc-mainnet','EVM','USDT','0xtoken','0xtreasury','10','10','10','0',$4,'pending')`, cryptoID, u.ID, u.WorkspaceID, "0xold"+cryptoID); e != nil {
		t.Fatal(e)
	}
	p := DefaultCommercialPolicy()
	p.Packs = []CreditPack{{ID: "next", Name: "Next", Credits: 2000, PriceCents: 4000, Currency: "usd"}}
	p.FeeReserveBPS = 300
	p.MediaReserveBPS = 200
	proposal, e := s.ProposeCommercialPolicy(ctx, p, initial.Revision, "prospective catalog revision", "finance-a")
	if e != nil {
		t.Fatal(e)
	}
	if e = s.ApproveCommercialPolicy(ctx, proposal, "finance-a", "self", base); e == nil {
		t.Fatal("self approval")
	}
	if e = s.ApproveCommercialPolicy(ctx, proposal, "finance-b", "independent margin review", base); e != nil {
		t.Fatal(e)
	}
	if e = s.ApproveCommercialPolicy(ctx, proposal, "finance-b", "retry", base); e != nil {
		t.Fatal(e)
	}
	again, e := app.checkout(ctx, u, "starter", "old-purchase-key")
	if e != nil || again != url {
		t.Fatalf("removed pack retry lost snapshot %s %v", again, e)
	}
	if _, e = app.checkout(ctx, u, "starter", "new-purchase-key"); e == nil {
		t.Fatal("removed pack still sold")
	}
	if _, e = app.checkout(ctx, u, "next", "next-purchase-key"); e != nil {
		t.Fatal(e)
	}
	var credits, price, revision int64
	s.DB.QueryRow(`SELECT credits,price_cents,commercial_revision FROM cloud_purchases WHERE workspace_id=$1 AND idempotency_key='next-purchase-key'`, u.WorkspaceID).Scan(&credits, &price, &revision)
	if credits != 2000 || price != 4000 || revision != initial.Revision+1 {
		t.Fatal("new purchase missed effective policy", credits, price, revision)
	}
	cfg := cryptoTestConfig()
	cfg.CreditsPerSecond = base.CreditsPerSecond
	cfg.StoryCredits = base.StoryCredits
	cfg.AssemblyCredits = base.AssemblyCredits
	cfg.ProviderMaxCostMicros = base.ProviderMaxCostMicros
	cfg.StoryMaxCostMicros = base.StoryMaxCostMicros
	cfg.ProviderDailyBudgetMicros = base.ProviderDailyBudgetMicros
	cryptoApp := NewServer(s, cfg)
	defer cryptoApp.Close()
	cryptoApp.HTTP = &http.Client{Transport: commerceRoundTrip(func(r *http.Request) (*http.Response, error) {
		t.Fatal("existing invoice retry made provider call")
		return nil, ErrUnavailable
	})}
	invoice, e := cryptoApp.newCryptoInvoice(ctx, u, "credits", "starter", "old-crypto-key")
	if e != nil || invoice.ID != cryptoID || invoice.Credits != 1000 || invoice.PriceCents != 1000 {
		t.Fatalf("old invoice snapshot changed %+v %v", invoice, e)
	}
	event := map[string]any{"id": "evt_" + purchase, "type": "checkout.session.completed", "data": map[string]any{"object": map[string]any{"id": session, "client_reference_id": purchase, "payment_status": "paid", "amount_total": 1000, "currency": "usd"}}}
	body, _ := json.Marshal(event)
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("POST", "/v1/webhooks/stripe", strings.NewReader(string(body)))
		req.Header.Set("Stripe-Signature", stripeSignature(body, "test-webhook", time.Now().Unix()))
		w := httptest.NewRecorder()
		app.paymentWebhook(w, req)
		if w.Code != 200 {
			t.Fatalf("old snapshot webhook %d %s", w.Code, w.Body)
		}
	}
	balance, e := s.Credits(ctx, u.WorkspaceID)
	if e != nil || balance.Available != 1000 {
		t.Fatalf("original grant changed %+v %v", balance, e)
	}
	if _, e = s.DB.Exec(`UPDATE cloud_purchases SET credits=2000 WHERE id=$1`, purchase); e == nil {
		t.Fatal("purchase snapshot mutable")
	}
	if _, e = s.DB.Exec(`UPDATE cloud_crypto_invoices SET price_cents=2000 WHERE id=$1`, cryptoID); e == nil {
		t.Fatal("invoice snapshot mutable")
	}
	bad := DefaultCommercialPolicy()
	bad.Packs = []CreditPack{{ID: "loss", Name: "Loss", Credits: 100000000, PriceCents: 100, Currency: "usd"}}
	r, e := s.CommercialPolicy(ctx)
	if e != nil {
		t.Fatal(e)
	}
	id, e := s.ProposeCommercialPolicy(ctx, bad, r.Revision, "unsafe margin fixture", "a")
	if e != nil {
		t.Fatal(e)
	}
	if e = s.ApproveCommercialPolicy(ctx, id, "b", "reviewed", base); e == nil {
		t.Fatal("insufficient margin approval")
	}
}

func TestCommercialPolicyReadWithoutActivatedGeneration(t *testing.T) {
	s, _ := revenueTestStore(t)
	app := NewServer(s, Config{DevMode: true})
	defer app.Close()
	w := httptest.NewRecorder()
	app.adminCommercialPolicy(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 200 {
		t.Fatalf("inactive read: %d %s", w.Code, w.Body.String())
	}
	var body struct {
		Preview *CommercialPreview `json:"preview"`
		Status  string             `json:"preview_status"`
	}
	if e := json.Unmarshal(w.Body.Bytes(), &body); e != nil || body.Preview != nil || body.Status != "generation_policy_required" {
		t.Fatalf("%+v %v", body, e)
	}
}
