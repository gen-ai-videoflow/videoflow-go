package cloud

import (
	"context"
	"database/sql"
	"github.com/google/uuid"
	"net/http"
	"strings"
	"time"
)

type RewardPayoutRun struct {
	ID       string `json:"id"`
	Batches  int    `json:"batches"`
	Failures int    `json:"failures"`
	Status   string `json:"status"`
}

func (s *Store) PrepareRewardPayoutRun(ctx context.Context, actor, reason string) (RewardPayoutRun, error) {
	out := RewardPayoutRun{Status: "prepared"}
	if actor == "" || strings.TrimSpace(reason) == "" || len(reason) > 1000 {
		return out, ErrValidation
	}
	p, e := s.RewardEnginePolicy(ctx)
	if e != nil {
		return out, e
	}
	if !p.Policy.PayoutsEnabled {
		return out, ErrUnavailable
	}
	conn, e := s.DB.Conn(ctx)
	if e != nil {
		return out, e
	}
	defer conn.Close()
	if _, e = conn.ExecContext(ctx, `SELECT pg_advisory_lock(814037242)`); e != nil {
		return out, e
	}
	defer conn.ExecContext(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock(814037242)`)
	interval := p.Policy.PayoutIntervalDays * 86400
	period := time.Unix(time.Now().Unix()/interval*interval, 0).UTC().Format("2006-01-02")
	e = s.DB.QueryRowContext(ctx, `SELECT id FROM cloud_reward_payout_runs WHERE period=$1`, period).Scan(&out.ID)
	if e == sql.ErrNoRows {
		rows, err := s.DB.QueryContext(ctx, `SELECT user_id FROM cloud_payout_accounts WHERE charges_enabled AND payouts_enabled ORDER BY user_id LIMIT 1001`)
		if err != nil {
			return out, err
		}
		users := []string{}
		for rows.Next() {
			var u string
			if err = rows.Scan(&u); err != nil {
				rows.Close()
				return out, err
			}
			users = append(users, u)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return out, err
		}
		if len(users) > 1000 {
			return out, ErrUnavailable
		}
		tx, err := s.DB.BeginTx(ctx, nil)
		if err != nil {
			return out, err
		}
		defer tx.Rollback()
		out.ID = uuid.NewString()
		if _, err = tx.ExecContext(ctx, `INSERT INTO cloud_reward_payout_runs(id,period,actor,reason)VALUES($1,$2,$3,$4)`, out.ID, period, actor, reason); err != nil {
			return out, err
		}
		for _, u := range users {
			if _, err = tx.ExecContext(ctx, `INSERT INTO cloud_reward_payout_run_accounts(run_id,user_id)VALUES($1,$2)`, out.ID, u); err != nil {
				return out, err
			}
		}
		if err = tx.Commit(); err != nil {
			return out, err
		}
	} else if e != nil {
		return out, e
	}
	rows, e := s.DB.QueryContext(ctx, `SELECT user_id FROM cloud_reward_payout_run_accounts WHERE run_id=$1 AND status IN('pending','failed') AND attempts<5 ORDER BY user_id LIMIT 100`, out.ID)
	if e != nil {
		return out, e
	}
	users := []string{}
	for rows.Next() {
		var u string
		if e = rows.Scan(&u); e != nil {
			rows.Close()
			return out, e
		}
		users = append(users, u)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	for _, u := range users {
		status, batchID, problem := "skipped", "", ""
		balance, err := s.RewardBalance(ctx, u, time.Now().UTC())
		if err != nil {
			status = "failed"
			problem = err.Error()
		} else if !balance.Held && balance.AvailableCents >= p.Policy.PayoutThresholdCents {
			batch, err := s.PreparePayout(ctx, u, actor)
			if err != nil {
				status = "failed"
				problem = err.Error()
			} else {
				status = "prepared"
				batchID = batch.ID
			}
		}
		if _, e = s.DB.ExecContext(ctx, `UPDATE cloud_reward_payout_run_accounts SET status=$3,attempts=attempts+1,batch_id=nullif($4,''),error=$5,updated_at=now() WHERE run_id=$1 AND user_id=$2`, out.ID, u, status, batchID, problem); e != nil {
			return out, e
		}
	}
	e = s.DB.QueryRowContext(ctx, `SELECT count(*)FILTER(WHERE status='prepared'),count(*)FILTER(WHERE status='failed') FROM cloud_reward_payout_run_accounts WHERE run_id=$1`, out.ID).Scan(&out.Batches, &out.Failures)
	if e != nil {
		return out, e
	}
	_, e = s.DB.ExecContext(ctx, `UPDATE cloud_reward_payout_runs SET batches=$2,failures=$3 WHERE id=$1`, out.ID, out.Batches, out.Failures)
	return out, e
}
func (s *Server) adminRewardPayoutRun(w http.ResponseWriter, r *http.Request) {
	if !s.Config.CashPayoutsEnabled {
		s.fail(w, ErrUnavailable)
		return
	}
	var b struct {
		Reason string `json:"reason"`
	}
	if !s.decode(w, r, &b) {
		return
	}
	out, e := s.Store.PrepareRewardPayoutRun(r.Context(), currentUser(r).ID, b.Reason)
	s.result(w, out, e)
}
