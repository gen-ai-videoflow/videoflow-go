package cloud

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"strings"
	"time"
)

type RewardCalculationResult struct {
	RunID          string `json:"run_id"`
	Status         string `json:"status"`
	Videos         int    `json:"videos"`
	EstimatedCents int64  `json:"estimated_cents"`
	DraftBatches   int    `json:"draft_batches"`
}

// QueueRewardCalculation freezes the approved policy. A unique day/mode/policy
// key makes repeated scheduler requests safe. It does not confirm money.
func (s *Store) QueueRewardCalculation(ctx context.Context, day string, shadow bool, actor, reason string) (RewardCalculationResult, error) {
	out := RewardCalculationResult{Status: "queued"}
	d, e := time.Parse("2006-01-02", day)
	if e != nil || !d.Before(time.Now().UTC().Truncate(24*time.Hour)) || time.Since(d) > 90*24*time.Hour || actor == "" || strings.TrimSpace(reason) == "" {
		return out, ErrValidation
	}
	p, e := s.RewardEnginePolicy(ctx)
	if e != nil {
		return out, e
	}
	if !shadow && (!p.Policy.Enabled || !p.Policy.EstimatesEnabled) {
		return out, ErrUnavailable
	}
	raw, _ := json.Marshal(p.Policy)
	out.RunID = uuid.NewString()
	e = s.DB.QueryRowContext(ctx, `INSERT INTO cloud_reward_calculation_runs(id,day,shadow,policy_version,policy_snapshot,actor,reason) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(day,shadow,policy_version) DO UPDATE SET day=excluded.day RETURNING id,status,videos,estimated_cents,draft_batches`, out.RunID, d, shadow, p.PolicyVersion, raw, actor, reason).Scan(&out.RunID, &out.Status, &out.Videos, &out.EstimatedCents, &out.DraftBatches)
	return out, e
}

// ProcessRewardCalculation claims one bounded job. A crashed lease can be
// reclaimed, and failures back off at most five times before operator review.
func (s *Store) ProcessRewardCalculation(ctx context.Context, shadowOnly ...bool) (bool, error) {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return false, e
	}
	defer tx.Rollback()
	onlyShadow := len(shadowOnly) > 0 && shadowOnly[0]
	var id, day, version, actor string
	var raw []byte
	var shadow bool
	e = tx.QueryRowContext(ctx, `SELECT id,day::text,policy_version,policy_snapshot,shadow,actor FROM cloud_reward_calculation_runs WHERE (NOT $1 OR shadow) AND attempt<5 AND next_attempt_at<=now() AND (status='queued' OR status='failed' OR (status='running' AND lease_until<now())) ORDER BY day,id FOR UPDATE SKIP LOCKED LIMIT 1`, onlyShadow).Scan(&id, &day, &version, &raw, &shadow, &actor)
	if e == sql.ErrNoRows {
		return false, nil
	}
	if e != nil {
		return false, e
	}
	var claim int
	e = tx.QueryRowContext(ctx, `UPDATE cloud_reward_calculation_runs SET status='running',attempt=attempt+1,lease_until=now()+interval '5 minutes',updated_at=now() WHERE id=$1 RETURNING attempt`, id).Scan(&claim)
	if e != nil {
		return false, e
	}
	if e = tx.Commit(); e != nil {
		return false, e
	}
	var p RewardEnginePolicy
	if e = json.Unmarshal(raw, &p); e == nil {
		e = s.calculateRewardRun(ctx, id, day, version, actor, shadow, p, claim)
	}
	if e != nil {
		_, save := s.DB.ExecContext(context.WithoutCancel(ctx), `UPDATE cloud_reward_calculation_runs SET status='failed',error=$2,next_attempt_at=now()+least(attempt*attempt,60)*interval '1 minute',lease_until=NULL,updated_at=now() WHERE id=$1 AND attempt=$3 AND status='running'`, id, e.Error(), claim)
		if save != nil {
			return true, fmt.Errorf("calculation: %v; persist failure: %w", e, save)
		}
	}
	return true, e
}

func (s *Store) calculateRewardRun(ctx context.Context, id, day, version, actor string, shadow bool, p RewardEnginePolicy, claim int) error {
	if e := validateRewardEnginePolicy(p); e != nil {
		return e
	}
	// Serialize budget projections globally. Drafts are subtracted from available
	// source capacity, avoiding multiple days displaying the same money.
	conn, e := s.DB.Conn(ctx)
	if e != nil {
		return e
	}
	defer conn.Close()
	if _, e = conn.ExecContext(ctx, `SELECT pg_advisory_lock(814037241)`); e != nil {
		return e
	}
	defer conn.ExecContext(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock(814037241)`)
	var currentClaim int
	var runStatus string
	if e = s.DB.QueryRowContext(ctx, `SELECT attempt,status FROM cloud_reward_calculation_runs WHERE id=$1`, id).Scan(&currentClaim, &runStatus); e != nil {
		return e
	}
	if currentClaim != claim || runStatus != "running" {
		return ErrConflict
	}

	type evidence struct {
		video, user                                                  string
		views, seconds, retention, originality, trust, score, weight int64
	}
	// Authenticated distinct viewers only; no owner watches, anonymous hashes,
	// exclusions, repeated sessions or unreviewed originality contribute money.
	rows, e := s.DB.QueryContext(ctx, `WITH watches AS (
 SELECT e.video_id,e.user_id,sum(e.seconds)::bigint seconds FROM cloud_watch_events e
 JOIN cloud_videos v ON v.id=e.video_id JOIN cloud_channels c ON c.id=v.channel_id JOIN cloud_watch_sessions ws ON ws.id=e.session_id AND ws.traffic_source='organic' AND ws.user_id=e.user_id AND ws.video_id=e.video_id
 WHERE ($3 OR e.created_at>=(SELECT created_at FROM cloud_reward_policies WHERE version=$2))
 AND NOT EXISTS(SELECT 1 FROM cloud_reward_allocations prior JOIN cloud_reward_batches prior_batch ON prior_batch.id=prior.batch_id WHERE prior.video_id=e.video_id AND prior_batch.day=$1::date AND prior_batch.status='confirmed' AND coalesce(prior_batch.input_snapshot->>'campaign_id','')='' AND prior_batch.evidence_reference<>$4)
 AND e.created_at >= $1::date AND e.created_at < $1::date+interval '1 day'
 AND e.created_at>=v.first_published_at AND e.created_at<v.first_published_at+make_interval(secs=>v.reward_window_seconds::int)
 AND EXISTS(SELECT 1 FROM cloud_reward_enrollments enrolled WHERE enrolled.user_id=c.user_id AND enrolled.policy_version=$2 AND enrolled.accepted_at<=e.created_at)
 AND e.user_id IS NOT NULL AND e.user_id<>c.user_id AND EXISTS(SELECT 1 FROM cloud_users viewer WHERE viewer.id=e.user_id AND viewer.verified) AND NOT EXISTS(SELECT 1 FROM cloud_watch_exclusions x WHERE x.event_id=e.id)
 GROUP BY e.video_id,e.user_id HAVING sum(e.seconds)>=10)
 SELECT v.id,c.user_id,count(*)::bigint,sum(least(w.seconds,greatest(coalesce(nullif(media.duration_ms,0)/1000,quote.duration,0),10)))::bigint,
 least(10000,(sum(least(w.seconds,greatest(coalesce(nullif(media.duration_ms,0)/1000,quote.duration,0),10)))*10000/(count(*)*greatest(coalesce(nullif(media.duration_ms,0)/1000,quote.duration,0),10))))::bigint,q.originality_bps,q.trust_bps
 FROM watches w JOIN cloud_videos v ON v.id=w.video_id JOIN cloud_channels c ON c.id=v.channel_id LEFT JOIN cloud_jobs j ON j.id=v.job_id LEFT JOIN cloud_quotes quote ON quote.id=j.quote_id LEFT JOIN cloud_media_assets media ON media.id=v.source_asset_id
 JOIN cloud_reward_quality_reviews q ON q.video_id=v.id AND q.eligible
 JOIN cloud_reward_enrollments enrollment ON enrollment.user_id=c.user_id AND enrollment.policy_version=$2
 WHERE v.status='published' AND v.visibility='public' AND NOT EXISTS(SELECT 1 FROM cloud_reward_holds h WHERE h.user_id=c.user_id)
 GROUP BY v.id,c.user_id,quote.duration,media.duration_ms,q.originality_bps,q.trust_bps ORDER BY v.id LIMIT 1001`, day, version, shadow, "calculation:"+id)
	if e != nil {
		return e
	}
	items := []evidence{}
	weights := map[string]int64{}
	for rows.Next() {
		var v evidence
		if e = rows.Scan(&v.video, &v.user, &v.views, &v.seconds, &v.retention, &v.originality, &v.trust); e != nil {
			rows.Close()
			return e
		}
		v.score, v.weight, e = ScoreRewardEvidence(RewardScoreInput{Views: v.views, WatchBPS: 10000, RetentionBPS: v.retention, OriginalityBPS: v.originality, TrustBPS: v.trust}, p)
		if e != nil {
			rows.Close()
			return e
		}
		items = append(items, v)
		if v.weight > 0 {
			weights[v.video] = v.weight
		}
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	if len(items) > 1000 {
		return fmt.Errorf("daily evidence exceeds reviewed pilot limit: %w", ErrUnavailable)
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var complete bool
	if e = tx.QueryRowContext(ctx, `SELECT evidence_complete FROM cloud_reward_calculation_runs WHERE id=$1 FOR UPDATE`, id).Scan(&complete); e != nil {
		return e
	}
	if !complete {
		for _, v := range items {
			_, e = tx.ExecContext(ctx, `INSERT INTO cloud_reward_evidence_snapshots(run_id,video_id,user_id,views,seconds,retention_bps,originality_bps,trust_bps,score,weight,cutoff) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11::date+interval '1 day') ON CONFLICT DO NOTHING`, id, v.video, v.user, v.views, v.seconds, v.retention, v.originality, v.trust, v.score, v.weight, day)
			if e != nil {
				return e
			}
		}
		if _, e = tx.ExecContext(ctx, `UPDATE cloud_reward_calculation_runs SET evidence_complete=true WHERE id=$1`, id); e != nil {
			return e
		}
	}
	if e = tx.Commit(); e != nil {
		return e
	}
	// Retries consume the original immutable evidence rather than re-score it.
	rows, e = s.DB.QueryContext(ctx, `SELECT video_id,weight FROM cloud_reward_evidence_snapshots WHERE run_id=$1 AND weight>0`, id)
	if e != nil {
		return e
	}
	weights = map[string]int64{}
	for rows.Next() {
		var video string
		var weight int64
		if e = rows.Scan(&video, &weight); e != nil {
			rows.Close()
			return e
		}
		weights[video] = weight
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	_, e = s.DB.ExecContext(ctx, `INSERT INTO cloud_reward_estimates(run_id,video_id,user_id,batch_id,cents) SELECT $1,a.video_id,a.user_id,a.batch_id,a.cents FROM cloud_reward_allocations a JOIN cloud_reward_batches b ON b.id=a.batch_id WHERE b.evidence_reference=$2 ON CONFLICT DO NOTHING`, id, "calculation:"+id)
	if e != nil {
		return e
	}
	total := int64(0)
	batches := 0
	if !shadow && len(weights) > 0 {
		current, e := s.RewardEnginePolicy(ctx)
		if e != nil {
			return e
		}
		if current.PolicyVersion != version || !current.Policy.Enabled || !current.Policy.EstimatesEnabled {
			return ErrUnavailable
		}
		rows, e = s.DB.QueryContext(ctx, `SELECT s.id,greatest(0,s.cents-s.reserved_cents-coalesce((SELECT sum(b.total_cents) FROM cloud_reward_batches b WHERE b.source_id=s.id AND b.status='draft' AND EXISTS(SELECT 1 FROM cloud_reward_policies active WHERE active.version=b.policy_version AND active.active)),0)) FROM cloud_reward_sources s WHERE s.kind<>'support' AND s.cents-s.reserved_cents-coalesce((SELECT sum(pending.total_cents) FROM cloud_reward_batches pending JOIN cloud_reward_policies pp ON pp.version=pending.policy_version WHERE pending.source_id=s.id AND pending.status='draft' AND pp.active),0)>0 AND NOT EXISTS(SELECT 1 FROM cloud_reward_revenue_source_holds h WHERE h.source_id=s.id) AND NOT EXISTS(SELECT 1 FROM cloud_sponsor_campaigns campaign WHERE campaign.source_id=s.id) ORDER BY s.reconciled_at,s.id LIMIT 100`)
		if e != nil {
			return e
		}
		type source struct {
			id        string
			available int64
		}
		sources := []source{}
		for rows.Next() {
			var f source
			if e = rows.Scan(&f.id, &f.available); e != nil {
				rows.Close()
				return e
			}
			sources = append(sources, f)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		// Recover already drafted work after a crash before the final run update.
		e = s.DB.QueryRowContext(ctx, `SELECT coalesce(sum(total_cents),0),count(*) FROM cloud_reward_batches WHERE evidence_reference=$1`, "calculation:"+id).Scan(&total, &batches)
		if e != nil {
			return e
		}
		var otherDay int64
		if e = s.DB.QueryRowContext(ctx, `SELECT coalesce(sum(total_cents),0) FROM cloud_reward_batches WHERE day=$1 AND evidence_reference<>$2 AND (status='confirmed' OR EXISTS(SELECT 1 FROM cloud_reward_policies active WHERE active.version=policy_version AND active.active))`, day, "calculation:"+id).Scan(&otherDay); e != nil {
			return e
		}
		for _, f := range sources {
			var exists bool
			if e = s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cloud_reward_batches WHERE id=$1)`, "calc:"+id+":"+f.id).Scan(&exists); e != nil {
				return e
			}
			if exists {
				continue
			}
			budget := min(f.available, p.DailyCapCents-total-otherDay)
			if budget <= 0 {
				continue
			}
			batchID := "calc:" + id + ":" + f.id
			if e = s.DraftRewardBatch(ctx, RewardBatchDraft{ID: batchID, Day: day, SourceID: f.id, PolicyVersion: version, EvidenceReference: "calculation:" + id, ProposedBy: actor, BudgetCents: budget, Weights: weights}); e != nil {
				return e
			}
			_, e = s.DB.ExecContext(ctx, `INSERT INTO cloud_reward_estimates(run_id,video_id,user_id,batch_id,cents) SELECT $1,video_id,user_id,batch_id,cents FROM cloud_reward_allocations WHERE batch_id=$2 ON CONFLICT DO NOTHING`, id, batchID)
			if e != nil {
				return e
			}
			total += budget
			batches++
		}
	}
	result, e := s.DB.ExecContext(ctx, `UPDATE cloud_reward_calculation_runs SET status='succeeded',videos=$2,estimated_cents=$3,draft_batches=$4,error='',lease_until=NULL,updated_at=now() WHERE id=$1 AND attempt=$5 AND status='running'`, id, len(weights), total, batches, claim)
	if e == nil {
		n, _ := result.RowsAffected()
		if n != 1 {
			return ErrConflict
		}
	}
	return e
}
