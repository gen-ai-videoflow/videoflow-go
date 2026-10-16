package cloud

import (
	"context"
	"database/sql"
	"github.com/google/uuid"
	"strings"
)

type RewardLegacyBackingProposal struct {
	ID                string `json:"id"`
	EarningID         string `json:"earning_id"`
	SourceID          string `json:"source_id"`
	Reason            string `json:"reason"`
	EvidenceReference string `json:"evidence_reference"`
}

func (s *Store) ProposeRewardLegacyBacking(ctx context.Context, p RewardLegacyBackingProposal, actor string) (string, error) {
	if p.EarningID == "" || p.SourceID == "" || actor == "" || strings.TrimSpace(p.Reason) == "" || len(p.Reason) > 1000 || strings.TrimSpace(p.EvidenceReference) == "" || len(p.EvidenceReference) > 2000 {
		return "", ErrValidation
	}
	p.ID = uuid.NewString()
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return "", e
	}
	defer tx.Rollback()
	var user string
	var cents int64
	e = tx.QueryRowContext(ctx, `SELECT user_id,amount_cents FROM cloud_earnings WHERE id=$1 AND amount_cents>0 AND currency='usd' AND status<>'paid' AND reference NOT LIKE 'vesting:%' AND reference NOT LIKE 'correction:%' AND NOT cloud_reward_earning_backed(id)`, p.EarningID).Scan(&user, &cents)
	if e != nil {
		return "", e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO cloud_reward_legacy_backing_proposals(id,earning_id,user_id,source_id,cents,reason,evidence_reference,proposed_by)VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, p.ID, p.EarningID, user, p.SourceID, cents, p.Reason, p.EvidenceReference, actor)
	if e != nil {
		return "", e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO cloud_admin_audit(actor,action,resource_id,reason)VALUES($1,'propose_legacy_reward_backing',$2,$3)`, actor, p.ID, p.Reason)
	if e != nil {
		return "", e
	}
	return p.ID, tx.Commit()
}
func (s *Store) ApproveRewardLegacyBacking(ctx context.Context, id, actor, reason string) error {
	if actor == "" || strings.TrimSpace(reason) == "" || len(reason) > 1000 {
		return ErrValidation
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var user string
	e = tx.QueryRowContext(ctx, `SELECT user_id FROM cloud_reward_legacy_backing_proposals WHERE id=$1`, id).Scan(&user)
	if e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,814037226))`, user); e != nil {
		return e
	}
	var earning, source, proposer string
	var cents int64
	var approved sql.NullString
	e = tx.QueryRowContext(ctx, `SELECT earning_id,source_id,cents,proposed_by,approved_by FROM cloud_reward_legacy_backing_proposals WHERE id=$1 FOR UPDATE`, id).Scan(&earning, &source, &cents, &proposer, &approved)
	if e != nil {
		return e
	}
	if actor == proposer || actor == user {
		return ErrUnauthorized
	}
	if approved.Valid {
		if approved.String == actor {
			return tx.Commit()
		}
		return ErrConflict
	}
	var actual int64
	var actualUser string
	e = tx.QueryRowContext(ctx, `SELECT amount_cents,user_id FROM cloud_earnings WHERE id=$1 AND amount_cents>0 AND currency='usd' AND status<>'paid' AND reference NOT LIKE 'vesting:%' AND reference NOT LIKE 'correction:%' AND NOT cloud_reward_earning_backed(id) FOR UPDATE`, earning).Scan(&actual, &actualUser)
	if e != nil {
		return e
	}
	if actual != cents || actualUser != user {
		return ErrConflict
	}
	var eligible bool
	e = tx.QueryRowContext(ctx, `SELECT kind<>'support' AND NOT EXISTS(SELECT 1 FROM cloud_reward_revenue_source_holds h WHERE h.source_id=s.id) AND NOT EXISTS(SELECT 1 FROM cloud_sponsor_campaigns c WHERE c.source_id=s.id) FROM cloud_reward_sources s WHERE id=$1 FOR UPDATE`, source).Scan(&eligible)
	if e != nil {
		return e
	}
	if !eligible {
		return ErrValidation
	}
	if e = reserveRewardFunding(ctx, tx, source, cents); e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO cloud_reward_legacy_backing(earning_id,user_id,source_id,cents,proposal_id)VALUES($1,$2,$3,$4,$5)`, earning, user, source, cents, id)
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `UPDATE cloud_reward_legacy_backing_proposals SET approved_by=$2,approved_at=now() WHERE id=$1`, id, actor)
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO cloud_admin_audit(actor,action,resource_id,reason)VALUES($1,'approve_legacy_reward_backing',$2,$3)`, actor, id, reason)
	if e != nil {
		return e
	}
	return tx.Commit()
}
