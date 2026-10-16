package cloud

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/google/uuid"
	"strings"
	"time"
)

type RewardQualityWeights struct {
	Watch       int64 `json:"watch"`
	Retention   int64 `json:"retention"`
	Originality int64 `json:"originality"`
	Trust       int64 `json:"trust"`
}
type RewardEnginePolicy struct {
	Enabled               bool                 `json:"enabled"`
	EarningWindowDays     int64                `json:"earning_window_days"`
	VestingDays           int64                `json:"vesting_days"`
	PayoutThresholdCents  int64                `json:"payout_threshold_cents"`
	PayoutIntervalDays    int64                `json:"payout_interval_days"`
	FundingBPS            int64                `json:"funding_bps"`
	ReserveBPS            int64                `json:"reserve_bps"`
	DailyCapCents         int64                `json:"daily_cap_cents"`
	MinimumQualifiedViews int64                `json:"minimum_qualified_views"`
	QualityWeights        RewardQualityWeights `json:"quality_weights"`
	EstimatesEnabled      bool                 `json:"estimates_enabled"`
	ConfirmationsEnabled  bool                 `json:"confirmations_enabled"`
	PayoutsEnabled        bool                 `json:"payouts_enabled"`
}
type RewardEnginePolicyRecord struct {
	Policy        RewardEnginePolicy `json:"policy"`
	Revision      int64              `json:"revision"`
	PolicyVersion string             `json:"policy_version"`
	UpdatedAt     *time.Time         `json:"updated_at"`
}

func DefaultRewardEnginePolicy() RewardEnginePolicy {
	return RewardEnginePolicy{EarningWindowDays: 30, VestingDays: 30, PayoutThresholdCents: 5000, PayoutIntervalDays: 30, MinimumQualifiedViews: 5, QualityWeights: RewardQualityWeights{4000, 3000, 2000, 1000}}
}
func validateRewardEnginePolicy(p RewardEnginePolicy) error {
	if p.EarningWindowDays < 1 || p.EarningWindowDays > 90 || p.VestingDays < 1 || p.VestingDays > 365 || p.PayoutThresholdCents < 100 || p.PayoutThresholdCents > 10000000 || p.PayoutIntervalDays < 1 || p.PayoutIntervalDays > 90 || p.FundingBPS < 0 || p.FundingBPS > 10000 || p.ReserveBPS < 0 || p.ReserveBPS > 10000 || p.DailyCapCents < 0 || p.DailyCapCents > 100000000 || p.MinimumQualifiedViews < 1 || p.MinimumQualifiedViews > 1000000 {
		return ErrValidation
	}
	w := p.QualityWeights
	sum := int64(0)
	for _, n := range []int64{w.Watch, w.Retention, w.Originality, w.Trust} {
		if n < 0 || n > 10000 {
			return ErrValidation
		}
		sum += n
	}
	if sum != 10000 {
		return ErrValidation
	}
	if p.Enabled && (p.FundingBPS == 0 || p.DailyCapCents == 0) {
		return ErrValidation
	}
	return nil
}
func (s *Store) RewardEnginePolicy(ctx context.Context) (RewardEnginePolicyRecord, error) {
	r := RewardEnginePolicyRecord{Policy: DefaultRewardEnginePolicy(), PolicyVersion: "pilot-v1"}
	var raw []byte
	var at time.Time
	err := s.DB.QueryRowContext(ctx, `SELECT policy,revision,policy_version,updated_at FROM cloud_reward_engine_policy WHERE id=true`).Scan(&raw, &r.Revision, &r.PolicyVersion, &at)
	if err == sql.ErrNoRows {
		return r, nil
	}
	if err != nil {
		return r, err
	}
	if err = json.Unmarshal(raw, &r.Policy); err != nil {
		return r, err
	}
	if err = validateRewardEnginePolicy(r.Policy); err != nil {
		return r, err
	}
	r.UpdatedAt = &at
	return r, nil
}
func (s *Store) ProposeRewardEnginePolicy(ctx context.Context, p RewardEnginePolicy, revision int64, reason, actor string) (string, error) {
	if validateRewardEnginePolicy(p) != nil || revision < 0 || strings.TrimSpace(reason) == "" || len(reason) > 1000 || actor == "" {
		return "", ErrValidation
	}
	current, err := s.RewardEnginePolicy(ctx)
	if err != nil {
		return "", err
	}
	if current.Revision != revision {
		return "", ErrConflict
	}
	raw, _ := json.Marshal(p)
	id := uuid.NewString()
	_, err = s.DB.ExecContext(ctx, `INSERT INTO cloud_reward_engine_proposals(id,base_revision,policy,reason,proposed_by)VALUES($1,$2,$3,$4,$5)`, id, revision, raw, reason, actor)
	return id, err
}
func (s *Store) ApproveRewardEnginePolicy(ctx context.Context, id, actor string) error {
	if actor == "" {
		return ErrValidation
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(814037240)`); err != nil {
		return err
	}
	var raw []byte
	var base int64
	var proposer, reason, status string
	err = tx.QueryRowContext(ctx, `SELECT policy,base_revision,proposed_by,reason,status FROM cloud_reward_engine_proposals WHERE id=$1 FOR UPDATE`, id).Scan(&raw, &base, &proposer, &reason, &status)
	if err != nil {
		return err
	}
	if proposer == actor {
		return ErrUnauthorized
	}
	if status == "approved" {
		return tx.Commit()
	}
	var current int64
	err = tx.QueryRowContext(ctx, `SELECT revision FROM cloud_reward_engine_policy WHERE id=true FOR UPDATE`).Scan(&current)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if base != current {
		return ErrConflict
	}
	var p RewardEnginePolicy
	if json.Unmarshal(raw, &p) != nil || validateRewardEnginePolicy(p) != nil {
		return ErrValidation
	}
	version := "rewards-" + id
	terms := version
	var priorVersion, priorTerms string
	var priorWindow, priorVesting int64
	err = tx.QueryRowContext(ctx, `SELECT p.version,p.terms_version,p.window_seconds,p.vesting_seconds FROM cloud_reward_policies p WHERE p.version=coalesce((SELECT policy_version FROM cloud_reward_engine_policy WHERE id=true),'pilot-v1')`).Scan(&priorVersion, &priorTerms, &priorWindow, &priorVesting)
	if err != nil {
		return err
	}
	sameTerms := priorWindow == p.EarningWindowDays*86400 && priorVesting == p.VestingDays*86400
	if sameTerms {
		terms = priorTerms
	}

	if _, err = tx.ExecContext(ctx, `UPDATE cloud_reward_policies SET active=false WHERE active`); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO cloud_reward_policies(version,terms_version,active,window_seconds,vesting_seconds,created_by)VALUES($1,$6,$2,$3,$4,$5)`, version, p.Enabled, p.EarningWindowDays*86400, p.VestingDays*86400, actor, terms); err != nil {
		return err
	}
	if sameTerms {
		if _, err = tx.ExecContext(ctx, `INSERT INTO cloud_reward_enrollments(user_id,policy_version,accepted_at) SELECT user_id,$1,accepted_at FROM cloud_reward_enrollments WHERE policy_version=$2 ON CONFLICT DO NOTHING`, version, priorVersion); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO cloud_reward_engine_policy(id,revision,policy_version,policy,updated_by)VALUES(true,$1,$2,$3,$4)ON CONFLICT(id)DO UPDATE SET revision=excluded.revision,policy_version=excluded.policy_version,policy=excluded.policy,updated_at=now(),updated_by=excluded.updated_by`, current+1, version, raw, actor); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO cloud_reward_engine_policy_history(revision,policy_version,policy,reason,proposed_by,approved_by)VALUES($1,$2,$3,$4,$5,$6)`, current+1, version, raw, reason, proposer, actor); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE cloud_reward_engine_proposals SET status='approved',approved_by=$2,approved_at=now() WHERE id=$1`, id, actor); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO cloud_admin_audit(actor,action,resource_id,reason)VALUES($1,'approve_reward_policy',$2,$3)`, actor, id, reason); err != nil {
		return err
	}
	return tx.Commit()
}
