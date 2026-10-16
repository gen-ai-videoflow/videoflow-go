package cloud

import (
	"context"
	"errors"
	"time"
)

// RunRewardMaintenance is called by the dedicated bounded worker, never by a
// creator page request. It cannot submit transfers. Immutable daily keys make
// multiple worker instances and restarts safe. Existing obligations keep moving
// even when new generation-funded estimates and confirmations are paused.
func (s *Store) RunRewardMaintenance(ctx context.Context, rewardsEnabled, cashEnabled bool) error {
	failures := []error{}
	if _, e := s.ProcessRewardCalculation(ctx, true); e != nil {
		failures = append(failures, e)
	}
	p, e := s.RewardEnginePolicy(ctx)
	if e != nil {
		return errors.Join(append(failures, e)...)
	}
	if rewardsEnabled && p.Policy.Enabled {
		day := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
		if _, e = s.ReconcileRewardRevenue(ctx, day, "system:reward-reconciliation"); e != nil {
			failures = append(failures, e)
		}
		if p.Policy.EstimatesEnabled {
			if _, e = s.QueueRewardCalculation(ctx, day, false, "system:reward-calculation", "Scheduled closed UTC day"); e != nil {
				failures = append(failures, e)
			} else if _, e = s.ProcessRewardCalculation(ctx); e != nil {
				failures = append(failures, e)
			}
		}
		if p.Policy.ConfirmationsEnabled {
			rows, e := s.DB.QueryContext(ctx, `SELECT b.id FROM cloud_reward_batches b JOIN cloud_reward_calculation_runs r ON b.evidence_reference='calculation:'||r.id WHERE b.status='draft' AND r.status='succeeded' AND r.policy_version=$1 AND r.actor='system:reward-calculation' ORDER BY b.day,b.id LIMIT 20`, p.PolicyVersion)
			if e != nil {
				failures = append(failures, e)
			} else {
				ids := []string{}
				for rows.Next() {
					var id string
					if e = rows.Scan(&id); e != nil {
						break
					}
					ids = append(ids, id)
				}
				if e == nil {
					e = rows.Err()
				}
				rows.Close()
				if e != nil {
					failures = append(failures, e)
				} else {
					for _, id := range ids {
						if e = s.ConfirmRewardBatch(ctx, id, "system:reward-confirmation"); e != nil {
							failures = append(failures, e)
						}
					}
				}
			}
		}
	}
	if cashEnabled && p.Policy.PayoutsEnabled {
		if _, e = s.PrepareRewardPayoutRun(ctx, "system:payout-preparation", "Scheduled payout review; no provider transfer submitted"); e != nil {
			failures = append(failures, e)
		}
	}
	return errors.Join(failures...)
}
