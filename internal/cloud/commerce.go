package cloud

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

func intString(n int64) string { return strconv.FormatInt(n, 10) }

// AllocateRewards distributes a capped share of recognized contribution using
// largest remainders and lexical creator IDs for deterministic tie breaking.
// Inputs and arithmetic are integer minor units; zero policy leaves rewards off.
func AllocateRewards(contribution, bps, capCents int64, weights map[string]int64) (map[string]int64, error) {
	out := map[string]int64{}
	if contribution < 0 || bps < 0 || bps > 10000 || capCents < 0 {
		return nil, ErrValidation
	}
	total := new(big.Int)
	keys := make([]string, 0, len(weights))
	for id, w := range weights {
		if id == "" || w < 0 {
			return nil, ErrValidation
		}
		if w > 0 {
			keys = append(keys, id)
			total.Add(total, big.NewInt(w))
		}
	}
	if contribution == 0 || bps == 0 || capCents == 0 || total.Sign() == 0 {
		return out, nil
	}
	pool := new(big.Int).Mul(big.NewInt(contribution), big.NewInt(bps))
	pool.Div(pool, big.NewInt(10000))
	if pool.Cmp(big.NewInt(capCents)) > 0 {
		pool.SetInt64(capCents)
	}
	type remainder struct {
		id string
		n  *big.Int
	}
	remainders := make([]remainder, 0, len(keys))
	var sum int64
	for _, id := range keys {
		n := new(big.Int).Mul(pool, big.NewInt(weights[id]))
		q, r := new(big.Int), new(big.Int)
		q.QuoRem(n, total, r)
		out[id] = q.Int64()
		sum += q.Int64()
		remainders = append(remainders, remainder{id, r})
	}
	sort.Slice(remainders, func(i, j int) bool {
		c := remainders[i].n.Cmp(remainders[j].n)
		if c == 0 {
			return remainders[i].id < remainders[j].id
		}
		return c > 0
	})
	for i := int64(0); i < pool.Int64()-sum; i++ {
		out[remainders[i].id]++
	}
	return out, nil
}

// CloseRewardPeriod records approved, versioned durable inputs once. This method
// has no HTTP route: an authenticated administrative approval workflow must own
// input qualification and call it only after the commercial policy is activated.
func (s *Store) CloseRewardPeriod(ctx context.Context, period string, contribution, bps, capCents int64, weights map[string]int64) error {
	date, e := time.Parse("2006-01", period)
	if e != nil || !date.Before(time.Now().UTC().Truncate(24*time.Hour)) || date.Format("2006-01") >= time.Now().UTC().Format("2006-01") {
		return ErrValidation
	}
	allocations, e := AllocateRewards(contribution, bps, capCents, weights)
	if e != nil {
		return e
	}
	input, _ := json.Marshal(struct {
		Contribution, BPS, Cap int64
		Weights                map[string]int64
	}{contribution, bps, capCents, weights})
	hash := sha256.Sum256(input)
	digest := hex.EncodeToString(hash[:])
	var total int64
	for _, n := range allocations {
		total += n
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	_, e = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "reward:"+period)
	if e != nil {
		return e
	}
	var previous string
	e = tx.QueryRowContext(ctx, `SELECT input_hash FROM cloud_reward_periods WHERE period=$1`, period).Scan(&previous)
	if e == nil {
		if previous != digest {
			return ErrValidation
		}
		return nil
	}
	if e != sql.ErrNoRows {
		return e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO cloud_reward_periods(period,currency,contribution_cents,reward_bps,cap_cents,allocated_cents,input_hash)VALUES($1,'usd',$2,$3,$4,$5,$6)`, period, contribution, bps, capCents, total, digest)
	if e != nil {
		return e
	}
	for id, n := range allocations {
		if n == 0 {
			continue
		}
		_, e = tx.ExecContext(ctx, `INSERT INTO cloud_earnings(id,user_id,reference,amount_cents,currency,status,description)VALUES($1,$2,$3,$4,'usd','pending','Approved monthly engagement reward')`, uuid.NewString(), id, "reward:"+period+":"+id, n)
		if e != nil {
			return e
		}
	}
	return tx.Commit()
}

func (s *Server) stripeForm(ctx context.Context, path, key string, form url.Values, out any) error {
	method := http.MethodPost
	var body io.Reader = strings.NewReader(form.Encode())
	if form == nil {
		method = http.MethodGet
		body = nil
	}
	req, e := http.NewRequestWithContext(ctx, method, "https://api.stripe.com/v1/"+path, body)
	if e != nil {
		return e
	}
	req.SetBasicAuth(s.Config.StripeSecret, "")
	req.Header.Set("Stripe-Version", "2024-06-20")
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	resp, e := s.HTTP.Do(req)
	if e != nil {
		return ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ErrUnavailable
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out) != nil {
		return ErrUnavailable
	}
	return nil
}
func (s *Server) membershipSalesEnabled() bool {
	return s.Config.BillingEnabled && s.Config.MembershipsEnabled && s.Config.MembershipPolicyConfigured && s.Config.MembershipCommissionBPS >= 0 && s.Config.MembershipCommissionBPS <= 10000 && s.Config.StripeSecret != ""
}
func (s *Server) legacyMembershipPlan(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if r.Method == http.MethodGet {
		var price, version int64
		var currency string
		var enabled bool
		e := s.Store.DB.QueryRowContext(r.Context(), `SELECT price_cents,currency,enabled,version FROM cloud_membership_plans WHERE channel_id=$1`, u.ChannelID).Scan(&price, &currency, &enabled, &version)
		if e == sql.ErrNoRows {
			s.json(w, 200, map[string]any{"configured": false, "sales_enabled": s.membershipSalesEnabled()})
			return
		}
		s.result(w, map[string]any{"configured": true, "price_cents": price, "currency": currency, "enabled": enabled, "version": version, "sales_enabled": s.membershipSalesEnabled()}, e)
		return
	}
	var b struct {
		PriceCents int64  `json:"price_cents"`
		Currency   string `json:"currency"`
		Enabled    bool   `json:"enabled"`
	}
	if !s.decode(w, r, &b) {
		return
	}
	b.Currency = strings.ToLower(b.Currency)
	if b.PriceCents < 100 || b.PriceCents > 100000 || b.Currency != "usd" {
		s.fail(w, ErrValidation)
		return
	}
	if b.Enabled && !s.membershipSalesEnabled() {
		s.fail(w, ErrUnavailable)
		return
	}
	_, e := s.Store.DB.ExecContext(r.Context(), `INSERT INTO cloud_membership_plans(channel_id,price_cents,currency,enabled)VALUES($1,$2,$3,$4)ON CONFLICT(channel_id)DO UPDATE SET price_cents=EXCLUDED.price_cents,currency=EXCLUDED.currency,enabled=EXCLUDED.enabled,version=cloud_membership_plans.version+1,updated_at=now()`, u.ChannelID, b.PriceCents, b.Currency, b.Enabled)
	s.result(w, map[string]bool{"ok": true}, e)
}
func (s *Server) legacyMembershipCheckout(w http.ResponseWriter, r *http.Request) {
	if !s.membershipSalesEnabled() {
		s.fail(w, ErrUnavailable)
		return
	}
	u := currentUser(r)
	if !u.Verified {
		s.fail(w, ErrValidation)
		return
	}
	var b struct {
		ChannelID string `json:"channel_id"`
	}
	if !s.decode(w, r, &b) {
		return
	}
	if b.ChannelID == u.ChannelID || b.ChannelID == "" {
		s.fail(w, ErrValidation)
		return
	}
	var price int64
	var currency, account string
	e := s.Store.DB.QueryRowContext(r.Context(), `SELECT p.price_cents,p.currency,a.provider_id FROM cloud_membership_plans p JOIN cloud_channels c ON c.id=p.channel_id JOIN cloud_payout_accounts a ON a.user_id=c.user_id WHERE p.channel_id=$1 AND p.enabled`, b.ChannelID).Scan(&price, &currency, &account)
	if e != nil {
		s.fail(w, ErrUnavailable)
		return
	}
	// Retrieve provider capabilities immediately; stale cached onboarding flags never authorize a sale.
	var capabilities struct {
		Charges bool `json:"charges_enabled"`
		Payouts bool `json:"payouts_enabled"`
	}
	if s.stripeForm(r.Context(), "accounts/"+url.PathEscape(account), "", nil, &capabilities) != nil || !capabilities.Charges || !capabilities.Payouts {
		s.fail(w, ErrUnavailable)
		return
	}
	var exists bool
	e = s.Store.DB.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM cloud_memberships WHERE user_id=$1 AND channel_id=$2 AND status='active' AND paid_through>now())`, u.ID, b.ChannelID).Scan(&exists)
	if e != nil {
		s.fail(w, e)
		return
	}
	if exists {
		s.fail(w, ErrValidation)
		return
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(key) < 8 || len(key) > 200 {
		s.fail(w, ErrValidation)
		return
	}
	id := uuid.NewString()
	tx, e := s.Store.DB.BeginTx(r.Context(), nil)
	if e != nil {
		s.fail(w, e)
		return
	}
	defer tx.Rollback()
	_, e = tx.ExecContext(r.Context(), `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "membership:"+u.ID+":"+b.ChannelID)
	if e != nil {
		s.fail(w, e)
		return
	}
	e = tx.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM cloud_membership_checkouts WHERE user_id=$1 AND channel_id=$2 AND idempotency_key IS DISTINCT FROM $3 AND created_at>now()-interval '31 minutes') OR EXISTS(SELECT 1 FROM cloud_memberships WHERE user_id=$1 AND channel_id=$2 AND status='active' AND paid_through>now())`, u.ID, b.ChannelID, key).Scan(&exists)
	if e != nil {
		s.fail(w, e)
		return
	}
	if exists {
		s.fail(w, ErrValidation)
		return
	}
	_, e = tx.ExecContext(r.Context(), `INSERT INTO cloud_membership_checkouts(id,user_id,channel_id,price_cents,currency,commission_bps,idempotency_key)VALUES($1,$2,$3,$4,$5,$6,$7)ON CONFLICT(user_id,idempotency_key)DO NOTHING`, id, u.ID, b.ChannelID, price, currency, s.Config.MembershipCommissionBPS, key)
	if e != nil {
		s.fail(w, e)
		return
	}
	var oldChannel, oldCurrency string
	var oldPrice, oldBPS int64
	var checkoutCreated time.Time
	e = tx.QueryRowContext(r.Context(), `SELECT id,channel_id,price_cents,currency,commission_bps,created_at FROM cloud_membership_checkouts WHERE user_id=$1 AND idempotency_key=$2`, u.ID, key).Scan(&id, &oldChannel, &oldPrice, &oldCurrency, &oldBPS, &checkoutCreated)
	if e != nil {
		s.fail(w, e)
		return
	}
	if oldChannel != b.ChannelID || oldPrice != price || oldCurrency != currency || oldBPS != s.Config.MembershipCommissionBPS {
		s.fail(w, ErrValidation)
		return
	}
	if e = tx.Commit(); e != nil {
		s.fail(w, e)
		return
	}
	if time.Since(checkoutCreated) > time.Minute {
		s.fail(w, ErrValidation)
		return
	}
	form := url.Values{"expires_at": {intString(checkoutCreated.Add(31 * time.Minute).Unix())}, "mode": {"subscription"}, "customer_email": {u.Email}, "client_reference_id": {id}, "success_url": {s.Config.PublicOrigin + "/memberships?checkout=success"}, "cancel_url": {s.Config.PublicOrigin + "/memberships?checkout=cancelled"}, "line_items[0][price_data][currency]": {currency}, "line_items[0][price_data][unit_amount]": {intString(price)}, "line_items[0][price_data][recurring][interval]": {"month"}, "line_items[0][price_data][product_data][name]": {"6eek creator membership"}, "line_items[0][quantity]": {"1"}, "metadata[membership_checkout_id]": {id}, "subscription_data[metadata][membership_checkout_id]": {id}}
	// Funds remain with the platform. Creator share is journaled on verified invoices;
	// transfers and cash payouts require a separate approved, reconciled batch.
	var out struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	e = s.stripeForm(r.Context(), "checkout/sessions", id, form, &out)
	if e == nil && (out.ID == "" || !strings.HasPrefix(out.URL, "https://checkout.stripe.com/")) {
		e = ErrUnavailable
	}
	if e == nil {
		_, e = s.Store.DB.ExecContext(r.Context(), `UPDATE cloud_membership_checkouts SET provider_session=$2 WHERE id=$1`, id, out.ID)
	}
	s.result(w, map[string]string{"url": out.URL}, e)
}
func (s *Server) memberships(w http.ResponseWriter, r *http.Request) {
	rows, e := s.Store.DB.QueryContext(r.Context(), `SELECT m.id,m.channel_id,c.handle,c.name,m.status,m.paid_through FROM cloud_memberships m JOIN cloud_channels c ON c.id=m.channel_id WHERE m.user_id=$1 ORDER BY m.paid_through DESC LIMIT 100`, currentUser(r).ID)
	if e != nil {
		s.fail(w, e)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, channel, handle, name, status string
		var end time.Time
		if e = rows.Scan(&id, &channel, &handle, &name, &status, &end); e != nil {
			s.fail(w, e)
			return
		}
		out = append(out, map[string]any{"id": id, "channel_id": channel, "handle": handle, "channel_name": name, "status": status, "paid_through": end, "active": status == "active" && end.After(time.Now())})
	}
	s.result(w, out, rows.Err())
}
func (s *Server) payoutAccount(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	var account, country string
	var charges, payouts bool
	e := s.Store.DB.QueryRowContext(r.Context(), `SELECT provider_id,country,charges_enabled,payouts_enabled FROM cloud_payout_accounts WHERE user_id=$1`, u.ID).Scan(&account, &country, &charges, &payouts)
	if e != nil && e != sql.ErrNoRows {
		s.fail(w, e)
		return
	}
	if r.Method == http.MethodGet {
		s.json(w, 200, map[string]any{"configured": account != "", "country": country, "charges_enabled": charges, "payouts_enabled": payouts, "onboarding_enabled": s.Config.ConnectEnabled && s.Config.StripeSecret != "", "cash_payouts_enabled": s.Config.CashPayoutsEnabled})
		return
	}
	if !s.Config.ConnectEnabled || s.Config.StripeSecret == "" || !u.Verified {
		s.fail(w, ErrUnavailable)
		return
	}
	var b struct {
		Country string `json:"country"`
	}
	if !s.decode(w, r, &b) {
		return
	}
	b.Country = strings.ToUpper(strings.TrimSpace(b.Country))
	allowed := false
	for _, c := range strings.Split(s.Config.SupportedPayoutCountries, ",") {
		if strings.ToUpper(strings.TrimSpace(c)) == b.Country && len(b.Country) == 2 {
			allowed = true
		}
	}
	if !allowed {
		s.fail(w, ErrValidation)
		return
	}
	if account == "" {
		var out struct {
			ID string `json:"id"`
		}
		e = s.stripeForm(r.Context(), "accounts", "connect:"+u.ID, url.Values{"type": {"express"}, "country": {b.Country}, "email": {u.Email}, "capabilities[transfers][requested]": {"true"}, "capabilities[card_payments][requested]": {"true"}}, &out)
		if e != nil || out.ID == "" {
			s.fail(w, ErrUnavailable)
			return
		}
		account = out.ID
		_, e = s.Store.DB.ExecContext(r.Context(), `INSERT INTO cloud_payout_accounts(user_id,provider_id,country)VALUES($1,$2,$3)ON CONFLICT(user_id)DO NOTHING`, u.ID, account, b.Country)
		if e != nil {
			s.fail(w, e)
			return
		}
		e = s.Store.DB.QueryRowContext(r.Context(), `SELECT provider_id,country FROM cloud_payout_accounts WHERE user_id=$1`, u.ID).Scan(&account, &country)
		if e != nil {
			s.fail(w, e)
			return
		}
	}
	if country != b.Country {
		s.fail(w, ErrValidation)
		return
	}
	var out struct {
		URL string `json:"url"`
	}
	e = s.stripeForm(r.Context(), "account_links", "", url.Values{"account": {account}, "type": {"account_onboarding"}, "refresh_url": {s.Config.PublicOrigin + "/creator?onboarding=refresh"}, "return_url": {s.Config.PublicOrigin + "/creator?onboarding=return"}}, &out)
	if e == nil && !strings.HasPrefix(out.URL, "https://connect.stripe.com/") {
		e = ErrUnavailable
	}
	s.result(w, map[string]string{"url": out.URL}, e)
}
func (s *Server) creatorEarnings(w http.ResponseWriter, r *http.Request) {
	rows, e := s.Store.DB.QueryContext(r.Context(), `SELECT id,reference,amount_cents,currency,status,description,created_at FROM cloud_earnings WHERE user_id=$1 ORDER BY created_at DESC LIMIT 100`, currentUser(r).ID)
	if e != nil {
		s.fail(w, e)
		return
	}
	defer rows.Close()
	entries := []map[string]any{}
	for rows.Next() {
		var id, reference, currency, status, description string
		var n int64
		var created time.Time
		if e = rows.Scan(&id, &reference, &n, &currency, &status, &description, &created); e != nil {
			s.fail(w, e)
			return
		}
		entries = append(entries, map[string]any{"id": id, "reference": reference, "amount_cents": n, "currency": currency, "status": status, "description": description, "created_at": created})
	}
	if e = rows.Err(); e != nil {
		s.fail(w, e)
		return
	}
	rows.Close()
	var available, pending, paid int64
	e = s.Store.DB.QueryRowContext(r.Context(), `SELECT coalesce(sum(amount_cents)FILTER(WHERE status='available'),0),coalesce(sum(amount_cents)FILTER(WHERE status='pending'),0),coalesce(sum(amount_cents)FILTER(WHERE status='paid'),0) FROM cloud_earnings WHERE user_id=$1 AND currency='usd'`, currentUser(r).ID).Scan(&available, &pending, &paid)
	s.result(w, map[string]any{"available_cents": available, "pending_cents": pending, "paid_cents": paid, "currency": "usd", "rewards_enabled": s.Config.RewardsEnabled, "payouts_enabled": s.Config.CashPayoutsEnabled, "entries": entries}, e)
}

// StripeID accepts expanded objects as well as provider ID strings.
type stripeID string

func (id *stripeID) UnmarshalJSON(raw []byte) error {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		*id = stripeID(s)
		return nil
	}
	var o struct {
		ID string `json:"id"`
	}
	if e := json.Unmarshal(raw, &o); e != nil {
		return e
	}
	*id = stripeID(o.ID)
	return nil
}

type membershipSubscription struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	PeriodEnd int64  `json:"current_period_end"`
	Metadata  struct {
		CheckoutID string `json:"membership_checkout_id"`
	} `json:"metadata"`
	Items struct {
		Data []struct {
			PeriodEnd int64 `json:"current_period_end"`
		} `json:"data"`
	} `json:"items"`
}

func (s *Server) getMembershipSubscription(ctx context.Context, id string) (membershipSubscription, error) {
	var sub membershipSubscription
	if id == "" || s.Config.StripeSecret == "" {
		return sub, ErrValidation
	}
	e := s.stripeForm(ctx, "subscriptions/"+url.PathEscape(id), "", nil, &sub)
	if e == nil && (sub.ID != id || sub.Metadata.CheckoutID == "") {
		e = ErrValidation
	}
	for _, item := range sub.Items.Data {
		if item.PeriodEnd > sub.PeriodEnd {
			sub.PeriodEnd = item.PeriodEnd
		}
	}
	return sub, e
}

// processMembershipWebhook is called only after signature validation by the
// webhook entrypoint. It owns its receipt transaction; callers must not insert
// cloud_webhook_receipts before invoking it. Provider state is re-read for
// subscription lifecycle events so delayed webhooks cannot restore stale access.
func (s *Server) processMembershipWebhook(ctx context.Context, eventID, eventType string, rawObject json.RawMessage) error {
	if eventID == "" {
		return ErrValidation
	}
	switch eventType {
	case "invoice.paid":
		return s.membershipInvoicePaid(ctx, eventID, rawObject)
	case "charge.refunded":
		return s.membershipRefunded(ctx, eventID, rawObject)
	case "customer.subscription.updated", "customer.subscription.deleted", "customer.subscription.created":
		var object struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(rawObject, &object) != nil {
			return ErrValidation
		}
		sub, e := s.getMembershipSubscription(ctx, object.ID)
		if e != nil {
			return e
		}
		tx, e := s.Store.DB.BeginTx(ctx, nil)
		if e != nil {
			return e
		}
		defer tx.Rollback()
		fresh, e := commerceReceipt(ctx, tx, eventID)
		if e != nil || !fresh {
			return e
		}
		// Paid-through remains invoice authority. Past_due does not extend it, and
		// cancellation with an already-paid remaining period keeps existing access.
		if sub.Status == "canceled" || sub.Status == "unpaid" || sub.Status == "incomplete_expired" {
			_, e = tx.ExecContext(ctx, `UPDATE cloud_memberships SET status='expired' WHERE provider_id=$1 AND paid_through<=now() AND status='active'`, sub.ID)
		}
		if e != nil {
			return e
		}
		return tx.Commit()
	case "checkout.session.completed", "checkout.session.async_payment_succeeded":
		var object struct {
			ID           string   `json:"id"`
			Subscription stripeID `json:"subscription"`
			Metadata     struct {
				CheckoutID string `json:"membership_checkout_id"`
			} `json:"metadata"`
		}
		if json.Unmarshal(rawObject, &object) != nil {
			return ErrValidation
		}
		if object.Metadata.CheckoutID == "" {
			return nil
		}
		sub, e := s.getMembershipSubscription(ctx, string(object.Subscription))
		if e != nil {
			return e
		}
		if sub.Metadata.CheckoutID != object.Metadata.CheckoutID {
			return ErrValidation
		}
		_, e = s.Store.DB.ExecContext(ctx, `UPDATE cloud_membership_checkouts SET provider_subscription=$2 WHERE id=$1 AND (provider_session IS NULL OR provider_session=$3) AND (provider_subscription IS NULL OR provider_subscription=$2)`, object.Metadata.CheckoutID, sub.ID, object.ID)
		return e
	case "account.updated":
		var obj struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(rawObject, &obj) != nil || obj.ID == "" {
			return ErrValidation
		}
		var current struct {
			Charges bool `json:"charges_enabled"`
			Payouts bool `json:"payouts_enabled"`
		}
		if e := s.stripeForm(ctx, "accounts/"+url.PathEscape(obj.ID), "", nil, &current); e != nil {
			return e
		}
		_, e := s.Store.DB.ExecContext(ctx, `UPDATE cloud_payout_accounts SET charges_enabled=$2,payouts_enabled=$3,updated_at=now() WHERE provider_id=$1`, obj.ID, current.Charges, current.Payouts)
		return e
	}
	return nil
}
func commerceReceipt(ctx context.Context, tx *sql.Tx, id string) (bool, error) {
	res, e := tx.ExecContext(ctx, `INSERT INTO cloud_webhook_receipts(id)VALUES($1)ON CONFLICT DO NOTHING`, id)
	if e != nil {
		return false, e
	}
	n, e := res.RowsAffected()
	return n == 1, e
}
func (s *Server) membershipInvoicePaid(ctx context.Context, eventID string, raw json.RawMessage) error {
	var obj struct {
		ID            string   `json:"id"`
		Subscription  stripeID `json:"subscription"`
		PaymentIntent stripeID `json:"payment_intent"`
		AmountPaid    int64    `json:"amount_paid"`
		Currency      string   `json:"currency"`
		Parent        struct {
			SubscriptionDetails struct {
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
		return ErrValidation
	}
	var manual struct {
		PaidOutOfBand bool `json:"paid_out_of_band"`
	}
	if json.Unmarshal(raw, &manual) != nil || manual.PaidOutOfBand {
		return ErrValidation
	}
	if obj.Subscription == "" {
		obj.Subscription = obj.Parent.SubscriptionDetails.Subscription
	}
	if obj.Subscription == "" {
		return nil
	}
	if obj.PaymentIntent == "" {
		// Event payload API versions are fixed by the webhook endpoint. Read the
		// invoice through our pinned API version to retain a refund correlation.
		var current struct {
			ID            string   `json:"id"`
			PaymentIntent stripeID `json:"payment_intent"`
		}
		if e := s.stripeForm(ctx, "invoices/"+url.PathEscape(obj.ID), "", nil, &current); e != nil {
			return e
		}
		if current.ID != obj.ID || current.PaymentIntent == "" {
			return ErrUnavailable
		}
		obj.PaymentIntent = current.PaymentIntent
	}
	sub, e := s.getMembershipSubscription(ctx, string(obj.Subscription))
	if e != nil {
		return e
	}
	var periodEnd int64
	for _, line := range obj.Lines.Data {
		if line.Period.End > periodEnd {
			periodEnd = line.Period.End
		}
	}
	if periodEnd == 0 {
		periodEnd = sub.PeriodEnd
	}
	if periodEnd <= 0 {
		return ErrValidation
	}
	tx, e := s.Store.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	fresh, e := commerceReceipt(ctx, tx, eventID)
	if e != nil || !fresh {
		return e
	}
	var user, channel, creator, currency string
	var price, bps int64
	e = tx.QueryRowContext(ctx, `SELECT p.user_id,p.channel_id,c.user_id,p.price_cents,p.currency,p.commission_bps FROM cloud_membership_checkouts p JOIN cloud_channels c ON c.id=p.channel_id WHERE p.id=$1 AND (p.provider_subscription IS NULL OR p.provider_subscription=$2) FOR UPDATE OF p`, sub.Metadata.CheckoutID, sub.ID).Scan(&user, &channel, &creator, &price, &currency, &bps)
	if e != nil {
		return e
	}
	if obj.AmountPaid < price || obj.Currency != currency {
		return ErrValidation
	}
	// Serialize all subscriptions for the same viewer/channel, including a new
	// subscription racing an older subscription's final webhook.
	_, e = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "membership:"+user+":"+channel)
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `UPDATE cloud_membership_checkouts SET provider_subscription=$2 WHERE id=$1`, sub.Metadata.CheckoutID, sub.ID)
	if e != nil {
		return e
	}
	share := new(big.Int).Mul(big.NewInt(price), big.NewInt(10000-bps))
	share.Div(share, big.NewInt(10000))
	creatorCents := share.Int64()
	var payment any
	if obj.PaymentIntent != "" {
		payment = string(obj.PaymentIntent)
	}
	res, e := tx.ExecContext(ctx, `INSERT INTO cloud_membership_invoices(id,checkout_id,payment_intent,gross_cents,creator_cents,currency,period_end)VALUES($1,$2,$3,$4,$5,$6,$7)ON CONFLICT(id)DO NOTHING`, obj.ID, sub.Metadata.CheckoutID, payment, obj.AmountPaid, creatorCents, currency, time.Unix(periodEnd, 0))
	if e != nil {
		return e
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return tx.Commit()
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO cloud_earnings(id,user_id,reference,amount_cents,currency,status,description)VALUES($1,$2,$3,$4,$5,'pending','Creator membership revenue share; held pending reconciliation')`, uuid.NewString(), creator, "membership:invoice:"+obj.ID, creatorCents, currency)
	if e != nil {
		return e
	}
	active := sub.Status == "active" || sub.Status == "trialing" || sub.Status == "past_due" || sub.Status == "canceled"
	_, e = tx.ExecContext(ctx, `INSERT INTO cloud_memberships(id,user_id,channel_id,status,paid_through,provider_id)VALUES($1,$2,$3,$4,$5,$6)ON CONFLICT(user_id,channel_id)DO UPDATE SET status=CASE WHEN EXCLUDED.paid_through>cloud_memberships.revoked_through AND EXCLUDED.status='active' THEN 'active' ELSE cloud_memberships.status END,paid_through=GREATEST(cloud_memberships.paid_through,EXCLUDED.paid_through),provider_id=CASE WHEN EXCLUDED.paid_through>=cloud_memberships.paid_through THEN EXCLUDED.provider_id ELSE cloud_memberships.provider_id END`, uuid.NewString(), user, channel, map[bool]string{true: "active", false: "revoked"}[active], time.Unix(periodEnd, 0), sub.ID)
	if e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Server) membershipRefunded(ctx context.Context, eventID string, raw json.RawMessage) error {
	var obj struct {
		PaymentIntent stripeID `json:"payment_intent"`
		Amount        int64    `json:"amount"`
		Refunded      int64    `json:"amount_refunded"`
	}
	if json.Unmarshal(raw, &obj) != nil {
		return ErrValidation
	}
	if obj.PaymentIntent == "" {
		return nil
	}
	tx, e := s.Store.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	fresh, e := commerceReceipt(ctx, tx, eventID)
	if e != nil || !fresh {
		return e
	}
	var invoice, creator, currency, user, channel string
	var gross, share, previous int64
	var end time.Time
	e = tx.QueryRowContext(ctx, `SELECT i.id,c.user_id,i.currency,i.gross_cents,i.creator_cents,i.refunded_cents,p.user_id,p.channel_id,i.period_end FROM cloud_membership_invoices i JOIN cloud_membership_checkouts p ON p.id=i.checkout_id JOIN cloud_channels c ON c.id=p.channel_id WHERE i.payment_intent=$1`, string(obj.PaymentIntent)).Scan(&invoice, &creator, &currency, &gross, &share, &previous, &user, &channel, &end)
	// A charge event may arrive before the invoice event. Do not acknowledge and
	// lose it: the provider will retry after the invoice has been reconciled.
	if e != nil {
		return e
	}
	if obj.Refunded < 0 || obj.Refunded > gross {
		return ErrValidation
	}
	// Serialize refund adjustments against the final payout eligibility check.
	if _, e = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,814037226))`, creator); e != nil {
		return e
	}
	// Match invoice processing lock order: scope lock precedes invoice row lock.
	_, e = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "membership:"+user+":"+channel)
	if e != nil {
		return e
	}
	if e = tx.QueryRowContext(ctx, `SELECT refunded_cents FROM cloud_membership_invoices WHERE id=$1 FOR UPDATE`, invoice).Scan(&previous); e != nil {
		return e
	}
	if obj.Refunded <= previous {
		return tx.Commit()
	}
	cumulative := func(n int64) int64 {
		v := new(big.Int).Mul(big.NewInt(share), big.NewInt(n))
		v.Div(v, big.NewInt(gross))
		return v.Int64()
	}
	delta := cumulative(obj.Refunded) - cumulative(previous)
	_, e = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "membership:"+user+":"+channel)
	if e != nil {
		return e
	}
	if delta > 0 {
		_, e = tx.ExecContext(ctx, `INSERT INTO cloud_earnings(id,user_id,reference,amount_cents,currency,status,description)VALUES($1,$2,$3,$4,$5,'pending','Membership refund adjustment')`, uuid.NewString(), creator, "membership:refund:"+invoice+":"+intString(obj.Refunded), -delta, currency)
		if e != nil {
			return e
		}
	}
	_, e = tx.ExecContext(ctx, `UPDATE cloud_membership_invoices SET refunded_cents=$2 WHERE id=$1`, invoice, obj.Refunded)
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `UPDATE cloud_memberships SET revoked_through=GREATEST(revoked_through,$3),status=CASE WHEN paid_through<=$3 THEN 'revoked' ELSE status END WHERE user_id=$1 AND channel_id=$2`, user, channel, end)
	if e != nil {
		return e
	}
	return tx.Commit()
}

// Legacy payment records remain reconcilable; new channel sales are retired.
func (s *Server) membershipCheckout(w http.ResponseWriter, r *http.Request) {
	s.json(w, http.StatusGone, map[string]string{"code": "channel_memberships_retired", "message": "Premium unlocks videos across all creators. Use /premium/checkout."})
}
func (s *Server) membershipPlan(w http.ResponseWriter, r *http.Request) {
	s.json(w, http.StatusGone, map[string]string{"code": "channel_memberships_retired", "message": "Individual channel subscription plans are retired."})
}
