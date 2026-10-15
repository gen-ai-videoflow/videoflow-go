package cloud

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestCryptoRecoveryVerifiesReferenceAndAudits(t *testing.T) {
	store, u := testPlatform(t)
	ctx := context.Background()
	id := "recover-" + u.ID
	address := "deposit-" + u.ID
	_, err := store.DB.ExecContext(ctx, `INSERT INTO cloud_crypto_invoices(id,user_id,workspace_id,purpose,pack_id,credits,price_cents,idempotency_key,reference,network,vm_type,token,token_address,treasury_address,amount,consolidate_amount,amount_net,fee_amount,status)VALUES($1,$2,$3,'credits','starter',1000,1000,$1,$1,'base-mainnet','EVM','USDC','token','treasury','10','10','10','0','review')`, id, u.ID, u.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(store, Config{CryptoRailsAPIURL: "https://provider.example/v1", CryptoRailsAPIKey: "secret"})
	defer server.Close()
	ref := "wrong"
	server.HTTP = &http.Client{Transport: commerceRoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("x-tenant-api-key") != "secret" || r.URL.Query().Get("address") != address || r.URL.Query().Get("blockchainName") != "base-mainnet" || r.URL.Query().Get("reference") != id {
			t.Fatalf("wrong recovery lookup %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"id":"address-id","address":"` + address + `","reference":"` + ref + `","vmType":{"type":"EVM","isActive":true},"isWithdrawalAddress":false}`)), Header: make(http.Header)}, nil
	})}
	if err = server.RecoverCryptoAddress(ctx, id, address, "operator", "Recovered in provider admin"); err != ErrConflict {
		t.Fatalf("wrong reference accepted: %v", err)
	}
	ref = id
	if err = server.RecoverCryptoAddress(ctx, id, address, "operator", "Recovered in provider admin"); err != nil {
		t.Fatal(err)
	}
	var storedAddress, status string
	if err = store.DB.QueryRowContext(ctx, `SELECT address,status FROM cloud_crypto_invoices WHERE id=$1`, id).Scan(&storedAddress, &status); err != nil || storedAddress != address || status != "pending" {
		t.Fatalf("recovery %s %s %v", address, status, err)
	}
	var count int
	if err = store.DB.QueryRowContext(ctx, `SELECT count(*) FROM cloud_admin_audit WHERE resource_id=$1 AND action='crypto_recover_address'`, id).Scan(&count); err != nil || count != 1 {
		t.Fatalf("audit %d %v", count, err)
	}
	providerStatus := "confirmed"
	server.HTTP = &http.Client{Transport: commerceRoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1/api/transaction" || r.URL.Query().Get("reference") != id {
			t.Fatalf("wrong inspect lookup %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"reference":"` + id + `","txStatus":"` + providerStatus + `","webhookStatus":"failed"}`))}, nil
	})}
	if _, err = server.InspectCryptoPayment(ctx, id, "operator"); err != nil {
		t.Fatal(err)
	}
	balance, err := store.Credits(ctx, u.WorkspaceID)
	if err != nil || balance.Available != 0 {
		t.Fatalf("inspection granted credits: %+v %v", balance, err)
	}
	providerStatus = "failed"
	if _, err = server.InspectCryptoPayment(ctx, id, "operator"); err != nil {
		t.Fatal(err)
	}
	if err = store.DB.QueryRowContext(ctx, `SELECT status FROM cloud_crypto_invoices WHERE id=$1`, id).Scan(&status); err != nil || status != "review" {
		t.Fatalf("failed sweep not flagged: %s %v", status, err)
	}
	if err = server.RecoverCryptoAddress(ctx, id, "another", "operator", "Attempt to replace"); err != ErrConflict {
		t.Fatalf("replaced bound address: %v", err)
	}
}

func TestCryptoReplayRejectsUnsignedBytes(t *testing.T) {
	store, _ := testPlatform(t)
	server := NewServer(store, Config{CryptoRailsWebhookSecret: "secret"})
	defer server.Close()
	if err := server.ReplayCryptoWebhook(context.Background(), []byte(`{"reference":"fake"}`), strings.Repeat("0", 64), "operator"); err == nil {
		t.Fatal("unsigned replay accepted")
	}
}
