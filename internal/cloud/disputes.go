package cloud

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
)

// Disputes fail closed: do not spend disputed prepaid credits or send creator
// payouts until won/withdrawn. A loss requires audited financial reconciliation;
// it is never silently turned into a refund or a second automatic charge.
func (s *Server) paymentDispute(ctx context.Context, eventID string, raw json.RawMessage) error {
	var obj struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(raw, &obj) != nil || obj.ID == "" {
		return ErrValidation
	}
	tx, err := s.Store.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Serialize before re-reading provider state: delayed events cannot restore an
	// old open status over a newer terminal decision.
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,814037225))`, obj.ID); err != nil {
		return err
	}
	fresh, err := commerceReceipt(ctx, tx, eventID)
	if err != nil {
		return err
	}
	if !fresh {
		return nil
	}
	var current struct {
		ID            string   `json:"id"`
		PaymentIntent stripeID `json:"payment_intent"`
		Status        string   `json:"status"`
		Amount        int64    `json:"amount"`
		Currency      string   `json:"currency"`
	}
	if err = s.stripeForm(ctx, "disputes/"+url.PathEscape(obj.ID), "", nil, &current); err != nil {
		return err
	}
	if current.ID != obj.ID || current.PaymentIntent == "" || current.Amount <= 0 || current.Currency == "" {
		return ErrValidation
	}
	switch current.Status {
	case "needs_response", "under_review", "warning_needs_response", "warning_under_review", "lost", "won", "warning_closed":
	default:
		return ErrValidation
	}
	var workspace, creator sql.NullString
	var gross int64
	var currency string
	err = tx.QueryRowContext(ctx, `SELECT workspace_id,price_cents,currency FROM cloud_purchases WHERE payment_intent=$1`, string(current.PaymentIntent)).Scan(&workspace, &gross, &currency)
	if errors.Is(err, sql.ErrNoRows) {
		err = tx.QueryRowContext(ctx, `SELECT c.user_id,i.gross_cents,i.currency FROM cloud_membership_invoices i JOIN cloud_membership_checkouts p ON p.id=i.checkout_id JOIN cloud_channels c ON c.id=p.channel_id WHERE i.payment_intent=$1`, string(current.PaymentIntent)).Scan(&creator, &gross, &currency)
	}
	// Unknown correlation can be a dispute delivered before its payment webhook.
	// Roll back the receipt so Stripe retries after that payment is recorded.
	if err != nil {
		return err
	}
	if current.Amount > gross || current.Currency != currency {
		return ErrValidation
	}
	if creator.Valid {
		if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,814037226))`, creator.String); err != nil {
			return err
		}
	}
	if workspace.Valid {
		if _, err = tx.ExecContext(ctx, `SELECT workspace_id FROM cloud_credit_accounts WHERE workspace_id=$1 FOR UPDATE`, workspace.String); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO cloud_payment_disputes(id,payment_intent,status,amount_cents,currency,workspace_id,creator_id)VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(id) DO UPDATE SET status=EXCLUDED.status,amount_cents=EXCLUDED.amount_cents,updated_at=now()`, current.ID, string(current.PaymentIntent), current.Status, current.Amount, current.Currency, workspace, creator)
	if err != nil {
		return err
	}
	if workspace.Valid {
		_, err = tx.ExecContext(ctx, `UPDATE cloud_credit_accounts SET dispute_hold=EXISTS(SELECT 1 FROM cloud_payment_disputes WHERE workspace_id=$1 AND status NOT IN ('won','warning_closed')) WHERE workspace_id=$1`, workspace.String)
		if err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO cloud_admin_audit(actor,action,resource_id,reason)VALUES('stripe:webhook','payment_dispute',$1,$2)`, current.ID, current.Status)
	if err != nil {
		return err
	}
	return tx.Commit()
}
