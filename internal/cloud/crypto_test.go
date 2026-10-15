package cloud

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/go-chi/chi/v5"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func cryptoTestConfig() Config {
	return Config{DevMode: true, BillingEnabled: true, CryptoRailsEnabled: true, CryptoRailsUSDPegAccepted: true, CryptoRailsAPIURL: "https://api.cryptorails.tech/v1", CryptoRailsAPIKey: "secret", CryptoRailsWebhookSecret: "webhook", CryptoRailsNetwork: "bsc-mainnet", CryptoRailsVMType: "EVM", CryptoRailsTokenDecimals: 6, CryptoRailsToken: "USDT", CryptoRailsTokenAddress: "0xtoken", CryptoRailsTreasuryAddress: "0xtreasury", CryptoRailsDepositFixed: "0.10", CryptoRailsDepositFeeBPS: 100, PremiumPriceCents: 900}
}
func newIntSub(a, b *big.Int) *big.Int { return new(big.Int).Sub(a, b) }
func TestCryptoExactQuote(t *testing.T) {
	c := cryptoTestConfig()
	deposit, gross, net, fee := cryptoQuote(c, 1000)
	g, _ := cryptoUnits(gross)
	n, _ := cryptoUnits(net)
	d, _ := cryptoUnits(deposit)
	f, _ := cryptoUnits(fee)
	if newIntSub(d, n).Cmp(f) != 0 || g.Cmp(n) <= 0 {
		t.Fatalf("invalid %s %s %s %s", deposit, gross, net, fee)
	}
	for _, bad := range []string{"-1", "1e3", "NaN", "1.0000000000000000001", "1.", ".1"} {
		if _, e := cryptoUnits(bad); e == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
}
func TestCryptoSettlementVerifiedExactlyOnceAndPremiumRenewal(t *testing.T) {
	store, u := testPlatform(t)
	u.Verified = true
	c := cryptoTestConfig()
	server := NewServer(store, c)
	calls := 0
	server.HTTP = &http.Client{Transport: commerceRoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.Method == "GET" {
			return cryptoPolicyResponse(), nil
		}
		if r.Header.Get("x-tenant-api-key") != "secret" || r.Header.Get("Authorization") != "" {
			t.Fatal("wrong secret transport")
		}
		var request map[string]string
		json.NewDecoder(r.Body).Decode(&request)
		calls++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"id":"gateway","address":"0xdeposit` + request["reference"] + `","reference":"` + request["reference"] + `","vmType":"EVM"}`)), Header: make(http.Header)}, nil
	})}
	ctx := context.Background()
	invoice, e := server.newCryptoInvoice(ctx, u, "credits", "starter", "test-credits-1")
	if e != nil {
		t.Fatal(e)
	}
	again, e := server.newCryptoInvoice(ctx, u, "credits", "starter", "test-credits-1")
	if e != nil || again.ID != invoice.ID || calls != 1 {
		t.Fatalf("idempotency %v %d", e, calls)
	}
	event := cryptoSettlement{FromAddress: invoice.Address, ToAddress: invoice.Treasury, Reference: invoice.Reference, Type: "consolidate", Status: "confirmed", OperationType: "api", TxHash: "0xtx" + invoice.ID, Amount: invoice.ConsolidateAmount, ProviderFee: "0.000000000000000000", NetAmount: invoice.ConsolidateAmount, TokenName: invoice.Token, TokenAddress: invoice.TokenAddress, Blockchain: invoice.Network, Completed: time.Now().UTC().Format(time.RFC3339)}
	body, _ := json.Marshal(event)
	bad := httptest.NewRequest("POST", "/v1/webhooks/cryptorails", strings.NewReader(string(body)))
	w := httptest.NewRecorder()
	server.cryptoWebhook(w, bad)
	if w.Code != 401 {
		t.Fatal(w.Code)
	}
	balance, _ := store.Credits(ctx, u.WorkspaceID)
	if balance.Available != 0 {
		t.Fatal("unsigned grant")
	}
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("POST", "/v1/webhooks/cryptorails", strings.NewReader(string(body)))
		m := hmac.New(sha256.New, []byte(c.CryptoRailsWebhookSecret))
		m.Write(body)
		req.Header.Set("x-cr-webhook-signature", hex.EncodeToString(m.Sum(nil)))
		w = httptest.NewRecorder()
		server.cryptoWebhook(w, req)
		if w.Code != 200 {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
	}
	balance, _ = store.Credits(ctx, u.WorkspaceID)
	if balance.Available != 1000 {
		t.Fatalf("duplicate grant %+v", balance)
	}
	event.NetAmount = "0.100000000000000000"
	if server.settleCrypto(ctx, event) == nil {
		t.Fatal("incorrect settlement accepted")
	}
	for _, key := range []string{"premium-first", "premium-renewal"} {
		p, e := server.newCryptoInvoice(ctx, u, "premium", "", key)
		if e != nil {
			t.Fatal(e)
		}
		event.TxHash = "0x" + p.ID
		event.Reference = p.Reference
		event.FromAddress = p.Address
		event.Amount = p.ConsolidateAmount
		event.NetAmount = p.ConsolidateAmount
		if e = server.settleCrypto(ctx, event); e != nil {
			t.Fatal(e)
		}
	}
	active, e := store.HasPremium(ctx, u.ID)
	if e != nil || !active {
		t.Fatalf("premium %v %v", active, e)
	}
	var end time.Time
	store.DB.QueryRowContext(ctx, `SELECT paid_through FROM cloud_crypto_premium WHERE user_id=$1`, u.ID).Scan(&end)
	if time.Until(end) < 59*24*time.Hour {
		t.Fatal("renewal did not extend")
	}
}
func TestCryptoAmbiguousAddressCreationNeverRetriesWrite(t *testing.T) {
	store, u := testPlatform(t)
	u.Verified = true
	server := NewServer(store, cryptoTestConfig())
	calls := 0
	server.HTTP = &http.Client{Transport: commerceRoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.Method == "GET" {
			return cryptoPolicyResponse(), nil
		}
		calls++
		return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader(`{}`)), Header: make(http.Header)}, nil
	})}
	for i := 0; i < 2; i++ {
		if _, e := server.newCryptoInvoice(context.Background(), u, "credits", "starter", "ambiguous-create"); e == nil {
			t.Fatal("accepted ambiguous address")
		}
	}
	if calls != 1 {
		t.Fatalf("repeated write %d", calls)
	}
}

func cryptoPolicyResponse() *http.Response {
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"data":[{"isActive":true,"depositProviderFeeFixed":"0.10","depositProviderFeePer":"1","tenantConsolidationAddress":"0xtreasury","tokenConfig":{"name":"USDT","decimal":6,"tokenContractAddress":"0xtoken","blockchainName":"bsc-mainnet","isActive":true,"type":"ERC20"}}],"meta":{"hasNextPage":false}}`))}
}
func TestCryptoLivePolicyRejectsStaleFeesAndDisabledAsset(t *testing.T) {
	for _, replacement := range []string{`"depositProviderFeePer":"2"`, `"isActive":false`} {
		server := NewServer(nil, cryptoTestConfig())
		server.HTTP = &http.Client{Transport: commerceRoundTrip(func(r *http.Request) (*http.Response, error) {
			resp := cryptoPolicyResponse()
			b, _ := io.ReadAll(resp.Body)
			body := strings.Replace(string(b), `"depositProviderFeePer":"1"`, replacement, 1)
			if strings.Contains(replacement, "isActive") {
				body = strings.Replace(string(b), `"isActive":true`, replacement, 1)
			}
			resp.Body = io.NopCloser(strings.NewReader(body))
			return resp, nil
		})}
		if server.validateLiveCryptoPolicy(context.Background()) == nil {
			t.Fatal("accepted stale/inactive configuration")
		}
	}
}
func TestCryptoWebhookRejectsWrongAssetAndUnderpayment(t *testing.T) {
	store, u := testPlatform(t)
	u.Verified = true
	server := NewServer(store, cryptoTestConfig())
	server.HTTP = &http.Client{Transport: commerceRoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.Method == "GET" {
			return cryptoPolicyResponse(), nil
		}
		var request map[string]string
		json.NewDecoder(r.Body).Decode(&request)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"id":"gateway","address":"0x` + request["reference"] + `","reference":"` + request["reference"] + `","vmType":{"type":"EVM","isActive":true}}`)), Header: make(http.Header)}, nil
	})}
	v, e := server.newCryptoInvoice(context.Background(), u, "credits", "starter", "wrong-asset-test")
	if e != nil {
		t.Fatal(e)
	}
	_, other := testPlatform(t)
	if _, e = server.ownedCrypto(context.Background(), other, v.ID); e != ErrNotFound {
		t.Fatal("foreign invoice accessible")
	}
	event := cryptoSettlement{FromAddress: v.Address, ToAddress: v.Treasury, Reference: v.Reference, Type: "consolidate", Status: "confirmed", OperationType: "api", TxHash: "0x" + v.ID, Amount: v.ConsolidateAmount, ProviderFee: "0", NetAmount: v.ConsolidateAmount, TokenName: v.Token, TokenAddress: "wrongtoken", Blockchain: v.Network, Completed: time.Now().UTC().Format(time.RFC3339)}
	if e = server.settleCrypto(context.Background(), event); e == nil {
		t.Fatal("wrong token accepted")
	}
	event.TokenAddress = v.TokenAddress
	event.ProviderFee = event.Amount
	event.NetAmount = "0"
	if e = server.settleCrypto(context.Background(), event); e == nil {
		t.Fatal("zero net accepted")
	}
	balance, _ := store.Credits(context.Background(), u.WorkspaceID)
	if balance.Available != 0 {
		t.Fatal("invalid settlement granted funds")
	}
}
func TestCryptoNotifyUsesFrozenServerTermsWithoutGrant(t *testing.T) {
	store, u := testPlatform(t)
	u.Verified = true
	server := NewServer(store, cryptoTestConfig())
	var observed map[string]string
	server.HTTP = &http.Client{Transport: commerceRoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.Method == "GET" {
			return cryptoPolicyResponse(), nil
		}
		var request map[string]string
		if e := json.NewDecoder(r.Body).Decode(&request); e != nil {
			t.Fatal(e)
		}
		if r.URL.Path == "/v1/api/consolidate" {
			observed = request
			return &http.Response{StatusCode: 400, Body: io.NopCloser(strings.NewReader(`{"message":"error.insufficientBalance"}`)), Header: make(http.Header)}, nil
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"id":"gateway","address":"0x` + request["reference"] + `","reference":"` + request["reference"] + `","vmType":"EVM"}`)), Header: make(http.Header)}, nil
	})}
	v, e := server.newCryptoInvoice(context.Background(), u, "credits", "starter", "notify-frozen-terms")
	if e != nil {
		t.Fatal(e)
	}
	req := httptest.NewRequest("POST", "/payments/invoices/"+v.ID+"/notify", strings.NewReader(`{}`))
	ctx := context.WithValue(req.Context(), userContext{}, u)
	route := chi.NewRouteContext()
	route.URLParams.Add("id", v.ID)
	req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, route))
	w := httptest.NewRecorder()
	server.cryptoNotify(w, req)
	if w.Code != 202 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if observed["transactionReference"] != v.Reference || observed["address"] != v.Address || observed["tokenAddress"] != v.TokenAddress || observed["amount"] == "" {
		t.Fatal("non-frozen terms")
	}
	balance, _ := store.Credits(context.Background(), u.WorkspaceID)
	if balance.Available != 0 {
		t.Fatal("notify granted credits")
	}
}
