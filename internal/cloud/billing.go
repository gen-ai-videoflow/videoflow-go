package cloud

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type CreditPack struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Credits    int64  `json:"credits"`
	PriceCents int64  `json:"price_cents"`
	Currency   string `json:"currency"`
}

// Price activation is explicit; these launch packs are not enabled by default.
var Packs = []CreditPack{{"starter", "Starter", 1000, 1000, "usd"}, {"creator", "Creator", 5500, 5000, "usd"}, {"studio", "Studio", 12000, 10000, "usd"}}

func findPack(id string) (CreditPack, error) {
	for _, p := range Packs {
		if p.ID == id {
			return p, nil
		}
	}
	return CreditPack{}, ErrValidation
}
func stripeSignature(body []byte, secret string, timestamp int64) string {
	m := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(m, "%d.", timestamp)
	m.Write(body)
	return fmt.Sprintf("t=%d,v1=%s", timestamp, hex.EncodeToString(m.Sum(nil)))
}
func verifyStripeSignature(body []byte, header, secret string, now time.Time) bool {
	if secret == "" {
		return false
	}
	var timestamp int64
	var signatures []string
	for _, part := range strings.Split(header, ",") {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		if kv[0] == "t" {
			timestamp, _ = strconv.ParseInt(kv[1], 10, 64)
		}
		if kv[0] == "v1" {
			signatures = append(signatures, kv[1])
		}
	}
	delta := now.Unix() - timestamp
	if delta < -300 || delta > 300 {
		return false
	}
	expected := strings.SplitN(stripeSignature(body, secret, timestamp), "v1=", 2)[1]
	for _, signature := range signatures {
		if hmac.Equal([]byte(expected), []byte(signature)) {
			return true
		}
	}
	return false
}

// Stripe and CryptoRails are independently activated; settlement verification is mandatory.
func (s *Server) stripeEnabled() bool {
	return s.Config.BillingEnabled && s.Config.StripeSecret != "" && s.Config.StripeWebhookSecret != ""
}
func (s *Server) billingCheckout(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PackID   string `json:"pack_id"`
		Provider string `json:"provider"`
	}
	if !s.decode(w, r, &body) {
		return
	}
	switch body.Provider {
	case "stripe":
		if !s.stripeEnabled() {
			s.fail(w, ErrUnavailable)
			return
		}
		result, err := s.checkout(r.Context(), currentUser(r), body.PackID, r.Header.Get("Idempotency-Key"))
		s.result(w, map[string]string{"provider": "stripe", "url": result}, err)
	case "", "cryptorails":
		result, err := s.newCryptoInvoice(r.Context(), currentUser(r), "credits", body.PackID, r.Header.Get("Idempotency-Key"))
		s.result(w, result, err)
	default:
		s.fail(w, ErrValidation)
	}
}
func (s *Server) checkout(ctx context.Context, u User, packID, key string) (string, error) {
	if !s.Config.BillingEnabled || s.Config.StripeSecret == "" {
		return "", ErrUnavailable
	}
	if !u.Verified {
		return "", fmt.Errorf("%w: verify your email before purchasing", ErrValidation)
	}
	if len(key) < 8 || len(key) > 128 {
		return "", ErrValidation
	}
	p := CreditPack{ID: packID, Name: packID}
	var commercialRevision int64
	id := uuid.NewString()
	tx, err := s.Store.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "checkout:"+u.WorkspaceID+":"+key); err != nil {
		return "", err
	}
	var previousPack, existingURL string
	err = tx.QueryRowContext(ctx, `SELECT id,pack_id,credits,price_cents,currency,checkout_url FROM cloud_purchases WHERE workspace_id=$1 AND idempotency_key=$2`, u.WorkspaceID, key).Scan(&id, &previousPack, &p.Credits, &p.PriceCents, &p.Currency, &existingURL)
	if err == nil {
		if previousPack != packID {
			return "", ErrConflict
		}
		if existingURL != "" {
			return existingURL, tx.Commit()
		}
	} else if errors.Is(err, sql.ErrNoRows) {
		commercial, e := commercialPolicyRead(ctx, tx)
		if e != nil {
			return "", e
		}
		p, e = findCommercialPack(commercial.Policy, packID)
		if e != nil {
			return "", e
		}
		commercialRevision = commercial.Revision
		_, err = tx.ExecContext(ctx, `INSERT INTO cloud_purchases(id,workspace_id,pack_id,credits,price_cents,currency,idempotency_key,commercial_revision)VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, id, u.WorkspaceID, p.ID, p.Credits, p.PriceCents, p.Currency, key, commercialRevision)
		if err != nil {
			return "", err
		}
	} else {
		return "", err
	}
	if err = tx.Commit(); err != nil {
		return "", err
	}
	data := url.Values{"mode": {"payment"}, "client_reference_id": {id}, "customer_email": {u.Email}, "success_url": {s.Config.PublicOrigin + "/credits?checkout=success"}, "cancel_url": {s.Config.PublicOrigin + "/credits?checkout=cancelled"}, "line_items[0][price_data][currency]": {p.Currency}, "line_items[0][price_data][unit_amount]": {strconv.FormatInt(p.PriceCents, 10)}, "line_items[0][price_data][product_data][name]": {fmt.Sprintf("6eek %s — %d credits", p.Name, p.Credits)}, "line_items[0][quantity]": {"1"}, "metadata[purchase_id]": {id}}
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.stripe.com/v1/checkout/sessions", strings.NewReader(data.Encode()))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(s.Config.StripeSecret, "")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Idempotency-Key", id)
	resp, err := s.HTTP.Do(req)
	if err != nil {
		return "", ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", ErrUnavailable
	}
	var out struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil || out.ID == "" || !strings.HasPrefix(out.URL, "https://checkout.stripe.com/") {
		return "", ErrUnavailable
	}
	_, err = s.Store.DB.ExecContext(ctx, `UPDATE cloud_purchases SET stripe_session=$2,checkout_url=$3 WHERE id=$1`, id, out.ID, out.URL)
	return out.URL, err
}
func (s *Server) paymentWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil || !verifyStripeSignature(body, r.Header.Get("Stripe-Signature"), s.Config.StripeWebhookSecret, time.Now()) {
		s.fail(w, ErrUnauthorized)
		return
	}
	var rawEvent struct {
		ID   string `json:"id"`
		Type string `json:"type"`
		Data struct {
			Object json.RawMessage `json:"object"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &rawEvent) != nil || rawEvent.ID == "" {
		s.fail(w, ErrValidation)
		return
	}
	if handled, e := s.processPremiumWebhook(r.Context(), rawEvent.ID, rawEvent.Type, rawEvent.Data.Object); handled || e != nil {
		s.result(w, map[string]bool{"received": true}, e)
		return
	}
	if strings.HasPrefix(rawEvent.Type, "charge.dispute.") {
		err := s.paymentDispute(r.Context(), rawEvent.ID, rawEvent.Data.Object)
		s.result(w, map[string]bool{"received": true}, err)
		return
	}
	if rawEvent.Type == "charge.refunded" {
		handled, e := s.creditRefund(r.Context(), rawEvent.ID, rawEvent.Data.Object)
		if handled || e != nil {
			s.result(w, map[string]bool{"received": true}, e)
			return
		}
	}
	var mode struct {
		Mode string `json:"mode"`
	}
	json.Unmarshal(rawEvent.Data.Object, &mode)
	if rawEvent.Type != "checkout.session.completed" && rawEvent.Type != "checkout.session.async_payment_succeeded" || mode.Mode == "subscription" {
		err := s.processMembershipWebhook(r.Context(), rawEvent.ID, rawEvent.Type, rawEvent.Data.Object)
		s.result(w, map[string]bool{"received": true}, err)
		return
	}
	var e struct {
		ID   string `json:"id"`
		Type string `json:"type"`
		Data struct {
			Object struct {
				ID            string   `json:"id"`
				PaymentIntent stripeID `json:"payment_intent"`
				Reference     string   `json:"client_reference_id"`
				PaymentStatus string   `json:"payment_status"`
				Amount        int64    `json:"amount_total"`
				Currency      string   `json:"currency"`
			} `json:"object"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &e) != nil || e.ID == "" {
		s.fail(w, ErrValidation)
		return
	}
	if e.Type != "checkout.session.completed" && e.Type != "checkout.session.async_payment_succeeded" {
		s.json(w, 200, map[string]bool{"received": true})
		return
	}
	if e.Data.Object.PaymentStatus != "paid" {
		s.json(w, 200, map[string]bool{"received": true})
		return
	}
	tx, err := s.Store.DB.BeginTx(r.Context(), nil)
	if err != nil {
		s.fail(w, err)
		return
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(r.Context(), `INSERT INTO cloud_webhook_receipts(id)VALUES($1)ON CONFLICT DO NOTHING`, e.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		s.json(w, 200, map[string]bool{"received": true})
		return
	}
	var workspace, status, currency string
	var credits, amount int64
	err = tx.QueryRowContext(r.Context(), `SELECT workspace_id,credits,price_cents,currency,status FROM cloud_purchases WHERE id=$1 AND stripe_session=$2 FOR UPDATE`, e.Data.Object.Reference, e.Data.Object.ID).Scan(&workspace, &credits, &amount, &currency, &status)
	if err != nil || amount != e.Data.Object.Amount || currency != e.Data.Object.Currency {
		s.fail(w, ErrValidation)
		return
	}
	if status != "paid" {
		if err = grantCreditsTx(r.Context(), tx, workspace, "purchase:"+e.Data.Object.Reference, credits, "Credit pack purchase"); err != nil {
			s.fail(w, err)
			return
		}
		if _, err = tx.ExecContext(r.Context(), `UPDATE cloud_purchases SET status='paid',payment_intent=NULLIF($2,'') WHERE id=$1`, e.Data.Object.Reference, string(e.Data.Object.PaymentIntent)); err != nil {
			s.fail(w, err)
			return
		}
	}
	if err = tx.Commit(); err != nil {
		s.fail(w, err)
		return
	}
	s.json(w, 200, map[string]bool{"received": true})
}
