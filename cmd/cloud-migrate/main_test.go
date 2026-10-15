package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"videoflow-go/internal/cloud"
)

func TestRuntimeRoleCannotChangeSchemaOrMigrationRecords(t *testing.T) {
	dsn := os.Getenv("CLOUD_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("PostgreSQL integration DSN required")
	}
	ctx := context.Background()
	store, e := cloud.OpenStore(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	if e = store.Migrate(ctx); e != nil {
		t.Fatal(e)
	}
	role := "runtime_" + uuid.NewString()[:8]
	if e = provisionRuntimeRole(ctx, store, role, "test-runtime-password"); e != nil {
		t.Fatal(e)
	}
	parsed, e := url.Parse(dsn)
	if e != nil {
		t.Fatal(e)
	}
	parsed.User = url.UserPassword(role, "test-runtime-password")
	runtimeStore, e := cloud.OpenStore(ctx, parsed.String())
	if e != nil {
		t.Fatal(e)
	}
	if e = runtimeStore.CheckMigrations(ctx); e != nil {
		t.Fatal(e)
	}
	if e = runtimeStore.CheckRuntimeRole(ctx); e != nil {
		t.Fatal(e)
	}
	user, e := store.Register(ctx, "role-"+role+"@example.com", "Test-password-123!", "Role test", role)
	if e != nil {
		t.Fatal(e)
	}
	if e = runtimeStore.GrantCredits(ctx, user.WorkspaceID, "role-journal-probe", 100, "Role test"); e != nil {
		t.Fatalf("credit trigger: %v", e)
	}
	if _, e = runtimeStore.DB.ExecContext(ctx, `INSERT INTO cloud_earnings(id,user_id,reference,amount_cents,currency,status,description)VALUES($1,$2,$3,100,'usd','pending','Role test')`, uuid.NewString(), user.ID, "role-"+role); e != nil {
		t.Fatalf("money trigger: %v", e)
	}
	if _, e = runtimeStore.DB.ExecContext(ctx, `INSERT INTO cloud_admin_audit(actor,action,resource_id)VALUES('test','dispute_probe',$1)`, role); e != nil {
		t.Fatalf("append-only audit insert: %v", e)
	}
	for _, query := range []string{`UPDATE cloud_credit_entries SET amount=0 WHERE false`, `DELETE FROM cloud_credit_postings WHERE false`, `UPDATE cloud_money_postings SET amount_cents=0 WHERE false`, `DELETE FROM cloud_earnings WHERE false`, `UPDATE cloud_earnings SET amount_cents=0 WHERE false`, `DELETE FROM cloud_premium_refunds WHERE false`, `UPDATE cloud_premium_invoices SET gross_cents=0 WHERE false`, `DELETE FROM cloud_premium_invoices WHERE false`, `UPDATE cloud_crypto_settlements SET net_amount='0' WHERE false`, `DELETE FROM cloud_crypto_settlements WHERE false`, `UPDATE cloud_crypto_invoices SET amount='0' WHERE false`, `UPDATE cloud_crypto_invoices SET credits=0 WHERE false`, `DELETE FROM cloud_crypto_invoices WHERE false`} {
		if _, e = runtimeStore.DB.ExecContext(ctx, query); e == nil {
			t.Fatalf("journal mutation allowed: %s", query)
		}
	}
	if _, e = runtimeStore.DB.ExecContext(ctx, `UPDATE cloud_premium_invoices SET refunded_cents=0 WHERE false`); e != nil {
		t.Fatalf("Premium refund update denied: %v", e)
	}
	if _, e = runtimeStore.DB.ExecContext(ctx, `UPDATE cloud_crypto_invoices SET status='review',last_notify_at=now() WHERE false`); e != nil {
		t.Fatalf("Crypto invoice status update denied: %v", e)
	}
	// Exercise newly migrated security/profile tables with the actual restricted login.
	for _, query := range []string{
		`INSERT INTO cloud_totp(user_id,secret_ciphertext) VALUES($1,'encrypted-fixture')`,
		`INSERT INTO cloud_recovery_codes(user_id,code_hash) VALUES($1,'hashed-fixture')`,
		`INSERT INTO cloud_auth_challenges(token_hash,user_id,purpose) VALUES('role-'||$1,$1,'password_mfa')`,
		`INSERT INTO cloud_passkeys(id,user_id,credential,name) VALUES('role-'||$1,$1,'{}','Fixture')`,
		`UPDATE cloud_channels SET bio='Updated profile' WHERE user_id=$1`,
		`DELETE FROM cloud_auth_challenges WHERE user_id=$1`,
	} {
		if _, e = runtimeStore.DB.ExecContext(ctx, query, user.ID); e != nil {
			t.Fatalf("security/profile runtime grant: %v", e)
		}
	}

	purchaseID := "role-purchase-" + role
	if _, e = store.DB.ExecContext(ctx, `INSERT INTO cloud_purchases(id,workspace_id,pack_id,credits,price_cents,currency,status,payment_intent,idempotency_key)VALUES($1,$2,'test',100,1000,'usd','paid',$3,$1)`, purchaseID, user.WorkspaceID, "pi_"+role); e != nil {
		t.Fatal(e)
	}
	server := cloud.NewServer(runtimeStore, cloud.Config{StripeSecret: "test", StripeWebhookSecret: "webhook-test"})
	server.HTTP = &http.Client{Transport: roleRoundTrip(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"id":"dp_` + role + `","payment_intent":"pi_` + role + `","status":"under_review","amount":1000,"currency":"usd"}`))}, nil
	})}
	body := `{"id":"evt_` + role + `","type":"charge.dispute.created","data":{"object":{"id":"dp_` + role + `"}}}`
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte("webhook-test"))
	mac.Write([]byte(timestamp + "." + body))
	request := httptest.NewRequest(http.MethodPost, "/v1/webhooks/stripe", strings.NewReader(body))
	request.Header.Set("Stripe-Signature", "t="+timestamp+",v1="+hex.EncodeToString(mac.Sum(nil)))
	response := httptest.NewRecorder()
	server.Router().ServeHTTP(response, request)
	if response.Code != 200 {
		t.Fatalf("restricted-role dispute webhook: %d %s", response.Code, response.Body.String())
	}
	cryptoID := "crypto-role-" + role
	if _, e = store.DB.ExecContext(ctx, `INSERT INTO cloud_crypto_invoices(id,user_id,workspace_id,purpose,credits,price_cents,idempotency_key,reference,address,network,vm_type,token,token_address,treasury_address,amount,consolidate_amount,amount_net,fee_amount)VALUES($1,$2,$3,'credits',50,100,$1,$1,$4,'base-mainnet','EVM','USDC','token','treasury','1','1','1','0')`, cryptoID, user.ID, user.WorkspaceID, "deposit-"+role); e != nil {
		t.Fatal(e)
	}
	server.Config.CryptoRailsWebhookSecret = "crypto-role-secret"
	cryptoBody := `{"fromAddress":"deposit-` + role + `","toAddress":"treasury","reference":"` + cryptoID + `","type":"consolidate","status":"confirmed","operationType":"api","txHash":"tx-` + role + `","amount":"1","providerFee":"0","netAmount":"1","tokenName":"USDC","tokenAddress":"token","blockchain":"base-mainnet","transactionCompletedAt":"` + time.Now().UTC().Format(time.RFC3339) + `"}`
	cryptoMAC := hmac.New(sha256.New, []byte("crypto-role-secret"))
	cryptoMAC.Write([]byte(cryptoBody))
	cryptoRequest := httptest.NewRequest(http.MethodPost, "/v1/webhooks/cryptorails", strings.NewReader(cryptoBody))
	cryptoRequest.Header.Set("x-cr-webhook-signature", hex.EncodeToString(cryptoMAC.Sum(nil)))
	cryptoResponse := httptest.NewRecorder()
	server.Router().ServeHTTP(cryptoResponse, cryptoRequest)
	if cryptoResponse.Code != 200 {
		t.Fatalf("restricted-role crypto settlement: %d %s", cryptoResponse.Code, cryptoResponse.Body.String())
	}
	balance, e := runtimeStore.Credits(ctx, user.WorkspaceID)
	if e != nil || balance.Available != 0 || !balance.Restricted || len(balance.Transactions) < 1 || balance.Transactions[0].Amount != 50 {
		t.Fatalf("restricted-role crypto credit grant: %+v %v", balance, e)
	}
	server.Close()
	runtimeStore.Close()
	conn, e := store.DB.Conn(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer conn.Close()
	if _, e = conn.ExecContext(ctx, `SET ROLE `+quoteIdentifier(role)); e != nil {
		t.Fatal(e)
	}
	for _, query := range []string{`CREATE TABLE public.cloud_forbidden(id int)`, `CREATE TEMP TABLE forbidden_temp(id int)`, `UPDATE cloud_schema_migrations SET applied_at=now()`, `UPDATE cloud_payout_batches SET status='paid'`, `UPDATE cloud_admin_audit SET reason='rewritten'`} {
		if _, e = conn.ExecContext(ctx, query); e == nil {
			t.Fatalf("runtime allowed forbidden SQL: %s", query)
		}
	}
	if _, e = conn.ExecContext(ctx, `SELECT id FROM cloud_users LIMIT 0`); e != nil {
		t.Fatal(e)
	}
	if _, e = conn.ExecContext(ctx, `UPDATE cloud_jobs SET updated_at=now() WHERE false`); e != nil {
		t.Fatal(e)
	}
	if _, e = conn.ExecContext(ctx, `SELECT name FROM cloud_schema_migrations LIMIT 1`); e != nil {
		t.Fatal(e)
	}
	conn.ExecContext(ctx, `RESET ROLE`)
	_, e = store.DB.ExecContext(ctx, `DROP OWNED BY `+quoteIdentifier(role))
	if e != nil {
		t.Fatal(e)
	}
	_, e = store.DB.ExecContext(ctx, `DROP ROLE `+quoteIdentifier(role))
	if e != nil {
		t.Fatal(e)
	}
}
func TestRoleNameValidation(t *testing.T) {
	for _, name := range []string{"postgres", "bad-name", "x;DROP ROLE postgres", "a", "runtime"} {
		valid := validRoleName(name)
		if name == "runtime" && !valid {
			t.Fatal(name)
		}
		if name != "runtime" && valid {
			t.Fatalf("accepted %s", name)
		}
	}
}

type roleRoundTrip func(*http.Request) (*http.Response, error)

func (f roleRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
