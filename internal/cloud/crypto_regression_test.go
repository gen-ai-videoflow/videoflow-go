package cloud

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCryptoPremiumDoesNotInheritDisputedLegacyTime(t *testing.T) {
	store, u := testPlatform(t)
	ctx := context.Background()
	id := "held-" + u.ID
	_, err := store.DB.ExecContext(ctx, `INSERT INTO cloud_premium_subscriptions(user_id,provider_subscription,status,paid_through)VALUES($1,$2,'active',now()+interval '200 days')`, u.ID, "sub-"+id)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.DB.ExecContext(ctx, `INSERT INTO cloud_premium_disputes(id,user_id,payment_intent,status,amount_cents,currency)VALUES($1,$2,$1,'under_review',900,'usd')`, id, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.DB.ExecContext(ctx, `INSERT INTO cloud_crypto_invoices(id,user_id,workspace_id,purpose,credits,price_cents,idempotency_key,reference,address,network,vm_type,token,token_address,treasury_address,amount,consolidate_amount,amount_net,fee_amount,status)VALUES($1,$2,$3,'premium',0,900,$1,$1,$4,'bsc-mainnet','EVM','USDT','token','treasury','9','9','9','0','processing')`, id, u.ID, u.WorkspaceID, "deposit-"+id)
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(store, Config{})
	defer server.Close()
	event := cryptoSettlement{FromAddress: "deposit-" + id, ToAddress: "treasury", Reference: id, Type: "consolidate", Status: "confirmed", OperationType: "api", TxHash: "tx-" + id, Amount: "9", ProviderFee: "0", NetAmount: "9", TokenName: "USDT", TokenAddress: "token", Blockchain: "bsc-mainnet", Completed: time.Now().UTC().Format(time.RFC3339)}
	if err = server.settleCrypto(ctx, event); err != nil {
		t.Fatal(err)
	}
	var end time.Time
	if err = store.DB.QueryRowContext(ctx, `SELECT paid_through FROM cloud_crypto_premium WHERE user_id=$1`, u.ID).Scan(&end); err != nil {
		t.Fatal(err)
	}
	_, session, err := store.Login(ctx, u.Email, "Test-password-123!")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/v1/premium", nil)
	req.AddCookie(&http.Cookie{Name: "videoflow_session", Value: session})
	rec := httptest.NewRecorder()
	server.Router().ServeHTTP(rec, req)
	var status struct {
		Active bool      `json:"active"`
		End    time.Time `json:"paid_through"`
	}
	if err = json.Unmarshal(rec.Body.Bytes(), &status); err != nil || !status.Active || !status.End.Equal(end) {
		t.Fatalf("misleading premium expiry: %s %v", rec.Body.String(), err)
	}
	if remaining := time.Until(end); remaining < 29*24*time.Hour || remaining > 31*24*time.Hour {
		t.Fatalf("held legacy time granted through crypto: %s", remaining)
	}
}

func TestCryptoConcurrentSettlementGrantsOnce(t *testing.T) {
	store, u := testPlatform(t)
	ctx := context.Background()
	id := "concurrent-" + u.ID
	_, err := store.DB.ExecContext(ctx, `INSERT INTO cloud_crypto_invoices(id,user_id,workspace_id,purpose,credits,price_cents,idempotency_key,reference,address,network,vm_type,token,token_address,treasury_address,amount,consolidate_amount,amount_net,fee_amount,status)VALUES($1,$2,$3,'credits',1000,1000,$1,$1,$4,'base-mainnet','EVM','USDC','token','treasury','10','10','10','0','processing')`, id, u.ID, u.WorkspaceID, "deposit-"+id)
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(store, Config{})
	defer server.Close()
	event := cryptoSettlement{FromAddress: "deposit-" + id, ToAddress: "treasury", Reference: id, Type: "consolidate", Status: "confirmed", OperationType: "api", TxHash: "tx-" + id, Amount: "10", ProviderFee: "0", NetAmount: "10", TokenName: "USDC", TokenAddress: "token", Blockchain: "base-mainnet", Completed: time.Now().UTC().Format(time.RFC3339)}
	results := make(chan error, 12)
	for i := 0; i < 12; i++ {
		go func() { results <- server.settleCrypto(ctx, event) }()
	}
	for i := 0; i < 12; i++ {
		if err = <-results; err != nil {
			t.Fatal(err)
		}
	}
	balance, err := store.Credits(ctx, u.WorkspaceID)
	if err != nil || balance.Available != 1000 || len(balance.Transactions) != 1 {
		t.Fatalf("duplicate concurrent grant: %+v %v", balance, err)
	}
}
