package cloud

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestStripePremiumActivationIndependentOfCrypto(t *testing.T) {
	server := NewServer(nil, Config{BillingEnabled: true, StripeSecret: "sk_test", StripeWebhookSecret: "whsec_test", PremiumPriceCents: 900})
	defer server.Close()
	if !server.premiumEnabled() {
		t.Fatal("explicitly configured Stripe Premium must work without crypto")
	}
}

func TestStripePremiumRejectsExistingCryptoAccess(t *testing.T) {
	store, u := testPlatform(t)
	_, err := store.DB.Exec(`INSERT INTO cloud_crypto_premium(user_id,paid_through) VALUES($1,$2)`, u.ID, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(store, Config{BillingEnabled: true, StripeSecret: "test", StripeWebhookSecret: "test", PremiumPriceCents: 900})
	defer server.Close()
	_, err = server.startPremiumCheckout(context.Background(), u, "cross-gateway-test")
	if err != ErrConflict {
		t.Fatalf("active prepaid access should block recurring purchase; got %v", err)
	}
}

func TestPremiumConcurrentGatewaysCreateOnlyOnePurchase(t *testing.T) {
	store, u := testPlatform(t)
	u.Verified = true
	cfg := cryptoTestConfig()
	cfg.StripeSecret = "test"
	cfg.StripeWebhookSecret = "test"
	server := NewServer(store, cfg)
	defer server.Close()
	server.HTTP = &http.Client{Transport: commerceRoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.Method == "GET" {
			return cryptoPolicyResponse(), nil
		}
		body := `{"id":"cs_concurrent_` + u.ID + `","url":"https://checkout.stripe.com/c/pay/concurrent"}`
		if strings.HasSuffix(r.URL.Path, "/api/create-address") {
			var request map[string]string
			json.NewDecoder(r.Body).Decode(&request)
			body = `{"id":"gateway","address":"0xconcurrent` + u.ID + `","reference":"` + request["reference"] + `","vmType":"EVM"}`
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	start := make(chan struct{})
	results := make(chan error, 2)
	go func() {
		<-start
		_, err := server.startPremiumCheckout(context.Background(), u, "concurrent-stripe")
		results <- err
	}()
	go func() {
		<-start
		_, err := server.newCryptoInvoice(context.Background(), u, "premium", "", "concurrent-crypto")
		results <- err
	}()
	close(start)
	successes, conflicts := 0, 0
	for i := 0; i < 2; i++ {
		err := <-results
		if err == nil {
			successes++
		} else if err == ErrConflict {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("got %d successes and %d conflicts", successes, conflicts)
	}
	var purchases int
	if err := store.DB.QueryRow(`SELECT (SELECT count(*) FROM cloud_premium_checkouts WHERE user_id=$1)+(SELECT count(*) FROM cloud_crypto_invoices WHERE user_id=$1)`, u.ID).Scan(&purchases); err != nil || purchases != 1 {
		t.Fatalf("purchase rows=%d err=%v", purchases, err)
	}
}
