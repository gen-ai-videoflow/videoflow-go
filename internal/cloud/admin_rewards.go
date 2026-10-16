package cloud

import (
	"encoding/json"
	"github.com/go-chi/chi/v5"
	"net/http"
	"strings"
)

func (s *Server) adminRewards(w http.ResponseWriter, r *http.Request) {
	// Bounded, scalar JSON projections keep private evidence in the admin surface.
	queries := map[string]string{
		"sources":  `SELECT jsonb_build_object('id',id,'reference',reference,'kind',kind,'cents',cents,'reserved_cents',reserved_cents,'evidence_reference',evidence_reference,'reconciled_at',reconciled_at) FROM cloud_reward_sources ORDER BY reconciled_at DESC LIMIT 100`,
		"requests": `SELECT jsonb_build_object('id',r.id,'reference',r.reference,'kind',r.kind,'cents',r.cents,'evidence_reference',r.evidence_reference,'proposed_by',r.proposed_by,'created_at',r.created_at,'reconciled',s.id IS NOT NULL) FROM cloud_reward_funding_requests r LEFT JOIN cloud_reward_sources s ON s.id=r.id ORDER BY r.created_at DESC LIMIT 100`,
		"batches":  `SELECT jsonb_build_object('id',id,'day',day,'total_cents',total_cents,'source_id',source_id,'status',status,'proposed_by',proposed_by,'confirmed_at',confirmed_at,'policy_version',policy_version,'evidence_reference',evidence_reference) FROM cloud_reward_batches ORDER BY day DESC,id LIMIT 100`,
		"policies": `SELECT jsonb_build_object('version',version,'terms_version',terms_version,'active',active,'window_seconds',window_seconds,'vesting_seconds',vesting_seconds) FROM cloud_reward_policies ORDER BY created_at DESC LIMIT 20`,
	}
	out := map[string]any{}
	for key, query := range queries {
		rows, err := s.Store.DB.QueryContext(r.Context(), query)
		if err != nil {
			s.fail(w, err)
			return
		}
		items := []json.RawMessage{}
		for rows.Next() {
			var item json.RawMessage
			if err = rows.Scan(&item); err != nil {
				rows.Close()
				s.fail(w, err)
				return
			}
			items = append(items, item)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			s.fail(w, err)
			return
		}
		out[key] = items
	}
	out["rewards_enabled"] = s.Config.RewardsEnabled
	out["payouts_enabled"] = s.Config.CashPayoutsEnabled
	out["funding_mode"] = "Independent manual reconciliation; automatic revenue adapters not activated"
	s.json(w, 200, out)
}
func (s *Server) adminRewardFunding(w http.ResponseWriter, r *http.Request) {
	var f RewardFunding
	if !s.decode(w, r, &f) {
		return
	}
	if f.ID == "" || len(f.ID) > 128 || strings.TrimSpace(f.Reference) == "" || len(f.Reference) > 300 || f.Cents <= 0 || strings.TrimSpace(f.EvidenceReference) == "" || len(f.EvidenceReference) > 1000 || (f.Kind != "generation" && f.Kind != "advertising" && f.Kind != "support") {
		s.fail(w, ErrValidation)
		return
	}
	// Actor fields always come from the authenticated session, never the body.
	actor := currentUser(r).ID
	tx, err := s.Store.DB.BeginTx(r.Context(), nil)
	if err != nil {
		s.fail(w, err)
		return
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(r.Context(), `INSERT INTO cloud_reward_funding_requests(id,reference,kind,cents,evidence_reference,proposed_by)VALUES($1,$2,$3,$4,$5,$6)`, f.ID, f.Reference, f.Kind, f.Cents, f.EvidenceReference, actor)
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `INSERT INTO cloud_admin_audit(actor,action,resource_id,reason)VALUES($1,'propose_reward_funding',$2,$3)`, actor, f.ID, f.EvidenceReference)
	}
	if err == nil {
		err = tx.Commit()
	}
	s.result(w, map[string]bool{"proposed": err == nil}, err)
}
func (s *Server) adminRewardReconcile(w http.ResponseWriter, r *http.Request) {
	var f RewardFunding
	f.ID = chi.URLParam(r, "id")
	f.ReconciledBy = currentUser(r).ID
	err := s.Store.DB.QueryRowContext(r.Context(), `SELECT reference,kind,cents,evidence_reference,proposed_by FROM cloud_reward_funding_requests WHERE id=$1`, f.ID).Scan(&f.Reference, &f.Kind, &f.Cents, &f.EvidenceReference, &f.ProposedBy)
	if err == nil {
		err = s.Store.RecordRewardFunding(r.Context(), f)
	}
	s.result(w, map[string]bool{"reconciled": err == nil}, err)
}
func (s *Server) adminRewardDraft(w http.ResponseWriter, r *http.Request) {
	if !s.Config.RewardsEnabled {
		s.fail(w, ErrUnavailable)
		return
	}
	var d RewardBatchDraft
	if !s.decode(w, r, &d) {
		return
	}
	d.ProposedBy = currentUser(r).ID
	err := s.Store.DraftRewardBatch(r.Context(), d)
	s.result(w, map[string]bool{"drafted": err == nil}, err)
}
func (s *Server) adminRewardConfirm(w http.ResponseWriter, r *http.Request) {
	if !s.Config.RewardsEnabled {
		s.fail(w, ErrUnavailable)
		return
	}
	err := s.Store.ConfirmRewardBatch(r.Context(), chi.URLParam(r, "id"), currentUser(r).ID)
	s.result(w, map[string]bool{"confirmed": err == nil}, err)
}

func (s *Server) adminRewardHold(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Held   bool   `json:"held"`
		Reason string `json:"reason"`
	}
	if !s.decode(w, r, &b) {
		return
	}
	if strings.TrimSpace(b.Reason) == "" || len(b.Reason) > 1000 {
		s.fail(w, ErrValidation)
		return
	}
	user := chi.URLParam(r, "id")
	actor := currentUser(r).ID
	tx, err := s.Store.DB.BeginTx(r.Context(), nil)
	if err != nil {
		s.fail(w, err)
		return
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(r.Context(), `SELECT pg_advisory_xact_lock(hashtextextended($1,814037226))`, user)
	if err == nil {
		if b.Held {
			_, err = tx.ExecContext(r.Context(), `INSERT INTO cloud_reward_holds(user_id,reason,actor)VALUES($1,$2,$3) ON CONFLICT(user_id) DO UPDATE SET reason=excluded.reason,actor=excluded.actor,updated_at=now()`, user, b.Reason, actor)
		} else {
			_, err = tx.ExecContext(r.Context(), `DELETE FROM cloud_reward_holds WHERE user_id=$1`, user)
		}
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `INSERT INTO cloud_admin_audit(actor,action,resource_id,reason)VALUES($1,$2,$3,$4)`, actor, map[bool]string{true: "hold_creator_rewards", false: "release_creator_reward_hold"}[b.Held], user, b.Reason)
	}
	if err == nil {
		err = tx.Commit()
	}
	s.result(w, map[string]bool{"held": b.Held}, err)
}

func (s *Server) adminRewardBatchDetail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var input json.RawMessage
	var hash, status string
	err := s.Store.DB.QueryRowContext(r.Context(), `SELECT input_snapshot,input_hash,status FROM cloud_reward_batches WHERE id=$1`, id).Scan(&input, &hash, &status)
	if err != nil {
		s.fail(w, err)
		return
	}
	rows, err := s.Store.DB.QueryContext(r.Context(), `SELECT video_id,user_id,cents FROM cloud_reward_allocations WHERE batch_id=$1 ORDER BY video_id`, id)
	if err != nil {
		s.fail(w, err)
		return
	}
	defer rows.Close()
	allocations := []map[string]any{}
	for rows.Next() {
		var video, user string
		var cents int64
		if err = rows.Scan(&video, &user, &cents); err != nil {
			s.fail(w, err)
			return
		}
		allocations = append(allocations, map[string]any{"video_id": video, "user_id": user, "cents": cents})
	}
	s.result(w, map[string]any{"id": id, "input_snapshot": input, "input_hash": hash, "status": status, "allocations": allocations}, rows.Err())
}
