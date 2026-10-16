package cloud

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (s *Server) premiumEnabled() bool {
	return (s.cryptoEnabled() || s.stripeEnabled()) && s.Config.PremiumPriceCents >= 100 && s.Config.PremiumPriceCents <= 100000
}
func (s *Store) HasPremium(ctx context.Context, userID string) (bool, error) {
	if userID == "" {
		return false, nil
	}
	var active bool
	e := s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cloud_crypto_premium WHERE user_id=$1 AND paid_through>now()) OR EXISTS(SELECT 1 FROM cloud_premium_subscriptions p WHERE p.user_id=$1 AND p.status='active' AND p.paid_through>now() AND p.paid_through>p.revoked_through AND NOT EXISTS(SELECT 1 FROM cloud_premium_disputes d WHERE d.user_id=p.user_id AND d.status NOT IN ('won','warning_closed')))`, userID).Scan(&active)
	return active, e
}
func (s *Server) premiumRoutes(r chi.Router) {
	r.Get("/premium", s.premiumStatus)
	r.Post("/premium/checkout", s.auth(s.premiumCheckout))
	r.Post("/premium/manage", s.auth(s.premiumManage))
}
func (s *Server) premiumStatus(w http.ResponseWriter, r *http.Request) {
	cfg, configErr := s.effectiveConfig(r.Context())
	if configErr != nil {
		s.fail(w, configErr)
		return
	}
	u := s.optionalUser(r)
	active, e := s.Store.HasPremium(r.Context(), u.ID)
	if e != nil {
		s.fail(w, e)
		return
	}
	var end any
	status := "none"
	legacyActive := false
	if u.ID != "" {
		var paid time.Time
		err := s.Store.DB.QueryRowContext(r.Context(), `SELECT status,paid_through,(status='active' AND paid_through>now() AND paid_through>revoked_through AND NOT EXISTS(SELECT 1 FROM cloud_premium_disputes WHERE user_id=$1 AND status NOT IN ('won','warning_closed'))) FROM cloud_premium_subscriptions WHERE user_id=$1`, u.ID).Scan(&status, &paid, &legacyActive)
		if err != nil && err != sql.ErrNoRows {
			s.fail(w, err)
			return
		}
		if err == nil {
			end = paid
		}
		if !active && status == "active" {
			status = "expired_or_held"
		}
	}
	if u.ID != "" {
		var cryptoEnd time.Time
		err := s.Store.DB.QueryRowContext(r.Context(), `SELECT paid_through FROM cloud_crypto_premium WHERE user_id=$1`, u.ID).Scan(&cryptoEnd)
		if err != nil && err != sql.ErrNoRows {
			s.fail(w, err)
			return
		}
		if err == nil {
			if prior, ok := end.(time.Time); !ok || !legacyActive || cryptoEnd.After(prior) {
				end = cryptoEnd
			}
			if active {
				status = "active"
			}
		}
	}
	canManage := false
	if u.ID != "" {
		if err := s.Store.DB.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM cloud_premium_checkouts WHERE user_id=$1 AND (provider_subscription IS NOT NULL OR provider_session IS NOT NULL))`, u.ID).Scan(&canManage); err != nil {
			s.fail(w, err)
			return
		}
		if status == "none" && canManage {
			status = "pending"
		}
	}
	s.json(w, 200, map[string]any{"enabled": (s.cryptoEnabled() || s.stripeEnabled()) && cfg.PremiumPriceCents >= 100 && cfg.PremiumPriceCents <= 100000, "price_cents": cfg.PremiumPriceCents, "currency": "usd", "interval": "30_days", "auto_renew": false, "active": active, "paid_through": end, "status": status, "can_manage": canManage, "stripe_active": legacyActive, "stripe_enabled": s.stripeEnabled(), "cryptorails_enabled": s.cryptoEnabled(), "payment_terms": map[string]any{"stripe": map[string]any{"interval": "month", "auto_renew": true}, "cryptorails": map[string]any{"interval": "30_days", "auto_renew": false}}})
}
func (s *Server) premiumCheckout(w http.ResponseWriter, r *http.Request) {
	cfg, configErr := s.effectiveConfig(r.Context())
	if configErr != nil {
		s.fail(w, configErr)
		return
	}
	if !(s.cryptoEnabled() || s.stripeEnabled()) || cfg.PremiumPriceCents < 100 || cfg.PremiumPriceCents > 100000 {
		s.fail(w, ErrUnavailable)
		return
	}
	u := currentUser(r)
	if !u.Verified {
		s.fail(w, ErrUnauthorized)
		return
	}
	var body struct {
		Provider string `json:"provider"`
	}
	if !s.decode(w, r, &body) {
		return
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(key) < 8 || len(key) > 200 {
		s.fail(w, ErrValidation)
		return
	}
	if body.Provider == "stripe" {
		if !s.stripeEnabled() {
			s.fail(w, ErrUnavailable)
			return
		}
		result, err := s.startPremiumCheckout(r.Context(), u, key)
		s.result(w, map[string]string{"provider": "stripe", "url": result}, err)
		return
	}
	if body.Provider != "" && body.Provider != "cryptorails" {
		s.fail(w, ErrValidation)
		return
	}
	result, e := s.newCryptoInvoice(r.Context(), u, "premium", "", key)
	s.result(w, result, e)
}
func (s *Server) startPremiumCheckout(ctx context.Context, u User, key string) (string, error) {
	cfg, configErr := s.effectiveConfig(ctx)
	if configErr != nil {
		return "", configErr
	}
	tx, e := s.Store.DB.BeginTx(ctx, nil)
	if e != nil {
		return "", e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,814037229))`, u.ID); e != nil {
		return "", e
	}
	var active bool
	e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cloud_premium_subscriptions WHERE user_id=$1 AND status='active' AND paid_through>now()) OR EXISTS(SELECT 1 FROM cloud_premium_disputes WHERE user_id=$1 AND status NOT IN ('won','warning_closed')) OR EXISTS(SELECT 1 FROM cloud_crypto_premium WHERE user_id=$1 AND paid_through>now()) OR EXISTS(SELECT 1 FROM cloud_crypto_invoices WHERE user_id=$1 AND purpose='premium' AND status IN ('pending','processing','review'))`, u.ID).Scan(&active)
	if e != nil {
		return "", e
	}
	if active {
		return "", ErrConflict
	}
	if e = s.reconcilePriorPremiumCheckouts(ctx, tx, u.ID, key); e != nil {
		return "", e
	}
	id := uuid.NewString()
	var pending bool
	e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cloud_premium_checkouts WHERE user_id=$1 AND idempotency_key<>$2 AND created_at>now()-interval '31 minutes')`, u.ID, key).Scan(&pending)
	if e != nil {
		return "", e
	}
	if pending {
		return "", ErrConflict
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO cloud_premium_checkouts(id,user_id,price_cents,idempotency_key)VALUES($1,$2,$3,$4)ON CONFLICT(user_id,idempotency_key)DO NOTHING`, id, u.ID, cfg.PremiumPriceCents, key)
	if e != nil {
		return "", e
	}
	var price int64
	var created time.Time
	var session, checkoutURL string
	e = tx.QueryRowContext(ctx, `SELECT id,price_cents,created_at,coalesce(provider_session,''),checkout_url FROM cloud_premium_checkouts WHERE user_id=$1 AND idempotency_key=$2`, u.ID, key).Scan(&id, &price, &created, &session, &checkoutURL)
	if e != nil {
		return "", e
	}
	if e = tx.Commit(); e != nil {
		return "", e
	}
	if checkoutURL != "" && created.Add(31*time.Minute).After(time.Now()) {
		return checkoutURL, nil
	}
	if time.Since(created) > time.Minute {
		return "", ErrConflict
	}
	form := url.Values{"mode": {"subscription"}, "customer_email": {u.Email}, "client_reference_id": {id}, "expires_at": {intString(created.Add(31 * time.Minute).Unix())}, "success_url": {s.Config.PublicOrigin + "/premium?checkout=success"}, "cancel_url": {s.Config.PublicOrigin + "/premium?checkout=cancelled"}, "line_items[0][price_data][currency]": {"usd"}, "line_items[0][price_data][unit_amount]": {intString(price)}, "line_items[0][price_data][recurring][interval]": {"month"}, "line_items[0][price_data][product_data][name]": {"6eek Premium — all creators"}, "line_items[0][quantity]": {"1"}, "metadata[premium_checkout_id]": {id}, "subscription_data[metadata][premium_checkout_id]": {id}}
	var out struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if e = s.stripeForm(ctx, "checkout/sessions", id, form, &out); e != nil {
		return "", e
	}
	if out.ID == "" || !strings.HasPrefix(out.URL, "https://checkout.stripe.com/") {
		return "", ErrUnavailable
	}
	_, e = s.Store.DB.ExecContext(ctx, `UPDATE cloud_premium_checkouts SET provider_session=$2,checkout_url=$3 WHERE id=$1 AND (provider_session IS NULL OR provider_session=$2)`, id, out.ID, out.URL)
	return out.URL, e
}

type premiumSubscription struct {
	Customer  stripeID `json:"customer"`
	ID        string   `json:"id"`
	Status    string   `json:"status"`
	PeriodEnd int64    `json:"current_period_end"`
	Metadata  struct {
		CheckoutID string `json:"premium_checkout_id"`
	} `json:"metadata"`
	Items struct {
		Data []struct {
			PeriodEnd int64 `json:"current_period_end"`
		} `json:"data"`
	} `json:"items"`
}

func (s *Server) premiumSubscription(ctx context.Context, id string) (premiumSubscription, error) {
	var sub premiumSubscription
	if id == "" {
		return sub, ErrValidation
	}
	e := s.stripeForm(ctx, "subscriptions/"+url.PathEscape(id), "", nil, &sub)
	if e == nil && sub.ID != id {
		return sub, ErrValidation
	}
	for _, item := range sub.Items.Data {
		if item.PeriodEnd > sub.PeriodEnd {
			sub.PeriodEnd = item.PeriodEnd
		}
	}
	return sub, e
}

// processPremiumWebhook owns its receipt transaction, after signature verification.
// Unknown legacy payments return handled=false so existing reconciliation survives.
func (s *Server) processPremiumWebhook(ctx context.Context, eventID, eventType string, raw json.RawMessage) (bool, error) {
	if eventID == "" {
		return true, ErrValidation
	}
	switch eventType {
	case "invoice.paid":
		return s.premiumInvoice(ctx, eventID, raw)
	case "charge.refunded":
		return s.premiumRefund(ctx, eventID, raw)
	case "charge.dispute.created", "charge.dispute.updated", "charge.dispute.closed":
		return s.premiumDispute(ctx, eventID, raw)
	case "checkout.session.completed", "checkout.session.async_payment_succeeded":
		var obj struct {
			ID           string   `json:"id"`
			Subscription stripeID `json:"subscription"`
			Metadata     struct {
				Checkout string `json:"premium_checkout_id"`
			} `json:"metadata"`
		}
		if json.Unmarshal(raw, &obj) != nil {
			return true, ErrValidation
		}
		if obj.Metadata.Checkout == "" {
			return false, nil
		}
		sub, e := s.premiumSubscription(ctx, string(obj.Subscription))
		if e != nil {
			return true, e
		}
		if sub.Metadata.CheckoutID != obj.Metadata.Checkout {
			return true, ErrValidation
		}
		_, e = s.Store.DB.ExecContext(ctx, `UPDATE cloud_premium_checkouts SET provider_subscription=$2 WHERE id=$1 AND (provider_session IS NULL OR provider_session=$3) AND (provider_subscription IS NULL OR provider_subscription=$2)`, obj.Metadata.Checkout, sub.ID, obj.ID)
		return true, e
	case "customer.subscription.created", "customer.subscription.updated", "customer.subscription.deleted":
		var obj struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(raw, &obj) != nil {
			return true, ErrValidation
		}
		sub, e := s.premiumSubscription(ctx, obj.ID)
		if e != nil {
			return true, e
		}
		if sub.Metadata.CheckoutID == "" {
			return false, nil
		}
		tx, e := s.Store.DB.BeginTx(ctx, nil)
		if e != nil {
			return true, e
		}
		defer tx.Rollback()
		fresh, e := commerceReceipt(ctx, tx, eventID)
		if e != nil || !fresh {
			return true, e
		}
		_, e = tx.ExecContext(ctx, `UPDATE cloud_premium_subscriptions SET status='expired',updated_at=now() WHERE provider_subscription=$1 AND paid_through<=now() AND status='active'`, sub.ID)
		if e != nil {
			return true, e
		}
		return true, tx.Commit()
	}
	return false, nil
}
func (s *Server) premiumInvoice(ctx context.Context, eventID string, raw json.RawMessage) (bool, error) {
	var obj struct {
		ID           string   `json:"id"`
		Subscription stripeID `json:"subscription"`
		Payment      stripeID `json:"payment_intent"`
		Amount       int64    `json:"amount_paid"`
		Currency     string   `json:"currency"`
		Manual       bool     `json:"paid_out_of_band"`
		Parent       struct {
			Details struct {
				Subscription stripeID `json:"subscription"`
			} `json:"subscription_details"`
		} `json:"parent"`
		Lines struct {
			Data []struct {
				Period struct {
					End int64 `json:"end"`
				} `json:"period"`
			} `json:"data"`
		} `json:"lines"`
	}
	if json.Unmarshal(raw, &obj) != nil || obj.ID == "" {
		return true, ErrValidation
	}
	if obj.Subscription == "" {
		obj.Subscription = obj.Parent.Details.Subscription
	}
	if obj.Subscription == "" {
		return false, nil
	}
	sub, e := s.premiumSubscription(ctx, string(obj.Subscription))
	if e != nil {
		return true, e
	}
	if sub.Metadata.CheckoutID == "" {
		return false, nil
	}
	if obj.Manual {
		return true, ErrValidation
	}
	if obj.Payment == "" {
		var invoice struct {
			ID      string   `json:"id"`
			Payment stripeID `json:"payment_intent"`
		}
		if e = s.stripeForm(ctx, "invoices/"+url.PathEscape(obj.ID), "", nil, &invoice); e != nil {
			return true, e
		}
		if invoice.ID != obj.ID || invoice.Payment == "" {
			return true, ErrUnavailable
		}
		obj.Payment = invoice.Payment
	}
	var end int64
	for _, line := range obj.Lines.Data {
		if line.Period.End > end {
			end = line.Period.End
		}
	}
	if end == 0 {
		end = sub.PeriodEnd
	}
	if end <= 0 {
		return true, ErrValidation
	}
	tx, e := s.Store.DB.BeginTx(ctx, nil)
	if e != nil {
		return true, e
	}
	defer tx.Rollback()
	fresh, e := commerceReceipt(ctx, tx, eventID)
	if e != nil || !fresh {
		return true, e
	}
	var user string
	var price int64
	e = tx.QueryRowContext(ctx, `SELECT user_id,price_cents FROM cloud_premium_checkouts WHERE id=$1 AND (provider_subscription IS NULL OR provider_subscription=$2)`, sub.Metadata.CheckoutID, sub.ID).Scan(&user, &price)
	if e != nil {
		return true, e
	}
	if obj.Amount != price || obj.Currency != "usd" {
		return true, ErrValidation
	}
	if _, e = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,814037229))`, user); e != nil {
		return true, e
	}
	_, e = tx.ExecContext(ctx, `UPDATE cloud_premium_checkouts SET provider_subscription=$2 WHERE id=$1 AND (provider_subscription IS NULL OR provider_subscription=$2)`, sub.Metadata.CheckoutID, sub.ID)
	if e != nil {
		return true, e
	}
	result, e := tx.ExecContext(ctx, `INSERT INTO cloud_premium_invoices(id,checkout_id,payment_intent,gross_cents,currency,period_end)VALUES($1,$2,$3,$4,'usd',$5)ON CONFLICT(id)DO NOTHING`, obj.ID, sub.Metadata.CheckoutID, string(obj.Payment), obj.Amount, time.Unix(end, 0))
	if e != nil {
		return true, e
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return true, tx.Commit()
	}
	// Payment is platform revenue only; creator rewards are separately approved.
	_, e = tx.ExecContext(ctx, `INSERT INTO cloud_premium_subscriptions(user_id,provider_subscription,status,paid_through)VALUES($1,$2,'active',$3)ON CONFLICT(user_id)DO UPDATE SET status=CASE WHEN EXCLUDED.paid_through>cloud_premium_subscriptions.revoked_through THEN 'active' ELSE cloud_premium_subscriptions.status END,provider_subscription=CASE WHEN EXCLUDED.paid_through>=cloud_premium_subscriptions.paid_through THEN EXCLUDED.provider_subscription ELSE cloud_premium_subscriptions.provider_subscription END,paid_through=GREATEST(cloud_premium_subscriptions.paid_through,EXCLUDED.paid_through),updated_at=now()`, user, sub.ID, time.Unix(end, 0))
	if e != nil {
		return true, e
	}
	return true, tx.Commit()
}
func (s *Server) premiumRefund(ctx context.Context, eventID string, raw json.RawMessage) (bool, error) {
	var obj struct {
		Payment  stripeID `json:"payment_intent"`
		Amount   int64    `json:"amount"`
		Refunded int64    `json:"amount_refunded"`
	}
	if json.Unmarshal(raw, &obj) != nil {
		return true, ErrValidation
	}
	if obj.Payment == "" {
		return false, nil
	}
	tx, e := s.Store.DB.BeginTx(ctx, nil)
	if e != nil {
		return true, e
	}
	defer tx.Rollback()
	var invoice, user string
	var gross, previous int64
	var end time.Time
	e = tx.QueryRowContext(ctx, `SELECT i.id,c.user_id,i.gross_cents,i.refunded_cents,i.period_end FROM cloud_premium_invoices i JOIN cloud_premium_checkouts c ON c.id=i.checkout_id WHERE i.payment_intent=$1`, string(obj.Payment)).Scan(&invoice, &user, &gross, &previous, &end)
	if errors.Is(e, sql.ErrNoRows) {
		return false, nil
	}
	if e != nil {
		return true, e
	}
	if obj.Amount != gross || obj.Refunded < 0 || obj.Refunded > gross {
		return true, ErrValidation
	}
	fresh, e := commerceReceipt(ctx, tx, eventID)
	if e != nil || !fresh {
		return true, e
	}
	if _, e = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,814037229))`, user); e != nil {
		return true, e
	}
	if e = tx.QueryRowContext(ctx, `SELECT refunded_cents FROM cloud_premium_invoices WHERE id=$1 FOR UPDATE`, invoice).Scan(&previous); e != nil {
		return true, e
	}
	if obj.Refunded <= previous {
		return true, tx.Commit()
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO cloud_premium_refunds(id,invoice_id,cumulative_cents,delta_cents)VALUES($1,$2,$3,$4)`, eventID, invoice, obj.Refunded, obj.Refunded-previous)
	if e != nil {
		return true, e
	}
	_, e = tx.ExecContext(ctx, `UPDATE cloud_premium_invoices SET refunded_cents=$2 WHERE id=$1`, invoice, obj.Refunded)
	if e != nil {
		return true, e
	}
	_, e = tx.ExecContext(ctx, `UPDATE cloud_premium_subscriptions SET revoked_through=GREATEST(revoked_through,$2),status=CASE WHEN paid_through<=$2 THEN 'revoked' ELSE status END,updated_at=now() WHERE user_id=$1`, user, end)
	if e != nil {
		return true, e
	}
	return true, tx.Commit()
}
func (s *Server) premiumDispute(ctx context.Context, eventID string, raw json.RawMessage) (bool, error) {
	var obj struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(raw, &obj) != nil || obj.ID == "" {
		return true, ErrValidation
	}
	tx, e := s.Store.DB.BeginTx(ctx, nil)
	if e != nil {
		return true, e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,814037230))`, obj.ID); e != nil {
		return true, e
	}
	var current struct {
		ID       string   `json:"id"`
		Payment  stripeID `json:"payment_intent"`
		Status   string   `json:"status"`
		Amount   int64    `json:"amount"`
		Currency string   `json:"currency"`
	}
	if e = s.stripeForm(ctx, "disputes/"+url.PathEscape(obj.ID), "", nil, &current); e != nil {
		return true, e
	}
	if current.ID != obj.ID || current.Payment == "" {
		return true, ErrValidation
	}
	var user string
	var gross int64
	e = tx.QueryRowContext(ctx, `SELECT c.user_id,i.gross_cents FROM cloud_premium_invoices i JOIN cloud_premium_checkouts c ON c.id=i.checkout_id WHERE i.payment_intent=$1`, string(current.Payment)).Scan(&user, &gross)
	if errors.Is(e, sql.ErrNoRows) {
		return false, nil
	}
	if e != nil {
		return true, e
	}
	if !hasValue([]string{"needs_response", "under_review", "warning_needs_response", "warning_under_review", "lost", "won", "warning_closed"}, current.Status) || current.Amount <= 0 || current.Amount > gross || current.Currency != "usd" {
		return true, ErrValidation
	}
	fresh, e := commerceReceipt(ctx, tx, eventID)
	if e != nil || !fresh {
		return true, e
	}
	if _, e = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,814037229))`, user); e != nil {
		return true, e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO cloud_premium_disputes(id,user_id,payment_intent,status,amount_cents,currency)VALUES($1,$2,$3,$4,$5,'usd')ON CONFLICT(id)DO UPDATE SET status=EXCLUDED.status,amount_cents=EXCLUDED.amount_cents,updated_at=now()`, current.ID, user, string(current.Payment), current.Status, current.Amount)
	if e != nil {
		return true, e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO cloud_admin_audit(actor,action,resource_id,reason)VALUES('stripe:webhook','premium_dispute',$1,$2)`, current.ID, current.Status)
	if e != nil {
		return true, e
	}
	return true, tx.Commit()
}

func (s *Server) premiumManage(w http.ResponseWriter, r *http.Request) {
	var body struct{}
	if !s.decode(w, r, &body) {
		return
	}
	if s.Config.StripeSecret == "" {
		s.fail(w, ErrUnavailable)
		return
	}
	u := currentUser(r)
	var checkout, provider, session string
	e := s.Store.DB.QueryRowContext(r.Context(), `SELECT c.id,coalesce(c.provider_subscription,''),coalesce(c.provider_session,'') FROM cloud_premium_checkouts c LEFT JOIN cloud_premium_subscriptions p ON p.user_id=c.user_id WHERE c.user_id=$1 AND (c.provider_subscription IS NOT NULL OR c.provider_session IS NOT NULL) ORDER BY CASE WHEN p.provider_subscription=c.provider_subscription AND p.paid_through>now() THEN 1 ELSE 0 END DESC,c.created_at DESC LIMIT 1`, u.ID).Scan(&checkout, &provider, &session)
	if e == sql.ErrNoRows {
		s.fail(w, ErrNotFound)
		return
	}
	if e != nil {
		s.fail(w, e)
		return
	}
	if provider == "" {
		var remote struct {
			ID           string   `json:"id"`
			Subscription stripeID `json:"subscription"`
			Metadata     struct {
				Checkout string `json:"premium_checkout_id"`
			} `json:"metadata"`
		}
		if e = s.stripeForm(r.Context(), "checkout/sessions/"+url.PathEscape(session), "", nil, &remote); e != nil {
			s.fail(w, e)
			return
		}
		if remote.ID != session || remote.Metadata.Checkout != checkout || remote.Subscription == "" {
			s.fail(w, ErrUnavailable)
			return
		}
		provider = string(remote.Subscription)
		_, e = s.Store.DB.ExecContext(r.Context(), `UPDATE cloud_premium_checkouts SET provider_subscription=$2 WHERE id=$1 AND (provider_subscription IS NULL OR provider_subscription=$2)`, checkout, provider)
		if e != nil {
			s.fail(w, e)
			return
		}
	}
	sub, e := s.premiumSubscription(r.Context(), provider)
	if e != nil {
		s.fail(w, e)
		return
	}
	var owned bool
	e = s.Store.DB.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM cloud_premium_checkouts WHERE id=$1 AND user_id=$2 AND provider_subscription=$3)`, sub.Metadata.CheckoutID, u.ID, provider).Scan(&owned)
	if e != nil {
		s.fail(w, e)
		return
	}
	if !owned || sub.Customer == "" {
		s.fail(w, ErrUnauthorized)
		return
	}
	var out struct {
		URL string `json:"url"`
	}
	e = s.stripeForm(r.Context(), "billing_portal/sessions", uuid.NewString(), url.Values{"customer": {string(sub.Customer)}, "return_url": {s.Config.PublicOrigin + "/premium"}}, &out)
	if e == nil && !strings.HasPrefix(out.URL, "https://billing.stripe.com/") {
		e = ErrUnavailable
	}
	s.result(w, map[string]string{"url": out.URL}, e)
}

// Reconcile every prior checkout before authorizing a new recurring purchase.
// Local age proves only that our link should expire, never that Stripe did not
// create a subscription or accept an asynchronous payment.
func (s *Server) reconcilePriorPremiumCheckouts(ctx context.Context, tx *sql.Tx, user, key string) error {
	rows, e := tx.QueryContext(ctx, `SELECT id,coalesce(provider_session,''),coalesce(provider_subscription,''),created_at FROM cloud_premium_checkouts WHERE user_id=$1 AND idempotency_key<>$2 ORDER BY created_at DESC LIMIT 101`, user, key)
	if e != nil {
		return e
	}
	type prior struct {
		id, session, subscription string
		created                   time.Time
	}
	previous := []prior{}
	for rows.Next() {
		var p prior
		if e = rows.Scan(&p.id, &p.session, &p.subscription, &p.created); e != nil {
			rows.Close()
			return e
		}
		previous = append(previous, p)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	if len(previous) > 100 {
		return ErrConflict
	}
	for _, p := range previous {
		if p.created.Add(31 * time.Minute).After(time.Now()) {
			return ErrConflict
		}
		if p.subscription == "" {
			if p.session == "" {
				return ErrConflict
			} // An interrupted provider response requires operator reconciliation.
			var session struct {
				ID            string   `json:"id"`
				Status        string   `json:"status"`
				PaymentStatus string   `json:"payment_status"`
				Subscription  stripeID `json:"subscription"`
				Metadata      struct {
					Checkout string `json:"premium_checkout_id"`
				} `json:"metadata"`
			}
			if e = s.stripeForm(ctx, "checkout/sessions/"+url.PathEscape(p.session), "", nil, &session); e != nil {
				return e
			}
			if session.ID != p.session || session.Metadata.Checkout != p.id {
				return ErrValidation
			}
			if session.Subscription == "" {
				if session.Status == "expired" && session.PaymentStatus != "paid" {
					continue
				}
				return ErrConflict
			}
			p.subscription = string(session.Subscription)
			if _, e = tx.ExecContext(ctx, `UPDATE cloud_premium_checkouts SET provider_subscription=$2 WHERE id=$1 AND provider_subscription IS NULL`, p.id, p.subscription); e != nil {
				return e
			}
		}
		sub, e := s.premiumSubscription(ctx, p.subscription)
		if e != nil {
			return e
		}
		if sub.Metadata.CheckoutID != p.id {
			return ErrValidation
		}
		switch sub.Status {
		case "incomplete_expired":
			continue
		case "canceled":
			if sub.PeriodEnd <= time.Now().Unix() {
				continue
			}
		}
		return ErrConflict
	}
	return nil
}
