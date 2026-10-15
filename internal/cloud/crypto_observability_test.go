package cloud

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/google/uuid"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCryptoPaymentHealthReportsStaleAndIntegrityWithoutRepair(t *testing.T) {
	server, u, broken, _ := cryptoFixture(t, "credits")
	ctx := context.Background()
	before, e := server.Store.CryptoPaymentHealth(ctx)
	if e != nil {
		t.Fatal(e)
	}
	// The broken settled invoice intentionally has no receipt or grant.
	if _, e = server.Store.DB.ExecContext(ctx, `UPDATE cloud_crypto_invoices SET status='settled' WHERE id=$1`, broken.ID); e != nil {
		t.Fatal(e)
	}
	for _, status := range []string{"pending", "processing", "review"} {
		_, _, v, _ := cryptoFixture(t, "credits")
		if _, e = server.Store.DB.ExecContext(ctx, `UPDATE cloud_crypto_invoices SET status=$2,created_at=now()-interval '61 minutes' WHERE id=$1`, v.ID, status); e != nil {
			t.Fatal(e)
		}
	}
	_, _, pending, event := cryptoFixture(t, "credits")
	if _, e = server.Store.DB.ExecContext(ctx, `INSERT INTO cloud_crypto_settlements(reference,payload_hash,tx_hash,network,gross_amount,provider_fee,net_amount)VALUES($1,'test-health',$2,$3,$4,'0',$4)`, pending.Reference, event.TxHash, pending.Network, event.Amount); e != nil {
		t.Fatal(e)
	}
	actor := "health-test-" + uuid.NewString()
	report, e := server.Store.AuditedCryptoPaymentHealth(ctx, actor)
	if e != nil {
		t.Fatal(e)
	}
	if report.Integrity.SettledWithoutReceipt < before.Integrity.SettledWithoutReceipt+1 || report.Integrity.MissingCreditGrant < before.Integrity.MissingCreditGrant+1 || report.Integrity.ReceiptWithoutSettledInvoice < before.Integrity.ReceiptWithoutSettledInvoice+1 {
		t.Fatalf("missing integrity conditions: before%+v after%+v", before.Integrity, report.Integrity)
	}
	for _, status := range []string{"pending", "processing", "review"} {
		if report.StaleCounts[status] < before.StaleCounts[status]+1 || report.OldestStaleAgeSeconds[status] < 3600 || report.StatusCounts[status] < 1 {
			t.Fatalf("missing stale %s %+v", status, report)
		}
	}
	body, e := json.Marshal(report)
	if e != nil {
		t.Fatal(e)
	}
	for _, secret := range []string{u.Email, u.ID, u.WorkspaceID, broken.ID, broken.Address, server.Config.CryptoRailsAPIKey, server.Config.CryptoRailsWebhookSecret} {
		if secret != "" && bytes.Contains(body, []byte(secret)) {
			t.Fatalf("report leaked identity or secret %q", secret)
		}
	}
	balance, e := server.Store.Credits(ctx, u.WorkspaceID)
	if e != nil || balance.Available != 0 {
		t.Fatal("health report granted credits", balance, e)
	}
	current, e := server.ownedCrypto(ctx, u, broken.ID)
	if e != nil || current.Status != "settled" {
		t.Fatal("health report changed financial state", e)
	}
	var audit int
	if e = server.Store.DB.QueryRowContext(ctx, `SELECT count(*) FROM cloud_admin_audit WHERE actor=$1 AND action='crypto_report' AND resource_id='aggregate'`, actor).Scan(&audit); e != nil || audit != 1 {
		t.Fatal("report audit missing", audit, e)
	}
	if _, e = server.Store.AuditedCryptoPaymentHealth(ctx, ""); e != ErrValidation {
		t.Fatal("anonymous report accepted", e)
	}
}
func TestCryptoWebhookRejectionLogsContainOnlySafeReasons(t *testing.T) {
	cfg := cryptoTestConfig()
	cfg.CryptoRailsWebhookSecret = "private-signing-secret-do-not-log"
	cfg.CryptoRailsAPIKey = "private-api-secret-do-not-log"
	server := NewServer(nil, cfg)
	var output bytes.Buffer
	prior := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	defer slog.SetDefault(prior)
	send := func(body string, signed bool) {
		req := httptest.NewRequest("POST", "/v1/webhooks/cryptorails", strings.NewReader(body))
		if signed {
			m := hmac.New(sha256.New, []byte(server.Config.CryptoRailsWebhookSecret))
			m.Write([]byte(body))
			req.Header.Set("x-cr-webhook-signature", hex.EncodeToString(m.Sum(nil)))
		}
		w := httptest.NewRecorder()
		server.cryptoWebhook(w, req)
		if w.Code < 400 {
			t.Fatal("rejection request accepted")
		}
	}
	send(`{"secret":"sensitive-body-do-not-log"}`, false)
	send(`malformed-sensitive-body-do-not-log`, true)
	send(`{"status":"pending","reference":"sensitive-reference-do-not-log"}`, true)
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("unexpected log count%d", len(lines))
	}
	for i, reason := range []string{"invalid_signature", "malformed_payload", "settlement_rejected"} {
		var entry map[string]any
		if e := json.Unmarshal([]byte(lines[i]), &entry); e != nil {
			t.Fatal(e)
		}
		if entry["reason"] != reason || entry["msg"] != "cryptorails webhook rejected" {
			t.Fatalf("unexpected log%v", entry)
		}
	}
	for _, sensitive := range []string{"sensitive-body", "sensitive-reference", server.Config.CryptoRailsAPIKey, server.Config.CryptoRailsWebhookSecret, "x-cr-webhook-signature"} {
		if strings.Contains(output.String(), sensitive) {
			t.Fatalf("secret in rejection logs%q", sensitive)
		}
	}
}
