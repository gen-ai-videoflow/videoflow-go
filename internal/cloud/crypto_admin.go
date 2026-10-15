package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
)

// InspectCryptoPayment reports provider state without converting it into a grant.
// Settlement remains exclusively authorized by the original signed webhook.
func (s *Server) InspectCryptoPayment(ctx context.Context, id, actor string) (map[string]any, error) {
	if strings.TrimSpace(actor) == "" || id == "" {
		return nil, ErrValidation
	}
	var reference string
	if err := s.Store.DB.QueryRowContext(ctx, `SELECT reference FROM cloud_crypto_invoices WHERE id=$1`, id).Scan(&reference); err != nil {
		return nil, err
	}
	var out map[string]any
	status, message, err := s.cryptoCall(ctx, http.MethodGet, "/api/transaction?reference="+url.QueryEscape(reference), nil, &out)
	if err != nil {
		return nil, fmt.Errorf("provider transaction lookup failed (HTTP %d, %s)", status, message)
	}
	if out["reference"] != reference {
		return nil, ErrConflict
	}
	if out["txStatus"] == "failed" {
		if _, err = s.Store.DB.ExecContext(ctx, `UPDATE cloud_crypto_invoices SET status='review' WHERE id=$1 AND status IN ('pending','processing')`, id); err != nil {
			return nil, err
		}
	}
	if _, err = s.Store.DB.ExecContext(ctx, `INSERT INTO cloud_admin_audit(actor,action,resource_id,reason)VALUES($1,'crypto_inspect',$2,$3)`, actor, id, "Provider lookup only; no entitlement granted"); err != nil {
		return nil, err
	}
	return out, nil
}

// RecoverCryptoAddress binds an address found by the operator after an ambiguous
// create response. The gateway must independently confirm the frozen reference.
func (s *Server) RecoverCryptoAddress(ctx context.Context, id, address, actor, reason string) error {
	if strings.TrimSpace(actor) == "" || strings.TrimSpace(reason) == "" || address == "" {
		return ErrValidation
	}
	tx, err := s.Store.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	invoice, err := scanCrypto(tx.QueryRowContext(ctx, `SELECT `+cryptoColumns+` FROM cloud_crypto_invoices WHERE id=$1 FOR UPDATE`, id))
	if err != nil {
		return err
	}
	if invoice.Address != "" || invoice.Status != "review" {
		return ErrConflict
	}
	var provider struct {
		ID                  string `json:"id"`
		Address             string `json:"address"`
		Reference           string `json:"reference"`
		IsWithdrawalAddress bool   `json:"isWithdrawalAddress"`
		VMType              struct {
			Type     string `json:"type"`
			IsActive bool   `json:"isActive"`
		} `json:"vmType"`
	}
	_, _, err = s.cryptoCall(ctx, http.MethodGet, "/api/address?"+url.Values{"address": {address}, "blockchainName": {invoice.Network}, "reference": {invoice.Reference}}.Encode(), nil, &provider)
	if err != nil {
		return err
	}
	if provider.ID == "" || provider.Address != address || provider.Reference != invoice.Reference || provider.IsWithdrawalAddress || provider.VMType.Type != invoice.VMType || !provider.VMType.IsActive {
		return ErrConflict
	}
	if _, err = tx.ExecContext(ctx, `UPDATE cloud_crypto_invoices SET address=$2,gateway_id=$3,status='pending' WHERE id=$1`, id, address, provider.ID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO cloud_admin_audit(actor,action,resource_id,reason)VALUES($1,'crypto_recover_address',$2,$3)`, actor, id, reason); err != nil {
		return err
	}
	return tx.Commit()
}

// ReplayCryptoWebhook passes captured raw bytes and their original signature
// through the exact public verifier. It cannot manufacture a settlement.
func (s *Server) ReplayCryptoWebhook(ctx context.Context, raw []byte, signature, actor string) error {
	if strings.TrimSpace(actor) == "" || len(raw) == 0 || len(raw) > 1<<20 || signature == "" {
		return ErrValidation
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/cryptorails", strings.NewReader(string(raw))).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-cr-webhook-signature", signature)
	rec := httptest.NewRecorder()
	s.Router().ServeHTTP(rec, req)
	if rec.Code < 200 || rec.Code >= 300 {
		return fmt.Errorf("signed webhook replay rejected (HTTP %d)", rec.Code)
	}
	var event struct {
		Reference string `json:"reference"`
	}
	if err := json.Unmarshal(raw, &event); err != nil {
		return err
	}
	_, err := s.Store.DB.ExecContext(ctx, `INSERT INTO cloud_admin_audit(actor,action,resource_id,reason)VALUES($1,'crypto_replay',$2,'Original signed webhook replayed through verifier')`, actor, event.Reference)
	return err
}
