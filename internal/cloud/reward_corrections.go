package cloud

import (
	"context"
	"database/sql"
	"github.com/google/uuid"
	"strings"
	"time"
)

type RewardCorrection struct {
	ID                string `json:"id"`
	OriginalBatchID   string `json:"original_batch_id"`
	UserID            string `json:"user_id"`
	Cents             int64  `json:"cents"`
	SourceID          string `json:"source_id"`
	Reason            string `json:"reason"`
	EvidenceReference string `json:"evidence_reference"`
}

func (c RewardCorrection) valid() bool {
	return c.ID != "" && len(c.ID) <= 128 && c.OriginalBatchID != "" && c.UserID != "" && c.Cents != 0 && c.Cents >= -100000000 && c.Cents <= 100000000 && strings.TrimSpace(c.Reason) != "" && len(c.Reason) <= 1000 && strings.TrimSpace(c.EvidenceReference) != "" && len(c.EvidenceReference) <= 2000 && ((c.Cents > 0 && c.SourceID != "") || (c.Cents < 0 && c.SourceID == ""))
}
func (s *Store) ProposeRewardCorrection(ctx context.Context, c RewardCorrection, actor string) error {
	if !c.valid() || actor == "" {
		return ErrValidation
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var duration int64
	e = tx.QueryRowContext(ctx, `SELECT p.vesting_seconds FROM cloud_reward_batches b JOIN cloud_reward_policies p ON p.version=b.policy_version WHERE b.id=$1 AND b.status='confirmed' AND EXISTS(SELECT 1 FROM cloud_reward_allocations a WHERE a.batch_id=b.id AND a.user_id=$2)`, c.OriginalBatchID, c.UserID).Scan(&duration)
	if e == sql.ErrNoRows {
		return ErrNotFound
	}
	if e != nil {
		return e
	}
	var source any
	if c.SourceID != "" {
		source = c.SourceID
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO cloud_reward_corrections(id,original_batch_id,user_id,cents,source_id,reason,evidence_reference,vesting_seconds,proposed_by)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT(id) DO NOTHING`, c.ID, c.OriginalBatchID, c.UserID, c.Cents, source, c.Reason, c.EvidenceReference, duration, actor)
	if e != nil {
		return e
	}
	var existing RewardCorrection
	var proposer string
	e = tx.QueryRowContext(ctx, `SELECT id,original_batch_id,user_id,cents,coalesce(source_id,''),reason,evidence_reference,proposed_by FROM cloud_reward_corrections WHERE id=$1`, c.ID).Scan(&existing.ID, &existing.OriginalBatchID, &existing.UserID, &existing.Cents, &existing.SourceID, &existing.Reason, &existing.EvidenceReference, &proposer)
	if e != nil {
		return e
	}
	if existing != c || proposer != actor {
		return ErrConflict
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO cloud_admin_audit(actor,action,resource_id,reason)SELECT $1,'propose_reward_correction',$2,$3 WHERE NOT EXISTS(SELECT 1 FROM cloud_admin_audit WHERE action='propose_reward_correction' AND resource_id=$2)`, actor, c.ID, c.Reason)
	if e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) ApproveRewardCorrection(ctx context.Context, id, actor, reason string) error {
	if actor == "" || strings.TrimSpace(reason) == "" || len(reason) > 1000 {
		return ErrValidation
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	// Match payouts/refunds: creator advisory lock precedes any financial row lock.
	var user string
	if e = tx.QueryRowContext(ctx, `SELECT user_id FROM cloud_reward_corrections WHERE id=$1`, id).Scan(&user); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,814037226))`, user); e != nil {
		return e
	}
	var c RewardCorrection
	var proposer string
	var approved sql.NullString
	e = tx.QueryRowContext(ctx, `SELECT id,original_batch_id,user_id,cents,coalesce(source_id,''),reason,evidence_reference,proposed_by,approved_by FROM cloud_reward_corrections WHERE id=$1 FOR UPDATE`, id).Scan(&c.ID, &c.OriginalBatchID, &c.UserID, &c.Cents, &c.SourceID, &c.Reason, &c.EvidenceReference, &proposer, &approved)
	if e != nil {
		return e
	}
	if proposer == actor || user == actor {
		return ErrUnauthorized
	}
	if approved.Valid {
		if approved.String != actor {
			return ErrConflict
		}
		return tx.Commit()
	}
	var principal, previous int64
	var originalSource string
	e = tx.QueryRowContext(ctx, `SELECT b.source_id,(SELECT coalesce(sum(cents),0) FROM cloud_reward_allocations WHERE batch_id=b.id AND user_id=$2),(SELECT coalesce(sum(cents),0) FROM cloud_reward_corrections WHERE original_batch_id=b.id AND user_id=$2 AND approved_by IS NOT NULL) FROM cloud_reward_batches b WHERE b.id=$1 AND b.status='confirmed'`, c.OriginalBatchID, user).Scan(&originalSource, &principal, &previous)
	if e != nil {
		return e
	}
	if principal == 0 || principal+previous+c.Cents < 0 {
		return ErrConflict
	}
	if c.Cents > 0 {
		var enabled bool
		policyErr := tx.QueryRowContext(ctx, `SELECT coalesce((policy->>'enabled')::boolean,false) AND coalesce((policy->>'confirmations_enabled')::boolean,false) FROM cloud_reward_engine_policy WHERE id=true FOR SHARE`).Scan(&enabled)
		if policyErr != nil && policyErr != sql.ErrNoRows {
			return policyErr
		}
		if policyErr == nil && !enabled {
			return ErrUnavailable
		}
		if c.SourceID == originalSource {
			return ErrValidation
		}
		var available int64
		e = tx.QueryRowContext(ctx, `SELECT cents-reserved_cents FROM cloud_reward_sources s WHERE id=$1 AND kind<>'support' AND NOT EXISTS(SELECT 1 FROM cloud_reward_revenue_source_holds h WHERE h.source_id=s.id) AND NOT EXISTS(SELECT 1 FROM cloud_sponsor_campaigns x WHERE x.source_id=s.id) FOR UPDATE`, c.SourceID).Scan(&available)
		if e == sql.ErrNoRows {
			return ErrConflict
		}
		if e != nil {
			return e
		}
		if available < c.Cents {
			return ErrInsufficientCredits
		}
		if _, e = tx.ExecContext(ctx, `UPDATE cloud_reward_sources SET reserved_cents=reserved_cents+$2 WHERE id=$1`, c.SourceID, c.Cents); e != nil {
			return e
		}
	}
	if _, e = tx.ExecContext(ctx, `UPDATE cloud_reward_corrections SET approved_by=$2,approved_at=now(),approval_reason=$3 WHERE id=$1`, id, actor, reason); e != nil {
		return e
	}
	if c.Cents > 0 {
		if _, e = tx.ExecContext(ctx, `INSERT INTO cloud_reward_correction_releases(correction_id)VALUES($1)`, id); e != nil {
			return e
		}
	} else {
		earning := uuid.NewString()
		if _, e = tx.ExecContext(ctx, `INSERT INTO cloud_earnings(id,user_id,reference,amount_cents,currency,status,description)VALUES($1,$2,$3,$4,'usd','available','Approved creator reward correction')`, earning, user, "correction:"+id, c.Cents); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO cloud_reward_correction_debts(correction_id,user_id,earning_id)VALUES($1,$2,$3)`, id, user, earning); e != nil {
			return e
		}
		var needsHold bool
		if e = tx.QueryRowContext(ctx, `SELECT (SELECT coalesce(sum(amount_cents),0) FROM cloud_earnings WHERE user_id=$1 AND currency='usd' AND (status='available' OR (status='pending' AND amount_cents<0)))<0 OR EXISTS(SELECT 1 FROM cloud_payout_batches WHERE user_id=$1 AND status IN('prepared','submitting'))`, user).Scan(&needsHold); e != nil {
			return e
		}
		if needsHold {
			if _, e = tx.ExecContext(ctx, `INSERT INTO cloud_reward_holds(user_id,reason,actor)VALUES($1,'Approved reward correction; reconcile debt and outstanding payments before release',$2) ON CONFLICT(user_id) DO NOTHING`, user, actor); e != nil {
				return e
			}
		}
	}
	if e = supportAudit(ctx, tx, actor, "approve_reward_correction", id, reason); e != nil {
		return e
	}
	message := "A funded reward correction of " + intString(c.Cents) + " USD cents was approved and will unlock gradually. "
	if c.Cents < 0 {
		message = "A reward correction of " + intString(c.Cents) + " USD cents was applied to your earnings. Payment availability may be held pending review. "
	}
	if e = rewardNotifyTx(ctx, tx, user, "correction:"+id, "correction", message+c.Reason+" Reference: "+id); e != nil {
		return e
	}
	return tx.Commit()
}

func releaseRewardCorrectionsTx(ctx context.Context, tx *sql.Tx, user string, at time.Time) error {
	rows, e := tx.QueryContext(ctx, `SELECT c.id,c.cents,c.approved_at,c.vesting_seconds,r.released_cents FROM cloud_reward_corrections c JOIN cloud_reward_correction_releases r ON r.correction_id=c.id WHERE c.user_id=$1 AND c.approved_by IS NOT NULL AND c.cents>0 ORDER BY c.id FOR UPDATE OF r`, user)
	if e != nil {
		return e
	}
	type release struct {
		id                    string
		cents, seconds, prior int64
		approved              time.Time
	}
	items := []release{}
	for rows.Next() {
		var x release
		if e = rows.Scan(&x.id, &x.cents, &x.approved, &x.seconds, &x.prior); e != nil {
			rows.Close()
			return e
		}
		items = append(items, x)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, x := range items {
		vested, e := VestedCents(x.cents, x.approved, at, time.Duration(x.seconds)*time.Second)
		if e != nil {
			return e
		}
		if vested <= x.prior {
			continue
		}
		earning := uuid.NewString()
		reference := "correction:release:" + x.id + ":" + intString(vested)
		if _, e = tx.ExecContext(ctx, `INSERT INTO cloud_earnings(id,user_id,reference,amount_cents,currency,status,description)VALUES($1,$2,$3,$4,'usd','available','Vested funded reward correction')`, earning, user, reference, vested-x.prior); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO cloud_reward_correction_events(id,correction_id,through_cents,earning_id)VALUES($1,$2,$3,$4)`, uuid.NewString(), x.id, vested, earning); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, `UPDATE cloud_reward_correction_releases SET released_cents=$2,updated_at=now() WHERE correction_id=$1`, x.id, vested); e != nil {
			return e
		}
		if e = rewardNotifyTx(ctx, tx, user, reference, "release", "An approved reward correction has finished unlocking in part or in full. See Earnings for payment availability."); e != nil {
			return e
		}
	}
	return nil
}

// Corrections join the same repeatable-read summary snapshot; released cents
// are subtracted only from the virtual maturity, never from principal twice.
func correctionBalanceTx(ctx context.Context, tx *sql.Tx, user string, at time.Time, out *RewardBalance) (int64, error) {
	var released int64
	rows, e := tx.QueryContext(ctx, `SELECT c.cents,c.approved_at,c.vesting_seconds,r.released_cents FROM cloud_reward_corrections c JOIN cloud_reward_correction_releases r ON r.correction_id=c.id WHERE c.user_id=$1 AND c.approved_by IS NOT NULL AND c.cents>0`, user)
	if e != nil {
		return 0, e
	}
	for rows.Next() {
		var cents, seconds, prior int64
		var approved time.Time
		if e = rows.Scan(&cents, &approved, &seconds, &prior); e != nil {
			rows.Close()
			return 0, e
		}
		vested, e := VestedCents(cents, approved, at, time.Duration(seconds)*time.Second)
		if e != nil {
			rows.Close()
			return 0, e
		}
		out.ConfirmedCents += cents
		out.VestedCents += vested
		out.UnvestedCents += cents - vested
		released += prior
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return 0, e
	}
	e = tx.QueryRowContext(ctx, `SELECT coalesce(sum(cents),0) FROM cloud_reward_corrections WHERE user_id=$1 AND cents<0 AND approved_by IS NOT NULL`, user).Scan(&out.AdjustmentCents)
	return released, e
}
