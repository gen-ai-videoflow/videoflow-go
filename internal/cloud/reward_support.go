package cloud

import (
	"context"
	"database/sql"
	"github.com/google/uuid"
	"strings"
)

type RewardSupportProgram struct {
	Reason              string `json:"reason"`
	ID                  string `json:"id"`
	Title               string `json:"title"`
	Criteria            string `json:"criteria"`
	SourceID            string `json:"source_id"`
	BudgetCents         int64  `json:"budget_cents"`
	GrantCredits        int64  `json:"grant_credits"`
	ServiceCostCents    int64  `json:"service_cost_cents"`
	CreatorCap          int    `json:"creator_cap"`
	GrantCap            int    `json:"grant_cap"`
	ExpiryDays          int    `json:"expiry_days"`
	MinimumPublications int    `json:"minimum_publications"`
}

func (p RewardSupportProgram) valid() bool {
	return len(p.Reason) <= 2000 && len(p.ID) > 0 && len(p.ID) <= 128 && len(strings.TrimSpace(p.Title)) > 0 && len(p.Title) <= 120 && len(strings.TrimSpace(p.Criteria)) > 0 && len(p.Criteria) <= 2000 && p.BudgetCents > 0 && p.BudgetCents <= 100000000 && p.GrantCredits > 0 && p.GrantCredits <= 1000000 && p.ServiceCostCents > 0 && p.ServiceCostCents <= 1000000 && p.BudgetCents >= p.ServiceCostCents && p.CreatorCap > 0 && p.CreatorCap <= 10 && p.GrantCap > 0 && p.GrantCap <= 10000 && p.ExpiryDays > 0 && p.ExpiryDays <= 365 && p.MinimumPublications > 0 && p.MinimumPublications <= 100
}
func (s *Store) ProposeRewardSupport(ctx context.Context, p RewardSupportProgram, actor string) error {
	if !p.valid() || actor == "" {
		return ErrValidation
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var kind string
	if e = tx.QueryRowContext(ctx, `SELECT kind FROM cloud_reward_sources WHERE id=$1`, p.SourceID).Scan(&kind); e != nil {
		return e
	}
	if kind != "support" {
		return ErrValidation
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO cloud_reward_support_programs(id,title,criteria,source_id,budget_cents,grant_credits,service_cost_cents,creator_cap,grant_cap,expiry_days,minimum_publications,proposed_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, p.ID, p.Title, p.Criteria, p.SourceID, p.BudgetCents, p.GrantCredits, p.ServiceCostCents, p.CreatorCap, p.GrantCap, p.ExpiryDays, p.MinimumPublications, actor)
	if e != nil {
		return e
	}
	reason := p.Reason
	if strings.TrimSpace(reason) == "" {
		reason = p.Criteria
	}
	if e = supportAudit(ctx, tx, actor, "propose_support", p.ID, reason); e != nil {
		return e
	}
	return tx.Commit()
}
func supportAudit(ctx context.Context, tx *sql.Tx, actor, action, id, reason string) error {
	_, e := tx.ExecContext(ctx, `INSERT INTO cloud_admin_audit(actor,action,resource_id,reason)VALUES($1,$2,$3,$4)`, actor, action, id, reason)
	return e
}
func (s *Store) ApproveRewardSupport(ctx context.Context, id, actor, reason string) error {
	if actor == "" || strings.TrimSpace(reason) == "" || len(reason) > 2000 {
		return ErrValidation
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	r, e := tx.ExecContext(ctx, `UPDATE cloud_reward_support_programs SET approved_by=$2,enabled=true WHERE id=$1 AND proposed_by<>$2 AND approved_by IS NULL`, id, actor)
	if e != nil {
		return e
	}
	n, _ := r.RowsAffected()
	if n != 1 {
		return ErrConflict
	}
	if e = supportAudit(ctx, tx, actor, "approve_support", id, reason); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) ClaimRewardSupport(ctx context.Context, u User, program, evidence string) (string, error) {
	if strings.TrimSpace(evidence) == "" || len(evidence) > 2000 {
		return "", ErrValidation
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return "", e
	}
	defer tx.Rollback()
	var locked string
	if e = tx.QueryRowContext(ctx, `SELECT id FROM cloud_users WHERE id=$1 FOR NO KEY UPDATE`, u.ID).Scan(&locked); e != nil {
		return "", e
	}
	var eligible bool
	e = tx.QueryRowContext(ctx, `SELECT enabled AND approved_by IS NOT NULL AND granted<grant_cap AND committed_cents+service_cost_cents<=budget_cents AND (SELECT count(*) FROM cloud_videos v JOIN cloud_channels c ON c.id=v.channel_id WHERE c.user_id=$2 AND v.status='published' AND v.visibility='public')>=minimum_publications AND (SELECT count(*) FROM cloud_reward_support_claims WHERE program_id=p.id AND user_id=$2 AND status='approved')<creator_cap FROM cloud_reward_support_programs p WHERE id=$1 FOR UPDATE`, program, u.ID).Scan(&eligible)
	if e != nil {
		return "", e
	}
	if !eligible {
		return "", ErrConflict
	}
	var open int
	if e = tx.QueryRowContext(ctx, `SELECT count(*) FROM cloud_reward_support_claims WHERE user_id=$1 AND status='pending'`, u.ID).Scan(&open); e != nil {
		return "", e
	}
	if open >= 10 {
		return "", ErrConflict
	}
	id := uuid.NewString()
	_, e = tx.ExecContext(ctx, `INSERT INTO cloud_reward_support_claims(id,program_id,user_id,workspace_id,evidence)VALUES($1,$2,$3,$4,$5)`, id, program, u.ID, u.WorkspaceID, evidence)
	if e != nil {
		return "", e
	}
	if e = supportAudit(ctx, tx, u.ID, "submit_support_claim", id, program); e != nil {
		return "", e
	}
	return id, tx.Commit()
}
func (s *Store) ReviewRewardSupport(ctx context.Context, id, actor, reason string, approve bool) error {
	if actor == "" || strings.TrimSpace(reason) == "" || len(reason) > 2000 {
		return ErrValidation
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var program, user, workspace, status string
	// Program lock serializes all creator/global cap checks and funding reservations.
	if e = tx.QueryRowContext(ctx, `SELECT program_id,user_id,workspace_id,status FROM cloud_reward_support_claims WHERE id=$1`, id).Scan(&program, &user, &workspace, &status); e != nil {
		return e
	}
	var credits, cost int64
	var days int
	var source string
	var eligible bool
	e = tx.QueryRowContext(ctx, `SELECT grant_credits,service_cost_cents,expiry_days,source_id,enabled AND approved_by IS NOT NULL AND granted<grant_cap AND committed_cents+service_cost_cents<=budget_cents AND (SELECT count(*) FROM cloud_reward_support_claims WHERE program_id=p.id AND user_id=$2 AND status='approved')<creator_cap AND (SELECT count(*) FROM cloud_videos v JOIN cloud_channels c ON c.id=v.channel_id WHERE c.user_id=$2 AND v.status='published' AND v.visibility='public')>=minimum_publications FROM cloud_reward_support_programs p WHERE id=$1 FOR UPDATE`, program, user).Scan(&credits, &cost, &days, &source, &eligible)
	if e != nil {
		return e
	}
	if e = tx.QueryRowContext(ctx, `SELECT status FROM cloud_reward_support_claims WHERE id=$1 FOR UPDATE`, id).Scan(&status); e != nil {
		return e
	}
	if status != "pending" || actor == user {
		return ErrConflict
	}
	next := "rejected"
	if approve {
		if !eligible {
			return ErrConflict
		}
		r, e := tx.ExecContext(ctx, `UPDATE cloud_reward_sources SET reserved_cents=reserved_cents+$2 WHERE id=$1 AND kind='support' AND cents-reserved_cents >= $2`, source, cost)
		if e != nil {
			return e
		}
		n, _ := r.RowsAffected()
		if n != 1 {
			return ErrInsufficientCredits
		}
		if _, e = tx.ExecContext(ctx, `UPDATE cloud_reward_support_programs SET committed_cents=committed_cents+$2,granted=granted+1 WHERE id=$1`, program, cost); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO cloud_promotional_credits(id,workspace_id,credits,service_cost_cents,expires_at)VALUES($1,$2,$3,$4,now()+$5*interval '1 day')`, id, workspace, credits, cost, days); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO cloud_promotional_credit_events(grant_id,kind,credits)VALUES($1,'grant',$2)`, id, credits); e != nil {
			return e
		}
		next = "approved"
	}
	if _, e = tx.ExecContext(ctx, `UPDATE cloud_reward_support_claims SET status=$2,reviewed_by=$3,reason=$4,reviewed_at=now() WHERE id=$1`, id, next, actor, reason); e != nil {
		return e
	}
	if e = supportAudit(ctx, tx, actor, "review_support_"+next, id, reason); e != nil {
		return e
	}
	if e = rewardNotifyTx(ctx, tx, user, "support:"+id, "support", "Your promotional credit request was "+next+". "+reason); e != nil {
		return e
	}
	return tx.Commit()
}
