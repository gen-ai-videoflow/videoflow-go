package cloud

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
)

func (s *Server) creditRefund(ctx context.Context, eventID string, raw json.RawMessage) (bool, error) {
	var event struct {
		PaymentIntent stripeID `json:"payment_intent"`
		Amount        int64    `json:"amount"`
		Refunded      int64    `json:"amount_refunded"`
	}
	if json.Unmarshal(raw, &event) != nil {
		return false, ErrValidation
	}
	if event.PaymentIntent == "" {
		return false, nil
	}
	tx, err := s.Store.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var id, workspace, status string
	var total, credits, revoked, oldRefund int64
	err = tx.QueryRowContext(ctx, `SELECT id,workspace_id,status,price_cents,credits,revoked_credits,refunded_cents FROM cloud_purchases WHERE payment_intent=$1 FOR UPDATE`, string(event.PaymentIntent)).Scan(&id, &workspace, &status, &total, &credits, &revoked, &oldRefund)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return true, err
	}
	if total != event.Amount || event.Refunded < 0 || event.Refunded > total {
		return true, ErrValidation
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO cloud_webhook_receipts(id)VALUES($1)ON CONFLICT DO NOTHING`, eventID)
	if err != nil {
		return true, err
	}
	n, _ := res.RowsAffected()
	if n == 0 || event.Refunded <= oldRefund {
		return true, tx.Commit()
	}
	// Round cumulative revoked units upward; each event applies only the delta.
	cumulative := (credits*event.Refunded + total - 1) / total
	delta := cumulative - revoked
	var balance, reserved int64
	if err = tx.QueryRowContext(ctx, `SELECT balance,reserved FROM cloud_credit_accounts WHERE workspace_id=$1 FOR UPDATE`, workspace).Scan(&balance, &reserved); err != nil {
		return true, err
	}
	taken := delta
	if taken > balance-reserved {
		taken = balance - reserved
	}
	if _, err = tx.ExecContext(ctx, `UPDATE cloud_credit_accounts SET balance=balance-$2,debt=debt+$3 WHERE workspace_id=$1`, workspace, taken, delta-taken); err != nil {
		return true, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO cloud_credit_entries(id,workspace_id,reference,amount,kind,description)VALUES($1,$2,$3,$4,'refund','Refunded credit purchase')`, uuid.NewString(), workspace, "refund:"+eventID, -delta); err != nil {
		return true, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE cloud_purchases SET refunded_cents=$2,revoked_credits=$3 WHERE id=$1`, id, event.Refunded, cumulative); err != nil {
		return true, err
	}
	return true, tx.Commit()
}
