package cloud

import (
	"context"
	"github.com/google/uuid"
	"strings"
)

func (s *Store) SubmitRewardAppeal(ctx context.Context, user, allocation, subject, evidence string) (string, error) {
	if strings.TrimSpace(evidence) == "" || len(evidence) > 4000 || !map[string]bool{"eligibility": true, "hold": true, "allocation": true, "support": true, "payout": true}[subject] || (subject == "allocation" && allocation == "") {
		return "", ErrValidation
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return "", e
	}
	defer tx.Rollback()
	var locked string
	if e = tx.QueryRowContext(ctx, `SELECT id FROM cloud_users WHERE id=$1 FOR NO KEY UPDATE`, user).Scan(&locked); e != nil {
		return "", e
	}
	var count int
	if e = tx.QueryRowContext(ctx, `SELECT count(*) FROM cloud_reward_appeals WHERE user_id=$1 AND status='open'`, user).Scan(&count); e != nil {
		return "", e
	}
	if count >= 10 {
		return "", ErrConflict
	}
	var target any
	if allocation != "" {
		var owns bool
		if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cloud_reward_allocations WHERE id=$1 AND user_id=$2)`, allocation, user).Scan(&owns); e != nil {
			return "", e
		}
		if !owns {
			return "", ErrNotFound
		}
		target = allocation
	}
	id := uuid.NewString()
	if _, e = tx.ExecContext(ctx, `INSERT INTO cloud_reward_appeals(id,user_id,allocation_id,subject,evidence)VALUES($1,$2,$3,$4,$5)`, id, user, target, subject, evidence); e != nil {
		return "", e
	}
	if e = supportAudit(ctx, tx, user, "submit_reward_appeal", id, subject); e != nil {
		return "", e
	}
	return id, tx.Commit()
}
func (s *Store) ResolveRewardAppeal(ctx context.Context, id, actor, resolution string, upheld bool) error {
	if actor == "" || strings.TrimSpace(resolution) == "" || len(resolution) > 2000 {
		return ErrValidation
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var user, status string
	if e = tx.QueryRowContext(ctx, `SELECT user_id,status FROM cloud_reward_appeals WHERE id=$1 FOR UPDATE`, id).Scan(&user, &status); e != nil {
		return e
	}
	if actor == user || status != "open" {
		return ErrConflict
	}
	status = "declined"
	if upheld {
		status = "upheld"
	}
	if _, e = tx.ExecContext(ctx, `UPDATE cloud_reward_appeals SET status=$2,reviewer=$3,resolution=$4,resolved_at=now() WHERE id=$1`, id, status, actor, resolution); e != nil {
		return e
	}
	if e = supportAudit(ctx, tx, actor, "resolve_reward_appeal_"+status, id, resolution); e != nil {
		return e
	}
	if e = rewardNotifyTx(ctx, tx, user, "appeal:"+id, "appeal", "Your reward appeal was "+status+". "+resolution); e != nil {
		return e
	}
	return tx.Commit()
}
