package cloud

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/google/uuid"
	"math/big"
	"strings"
	"time"
)

type RewardRevenueEvidence struct {
	ReplacementSourceID string `json:"replacement_source_id,omitempty"`
	ID                  string `json:"id"`
	Kind                string `json:"kind"`
	ResourceID          string `json:"resource_id"`
	Reason              string `json:"reason"`
	Provider            string `json:"provider"`
	GrossCents          *int64 `json:"gross_cents"`
	FeeCents            *int64 `json:"fee_cents"`
	TaxCents            *int64 `json:"tax_cents"`
	RefundCents         *int64 `json:"refund_cents"`
	ProviderCostCents   *int64 `json:"provider_cost_cents"`
	MediaCostCents      *int64 `json:"media_cost_cents"`
	SettledAt           string `json:"settled_at"`
	EvidenceReference   string `json:"evidence_reference"`
}
type RewardRevenueResult struct {
	Day         string `json:"day"`
	Status      string `json:"status"`
	Jobs        int    `json:"jobs"`
	Pending     int    `json:"pending"`
	NetCents    int64  `json:"net_cents"`
	FundedCents int64  `json:"funded_cents"`
	SourceID    string `json:"source_id,omitempty"`
	Message     string `json:"message,omitempty"`
}

const revenueMaxCents int64 = 100000000000

func revenueAmount(p *int64) bool { return p != nil && *p >= 0 && *p <= revenueMaxCents }
func validRevenueEvidence(e RewardRevenueEvidence) bool {
	if strings.TrimSpace(e.ResourceID) == "" || strings.TrimSpace(e.Reason) == "" || len(e.Reason) > 1000 || strings.TrimSpace(e.EvidenceReference) == "" || len(e.EvidenceReference) > 2000 {
		return false
	}
	switch e.Kind {
	case "receipt":
		_, err := time.Parse(time.RFC3339, e.SettledAt)
		return err == nil && (e.Provider == "stripe" || e.Provider == "cryptorails") && revenueAmount(e.GrossCents) && revenueAmount(e.FeeCents) && revenueAmount(e.TaxCents) && revenueAmount(e.RefundCents) && *e.FeeCents+*e.TaxCents+*e.RefundCents <= *e.GrossCents
	case "cost":
		return revenueAmount(e.ProviderCostCents) && revenueAmount(e.MediaCostCents)
	case "recovery":
		return strings.TrimSpace(e.ReplacementSourceID) != "" && e.ReplacementSourceID != e.ResourceID
	case "reversal":
		return revenueAmount(e.RefundCents) && *e.RefundCents > 0
	}
	return false
}
func (s *Store) ProposeRewardRevenueEvidence(ctx context.Context, e RewardRevenueEvidence, actor string) (string, error) {
	if !validRevenueEvidence(e) || actor == "" {
		return "", ErrValidation
	}
	e.ID = uuid.NewString()
	raw, err := json.Marshal(e)
	if err != nil {
		return "", err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO cloud_reward_revenue_evidence(id,kind,resource_id,payload,reason,proposed_by)VALUES($1,$2,$3,$4,$5,$6)`, e.ID, e.Kind, e.ResourceID, raw, e.Reason, actor)
	if err != nil {
		return "", err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO cloud_admin_audit(actor,action,resource_id,reason)VALUES($1,'propose_reward_revenue',$2,$3)`, actor, e.ID, e.Reason)
	if err != nil {
		return "", err
	}
	return e.ID, tx.Commit()
}
func (s *Store) ApproveRewardRevenueEvidence(ctx context.Context, id, actor, reason string) error {
	if actor == "" || strings.TrimSpace(reason) == "" || len(reason) > 1000 {
		return ErrValidation
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(814037253)`); err != nil {
		return err
	}
	var raw []byte
	var proposer string
	var approved sql.NullString
	if err = tx.QueryRowContext(ctx, `SELECT payload,proposed_by,approved_by FROM cloud_reward_revenue_evidence WHERE id=$1 FOR UPDATE`, id).Scan(&raw, &proposer, &approved); err != nil {
		return err
	}
	if proposer == actor {
		return ErrValidation
	}
	if approved.Valid {
		return ErrConflict
	}
	var e RewardRevenueEvidence
	if err = json.Unmarshal(raw, &e); err != nil {
		return err
	}
	if !validRevenueEvidence(e) {
		return ErrValidation
	}
	if e.Kind == "receipt" {
		var workspace string
		var credits int64
		var at time.Time
		if e.Provider == "stripe" {
			var gross int64
			err = tx.QueryRowContext(ctx, `SELECT p.workspace_id,p.credits,p.price_cents,e.created_at FROM cloud_purchases p JOIN cloud_credit_entries e ON e.workspace_id=p.workspace_id AND e.reference='purchase:'||p.id AND e.kind='purchase' WHERE p.id=$1 AND p.status='paid' AND p.currency='usd' AND e.amount=p.credits`, e.ResourceID).Scan(&workspace, &credits, &gross, &at)
			if err == nil && *e.GrossCents != gross {
				return ErrValidation
			}
		} else {
			err = tx.QueryRowContext(ctx, `SELECT i.workspace_id,i.credits,s.created_at FROM cloud_crypto_invoices i JOIN cloud_crypto_settlements s ON s.reference=i.reference JOIN cloud_credit_entries e ON e.workspace_id=i.workspace_id AND e.reference='cryptorails:'||i.reference AND e.amount=i.credits AND e.kind='purchase' WHERE i.id=$1 AND i.status='settled' AND i.purpose='credits'`, e.ResourceID).Scan(&workspace, &credits, &at)
		}
		if err != nil {
			return err
		}
		settled, _ := time.Parse(time.RFC3339, e.SettledAt)
		if settled.After(time.Now().Add(time.Minute)) {
			return ErrValidation
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO cloud_reward_revenue_lots(id,provider,receipt_id,workspace_id,credits,gross_cents,fee_cents,tax_cents,refund_cents,settled_at,evidence_reference,acquired_at)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, id, e.Provider, e.ResourceID, workspace, credits, *e.GrossCents, *e.FeeCents, *e.TaxCents, *e.RefundCents, settled, e.EvidenceReference, at)
	} else if e.Kind == "cost" {
		var yes bool
		err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cloud_jobs WHERE id=$1 AND status='succeeded')`, e.ResourceID).Scan(&yes)
		if err == nil && !yes {
			return ErrValidation
		}
	} else if e.Kind == "recovery" {
		var needed int64
		err = tx.QueryRowContext(ctx, `SELECT s.reserved_cents FROM cloud_reward_sources s JOIN cloud_reward_revenue_source_holds h ON h.source_id=s.id WHERE s.id=$1 AND NOT EXISTS(SELECT 1 FROM cloud_reward_revenue_recoveries c WHERE c.held_source_id=s.id) FOR UPDATE OF s`, e.ResourceID).Scan(&needed)
		if err != nil {
			return err
		}
		var available int64
		err = tx.QueryRowContext(ctx, `SELECT cents-reserved_cents FROM cloud_reward_sources s WHERE id=$1 AND kind<>'support' AND NOT EXISTS(SELECT 1 FROM cloud_reward_revenue_source_holds h WHERE h.source_id=s.id) AND NOT EXISTS(SELECT 1 FROM cloud_sponsor_campaigns c WHERE c.source_id=s.id) FOR UPDATE`, e.ReplacementSourceID).Scan(&available)
		if err != nil {
			return err
		}
		if available < needed {
			return ErrInsufficientCredits
		}
		_, err = tx.ExecContext(ctx, `UPDATE cloud_reward_sources SET reserved_cents=reserved_cents+$2 WHERE id=$1`, e.ReplacementSourceID, needed)
		if err == nil {
			_, err = tx.ExecContext(ctx, `INSERT INTO cloud_reward_revenue_recoveries(held_source_id,replacement_source_id,cents,evidence_id)VALUES($1,$2,$3,$4)`, e.ResourceID, e.ReplacementSourceID, needed, id)
		}
	} else {
		var yes bool
		err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cloud_reward_revenue_lots WHERE id=$1)`, e.ResourceID).Scan(&yes)
		if err == nil && !yes {
			return ErrValidation
		}
		if err == nil {
			_, err = tx.ExecContext(ctx, `INSERT INTO cloud_reward_revenue_source_holds(source_id,reason) SELECT DISTINCT d.source_id,$2 FROM cloud_reward_revenue_consumption c JOIN cloud_reward_revenue_recognition r ON r.job_id=c.job_id JOIN cloud_reward_revenue_days d ON d.day=r.day WHERE c.lot_id=$1 AND d.source_id IS NOT NULL ON CONFLICT DO NOTHING`, e.ResourceID, "Approved reversal: "+id+" · "+reason)
		}
	}
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE cloud_reward_revenue_evidence SET approved_by=$2,approved_at=now() WHERE id=$1`, id, actor)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO cloud_admin_audit(actor,action,resource_id,reason)VALUES($1,'approve_reward_revenue',$2,$3)`, actor, id, reason)
	if err != nil {
		return err
	}
	return tx.Commit()
}
func revenuePortion(total, consumed, credits int64) int64 {
	n := new(big.Int).Mul(big.NewInt(total), big.NewInt(consumed))
	n.Div(n, big.NewInt(credits))
	return n.Int64()
}

// ReconcileRewardRevenue only recognizes a closed UTC activity day. Missing
// evidence rolls back the entire day: no partial FIFO consumption or funding.
func (s *Store) ReconcileRewardRevenue(ctx context.Context, day, actor string) (RewardRevenueResult, error) {
	return s.reconcileRewardRevenue(ctx, day, actor, "Scheduled reconciliation of consumed purchased-credit revenue")
}
func (s *Store) reconcileRewardRevenue(ctx context.Context, day, actor, reason string) (RewardRevenueResult, error) {
	out := RewardRevenueResult{Day: day, Status: "pending_reconciliation"}
	date, err := time.Parse("2006-01-02", day)
	if err != nil || actor == "" || !date.Before(time.Now().UTC().Truncate(24*time.Hour)) {
		return out, ErrValidation
	}
	policy, err := s.RewardEnginePolicy(ctx)
	if err != nil {
		return out, err
	}
	p := policy.Policy
	if p.FundingBPS < 0 || p.FundingBPS > 10000 || p.ReserveBPS < 0 || p.ReserveBPS > 10000 || p.DailyCapCents < 0 {
		return out, ErrValidation
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(814037253)`); err != nil {
		return out, err
	}
	// Known refund/dispute evidence freezes all linked backing. Confirmed reserves
	// remain untouched and are reported as coverage shortfall for finance recovery.
	_, err = tx.ExecContext(ctx, `INSERT INTO cloud_reward_revenue_source_holds(source_id,reason)
 SELECT DISTINCT d.source_id,'Receipt refund/dispute or approved reversal; independently reconcile coverage'
 FROM cloud_reward_revenue_days d JOIN cloud_reward_revenue_recognition r ON r.day=d.day
 JOIN cloud_reward_revenue_consumption c ON c.job_id=r.job_id JOIN cloud_reward_revenue_lots l ON l.id=c.lot_id
 WHERE d.source_id IS NOT NULL AND (EXISTS(SELECT 1 FROM cloud_purchases p WHERE l.provider='stripe' AND p.id=l.receipt_id AND p.refunded_cents>l.refund_cents)
 OR EXISTS(SELECT 1 FROM cloud_payment_disputes x JOIN cloud_purchases p ON p.payment_intent=x.payment_intent WHERE l.provider='stripe' AND p.id=l.receipt_id AND x.status NOT IN('won','warning_closed'))
 OR EXISTS(SELECT 1 FROM cloud_reward_revenue_evidence e WHERE e.kind='reversal' AND e.resource_id=l.id AND e.approved_by IS NOT NULL)) ON CONFLICT DO NOTHING`)
	if err != nil {
		return out, err
	}
	pending := func(message string) (RewardRevenueResult, error) {
		out.Pending++
		out.Jobs = 0
		out.NetCents = 0
		out.FundedCents = 0
		out.Message = message
		_, e := tx.ExecContext(ctx, `ROLLBACK TO SAVEPOINT recognition`)
		if e != nil {
			return out, e
		}
		return out, tx.Commit()
	}
	if _, err = tx.ExecContext(ctx, `SAVEPOINT recognition`); err != nil {
		return out, err
	}
	var source sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT net_cents,funded_cents,source_id FROM cloud_reward_revenue_days WHERE day=$1`, day).Scan(&out.NetCents, &out.FundedCents, &source)
	if err == nil {
		out.Status = "already_reconciled"
		out.SourceID = source.String
		return out, tx.Commit()
	}
	if err != sql.ErrNoRows {
		return out, err
	}
	var earlier int
	err = tx.QueryRowContext(ctx, `SELECT count(*) FROM cloud_jobs j WHERE j.status='succeeded' AND j.credits>j.promotional_credits AND j.updated_at<$1::date AND NOT EXISTS(SELECT 1 FROM cloud_reward_revenue_recognition r WHERE r.job_id=j.id)`, day).Scan(&earlier)
	if err != nil {
		return out, err
	}
	if earlier > 0 {
		return pending("Earlier purchased-credit jobs require reconciliation first; process oldest UTC day.")
	}
	type job struct {
		id, workspace string
		credits       int64
		at            time.Time
	}
	jobs := []job{}
	rows, err := tx.QueryContext(ctx, `SELECT id,workspace_id,credits-promotional_credits,updated_at FROM cloud_jobs WHERE status='succeeded' AND credits>promotional_credits AND updated_at >= $1::date AND updated_at < $1::date+interval '1 day' ORDER BY updated_at,id LIMIT 10001`, day)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var j job
		if err = rows.Scan(&j.id, &j.workspace, &j.credits, &j.at); err != nil {
			rows.Close()
			return out, err
		}
		jobs = append(jobs, j)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(jobs) > 10000 {
		return pending("Day exceeds the bounded 10,000-job reconciliation limit; operator review is required.")
	}
	for _, j := range jobs {
		var raw []byte
		var costID string
		err = tx.QueryRowContext(ctx, `SELECT id,payload FROM cloud_reward_revenue_evidence WHERE kind='cost' AND resource_id=$1 AND approved_by IS NOT NULL`, j.id).Scan(&costID, &raw)
		if err == sql.ErrNoRows {
			return pending("Missing independently approved provider/media cost evidence for job " + j.id)
		}
		if err != nil {
			return out, err
		}
		var cost RewardRevenueEvidence
		if err = json.Unmarshal(raw, &cost); err != nil {
			return out, err
		}
		if !validRevenueEvidence(cost) {
			return out, ErrValidation
		}
		type lot struct {
			id                 string
			credits, used, net int64
		}
		lots := []lot{}
		rows, err = tx.QueryContext(ctx, `SELECT l.id,l.credits,coalesce((SELECT sum(c.credits) FROM cloud_reward_revenue_consumption c WHERE c.lot_id=l.id),0),l.gross_cents-l.fee_cents-l.tax_cents-l.refund_cents FROM cloud_reward_revenue_lots l WHERE l.workspace_id=$1 AND l.acquired_at<=$2 AND NOT EXISTS(SELECT 1 FROM cloud_reward_revenue_evidence e WHERE e.kind='reversal' AND e.resource_id=l.id AND e.approved_by IS NOT NULL) AND NOT EXISTS(SELECT 1 FROM cloud_purchases p WHERE l.provider='stripe' AND p.id=l.receipt_id AND p.refunded_cents>l.refund_cents) AND NOT EXISTS(SELECT 1 FROM cloud_payment_disputes x JOIN cloud_purchases p ON p.payment_intent=x.payment_intent WHERE l.provider='stripe' AND p.id=l.receipt_id AND x.status NOT IN('won','warning_closed')) ORDER BY l.acquired_at,l.id`, j.workspace, j.at)
		if err != nil {
			return out, err
		}
		for rows.Next() {
			var l lot
			if err = rows.Scan(&l.id, &l.credits, &l.used, &l.net); err != nil {
				rows.Close()
				return out, err
			}
			lots = append(lots, l)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return out, err
		}
		remaining := j.credits
		var recognized int64
		for _, l := range lots {
			n := min(remaining, l.credits-l.used)
			if n <= 0 {
				continue
			}
			cents := revenuePortion(l.net, l.used+n, l.credits) - revenuePortion(l.net, l.used, l.credits)
			_, err = tx.ExecContext(ctx, `INSERT INTO cloud_reward_revenue_consumption(job_id,lot_id,credits,net_cents)VALUES($1,$2,$3,$4)`, j.id, l.id, n, cents)
			if err != nil {
				return out, err
			}
			recognized += cents
			remaining -= n
			if remaining == 0 {
				break
			}
		}
		if remaining > 0 {
			return pending("Missing settled purchased-credit lot coverage for job " + j.id + ". Unspent deposits and promotional credits cannot fund rewards.")
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO cloud_reward_revenue_recognition(job_id,day,cost_evidence_id,purchased_credits,net_receipts_cents,provider_cost_cents,media_cost_cents)VALUES($1,$2,$3,$4,$5,$6,$7)`, j.id, day, costID, j.credits, recognized, *cost.ProviderCostCents, *cost.MediaCostCents)
		if err != nil {
			return out, err
		}
		out.NetCents += recognized - *cost.ProviderCostCents - *cost.MediaCostCents
		out.Jobs++
	}
	var carry int64
	err = tx.QueryRowContext(ctx, `SELECT carry_loss_cents FROM cloud_reward_revenue_days WHERE day<$1 ORDER BY day DESC LIMIT 1`, day).Scan(&carry)
	if err != nil && err != sql.ErrNoRows {
		return out, err
	}
	net := out.NetCents - carry
	carry = max(0, -net)
	if net > 0 {
		share := revenuePortion(net, p.FundingBPS, 10000)
		out.FundedCents = min(p.DailyCapCents, revenuePortion(share, 10000-p.ReserveBPS, 10000))
	}
	if out.FundedCents > 0 {
		out.SourceID = "revenue:" + day
		_, err = tx.ExecContext(ctx, `INSERT INTO cloud_reward_sources(id,reference,kind,cents,evidence_reference,proposed_by,reconciled_by)VALUES($1,$1,'generation',$2,$3,'system:revenue-recognition',$4)`, out.SourceID, out.FundedCents, "FIFO recognized consumption / "+day+" / policy "+policy.PolicyVersion, actor)
		if err != nil {
			return out, err
		}
	}
	var sourceID any
	if out.SourceID != "" {
		sourceID = out.SourceID
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO cloud_reward_revenue_days(day,policy_revision,funding_bps,reserve_bps,net_cents,funded_cents,source_id,actor,carry_loss_cents,daily_cap_cents)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, day, policy.Revision, p.FundingBPS, p.ReserveBPS, out.NetCents, out.FundedCents, sourceID, actor, carry, p.DailyCapCents)
	if err != nil {
		return out, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO cloud_admin_audit(actor,action,resource_id,reason)VALUES($1,'reconcile_reward_revenue',$2,$3)`, actor, day, reason)
	if err != nil {
		return out, err
	}
	out.Status = "reconciled"
	return out, tx.Commit()
}
