package cloud

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"io"
	"math"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func cryptoFixture(t *testing.T, purpose string) (*Server, User, CryptoInvoice, cryptoSettlement) {
	t.Helper()
	store, u := testPlatform(t)
	u.Verified = true
	server := NewServer(store, cryptoTestConfig())
	server.HTTP = &http.Client{Transport: commerceRoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.Method == "GET" {
			return cryptoPolicyResponse(), nil
		}
		var request map[string]string
		if e := json.NewDecoder(r.Body).Decode(&request); e != nil {
			return nil, e
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"id":"` + uuid.NewString() + `","address":"0x` + request["reference"] + `","reference":"` + request["reference"] + `","vmType":{"type":"EVM","isActive":true}}`))}, nil
	})}
	pack := ""
	if purpose == "credits" {
		pack = "starter"
	}
	v, e := server.newCryptoInvoice(context.Background(), u, purpose, pack, uuid.NewString())
	if e != nil {
		t.Fatal(e)
	}
	event := cryptoSettlement{FromAddress: v.Address, ToAddress: v.Treasury, Reference: v.Reference, Type: "consolidate", Status: "confirmed", OperationType: "api", TxHash: "0x" + v.ID, Amount: v.ConsolidateAmount, ProviderFee: "0", NetAmount: v.ConsolidateAmount, TokenName: v.Token, TokenAddress: v.TokenAddress, Blockchain: v.Network, Completed: time.Now().UTC().Format(time.RFC3339Nano)}
	return server, u, v, event
}
func signedCryptoRequest(server *Server, event cryptoSettlement) *httptest.ResponseRecorder {
	b, _ := json.Marshal(event)
	m := hmac.New(sha256.New, []byte(server.Config.CryptoRailsWebhookSecret))
	m.Write(b)
	req := httptest.NewRequest("POST", "/v1/webhooks/cryptorails", strings.NewReader(string(b)))
	req.Header.Set("x-cr-webhook-signature", hex.EncodeToString(m.Sum(nil)))
	w := httptest.NewRecorder()
	server.cryptoWebhook(w, req)
	return w
}
func TestCryptoDecimalBoundariesAndJSONStrings(t *testing.T) {
	for _, test := range []struct{ input, want string }{{"0", "0.000000000000000000"}, {"0.000000000000000001", "0.000000000000000001"}, {"999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999", ""}, {"9007199254740993.123456789012345678", "9007199254740993.123456789012345678"}, {"0000001.00000", "1.000000000000000000"}} {
		n, e := cryptoUnits(test.input)
		if test.want == "" {
			if e == nil {
				t.Fatal("accepted oversized decimal")
			}
			continue
		}
		if e != nil || cryptoDecimal(n) != test.want {
			t.Fatalf("%s => %v %v", test.input, n, e)
		}
	}
	for _, bad := range []string{"-0", "+1", "1e18", " 1", "1 ", "", ".", "1..0", "0.0000000000000000001", "NaN", "Infinity", "1,000", "１２"} {
		if _, e := cryptoUnits(bad); e == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
	for _, cents := range []int64{1, 99, 100, 100001, math.MaxInt64} {
		for _, bps := range []int64{0, 1, 3333, 9999} {
			c := cryptoTestConfig()
			c.CryptoRailsDepositFeeBPS = bps
			c.CryptoRailsDepositFixed = "0.00001"
			d, g, n, f := cryptoQuote(c, cents)
			di, _ := cryptoUnits(d)
			gi, _ := cryptoUnits(g)
			ni, _ := cryptoUnits(n)
			fi, _ := cryptoUnits(f)
			expectedTarget := new(big.Int).Mul(big.NewInt(cents), new(big.Int).Exp(big.NewInt(10), big.NewInt(16), nil))
			actualFee := new(big.Int).Mul(gi, big.NewInt(bps))
			actualFee.Add(actualFee, big.NewInt(9999))
			actualFee.Div(actualFee, big.NewInt(10000))
			fixed, _ := cryptoUnits(c.CryptoRailsDepositFixed)
			actualFee.Add(actualFee, fixed)
			if ni.Cmp(expectedTarget) != 0 || new(big.Int).Sub(gi, actualFee).Cmp(ni) < 0 || di.Cmp(new(big.Int).Add(gi, actualFee)) < 0 || new(big.Int).Sub(di, ni).Cmp(fi) != 0 {
				t.Fatalf("unsafe rounding cents%d bps%d: %s/%s/%s/%s", cents, bps, d, g, n, f)
			}
			quantum := new(big.Int).Exp(big.NewInt(10), big.NewInt(13), nil)
			if new(big.Int).Mod(di, quantum).Sign() != 0 || new(big.Int).Mod(gi, quantum).Sign() != 0 {
				t.Fatal("unpayable precision")
			}
		}
	}
	payload := CryptoInvoice{Amount: "9007199254740993.123456789012345678", AmountNet: "0.000000000000000001", FeeAmount: "0.00001"}
	b, e := json.Marshal(payload)
	if e != nil {
		t.Fatal(e)
	}
	var decoded map[string]any
	if json.Unmarshal(b, &decoded) != nil {
		t.Fatal("invalid JSON")
	}
	for _, key := range []string{"amount", "amount_net", "fee_amount"} {
		if _, ok := decoded[key].(string); !ok {
			t.Fatal("money serialized as numeric", key)
		}
	}
	var event cryptoSettlement
	if json.Unmarshal([]byte(`{"amount":9007199254740993}`), &event) == nil {
		t.Fatal("numeric webhook amount accepted")
	}
}
func TestCryptoSignedSettlementMutationMatrix(t *testing.T) {
	server, u, v, valid := cryptoFixture(t, "credits")
	cases := []struct {
		name   string
		mutate func(*cryptoSettlement)
	}{
		{"source", func(e *cryptoSettlement) { e.FromAddress = "0xwrong" }}, {"destination", func(e *cryptoSettlement) { e.ToAddress = "0xwrong" }}, {"contract", func(e *cryptoSettlement) { e.TokenAddress = "0xwrong" }}, {"chain", func(e *cryptoSettlement) { e.Blockchain = "other-chain" }}, {"token", func(e *cryptoSettlement) { e.TokenName = "ETH" }}, {"manual operation", func(e *cryptoSettlement) { e.OperationType = "manual" }}, {"withdrawal", func(e *cryptoSettlement) { e.Type = "withdraw" }}, {"unknown reference", func(e *cryptoSettlement) { e.Reference = uuid.NewString() }}, {"missing reference", func(e *cryptoSettlement) { e.Reference = "" }}, {"gross", func(e *cryptoSettlement) { e.Amount = "1"; e.NetAmount = "1" }}, {"negative fee", func(e *cryptoSettlement) { e.ProviderFee = "-1" }}, {"fee exceeds gross", func(e *cryptoSettlement) { e.ProviderFee = "999" }}, {"underfunded consistent net", func(e *cryptoSettlement) {
			e.NetAmount = "0.000000000000000001"
			g, _ := cryptoUnits(e.Amount)
			e.ProviderFee = cryptoDecimal(new(big.Int).Sub(g, big.NewInt(1)))
		}}, {"inconsistent net", func(e *cryptoSettlement) { e.NetAmount = "10" }}, {"pending", func(e *cryptoSettlement) { e.Status = "pending" }}, {"failed", func(e *cryptoSettlement) { e.Status = "failed" }}, {"missing tx", func(e *cryptoSettlement) { e.TxHash = "" }}, {"bad timestamp", func(e *cryptoSettlement) { e.Completed = "tomorrow" }}, {"provider error", func(e *cryptoSettlement) { s := "failed"; e.ErrorMessage = &s }}, {"overprecision", func(e *cryptoSettlement) { e.Amount += "1" }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			event := valid
			test.mutate(&event)
			w := signedCryptoRequest(server, event)
			if w.Code < 400 {
				t.Fatalf("accepted mutation: %d %s", w.Code, w.Body.String())
			}
			var count int
			if e := server.Store.DB.QueryRow(`SELECT count(*) FROM cloud_crypto_settlements WHERE reference=$1`, v.Reference).Scan(&count); e != nil || count != 0 {
				t.Fatal("rejected event left settlement", e, count)
			}
		})
	}
	balance, e := server.Store.Credits(context.Background(), u.WorkspaceID)
	if e != nil || balance.Available != 0 {
		t.Fatal("mutation granted credits", e, balance)
	}
	if w := signedCryptoRequest(server, valid); w.Code != 200 {
		t.Fatalf("valid settlement rejected %d %s", w.Code, w.Body.String())
	}
	// A delayed failed notification cannot undo a committed paid receipt.
	stale := valid
	stale.Status = "failed"
	if w := signedCryptoRequest(server, stale); w.Code < 400 {
		t.Fatal("stale failure accepted")
	}
	balance, _ = server.Store.Credits(context.Background(), u.WorkspaceID)
	current, _ := server.ownedCrypto(context.Background(), u, v.ID)
	if balance.Available != 1000 || current.Status != "settled" {
		t.Fatal("stale failure changed settled invoice")
	}
}
func TestCryptoAtomicCommitFailureAndLostResponseReplay(t *testing.T) {
	for _, purpose := range []string{"credits", "premium"} {
		t.Run(purpose, func(t *testing.T) {
			server, u, v, event := cryptoFixture(t, purpose)
			ctx := context.Background()
			name := "crypto_fault_" + strings.ReplaceAll(v.ID, "-", "")
			function := fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.id='%s' AND NEW.status='settled' THEN RAISE EXCEPTION 'injected failure at deferred commit'; END IF; RETURN NEW; END $$`, name, v.ID)
			if _, e := server.Store.DB.ExecContext(ctx, function); e != nil {
				t.Fatal(e)
			}
			defer server.Store.DB.ExecContext(ctx, `DROP FUNCTION `+name+`() CASCADE`)
			if _, e := server.Store.DB.ExecContext(ctx, fmt.Sprintf(`CREATE CONSTRAINT TRIGGER %s AFTER UPDATE ON cloud_crypto_invoices DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION %s()`, name, name)); e != nil {
				t.Fatal(e)
			}
			if e := server.settleCrypto(ctx, event); e == nil {
				t.Fatal("commit fault did not fail")
			}
			var receipts int
			server.Store.DB.QueryRowContext(ctx, `SELECT count(*) FROM cloud_crypto_settlements WHERE reference=$1`, v.Reference).Scan(&receipts)
			balance, _ := server.Store.Credits(ctx, u.WorkspaceID)
			active, _ := server.Store.HasPremium(ctx, u.ID)
			current, _ := server.ownedCrypto(ctx, u, v.ID)
			if receipts != 0 || balance.Available != 0 || active || current.Status != "pending" {
				t.Fatalf("non-atomic rollback receipt%d balance%v active%v status%s", receipts, balance, active, current.Status)
			}
			if _, e := server.Store.DB.ExecContext(ctx, `DROP TRIGGER `+name+` ON cloud_crypto_invoices`); e != nil {
				t.Fatal(e)
			}
			if e := server.settleCrypto(ctx, event); e != nil {
				t.Fatal(e)
			}
			var endBefore time.Time
			if purpose == "premium" {
				server.Store.DB.QueryRowContext(ctx, `SELECT paid_through FROM cloud_crypto_premium WHERE user_id=$1`, u.ID).Scan(&endBefore)
			}
			// New application instance simulates a successful commit whose response was lost.
			restarted := NewServer(server.Store, server.Config)
			if e := restarted.settleCrypto(ctx, event); e != nil {
				t.Fatal(e)
			}
			server.Store.DB.QueryRowContext(ctx, `SELECT count(*) FROM cloud_crypto_settlements WHERE reference=$1`, v.Reference).Scan(&receipts)
			balance, _ = server.Store.Credits(ctx, u.WorkspaceID)
			if receipts != 1 {
				t.Fatal("replay duplicated receipt")
			}
			if purpose == "credits" && balance.Available != 1000 {
				t.Fatal("replay duplicated credits")
			}
			if purpose == "premium" {
				var endAfter time.Time
				server.Store.DB.QueryRowContext(ctx, `SELECT paid_through FROM cloud_crypto_premium WHERE user_id=$1`, u.ID).Scan(&endAfter)
				if !endBefore.Equal(endAfter) || balance.Available != 0 {
					t.Fatal("replay changed premium or credits")
				}
			}
		})
	}
}
func TestCryptoPurposeSubstitutionCannotReuseInvoice(t *testing.T) {
	server, u, v, event := cryptoFixture(t, "credits")
	var key string
	if e := server.Store.DB.QueryRow(`SELECT idempotency_key FROM cloud_crypto_invoices WHERE id=$1`, v.ID).Scan(&key); e != nil {
		t.Fatal(e)
	}
	if _, e := server.newCryptoInvoice(context.Background(), u, "premium", "", key); e != ErrConflict {
		t.Fatal("purpose substitution accepted", e)
	}
	if e := server.settleCrypto(context.Background(), event); e != nil {
		t.Fatal(e)
	}
	active, _ := server.Store.HasPremium(context.Background(), u.ID)
	if active {
		t.Fatal("credit invoice granted premium")
	}
}

func TestCryptoDatabaseDisconnectBeforeCommitIsRecoverable(t *testing.T) {
	server, u, v, event := cryptoFixture(t, "credits")
	ctx := context.Background()
	name := "crypto_disconnect_" + strings.ReplaceAll(v.ID, "-", "")
	if _, e := server.Store.DB.ExecContext(ctx, fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.id='%s' AND NEW.status='settled' THEN PERFORM pg_terminate_backend(pg_backend_pid()); END IF; RETURN NEW; END $$`, name, v.ID)); e != nil {
		t.Fatal(e)
	}
	defer server.Store.DB.ExecContext(ctx, `DROP FUNCTION `+name+`() CASCADE`)
	if _, e := server.Store.DB.ExecContext(ctx, fmt.Sprintf(`CREATE CONSTRAINT TRIGGER %s AFTER UPDATE ON cloud_crypto_invoices DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION %s()`, name, name)); e != nil {
		t.Fatal(e)
	}
	if e := server.settleCrypto(ctx, event); e == nil {
		t.Fatal("connection loss did not fail commit")
	}
	var receipts int
	if e := server.Store.DB.QueryRowContext(ctx, `SELECT count(*) FROM cloud_crypto_settlements WHERE reference=$1`, v.Reference).Scan(&receipts); e != nil {
		t.Fatal(e)
	}
	balance, e := server.Store.Credits(ctx, u.WorkspaceID)
	if e != nil || receipts != 0 || balance.Available != 0 {
		t.Fatalf("disconnect left partial state %d %+v %v", receipts, balance, e)
	}
	if _, e = server.Store.DB.ExecContext(ctx, `DROP TRIGGER `+name+` ON cloud_crypto_invoices`); e != nil {
		t.Fatal(e)
	}
	restarted := NewServer(server.Store, server.Config)
	if e = restarted.settleCrypto(ctx, event); e != nil {
		t.Fatal(e)
	}
	if e = restarted.settleCrypto(ctx, event); e != nil {
		t.Fatal(e)
	}
	balance, e = server.Store.Credits(ctx, u.WorkspaceID)
	if e != nil || balance.Available != 1000 {
		t.Fatal("recovery not exactly once", balance, e)
	}
}
