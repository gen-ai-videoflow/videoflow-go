package cloud

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"github.com/google/uuid"
	"sort"
	"strings"
	"time"
)

type RewardBatchDraft struct {
	ID                string           `json:"id"`
	Day               string           `json:"day"`
	SourceID          string           `json:"source_id"`
	PolicyVersion     string           `json:"policy_version"`
	EvidenceReference string           `json:"evidence_reference"`
	ProposedBy        string           `json:"proposed_by"`
	BudgetCents       int64            `json:"budget_cents"`
	Weights           map[string]int64 `json:"weights"`
}

// DraftRewardBatch consumes finance-reviewed, qualified evidence weights. It
// does not infer payable evidence from public view counts or customer spending.
func (s *Store) DraftRewardBatch(ctx context.Context, d RewardBatchDraft) error {
	day, err := time.Parse("2006-01-02", d.Day)
	if err != nil || !day.Before(time.Now().UTC().Truncate(24*time.Hour)) || d.ID == "" || d.SourceID == "" || d.PolicyVersion == "" || strings.TrimSpace(d.EvidenceReference) == "" || d.ProposedBy == "" || d.BudgetCents <= 0 || len(d.Weights) == 0 || len(d.Weights) > 1000 {
		return ErrValidation
	}
	allocations, err := AllocateRewards(d.BudgetCents, 10000, d.BudgetCents, d.Weights)
	if err != nil {
		return err
	}
	if len(allocations) == 0 {
		return ErrValidation
	}
	raw, _ := json.Marshal(d)
	hash := sha256.Sum256(raw)
	digest := hex.EncodeToString(hash[:])
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,814037230))`, d.ID); err != nil {
		return err
	}
	var old string
	err = tx.QueryRowContext(ctx, `SELECT input_hash FROM cloud_reward_batches WHERE id=$1`, d.ID).Scan(&old)
	if err == nil {
		if old != digest {
			return ErrConflict
		}
		return tx.Commit()
	}
	if err != sql.ErrNoRows {
		return err
	}
	var active bool
	if err = tx.QueryRowContext(ctx, `SELECT active FROM cloud_reward_policies WHERE version=$1 FOR SHARE`, d.PolicyVersion).Scan(&active); err != nil {
		return err
	}
	if !active {
		return ErrUnavailable
	}
	var kind string
	if err = tx.QueryRowContext(ctx, `SELECT kind FROM cloud_reward_sources WHERE id=$1`, d.SourceID).Scan(&kind); err != nil {
		return err
	}
	if kind == "support" {
		return ErrValidation
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO cloud_reward_batches(id,day,source_id,policy_version,input_hash,evidence_reference,total_cents,proposed_by,input_snapshot)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, d.ID, day, d.SourceID, d.PolicyVersion, digest, d.EvidenceReference, d.BudgetCents, d.ProposedBy, raw); err != nil {
		return err
	}
	keys := make([]string, 0, len(allocations))
	for id := range allocations {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	for _, video := range keys {
		cents := allocations[video]
		if cents == 0 {
			continue
		}
		var owner string
		err = tx.QueryRowContext(ctx, `SELECT c.user_id FROM cloud_videos v JOIN cloud_channels c ON c.id=v.channel_id JOIN cloud_reward_enrollments e ON e.user_id=c.user_id AND e.policy_version=$3 WHERE v.id=$1 AND v.status='published' AND v.visibility='public' AND v.first_published_at < $2::timestamptz+interval '1 day' AND v.first_published_at+interval '30 days'>$2::timestamptz AND NOT EXISTS(SELECT 1 FROM cloud_reward_holds h WHERE h.user_id=c.user_id)`, video, day, d.PolicyVersion).Scan(&owner)
		if err == sql.ErrNoRows {
			return ErrValidation
		}
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO cloud_reward_allocations(id,batch_id,video_id,user_id,cents)VALUES($1,$2,$3,$4,$5)`, uuid.NewString(), d.ID, video, owner, cents); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ConfirmRewardBatch(ctx context.Context, id, actor string) error {
	if actor == "" {
		return ErrValidation
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var source, proposer, status, policy string
	var cents int64
	err = tx.QueryRowContext(ctx, `SELECT source_id,proposed_by,status,total_cents,policy_version FROM cloud_reward_batches WHERE id=$1 FOR UPDATE`, id).Scan(&source, &proposer, &status, &cents, &policy)
	if err != nil {
		return err
	}
	if proposer == actor {
		return ErrUnauthorized
	}
	if status == "confirmed" {
		return tx.Commit()
	}
	var active bool
	if err = tx.QueryRowContext(ctx, `SELECT active FROM cloud_reward_policies WHERE version=$1 FOR SHARE`, policy).Scan(&active); err != nil {
		return err
	}
	if !active {
		return ErrUnavailable
	}
	var sum int64
	var invalid bool
	err = tx.QueryRowContext(ctx, `SELECT COALESCE(sum(a.cents),0),COALESCE(bool_or(v.status<>'published' OR v.visibility<>'public' OR h.user_id IS NOT NULL),false) FROM cloud_reward_allocations a JOIN cloud_videos v ON v.id=a.video_id LEFT JOIN cloud_reward_holds h ON h.user_id=a.user_id WHERE a.batch_id=$1`, id).Scan(&sum, &invalid)
	if err != nil {
		return err
	}
	if sum != cents || invalid {
		return ErrConflict
	}
	if err = reserveRewardFunding(ctx, tx, source, cents); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE cloud_reward_batches SET status='confirmed',confirmed_by=$2,confirmed_at=now() WHERE id=$1`, id, actor); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO cloud_reward_releases(allocation_id) SELECT id FROM cloud_reward_allocations WHERE batch_id=$1`, id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO cloud_admin_audit(actor,action,resource_id,reason)VALUES($1,'confirm_reward_batch',$2,'Full liability reserved from reconciled source')`, actor, id); err != nil {
		return err
	}
	return tx.Commit()
}

// ReleaseRewardVesting materializes maturity into the existing payout ledger.
// at is supplied by trusted workers; HTTP clients must never control it.
func (s *Store) ReleaseRewardVesting(ctx context.Context, user string, at time.Time) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,814037226))`, user); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `SELECT a.id,a.cents,b.confirmed_at,r.released_cents,p.vesting_seconds FROM cloud_reward_allocations a JOIN cloud_reward_batches b ON b.id=a.batch_id JOIN cloud_reward_policies p ON p.version=b.policy_version JOIN cloud_reward_releases r ON r.allocation_id=a.id WHERE a.user_id=$1 AND b.status='confirmed' ORDER BY a.id FOR UPDATE OF r`, user)
	if err != nil {
		return err
	}
	type release struct {
		id                            string
		principal, previous, duration int64
		confirmed                     time.Time
	}
	var releases []release
	for rows.Next() {
		var r release
		if err = rows.Scan(&r.id, &r.principal, &r.confirmed, &r.previous, &r.duration); err != nil {
			rows.Close()
			return err
		}
		releases = append(releases, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, r := range releases {
		vested, e := VestedCents(r.principal, r.confirmed, at, time.Duration(r.duration)*time.Second)
		if e != nil {
			return e
		}
		if vested <= r.previous {
			continue
		}
		earning := uuid.NewString()
		reference := "vesting:" + r.id + ":" + intString(vested)
		if _, err = tx.ExecContext(ctx, `INSERT INTO cloud_earnings(id,user_id,reference,amount_cents,currency,status,description)VALUES($1,$2,$3,$4,'usd','available','Vested creator reward')`, earning, user, reference, vested-r.previous); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO cloud_reward_release_events(id,allocation_id,through_cents,earning_id)VALUES($1,$2,$3,$4)`, uuid.NewString(), r.id, vested, earning); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE cloud_reward_releases SET released_cents=$2,updated_at=now() WHERE allocation_id=$1`, r.id, vested); err != nil {
			return err
		}
	}
	return tx.Commit()
}
