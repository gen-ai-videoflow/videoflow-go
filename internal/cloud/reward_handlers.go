package cloud

import (
	"context"
	"database/sql"
	"github.com/go-chi/chi/v5"
	"net/http"
	"strconv"
	"time"
)

func (s *Server) rewardProgram(w http.ResponseWriter, r *http.Request) {
	var active, enrolled bool
	var version, terms string
	err := s.Store.DB.QueryRowContext(r.Context(), `SELECT p.active,p.version,p.terms_version,EXISTS(SELECT 1 FROM cloud_reward_enrollments e WHERE e.user_id=$1 AND e.policy_version=p.version) FROM cloud_reward_policies p ORDER BY p.active DESC,p.created_at DESC,p.version DESC LIMIT 1`, currentUser(r).ID).Scan(&active, &version, &terms, &enrolled)
	s.result(w, map[string]any{"enabled": s.Config.RewardsEnabled && active, "enrolled": enrolled, "earning_window_days": 30, "vesting_days": 30, "confirmation": "daily_funded_batches", "payout_cadence": "monthly_approved_runs", "individual_spending_cap": false, "currency": "usd", "estimate_is_guaranteed": false, "policy_version": version, "terms_version": terms}, err)
}
func (s *Server) rewardEnroll(w http.ResponseWriter, r *http.Request) {
	if !s.Config.RewardsEnabled {
		s.fail(w, ErrUnavailable)
		return
	}
	u := currentUser(r)
	if !u.Verified {
		s.fail(w, ErrUnauthorized)
		return
	}
	var b struct {
		PolicyVersion string `json:"policy_version"`
		Accepted      bool   `json:"accepted"`
	}
	if !s.decode(w, r, &b) {
		return
	}
	if !b.Accepted {
		s.fail(w, ErrValidation)
		return
	}
	err := s.Store.EnrollRewards(r.Context(), u.ID, b.PolicyVersion)
	s.result(w, map[string]bool{"enrolled": err == nil}, err)
}
func (s *Server) rewardVideos(w http.ResponseWriter, r *http.Request) {
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	v, err := s.Store.RewardVideos(r.Context(), currentUser(r).ID, 25, offset)
	s.result(w, map[string]any{"videos": v, "offset": offset, "limit": 25, "calculated_at": time.Now().UTC()}, err)
}

type RewardBalance struct {
	Currency            string    `json:"currency"`
	ConfirmedCents      int64     `json:"lifetime_confirmed_cents"`
	UnvestedCents       int64     `json:"confirmed_unvested_cents"`
	VestedCents         int64     `json:"vested_cents"`
	AvailableCents      int64     `json:"available_cents"`
	PendingLegacyCents  int64     `json:"legacy_pending_cents"`
	PayoutReservedCents int64     `json:"payout_reserved_cents"`
	PaidCents           int64     `json:"paid_cents"`
	Held                bool      `json:"held"`
	Estimate            *int64    `json:"estimated_additional_cents"`
	EstimateStatus      string    `json:"estimate_status"`
	CalculatedAt        time.Time `json:"calculated_at"`
}

func (s *Store) RewardBalance(ctx context.Context, user string, at time.Time) (RewardBalance, error) {
	out := RewardBalance{Currency: "usd", EstimateStatus: "insufficient_data", CalculatedAt: at}
	// A consistent snapshot makes the arithmetic safe while confirmation/releases run.
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT a.cents,b.confirmed_at,p.vesting_seconds,r.released_cents FROM cloud_reward_allocations a JOIN cloud_reward_batches b ON b.id=a.batch_id JOIN cloud_reward_policies p ON p.version=b.policy_version JOIN cloud_reward_releases r ON r.allocation_id=a.id WHERE a.user_id=$1 AND b.status='confirmed'`, user)
	if err != nil {
		return out, err
	}
	var released int64
	for rows.Next() {
		var cents, duration, prior int64
		var confirmed time.Time
		if err = rows.Scan(&cents, &confirmed, &duration, &prior); err != nil {
			rows.Close()
			return out, err
		}
		vested, e := VestedCents(cents, confirmed, at, time.Duration(duration)*time.Second)
		if e != nil {
			rows.Close()
			return out, e
		}
		out.ConfirmedCents += cents
		out.VestedCents += vested
		out.UnvestedCents += cents - vested
		released += prior
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	err = tx.QueryRowContext(ctx, `SELECT coalesce(sum(amount_cents)FILTER(WHERE (status='available' OR (status='pending' AND amount_cents<0))),0),coalesce(sum(amount_cents)FILTER(WHERE status='pending' AND amount_cents>0),0),coalesce(sum(amount_cents)FILTER(WHERE status='held'),0),coalesce(sum(amount_cents)FILTER(WHERE status='paid'),0) FROM cloud_earnings WHERE user_id=$1 AND currency='usd'`, user).Scan(&out.AvailableCents, &out.PendingLegacyCents, &out.PayoutReservedCents, &out.PaidCents)
	if err != nil {
		return out, err
	}
	out.AvailableCents += out.VestedCents - released
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cloud_reward_holds WHERE user_id=$1) OR EXISTS(SELECT 1 FROM cloud_payment_disputes WHERE creator_id=$1 AND status NOT IN ('won','warning_closed'))`, user).Scan(&out.Held)
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
}
func (s *Server) rewardSummary(w http.ResponseWriter, r *http.Request) {
	out, err := s.Store.RewardBalance(r.Context(), currentUser(r).ID, time.Now().UTC())
	s.result(w, out, err)
}

func (s *Server) rewardVideoDetail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	user := currentUser(r).ID
	var owns bool
	err := s.Store.DB.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM cloud_videos v JOIN cloud_channels c ON c.id=v.channel_id WHERE v.id=$1 AND c.user_id=$2)`, id, user).Scan(&owns)
	if err != nil {
		s.fail(w, err)
		return
	}
	if !owns {
		s.fail(w, ErrNotFound)
		return
	}
	rows, err := s.Store.DB.QueryContext(r.Context(), `SELECT a.id,a.cents,b.day,b.confirmed_at,p.vesting_seconds,b.policy_version FROM cloud_reward_allocations a JOIN cloud_reward_batches b ON b.id=a.batch_id JOIN cloud_reward_policies p ON p.version=b.policy_version WHERE a.video_id=$1 AND a.user_id=$2 AND b.status='confirmed' ORDER BY b.confirmed_at DESC LIMIT 100`, id, user)
	if err != nil {
		s.fail(w, err)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	at := time.Now().UTC()
	for rows.Next() {
		var allocation, policy string
		var cents, duration int64
		var day, confirmed time.Time
		if err = rows.Scan(&allocation, &cents, &day, &confirmed, &duration, &policy); err != nil {
			s.fail(w, err)
			return
		}
		vested, e := VestedCents(cents, confirmed, at, time.Duration(duration)*time.Second)
		if e != nil {
			s.fail(w, e)
			return
		}
		out = append(out, map[string]any{"id": allocation, "activity_day": day.Format("2006-01-02"), "confirmed_at": confirmed, "cents": cents, "vested_cents": vested, "fully_vested_at": confirmed.Add(time.Duration(duration) * time.Second), "policy_version": policy})
	}
	s.result(w, map[string]any{"allocations": out, "calculated_at": at, "limit": 100}, rows.Err())
}
func (s *Server) rewardPayoutHistory(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Store.DB.QueryContext(r.Context(), `SELECT id,amount_cents,currency,status,provider_transfer FROM cloud_payout_batches WHERE user_id=$1 ORDER BY created_at DESC,id DESC LIMIT 50`, currentUser(r).ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, currency, status string
		var provider sql.NullString
		var cents int64
		if err = rows.Scan(&id, &cents, &currency, &status, &provider); err != nil {
			s.fail(w, err)
			return
		}
		out = append(out, map[string]any{"id": id, "amount_cents": cents, "currency": currency, "status": status, "transfer_reference": provider.String})
	}
	s.result(w, map[string]any{"payouts": out, "limit": 50}, rows.Err())
}
