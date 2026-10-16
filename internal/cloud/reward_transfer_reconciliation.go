package cloud

import (
	"context"
	"github.com/go-chi/chi/v5"
	"net/http"
	"net/url"
	"strings"
)

// ReconcileRewardTransfer only retrieves a known provider transfer. It never
// retries an ambiguous POST beyond the provider idempotency retention window.
func (s *Server) ReconcileRewardTransfer(ctx context.Context, batchID, transferID, actor, reason string) error {
	if !strings.HasPrefix(transferID, "tr_") || len(transferID) > 200 || actor == "" || strings.TrimSpace(reason) == "" {
		return ErrValidation
	}
	conn, e := s.Store.DB.Conn(ctx)
	if e != nil {
		return e
	}
	defer conn.Close()
	if _, e = conn.ExecContext(ctx, `SELECT pg_advisory_lock(hashtextextended($1,814037221))`, batchID); e != nil {
		return e
	}
	defer conn.ExecContext(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock(hashtextextended($1,814037221))`, batchID)
	b, e := scanPayout(conn.QueryRowContext(ctx, `SELECT `+payoutColumns+` FROM cloud_payout_batches WHERE id=$1`, batchID))
	if e != nil {
		return e
	}
	if b.Status == "paid" {
		if b.TransferID == transferID {
			return nil
		}
		return ErrConflict
	}
	if b.Status != "submitting" {
		return ErrConflict
	}
	var transfer struct {
		ID             string            `json:"id"`
		Amount         int64             `json:"amount"`
		Currency       string            `json:"currency"`
		Destination    stripeID          `json:"destination"`
		AmountReversed int64             `json:"amount_reversed"`
		Reversed       bool              `json:"reversed"`
		Metadata       map[string]string `json:"metadata"`
	}
	if e = s.stripeForm(ctx, "transfers/"+url.PathEscape(transferID), "", nil, &transfer); e != nil {
		return e
	}
	if transfer.ID != transferID || transfer.Amount != b.AmountCents || transfer.Currency != b.Currency || string(transfer.Destination) != b.Destination || transfer.Reversed || transfer.AmountReversed != 0 || transfer.Metadata["batch_id"] != batchID {
		return ErrConflict
	}
	tx, e := conn.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, `UPDATE cloud_payout_batches SET status='paid',provider_transfer=$2,paid_at=now() WHERE id=$1 AND status='submitting'`, batchID, transferID); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO cloud_reward_legacy_transfer_exceptions(earning_id,batch_id,transfer_id,actor,reason) SELECT earning.id,$1,$2,$3,$4 FROM cloud_payout_batch_entries pe JOIN cloud_earnings earning ON earning.id=pe.earning_id WHERE pe.batch_id=$1 AND earning.amount_cents>0 AND NOT cloud_reward_earning_backed(earning.id) ON CONFLICT(earning_id) DO NOTHING`, batchID, transferID, actor, reason); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE cloud_earnings SET status='paid' WHERE id IN(SELECT earning_id FROM cloud_payout_batch_entries WHERE batch_id=$1)`, batchID); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO cloud_admin_audit(actor,action,resource_id,reason)VALUES($1,'reconcile_payout_transfer',$2,$3)`, actor, batchID, reason+"; "+transferID); e != nil {
		return e
	}
	if e = rewardNotifyTx(ctx, tx, b.UserID, "payout-transferred:"+batchID, "payout", "Your creator payment was transferred to your connected payment account. Bank arrival depends on your payment provider."); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Server) adminRewardReconcileTransfer(w http.ResponseWriter, r *http.Request) {
	var b struct {
		TransferID string `json:"transfer_id"`
		Reason     string `json:"reason"`
	}
	if !s.decode(w, r, &b) {
		return
	}
	e := s.ReconcileRewardTransfer(r.Context(), chi.URLParam(r, "id"), b.TransferID, currentUser(r).ID, b.Reason)
	s.result(w, map[string]bool{"reconciled": e == nil}, e)
}
