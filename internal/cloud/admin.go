package cloud

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"net/url"
	"strings"
	"time"
)

type PayoutBatch struct {
	ID          string `json:"id"`
	UserID      string `json:"user_id"`
	Destination string `json:"destination"`
	Country     string `json:"country"`
	AmountCents int64  `json:"amount_cents"`
	Currency    string `json:"currency"`
	Status      string `json:"status"`
	TransferID  string `json:"transfer_id"`
}

const payoutColumns = `id,user_id,destination,country,amount_cents,currency,status,COALESCE(provider_transfer,'')`

func scanPayout(row interface{ Scan(...any) error }) (PayoutBatch, error) {
	var b PayoutBatch
	err := row.Scan(&b.ID, &b.UserID, &b.Destination, &b.Country, &b.AmountCents, &b.Currency, &b.Status, &b.TransferID)
	return b, err
}
func (s *Store) BlockVideo(ctx context.Context, id, reason, actor string) error {
	if strings.TrimSpace(reason) == "" || actor == "" {
		return ErrValidation
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE cloud_videos SET status='blocked' WHERE id=$1 AND status<>'deleted'`, id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return ErrNotFound
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO cloud_admin_audit(actor,action,resource_id,reason)VALUES($1,'block_video',$2,$3)`, actor, id, reason); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) OpenReports(ctx context.Context) ([]map[string]any, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id,video_id,reason,created_at FROM cloud_reports WHERE status='open' ORDER BY created_at LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, video, reason string
		var created time.Time
		if err = rows.Scan(&id, &video, &reason, &created); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"id": id, "video_id": video, "reason": reason, "created_at": created})
	}
	return out, rows.Err()
}
func (s *Store) ResolveProviderJob(ctx context.Context, id, providerID, actor string, fail bool) error {
	if actor == "" || (providerID == "") == !fail {
		return ErrValidation
	}
	if fail { // Check state, release reservation, and audit under one job row lock.
		return s.finishJob(ctx, id, 0, "failed", "", "Operator reconciled ambiguous provider submission: "+actor, actor)
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE cloud_jobs SET provider_id=$2,status='running',lease_until=NULL,updated_at=now() WHERE id=$1 AND status='reconciliation_required' AND provider_id=''`, id, providerID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return ErrConflict
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO cloud_admin_audit(actor,action,resource_id,reason)VALUES($1,'resolve_job_provider',$2,$3)`, actor, id, providerID); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) PreparePayout(ctx context.Context, userID, actor string) (PayoutBatch, error) {
	if actor == "" {
		return PayoutBatch{}, ErrValidation
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return PayoutBatch{}, err
	}
	defer tx.Rollback()
	// Keep creator lock ordering identical to dispute/refund writers. Acquire
	// the advisory lock before touching the user row or earnings FK rows.
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,814037226))`, userID); err != nil {
		return PayoutBatch{}, err
	}
	var locked string
	if err = tx.QueryRowContext(ctx, `SELECT id FROM cloud_users WHERE id=$1 FOR NO KEY UPDATE`, userID).Scan(&locked); err != nil {
		return PayoutBatch{}, err
	}
	var disputed bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cloud_payment_disputes WHERE creator_id=$1 AND status NOT IN ('won','warning_closed'))`, userID).Scan(&disputed); err != nil {
		return PayoutBatch{}, err
	}
	if disputed {
		return PayoutBatch{}, ErrConflict
	}
	existing, e := scanPayout(tx.QueryRowContext(ctx, `SELECT `+payoutColumns+` FROM cloud_payout_batches WHERE user_id=$1 AND status IN ('prepared','submitting')`, userID))
	if e == nil {
		return existing, tx.Commit()
	}
	if !errors.Is(e, sql.ErrNoRows) {
		return PayoutBatch{}, e
	}
	var destination, country string
	if err = tx.QueryRowContext(ctx, `SELECT provider_id,country FROM cloud_payout_accounts WHERE user_id=$1 AND charges_enabled AND payouts_enabled`, userID).Scan(&destination, &country); err != nil {
		return PayoutBatch{}, ErrUnauthorized
	}
	// Mature credits and include all negative adjustments, even during their hold.
	if _, err = tx.ExecContext(ctx, `UPDATE cloud_earnings SET status='available' WHERE user_id=$1 AND currency='usd' AND status='pending' AND created_at<=now()-interval '30 days'`, userID); err != nil {
		return PayoutBatch{}, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,amount_cents FROM cloud_earnings WHERE user_id=$1 AND currency='usd' AND (status='available' OR (status='pending' AND amount_cents<0)) FOR UPDATE`, userID)
	if err != nil {
		return PayoutBatch{}, err
	}
	ids := []string{}
	var amount int64
	for rows.Next() {
		var id string
		var value int64
		if err = rows.Scan(&id, &value); err != nil {
			rows.Close()
			return PayoutBatch{}, err
		}
		ids = append(ids, id)
		amount += value
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return PayoutBatch{}, err
	}
	if amount < 5000 {
		return PayoutBatch{}, ErrInsufficientCredits
	}
	b := PayoutBatch{ID: uuid.NewString(), UserID: userID, Destination: destination, Country: country, AmountCents: amount, Currency: "usd", Status: "prepared"}
	if _, err = tx.ExecContext(ctx, `INSERT INTO cloud_payout_batches(id,user_id,destination,country,amount_cents,currency,status)VALUES($1,$2,$3,$4,$5,'usd','prepared')`, b.ID, userID, destination, country, amount); err != nil {
		return PayoutBatch{}, err
	}
	for _, id := range ids {
		if _, err = tx.ExecContext(ctx, `INSERT INTO cloud_payout_batch_entries(batch_id,earning_id)VALUES($1,$2)`, b.ID, id); err != nil {
			return PayoutBatch{}, err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE cloud_earnings SET status='held' WHERE id=$1`, id); err != nil {
			return PayoutBatch{}, err
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO cloud_admin_audit(actor,action,resource_id)VALUES($1,'prepare_payout',$2)`, actor, b.ID); err != nil {
		return PayoutBatch{}, err
	}
	return b, tx.Commit()
}
func (s *Server) ExecutePayout(ctx context.Context, batchID, actor string, execute bool) error {
	if !execute || !s.Config.CashPayoutsEnabled || s.Config.StripeSecret == "" || actor == "" {
		return ErrUnauthorized
	}
	conn, err := s.Store.DB.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err = conn.ExecContext(ctx, `SELECT pg_advisory_lock(hashtextextended($1,814037221))`, batchID); err != nil {
		return err
	}
	defer conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock(hashtextextended($1,814037221))`, batchID)
	b, err := scanPayout(conn.QueryRowContext(ctx, `SELECT `+payoutColumns+` FROM cloud_payout_batches WHERE id=$1`, batchID))
	if err != nil {
		return err
	}
	if b.Status == "paid" {
		return nil
	}
	if _, err = conn.ExecContext(ctx, `SELECT pg_advisory_lock(hashtextextended($1,814037226))`, b.UserID); err != nil {
		return err
	}
	defer conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock(hashtextextended($1,814037226))`, b.UserID)
	var disputed bool
	if err = conn.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cloud_payment_disputes WHERE creator_id=$1 AND status NOT IN ('won','warning_closed'))`, b.UserID).Scan(&disputed); err != nil {
		return err
	}
	if disputed {
		return ErrConflict
	}
	var first sql.NullTime
	if err = conn.QueryRowContext(ctx, `SELECT first_attempt_at FROM cloud_payout_batches WHERE id=$1`, batchID).Scan(&first); err != nil {
		return err
	}
	// A refund posted after preparation must reduce the spendable creator balance.
	// Pending positive earnings do not cover a matured payout; negative entries do.
	if !first.Valid {
		var net int64
		if err = conn.QueryRowContext(ctx, `SELECT COALESCE(sum(amount_cents),0) FROM cloud_earnings WHERE user_id=$1 AND currency=$2 AND (status='available' OR (status='pending' AND amount_cents<0) OR id IN(SELECT earning_id FROM cloud_payout_batch_entries WHERE batch_id=$3))`, b.UserID, b.Currency, batchID).Scan(&net); err != nil {
			return err
		}
		if net < b.AmountCents {
			return ErrConflict
		}
	}
	if first.Valid && time.Since(first.Time) > 23*time.Hour {
		return fmt.Errorf("%w: old ambiguous payout requires provider reconciliation", ErrConflict)
	}
	if !hasValue(strings.Split(s.Config.SupportedPayoutCountries, ","), b.Country) {
		return ErrUnauthorized
	}
	var account struct {
		ID           string            `json:"id"`
		Country      string            `json:"country"`
		Charges      bool              `json:"charges_enabled"`
		Payouts      bool              `json:"payouts_enabled"`
		Details      bool              `json:"details_submitted"`
		Capabilities map[string]string `json:"capabilities"`
	}
	if err = s.stripeForm(ctx, "accounts/"+url.PathEscape(b.Destination), "", nil, &account); err != nil {
		return err
	}
	if account.ID != b.Destination || account.Country != b.Country || !account.Charges || !account.Payouts || !account.Details || account.Capabilities["transfers"] != "active" {
		return ErrUnauthorized
	}
	var funds struct {
		Available []struct {
			Amount   int64  `json:"amount"`
			Currency string `json:"currency"`
		} `json:"available"`
	}
	if err = s.stripeForm(ctx, "balance", "", nil, &funds); err != nil {
		return err
	}
	var available int64
	for _, f := range funds.Available {
		if f.Currency == b.Currency {
			available += f.Amount
		}
	}
	if !first.Valid && available < b.AmountCents {
		return ErrInsufficientCredits
	}
	if _, err = conn.ExecContext(ctx, `WITH changed AS (UPDATE cloud_payout_batches SET status='submitting',first_attempt_at=COALESCE(first_attempt_at,now()) WHERE id=$1 RETURNING id) INSERT INTO cloud_admin_audit(actor,action,resource_id)SELECT $2,'attempt_payout',id FROM changed`, batchID, actor); err != nil {
		return err
	}
	var transfer struct {
		ID          string   `json:"id"`
		Amount      int64    `json:"amount"`
		Currency    string   `json:"currency"`
		Destination stripeID `json:"destination"`
	}
	if err = s.stripeForm(ctx, "transfers", "payout:"+batchID, url.Values{"amount": {intString(b.AmountCents)}, "currency": {b.Currency}, "destination": {b.Destination}, "metadata[batch_id]": {batchID}}, &transfer); err != nil {
		return err
	}
	if transfer.ID == "" || transfer.Amount != b.AmountCents || transfer.Currency != b.Currency || string(transfer.Destination) != b.Destination {
		return ErrConflict
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `UPDATE cloud_payout_batches SET status='paid',provider_transfer=$2,paid_at=now() WHERE id=$1`, batchID, transfer.ID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE cloud_earnings SET status='paid' WHERE id IN(SELECT earning_id FROM cloud_payout_batch_entries WHERE batch_id=$1)`, batchID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO cloud_admin_audit(actor,action,resource_id,reason)VALUES($1,'execute_payout',$2,$3)`, actor, batchID, transfer.ID); err != nil {
		return err
	}
	return tx.Commit()
}
