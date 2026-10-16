package cloud

import (
	"context"
	"database/sql"
	"strings"
)

type RewardFunding struct {
	ID                string `json:"id"`
	Reference         string `json:"reference"`
	Kind              string `json:"kind"`
	Cents             int64  `json:"cents"`
	EvidenceReference string `json:"evidence_reference"`
	ProposedBy        string `json:"proposed_by"`
	ReconciledBy      string `json:"reconciled_by"`
}

// RecordRewardFunding records a finance reconciliation, not an arbitrary deposit
// or automatic revenue recognition. The caller must own the reconciled evidence.
func (s *Store) RecordRewardFunding(ctx context.Context, f RewardFunding) error {
	if f.ID == "" || f.Reference == "" || f.Cents <= 0 || strings.TrimSpace(f.EvidenceReference) == "" || f.ProposedBy == "" || f.ReconciledBy == "" || f.ProposedBy == f.ReconciledBy || (f.Kind != "generation" && f.Kind != "advertising" && f.Kind != "support") {
		return ErrValidation
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT INTO cloud_reward_sources(id,reference,kind,cents,evidence_reference,proposed_by,reconciled_by)VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(id) DO NOTHING`, f.ID, f.Reference, f.Kind, f.Cents, f.EvidenceReference, f.ProposedBy, f.ReconciledBy); err != nil {
		return err
	}
	var existing RewardFunding
	err = tx.QueryRowContext(ctx, `SELECT id,reference,kind,cents,evidence_reference,proposed_by,reconciled_by FROM cloud_reward_sources WHERE id=$1`, f.ID).Scan(&existing.ID, &existing.Reference, &existing.Kind, &existing.Cents, &existing.EvidenceReference, &existing.ProposedBy, &existing.ReconciledBy)
	if err != nil {
		return err
	}
	if existing != f {
		return ErrConflict
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO cloud_admin_audit(actor,action,resource_id,reason) SELECT $1,'reconcile_reward_funding',$2,$3 WHERE NOT EXISTS(SELECT 1 FROM cloud_admin_audit WHERE action='reconcile_reward_funding' AND resource_id=$2)`, f.ReconciledBy, f.ID, f.EvidenceReference); err != nil {
		return err
	}
	return tx.Commit()
}

func reserveRewardFunding(ctx context.Context, tx *sql.Tx, source string, cents int64) error {
	result, err := tx.ExecContext(ctx, `UPDATE cloud_reward_sources SET reserved_cents=reserved_cents+$2 WHERE id=$1 AND kind<>'support' AND cents-reserved_cents >= $2`, source, cents)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrInsufficientCredits
	}
	return nil
}
