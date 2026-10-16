package cloud

import (
	"context"
	"database/sql"
	"github.com/go-chi/chi/v5"
	"net/http"
	"strings"
)

func (s *Server) adminUserRestricted(ctx context.Context, id string) (bool, error) {
	var suspended bool
	err := s.Store.DB.QueryRowContext(ctx, `SELECT suspended FROM cloud_user_restrictions WHERE user_id=$1`, id).Scan(&suspended)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return suspended, err
}
func (s *Server) adminRestrictUser(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Suspended bool   `json:"suspended"`
		Reason    string `json:"reason"`
	}
	if !s.decode(w, r, &b) {
		return
	}
	id := chi.URLParam(r, "id")
	if id == currentUser(r).ID || strings.TrimSpace(b.Reason) == "" || len(b.Reason) > 1000 {
		s.fail(w, ErrValidation)
		return
	}
	tx, err := s.Store.DB.BeginTx(r.Context(), nil)
	if err != nil {
		s.fail(w, err)
		return
	}
	defer tx.Rollback()
	var user string
	err = tx.QueryRowContext(r.Context(), `SELECT id FROM cloud_users WHERE id=$1 FOR UPDATE`, id).Scan(&user)
	if err == sql.ErrNoRows {
		err = ErrNotFound
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	var admin bool
	err = tx.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM cloud_admin_roles WHERE user_id=$1)`, id).Scan(&admin)
	if err != nil {
		s.fail(w, err)
		return
	}
	if admin {
		s.fail(w, ErrValidation)
		return
	}
	_, err = tx.ExecContext(r.Context(), `INSERT INTO cloud_user_restrictions(user_id,suspended,reason,updated_by)VALUES($1,$2,$3,$4) ON CONFLICT(user_id) DO UPDATE SET suspended=EXCLUDED.suspended,reason=EXCLUDED.reason,updated_by=EXCLUDED.updated_by,updated_at=now()`, id, b.Suspended, b.Reason, currentUser(r).ID)
	if err == nil && b.Suspended {
		_, err = tx.ExecContext(r.Context(), `DELETE FROM cloud_sessions WHERE user_id=$1`, id)
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `INSERT INTO cloud_admin_audit(actor,action,resource_id,reason)VALUES($1,$2,$3,$4)`, currentUser(r).ID, map[bool]string{true: "suspend_user", false: "restore_user"}[b.Suspended], id, b.Reason)
	}
	if err == nil {
		err = tx.Commit()
	}
	s.result(w, map[string]bool{"suspended": b.Suspended}, err)
}
func (s *Server) adminReadiness(w http.ResponseWriter, r *http.Request) {
	c, err := s.effectiveConfig(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	s.json(w, 200, map[string]any{"configuration_source": "effective runtime policy; provider secrets and activation remain deployment managed", "provider": "OpenRouter", "openrouter_key_configured": c.OpenRouterKey != "", "story_model": c.StoryModel, "video_model": c.VideoModel, "story_credits": c.StoryCredits, "assembly_credits": c.AssemblyCredits, "credits_per_second": c.CreditsPerSecond, "story_max_cost_micros": c.StoryMaxCostMicros, "provider_daily_budget_micros": c.ProviderDailyBudgetMicros, "provider_max_cost_micros": c.ProviderMaxCostMicros, "stripe_enabled": c.BillingEnabled, "stripe_credentials_configured": c.StripeSecret != "" && c.StripeWebhookSecret != "", "cryptorails_enabled": c.CryptoRailsEnabled, "cryptorails_credentials_configured": c.CryptoRailsAPIKey != "" && c.CryptoRailsWebhookSecret != "", "premium_price_cents": c.PremiumPriceCents, "reward_bps": c.RewardBPS, "reward_monthly_cap_cents": c.RewardMonthlyCapCents, "cash_payouts_enabled": c.CashPayoutsEnabled, "redis_configured": s.Redis != nil, "max_concurrent_requests": c.MaxConcurrentRequests, "note": "Configured credentials do not prove provider connectivity. No live provider call is made by this page."})
}

func (s *Server) adminResolveReport(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Reason string `json:"reason"`
	}
	if !s.decode(w, r, &b) {
		return
	}
	if strings.TrimSpace(b.Reason) == "" || len(b.Reason) > 1000 {
		s.fail(w, ErrValidation)
		return
	}
	id := chi.URLParam(r, "id")
	tx, err := s.Store.DB.BeginTx(r.Context(), nil)
	if err != nil {
		s.fail(w, err)
		return
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(r.Context(), `UPDATE cloud_reports SET status='resolved' WHERE id=$1 AND status='open'`, id)
	if err == nil {
		n, _ := res.RowsAffected()
		if n != 1 {
			err = ErrConflict
		}
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `INSERT INTO cloud_admin_audit(actor,action,resource_id,reason)VALUES($1,'resolve_report',$2,$3)`, currentUser(r).ID, id, b.Reason)
	}
	if err == nil {
		err = tx.Commit()
	}
	s.result(w, map[string]bool{"resolved": err == nil}, err)
}
func (s *Server) adminRestoreVideo(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Reason string `json:"reason"`
	}
	if !s.decode(w, r, &b) {
		return
	}
	if strings.TrimSpace(b.Reason) == "" || len(b.Reason) > 1000 {
		s.fail(w, ErrValidation)
		return
	}
	id := chi.URLParam(r, "id")
	tx, err := s.Store.DB.BeginTx(r.Context(), nil)
	if err != nil {
		s.fail(w, err)
		return
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(r.Context(), `UPDATE cloud_videos SET status='draft' WHERE id=$1 AND status='blocked'`, id)
	if err == nil {
		n, _ := res.RowsAffected()
		if n != 1 {
			err = ErrConflict
		}
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `INSERT INTO cloud_admin_audit(actor,action,resource_id,reason)VALUES($1,'restore_video_draft',$2,$3)`, currentUser(r).ID, id, b.Reason)
	}
	if err == nil {
		err = tx.Commit()
	}
	s.result(w, map[string]bool{"restored_to_draft": err == nil}, err)
}

func (s *Server) adminUserRecords(w http.ResponseWriter, r *http.Request) {
	if s.adminPagedRecords(w, r, chi.URLParam(r, "id")) {
		return
	}
	id, kind := chi.URLParam(r, "id"), chi.URLParam(r, "kind")
	queries := map[string]string{
		"content":  `SELECT v.id,v.title,v.visibility,v.audience,v.status,v.views,v.watch_seconds,v.created_at FROM cloud_videos v JOIN cloud_channels c ON c.id=v.channel_id WHERE c.user_id=$1 ORDER BY v.created_at DESC,v.id LIMIT 100`,
		"projects": `SELECT p.id,p.name,p.created_at FROM cloud_projects p JOIN cloud_workspaces w ON w.id=p.workspace_id WHERE w.owner_id=$1 ORDER BY p.created_at DESC,p.id LIMIT 100`,
		"jobs":     `SELECT j.id,j.status,j.credits,j.attempt,j.created_at FROM cloud_jobs j JOIN cloud_workspaces w ON w.id=j.workspace_id WHERE w.owner_id=$1 ORDER BY j.created_at DESC,j.id LIMIT 100`,
	}
	query, ok := queries[kind]
	if !ok {
		s.fail(w, ErrNotFound)
		return
	}
	rows, err := s.Store.DB.QueryContext(r.Context(), query, id)
	if err != nil {
		s.fail(w, err)
		return
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		s.fail(w, err)
		return
	}
	out := []map[string]any{}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err = rows.Scan(ptrs...); err != nil {
			s.fail(w, err)
			return
		}
		v := map[string]any{}
		for i, k := range cols {
			if b, ok := vals[i].([]byte); ok {
				v[k] = string(b)
			} else {
				v[k] = vals[i]
			}
		}
		out = append(out, v)
	}
	if rows.Err() != nil {
		s.fail(w, rows.Err())
		return
	}
	s.json(w, 200, map[string]any{"items": out, "limit": 100})
}
