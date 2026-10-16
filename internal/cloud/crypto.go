package cloud

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// All monetary calculations use integer 18-decimal token units, never floats.
var cryptoScale = new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)

func cryptoUnits(v string) (*big.Int, error) {
	if len(v) > 90 || v == "" {
		return nil, ErrValidation
	}
	p := strings.Split(v, ".")
	if len(p) > 2 || p[0] == "" {
		return nil, ErrValidation
	}
	f := ""
	if len(p) == 2 {
		f = p[1]
		if f == "" || len(f) > 18 {
			return nil, ErrValidation
		}
	}
	for _, c := range p[0] + f {
		if c < '0' || c > '9' {
			return nil, ErrValidation
		}
	}
	n, ok := new(big.Int).SetString(p[0]+f+strings.Repeat("0", 18-len(f)), 10)
	if !ok {
		return nil, ErrValidation
	}
	return n, nil
}
func cryptoDecimal(n *big.Int) string {
	s := n.String()
	if len(s) <= 18 {
		s = strings.Repeat("0", 19-len(s)) + s
	}
	return s[:len(s)-18] + "." + s[len(s)-18:]
}
func (c Config) validateCryptoRails() error {
	u, e := url.Parse(c.CryptoRailsAPIURL)
	if e != nil || u.Host == "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil || u.Scheme != "https" && !(c.DevMode && u.Scheme == "http") {
		return fmt.Errorf("invalid CRYPTORAILS_API_URL")
	}
	if c.CryptoRailsTokenDecimals < 5 || c.CryptoRailsTokenDecimals > 18 || !c.CryptoRailsUSDPegAccepted || c.CryptoRailsAPIKey == "" || c.CryptoRailsWebhookSecret == "" || c.CryptoRailsNetwork == "" || c.CryptoRailsTokenAddress == "" || c.CryptoRailsTreasuryAddress == "" || (c.CryptoRailsVMType != "EVM" && c.CryptoRailsVMType != "TRON") || (c.CryptoRailsToken != "USDC" && c.CryptoRailsToken != "USDT") || c.CryptoRailsDepositFeeBPS < 0 || c.CryptoRailsDepositFeeBPS >= 10000 {
		return fmt.Errorf("CryptoRails requires explicit stablecoin, chain, fee, treasury and secret configuration")
	}
	_, e = cryptoUnits(c.CryptoRailsDepositFixed)
	if p := strings.Split(c.CryptoRailsDepositFixed, "."); len(p) == 2 && len(p[1]) > 5 {
		return fmt.Errorf("CRYPTORAILS_DEPOSIT_FIXED supports at most five decimals")
	}
	return e
}
func (s *Server) cryptoEnabled() bool {
	return s.Config.BillingEnabled && s.Config.CryptoRailsEnabled && s.Config.validateCryptoRails() == nil
}

type CryptoInvoice struct {
	ID                string    `json:"id"`
	Purpose           string    `json:"purpose"`
	PackID            string    `json:"pack_id"`
	Credits           int64     `json:"credits"`
	PriceCents        int64     `json:"price_cents"`
	Reference         string    `json:"-"`
	Address           string    `json:"address"`
	Network           string    `json:"network"`
	VMType            string    `json:"-"`
	Token             string    `json:"token"`
	TokenAddress      string    `json:"token_address"`
	Treasury          string    `json:"-"`
	Amount            string    `json:"amount"`
	ConsolidateAmount string    `json:"-"`
	AmountNet         string    `json:"amount_net"`
	FeeAmount         string    `json:"fee_amount"`
	Status            string    `json:"status"`
	CreatedAt         time.Time `json:"created_at"`
	ExpiresAt         time.Time `json:"expires_at"`
}

const cryptoColumns = `id,purpose,pack_id,credits,price_cents,reference,address,network,vm_type,token,token_address,treasury_address,amount,consolidate_amount,amount_net,fee_amount,status,created_at,expires_at`

func scanCrypto(row interface{ Scan(...any) error }) (v CryptoInvoice, e error) {
	e = row.Scan(&v.ID, &v.Purpose, &v.PackID, &v.Credits, &v.PriceCents, &v.Reference, &v.Address, &v.Network, &v.VMType, &v.Token, &v.TokenAddress, &v.Treasury, &v.Amount, &v.ConsolidateAmount, &v.AmountNet, &v.FeeAmount, &v.Status, &v.CreatedAt, &v.ExpiresAt)
	if e == sql.ErrNoRows {
		e = ErrNotFound
	}
	return
}
func (s *Server) cryptoRoutes(r chi.Router) {
	r.Get("/payments/options", func(w http.ResponseWriter, r *http.Request) {
		s.json(w, 200, map[string]any{"enabled": s.cryptoEnabled() || s.stripeEnabled(), "stripe_enabled": s.stripeEnabled(), "cryptorails_enabled": s.cryptoEnabled(), "network": s.Config.CryptoRailsNetwork, "token": s.Config.CryptoRailsToken, "token_address": s.Config.CryptoRailsTokenAddress, "credit_packs": Packs, "premium_price_cents": s.Config.PremiumPriceCents, "premium_days": 30})
	})
	r.Group(func(r chi.Router) {
		r.Use(s.requireUser, s.mutationLimits)
		r.Post("/payments/invoices", s.cryptoCreate)
		r.Get("/payments/invoices", s.cryptoList)
		r.Get("/payments/invoices/{id}", s.cryptoGet)
		r.Post("/payments/invoices/{id}/notify", s.cryptoNotify)
	})
}
func (s *Server) cryptoCall(ctx context.Context, method, path string, body any, out any) (int, string, error) {
	if s.cryptoCoolingDown(ctx) {
		return http.StatusTooManyRequests, "", ErrUnavailable
	}
	var data io.Reader
	if body != nil {
		b, e := json.Marshal(body)
		if e != nil {
			return 0, "", e
		}
		data = strings.NewReader(string(b))
	}
	req, e := http.NewRequestWithContext(ctx, method, strings.TrimRight(s.Config.CryptoRailsAPIURL, "/")+path, data)
	if e != nil {
		return 0, "", e
	}
	req.Header.Set("x-tenant-api-key", s.Config.CryptoRailsAPIKey)
	req.Header.Set("Content-Type", "application/json")
	// Provider redirects are never followed with tenant credentials attached.
	client := *s.HTTP
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	resp, e := client.Do(req)
	if e != nil {
		return 0, "", ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		s.cryptoRecordCooldown(ctx, resp.Header.Get("Retry-After"))
	}
	b, e := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if e != nil {
		return resp.StatusCode, "", ErrUnavailable
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var v struct {
			Message string `json:"message"`
		}
		json.Unmarshal(b, &v)
		return resp.StatusCode, v.Message, ErrUnavailable
	}
	if out != nil && json.Unmarshal(b, out) != nil {
		return resp.StatusCode, "", ErrUnavailable
	}
	return resp.StatusCode, "", nil
}
func cryptoQuote(c Config, cents int64) (deposit, gross, net, fee string) {
	target := new(big.Int).Mul(big.NewInt(cents), new(big.Int).Div(new(big.Int).Set(cryptoScale), big.NewInt(100)))
	fixed, _ := cryptoUnits(c.CryptoRailsDepositFixed)
	numerator := new(big.Int).Mul(new(big.Int).Add(target, fixed), big.NewInt(10000))
	den := big.NewInt(10000 - c.CryptoRailsDepositFeeBPS)
	g := new(big.Int).Div(new(big.Int).Add(numerator, new(big.Int).Sub(den, big.NewInt(1))), den)
	// Gateway's consolidation request permits at most five decimal places.
	quantum := new(big.Int).Exp(big.NewInt(10), big.NewInt(13), nil)
	g.Div(new(big.Int).Add(g, new(big.Int).Sub(quantum, big.NewInt(1))), quantum)
	g.Mul(g, quantum)
	f := new(big.Int).Mul(g, big.NewInt(c.CryptoRailsDepositFeeBPS))
	f.Add(f, big.NewInt(9999))
	f.Div(f, big.NewInt(10000))
	f.Add(f, fixed)
	d := new(big.Int).Add(g, f)
	d.Div(new(big.Int).Add(d, new(big.Int).Sub(quantum, big.NewInt(1))), quantum)
	d.Mul(d, quantum)
	return cryptoDecimal(d), cryptoDecimal(g), cryptoDecimal(target), cryptoDecimal(new(big.Int).Sub(d, target))
}
func (s *Server) cryptoCreate(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Purpose string `json:"purpose"`
		PackID  string `json:"pack_id"`
	}
	if !s.decode(w, r, &b) {
		return
	}
	v, e := s.newCryptoInvoice(r.Context(), currentUser(r), b.Purpose, b.PackID, r.Header.Get("Idempotency-Key"))
	s.result(w, v, e)
}
func (s *Server) newCryptoInvoice(ctx context.Context, u User, purpose, pack, key string) (CryptoInvoice, error) {
	var empty CryptoInvoice
	if !s.cryptoEnabled() {
		return empty, ErrUnavailable
	}
	if !u.Verified {
		return empty, fmt.Errorf("%w: verify your email before purchasing", ErrValidation)
	}
	if len(key) < 8 || len(key) > 200 {
		return empty, ErrValidation
	}
	var credits, price int64
	switch purpose {
	case "credits":
		p, e := findPack(pack)
		if e != nil {
			return empty, e
		}
		credits, price = p.Credits, p.PriceCents
	case "premium":
		if pack != "" || s.Config.PremiumPriceCents < 100 || s.Config.PremiumPriceCents > 100000 {
			return empty, ErrValidation
		}
		price = s.Config.PremiumPriceCents
	default:
		return empty, ErrValidation
	}
	var prior string
	priorErr := s.Store.DB.QueryRowContext(ctx, `SELECT id FROM cloud_crypto_invoices WHERE user_id=$1 AND idempotency_key=$2`, u.ID, key).Scan(&prior)
	if priorErr != nil && priorErr != sql.ErrNoRows {
		return empty, priorErr
	}
	if priorErr == sql.ErrNoRows {
		if e := s.validateLiveCryptoPolicy(ctx); e != nil {
			return empty, e
		}
	}
	amount, gross, net, fee := cryptoQuote(s.Config, price)
	id := uuid.NewString()
	insertTx, e := s.Store.DB.BeginTx(ctx, nil)
	if e != nil {
		return empty, e
	}
	defer insertTx.Rollback()
	if purpose == "premium" {
		if _, e = insertTx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,814037229))`, u.ID); e != nil {
			return empty, e
		}
		// A completed/ambiguous recurring checkout must be reconciled before switching rails.
		if e = s.reconcilePriorPremiumCheckouts(ctx, insertTx, u.ID, "crypto:"+key); e != nil {
			return empty, e
		}
	}
	_, e = insertTx.ExecContext(ctx, `INSERT INTO cloud_crypto_invoices(id,user_id,workspace_id,purpose,pack_id,credits,price_cents,idempotency_key,reference,network,vm_type,token,token_address,treasury_address,amount,consolidate_amount,amount_net,fee_amount)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$1,$9,$10,$11,$12,$13,$14,$15,$16,$17) ON CONFLICT(user_id,idempotency_key)DO NOTHING`, id, u.ID, u.WorkspaceID, purpose, pack, credits, price, key, s.Config.CryptoRailsNetwork, s.Config.CryptoRailsVMType, s.Config.CryptoRailsToken, s.Config.CryptoRailsTokenAddress, s.Config.CryptoRailsTreasuryAddress, amount, gross, net, fee)
	if e != nil {
		return empty, e
	}
	if e = insertTx.Commit(); e != nil {
		return empty, e
	}
	tx, e := s.Store.DB.BeginTx(ctx, nil)
	if e != nil {
		return empty, e
	}
	defer tx.Rollback()
	v, e := scanCrypto(tx.QueryRowContext(ctx, `SELECT `+cryptoColumns+` FROM cloud_crypto_invoices WHERE user_id=$1 AND idempotency_key=$2 FOR UPDATE`, u.ID, key))
	if e != nil {
		return empty, e
	}
	if v.Purpose != purpose || v.PackID != pack {
		return empty, ErrConflict
	}
	if v.Address == "" {
		var addr struct {
			ID, Address, Reference string
			VMType                 json.RawMessage
			IsWithdrawalAddress    bool
		}
		if v.Status == "review" {
			return empty, ErrConflict
		}
		// Persist the ambiguous-write fence before requesting a fresh provider address.
		if _, e = tx.ExecContext(ctx, `UPDATE cloud_crypto_invoices SET status='review' WHERE id=$1`, v.ID); e != nil {
			return empty, e
		}
		if e = tx.Commit(); e != nil {
			return empty, e
		}
		tx, e = s.Store.DB.BeginTx(ctx, nil)
		if e != nil {
			return empty, e
		}
		defer tx.Rollback()
		_, _, err := s.cryptoCall(ctx, "POST", "/api/create-address", map[string]string{"vmType": v.VMType, "reference": v.Reference, "name": "VideoFlow invoice"}, &addr)
		if err != nil {
			return empty, err
		}
		if addr.ID == "" || addr.Address == "" || addr.Reference != v.Reference || cryptoVMType(addr.VMType) != v.VMType || addr.IsWithdrawalAddress {
			return empty, ErrUnavailable
		}
		v.Address = addr.Address
		v.Status = "pending"
		_, e = tx.ExecContext(ctx, `UPDATE cloud_crypto_invoices SET address=$2,gateway_id=$3,status='pending' WHERE id=$1`, v.ID, v.Address, addr.ID)
		if e != nil {
			return empty, e
		}
	}
	return v, tx.Commit()
}
func (s *Server) cryptoList(w http.ResponseWriter, r *http.Request) {
	rows, e := s.Store.DB.QueryContext(r.Context(), `SELECT `+cryptoColumns+` FROM cloud_crypto_invoices WHERE user_id=$1 ORDER BY created_at DESC LIMIT 100`, currentUser(r).ID)
	if e != nil {
		s.fail(w, e)
		return
	}
	defer rows.Close()
	list := []CryptoInvoice{}
	for rows.Next() {
		v, e := scanCrypto(rows)
		if e != nil {
			s.fail(w, e)
			return
		}
		list = append(list, v)
	}
	s.result(w, list, rows.Err())
}
func (s *Server) ownedCrypto(ctx context.Context, u User, id string) (CryptoInvoice, error) {
	return scanCrypto(s.Store.DB.QueryRowContext(ctx, `SELECT `+cryptoColumns+` FROM cloud_crypto_invoices WHERE id=$1 AND user_id=$2`, id, u.ID))
}
func (s *Server) cryptoGet(w http.ResponseWriter, r *http.Request) {
	v, e := s.ownedCrypto(r.Context(), currentUser(r), chi.URLParam(r, "id"))
	s.result(w, v, e)
}
func (s *Server) cryptoNotify(w http.ResponseWriter, r *http.Request) {
	var body struct{}
	if !s.decode(w, r, &body) {
		return
	}
	if !s.cryptoEnabled() {
		s.fail(w, ErrUnavailable)
		return
	}
	u := currentUser(r)
	if !s.rate(w, r, "crypto-notify:"+u.ID, 6, time.Minute, true) {
		return
	}
	v, e := s.ownedCrypto(r.Context(), u, chi.URLParam(r, "id"))
	if e != nil {
		s.fail(w, e)
		return
	}
	if v.Status == "settled" {
		s.json(w, 200, v)
		return
	}
	if v.Address == "" {
		s.fail(w, ErrConflict)
		return
	}
	res, e := s.Store.DB.ExecContext(r.Context(), `UPDATE cloud_crypto_invoices SET last_notify_at=now() WHERE id=$1 AND (last_notify_at IS NULL OR last_notify_at<now()-interval '10 seconds') AND status IN ('pending','processing')`, v.ID)
	if e != nil {
		s.fail(w, e)
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		s.json(w, 202, v)
		return
	}
	status, message, e := s.cryptoCall(r.Context(), "POST", "/api/consolidate", map[string]any{"address": v.Address, "amount": strings.TrimRight(strings.TrimRight(v.ConsolidateAmount, "0"), "."), "transactionReference": v.Reference, "blockchainName": v.Network, "tokenAddress": v.TokenAddress}, nil)
	if e != nil && !(status == 400 && message == "error.insufficientBalance") && !(status == 409 && (message == "error.transactionAlreadyExist" || message == "error.consolidationIsProcessing")) {
		s.fail(w, ErrUnavailable)
		return
	}
	if e == nil || status == 409 {
		_, e = s.Store.DB.ExecContext(r.Context(), `UPDATE cloud_crypto_invoices SET status='processing' WHERE id=$1 AND status='pending'`, v.ID)
		if e != nil {
			s.fail(w, e)
			return
		}
	}
	v, e = s.ownedCrypto(r.Context(), u, v.ID)
	if e != nil {
		s.fail(w, e)
		return
	}
	s.json(w, 202, v)
}

type cryptoSettlement struct {
	FromAddress   string  `json:"fromAddress"`
	ToAddress     string  `json:"toAddress"`
	Reference     string  `json:"reference"`
	Type          string  `json:"type"`
	Status        string  `json:"status"`
	OperationType string  `json:"operationType"`
	TxHash        string  `json:"txHash"`
	Amount        string  `json:"amount"`
	ProviderFee   string  `json:"providerFee"`
	NetAmount     string  `json:"netAmount"`
	TokenName     string  `json:"tokenName"`
	TokenAddress  string  `json:"tokenAddress"`
	Blockchain    string  `json:"blockchain"`
	Completed     string  `json:"transactionCompletedAt"`
	ErrorMessage  *string `json:"errorMessage"`
}

func (s *Server) cryptoWebhook(w http.ResponseWriter, r *http.Request) {
	if s.Config.CryptoRailsWebhookSecret == "" {
		s.fail(w, ErrUnavailable)
		return
	}
	b, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if e != nil {
		slog.Warn("cryptorails webhook rejected", "reason", "malformed_payload")
		s.fail(w, ErrValidation)
		return
	}
	sig, e := hex.DecodeString(r.Header.Get("x-cr-webhook-signature"))
	m := hmac.New(sha256.New, []byte(s.Config.CryptoRailsWebhookSecret))
	m.Write(b)
	if e != nil || !hmac.Equal(sig, m.Sum(nil)) {
		slog.Warn("cryptorails webhook rejected", "reason", "invalid_signature")
		s.fail(w, ErrUnauthorized)
		return
	}
	var event cryptoSettlement
	if json.Unmarshal(b, &event) != nil {
		slog.Warn("cryptorails webhook rejected", "reason", "malformed_payload")
		s.fail(w, ErrValidation)
		return
	}
	e = s.settleCrypto(r.Context(), event)
	if e != nil {
		slog.Warn("cryptorails webhook rejected", "reason", "settlement_rejected")
	}
	s.result(w, map[string]bool{"ok": true}, e)
}
func (s *Server) settleCrypto(ctx context.Context, event cryptoSettlement) error {
	if event.Type != "consolidate" || event.Status != "confirmed" || event.OperationType != "api" || event.TxHash == "" || event.Completed == "" || event.ErrorMessage != nil && *event.ErrorMessage != "" {
		return ErrValidation
	}
	if _, e := time.Parse(time.RFC3339Nano, event.Completed); e != nil {
		return ErrValidation
	}
	gross, e := cryptoUnits(event.Amount)
	if e != nil {
		return e
	}
	fee, e := cryptoUnits(event.ProviderFee)
	if e != nil {
		return e
	}
	net, e := cryptoUnits(event.NetAmount)
	if e != nil || net.Sign() <= 0 || new(big.Int).Sub(gross, fee).Cmp(net) != 0 {
		return ErrValidation
	}
	tx, e := s.Store.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	v, e := scanCrypto(tx.QueryRowContext(ctx, `SELECT `+cryptoColumns+` FROM cloud_crypto_invoices WHERE reference=$1 FOR UPDATE`, event.Reference))
	if e != nil {
		return e
	}
	equal := func(a, b string) bool {
		if v.VMType == "EVM" {
			return strings.EqualFold(a, b)
		}
		return a == b
	}
	if !equal(event.FromAddress, v.Address) || !equal(event.ToAddress, v.Treasury) || !equal(event.TokenAddress, v.TokenAddress) || event.Blockchain != v.Network || event.TokenName != v.Token {
		return ErrValidation
	}
	expected, _ := cryptoUnits(v.ConsolidateAmount)
	required, _ := cryptoUnits(v.AmountNet)
	if gross.Cmp(expected) != 0 || net.Cmp(required) < 0 {
		return ErrValidation
	}
	canonical, _ := json.Marshal(event)
	hash := sha256.Sum256(canonical)
	digest := hex.EncodeToString(hash[:])
	var existing string
	e = tx.QueryRowContext(ctx, `SELECT payload_hash FROM cloud_crypto_settlements WHERE reference=$1`, v.Reference).Scan(&existing)
	if e == nil {
		if existing != digest {
			return ErrConflict
		}
		return nil
	}
	if e != sql.ErrNoRows {
		return e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO cloud_crypto_settlements(reference,payload_hash,tx_hash,gross_amount,provider_fee,net_amount,network)VALUES($1,$2,$3,$4,$5,$6,$7)`, v.Reference, digest, event.TxHash, event.Amount, event.ProviderFee, event.NetAmount, v.Network)
	if e != nil {
		return e
	}
	var user, workspace string
	e = tx.QueryRowContext(ctx, `SELECT user_id,workspace_id FROM cloud_crypto_invoices WHERE id=$1`, v.ID).Scan(&user, &workspace)
	if e != nil {
		return e
	}
	if v.Purpose == "credits" {
		e = grantCreditsTx(ctx, tx, workspace, "cryptorails:"+v.Reference, v.Credits, "Crypto credit purchase")
	} else {
		// Serializes simultaneous renewals and stacks after legacy Premium where present.
		if _, e = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,814037229))`, user); e != nil {
			return e
		}
		_, e = tx.ExecContext(ctx, `INSERT INTO cloud_crypto_premium(user_id,paid_through)VALUES($1,greatest(now(),coalesce((SELECT paid_through FROM cloud_premium_subscriptions WHERE user_id=$1 AND status='active' AND paid_through>revoked_through AND NOT EXISTS(SELECT 1 FROM cloud_premium_disputes WHERE user_id=$1 AND status NOT IN ('won','warning_closed'))),now()))+interval '30 days') ON CONFLICT(user_id)DO UPDATE SET paid_through=greatest(now(),cloud_crypto_premium.paid_through,coalesce((SELECT paid_through FROM cloud_premium_subscriptions WHERE user_id=$1 AND status='active' AND paid_through>revoked_through AND NOT EXISTS(SELECT 1 FROM cloud_premium_disputes WHERE user_id=$1 AND status NOT IN ('won','warning_closed'))),now()))+interval '30 days',updated_at=now()`, user)
	}
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `UPDATE cloud_crypto_invoices SET status='settled' WHERE id=$1`, v.ID)
	if e != nil {
		return e
	}
	return tx.Commit()
}

func cryptoVMType(raw json.RawMessage) string {
	var name string
	if json.Unmarshal(raw, &name) == nil {
		return name
	}
	var v struct {
		Type     string `json:"type"`
		IsActive bool   `json:"isActive"`
	}
	if json.Unmarshal(raw, &v) == nil && v.IsActive {
		return v.Type
	}
	return ""
}
