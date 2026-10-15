package cloud

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPremiumInvoiceGrantsEveryCreatorWithoutPerInvoiceEarnings(t *testing.T) {
	store, viewer := testPlatform(t)
	_, creatorA := testPlatform(t)
	_, creatorB := testPlatform(t)
	ctx := context.Background()
	id := "premium-" + viewer.ID
	end := time.Now().Add(30 * 24 * time.Hour).Unix()
	_, e := store.DB.ExecContext(ctx, `INSERT INTO cloud_premium_checkouts(id,user_id,price_cents,idempotency_key,provider_subscription)VALUES($1,$2,900,$1,$3)`, id, viewer.ID, "sub_"+id)
	if e != nil {
		t.Fatal(e)
	}
	server := NewServer(store, Config{StripeSecret: "sk_test"})
	server.HTTP = &http.Client{Transport: commerceRoundTrip(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"id":"sub_` + id + `","status":"active","metadata":{"premium_checkout_id":"` + id + `"},"current_period_end":` + intString(end) + `}`))}, nil
	})}
	invoice := json.RawMessage(`{"id":"in_` + id + `","subscription":"sub_` + id + `","payment_intent":"pi_` + id + `","amount_paid":900,"currency":"usd","lines":{"data":[{"period":{"end":` + intString(end) + `}}]}}`)
	for _, event := range []string{"evt1_" + id, "evt2_" + id} {
		handled, err := server.processPremiumWebhook(ctx, event, "invoice.paid", invoice)
		if err != nil || !handled {
			t.Fatalf("%v %v", handled, err)
		}
	}
	for _, creator := range []User{creatorA, creatorB} {
		if !store.CanWatch(ctx, viewer, Video{ChannelID: creator.ChannelID, Status: "published", Visibility: "public", Audience: "members"}) {
			t.Fatal("Premium did not unlock another creator")
		}
	}
	var earnings int
	store.DB.QueryRowContext(ctx, `SELECT count(*) FROM cloud_earnings WHERE user_id IN ($1,$2,$3)`, viewer.ID, creatorA.ID, creatorB.ID).Scan(&earnings)
	if earnings != 0 {
		t.Fatal("Premium invoice minted creator earnings")
	}
	refund := json.RawMessage(`{"payment_intent":"pi_` + id + `","amount":900,"amount_refunded":900}`)
	for _, event := range []string{"refund1_" + id, "refund2_" + id} {
		handled, err := server.processPremiumWebhook(ctx, event, "charge.refunded", refund)
		if err != nil || !handled {
			t.Fatalf("refund %v %v", handled, err)
		}
	}
	if active, err := store.HasPremium(ctx, viewer.ID); err != nil || active {
		t.Fatalf("refunded premium active=%v %v", active, err)
	}
	if _, err := server.processPremiumWebhook(ctx, "replay_"+id, "invoice.paid", invoice); err != nil {
		t.Fatal(err)
	}
	if active, _ := store.HasPremium(ctx, viewer.ID); active {
		t.Fatal("duplicate paid invoice resurrected refunded Premium")
	}
}
func TestPremiumDisabledByDefault(t *testing.T) {
	if NewServer(nil, Config{BillingEnabled: true, StripeSecret: "test"}).premiumEnabled() {
		t.Fatal("unpriced Premium enabled")
	}
}

func TestPremiumCheckoutIdempotencyAndPortalOwnership(t *testing.T) {
	store, u := testPlatform(t)
	_, other := testPlatform(t)
	ctx := context.Background()
	server := NewServer(store, Config{BillingEnabled: true, PremiumPriceCents: 900, StripeSecret: "test", PublicOrigin: "https://studio.example.com"})
	calls := 0
	sessionID := "cs_" + u.ID
	subscriptionID := "sub_" + u.ID
	var checkoutID string
	server.HTTP = &http.Client{Transport: commerceRoundTrip(func(r *http.Request) (*http.Response, error) {
		var body string
		switch r.URL.Path {
		case "/v1/checkout/sessions":
			calls++
			r.ParseForm()
			checkoutID = r.Form.Get("metadata[premium_checkout_id]")
			if r.Form.Get("mode") != "subscription" || r.Form.Get("line_items[0][price_data][unit_amount]") != "900" || r.Form.Get("subscription_data[metadata][premium_checkout_id]") != checkoutID {
				t.Error("checkout did not freeze platform price/metadata")
			}
			body = `{"id":"` + sessionID + `","url":"https://checkout.stripe.com/c/pay/test"}`
		case "/v1/subscriptions/" + subscriptionID:
			body = `{"id":"` + subscriptionID + `","customer":"cus_owned","metadata":{"premium_checkout_id":"` + checkoutID + `"}}`
		case "/v1/billing_portal/sessions":
			r.ParseForm()
			if r.Form.Get("customer") != "cus_owned" || r.Form.Get("return_url") != "https://studio.example.com/premium" {
				t.Error("portal accepted client authority")
			}
			body = `{"url":"https://billing.stripe.com/p/session/test"}`
		default:
			t.Errorf("unexpected provider request %s", r.URL.Path)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	first, e := server.startPremiumCheckout(ctx, u, "stable-premium-key")
	if e != nil {
		t.Fatal(e)
	}
	again, e := server.startPremiumCheckout(ctx, u, "stable-premium-key")
	if e != nil || first != again || calls != 1 {
		t.Fatalf("replay %s %d %v", again, calls, e)
	}
	if _, e = server.startPremiumCheckout(ctx, u, "different-premium-key"); e == nil {
		t.Fatal("parallel duplicate subscription allowed")
	}
	_, e = store.DB.ExecContext(ctx, `UPDATE cloud_premium_checkouts SET provider_subscription=$2 WHERE id=$1`, checkoutID, subscriptionID)
	if e != nil {
		t.Fatal(e)
	}
	_, e = store.DB.ExecContext(ctx, `INSERT INTO cloud_premium_subscriptions(user_id,provider_subscription,status,paid_through)VALUES($1,$2,'active',now()+interval '30 days')`, u.ID, subscriptionID)
	if e != nil {
		t.Fatal(e)
	}
	for _, test := range []struct {
		user   User
		status int
	}{{u, 200}, {other, 404}} {
		req := httptest.NewRequest(http.MethodPost, "/premium/manage", strings.NewReader(`{}`)).WithContext(context.WithValue(ctx, userContext{}, test.user))
		response := httptest.NewRecorder()
		server.premiumManage(response, req)
		if response.Code != test.status {
			t.Fatalf("portal owner isolation: %d %s", response.Code, response.Body.String())
		}
	}
}

func TestPremiumReconcilesExpiredLocalCheckoutBeforeAnotherPurchase(t *testing.T) {
	store, u := testPlatform(t)
	ctx := context.Background()
	id := "prior-" + u.ID
	oldSession := "cs_old_" + u.ID
	oldSubscription := "sub_old_" + u.ID
	_, e := store.DB.ExecContext(ctx, `INSERT INTO cloud_premium_checkouts(id,user_id,price_cents,idempotency_key,provider_session,created_at)VALUES($1,$2,900,'old-key',$3,now()-interval '2 hours')`, id, u.ID, oldSession)
	if e != nil {
		t.Fatal(e)
	}
	server := NewServer(store, Config{PremiumPriceCents: 900, StripeSecret: "test"})
	created := 0
	priorExpired := false
	server.HTTP = &http.Client{Transport: commerceRoundTrip(func(r *http.Request) (*http.Response, error) {
		var body string
		switch r.URL.Path {
		case "/v1/checkout/sessions/" + oldSession:
			body = `{"id":"` + oldSession + `","status":"complete","payment_status":"paid","subscription":"` + oldSubscription + `","metadata":{"premium_checkout_id":"` + id + `"}}`
			if priorExpired {
				body = `{"id":"` + oldSession + `","status":"expired","payment_status":"unpaid","metadata":{"premium_checkout_id":"` + id + `"}}`
			}
		case "/v1/subscriptions/" + oldSubscription:
			body = `{"id":"` + oldSubscription + `","status":"active","metadata":{"premium_checkout_id":"` + id + `"}}`
		case "/v1/checkout/sessions":
			created++
			body = `{"id":"cs_new_` + u.ID + `","url":"https://checkout.stripe.com/new"}`
		default:
			t.Errorf("unexpected %s", r.URL.Path)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	if _, e = server.startPremiumCheckout(ctx, u, "new-key-delayed-invoice"); e == nil {
		t.Fatal("delayed invoice allowed second recurring subscription")
	}
	if created != 0 {
		t.Fatal("provider charged through a duplicate subscription")
	}
	// Even an expired local checkout with no provider response is ambiguous.
	store.DB.ExecContext(ctx, `UPDATE cloud_premium_checkouts SET provider_session=NULL WHERE id=$1`, id)
	if _, e = server.startPremiumCheckout(ctx, u, "new-key-ambiguous"); e == nil {
		t.Fatal("ambiguous provider submission was treated as unpaid expiry")
	}
	if _, e = store.DB.ExecContext(ctx, `UPDATE cloud_premium_checkouts SET provider_session=$2 WHERE id=$1`, id, oldSession); e != nil {
		t.Fatal(e)
	}
	priorExpired = true
	if _, e = server.startPremiumCheckout(ctx, u, "new-key-confirmed-unused"); e != nil {
		t.Fatalf("provider-confirmed unpaid expiry should allow a purchase: %v", e)
	}
	if created != 1 {
		t.Fatalf("new provider checkout calls=%d", created)
	}
}
