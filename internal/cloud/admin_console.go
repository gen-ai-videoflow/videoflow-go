package cloud

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/go-chi/chi/v5"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

type adminRoleContext struct{}

type adminSetting struct {
	Key       string          `json:"key"`
	Value     json.RawMessage `json:"value"`
	Revision  int64           `json:"revision"`
	UpdatedAt time.Time       `json:"updated_at"`
}

func validAdminSetting(key string, value json.RawMessage) bool {
	switch key {
	case "generation_enabled", "publishing_enabled":
		var v bool
		return json.Unmarshal(value, &v) == nil && (string(value) == "true" || string(value) == "false")
	case "announcement":
		var v string
		return strings.TrimSpace(string(value)) != "null" && json.Unmarshal(value, &v) == nil && utf8.RuneCountInString(v) <= 500
	}
	return false
}
func (s *Server) adminFeatureEnabled(ctx context.Context, key string) bool {
	var enabled bool
	return s.Store.DB.QueryRowContext(ctx, `SELECT value::boolean FROM cloud_admin_settings WHERE key=$1`, key).Scan(&enabled) == nil && enabled
}
func (s *Server) adminAccess(next http.Handler) http.Handler {
	return s.requireUser(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		r = r.WithContext(ctx)
		var role string
		err := s.Store.DB.QueryRowContext(r.Context(), `SELECT role FROM cloud_admin_roles WHERE user_id=$1`, currentUser(r).ID).Scan(&role)
		if err == sql.ErrNoRows {
			s.json(w, 403, map[string]string{"message": "Administrator access required."})
			return
		}
		if err != nil {
			s.fail(w, err)
			return
		}
		enrolled, err := s.Store.mfaRequired(r.Context(), currentUser(r).ID)
		if err != nil {
			s.fail(w, err)
			return
		}
		if !enrolled {
			s.json(w, 403, map[string]string{"message": "Set up a passkey or authenticator in 6eek security settings before opening administration."})
			return
		}
		cookie, _ := sessionCookie(r)
		fresh := false
		window := "24 hours"
		if r.Method != http.MethodGet {
			window = "15 minutes"
		}
		if cookie != nil {
			err = s.Store.DB.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM cloud_sessions WHERE token_hash=$1 AND user_id=$2 AND mfa_verified_at>now()-$3::interval)`, tokenHash(cookie.Value), currentUser(r).ID, window).Scan(&fresh)
			if err != nil {
				s.fail(w, err)
				return
			}
		}
		if !fresh {
			s.json(w, 403, map[string]string{"message": "Sign in again with your passkey or authenticator to verify this administrator session. Changes require verification within 15 minutes."})
			return
		}
		allowed := adminAllowed(role, r.Method, r.URL.Path)
		if !allowed {
			s.json(w, 403, map[string]string{"message": "Your administrator role cannot perform this action."})
			return
		}
		if !s.rate(w, r, "admin:"+currentUser(r).ID, 120, time.Minute, true) {
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), adminRoleContext{}, role)))
	}))
}
func (s *Server) adminConsoleRoutes(r chi.Router) {
	r.Route("/admin", func(r chi.Router) {
		r.Use(s.adminAccess)
		r.Get("/me", func(w http.ResponseWriter, r *http.Request) {
			var role string
			err := s.Store.DB.QueryRowContext(r.Context(), `SELECT role FROM cloud_admin_roles WHERE user_id=$1`, currentUser(r).ID).Scan(&role)
			s.result(w, map[string]any{"user": currentUser(r), "role": role, "sections": adminSections(role), "readable_kinds": adminReadableKinds(role), "export_kinds": adminExportKinds(role)}, err)
		})
		r.Get("/rewards", s.adminRewards)
		s.rewardSupportAdminRoutes(r)
		s.rewardCorrectionAdminRoutes(r)
		s.discoveryAdminRoutes(r)
		s.campaignAdminRoutes(r)
		s.rewardEngineAdminRoutes(r)
		s.rewardRevenueAdminRoutes(r)
		s.commercialAdminRoutes(r)
		s.rewardLegacyBackingRoutes(r)
		r.Post("/rewards/users/{id}/hold", s.adminRewardHold)
		r.Post("/rewards/funding", s.adminRewardFunding)
		r.Post("/rewards/funding/{id}/reconcile", s.adminRewardReconcile)
		r.Get("/rewards/batches/{id}", s.adminRewardBatchDetail)
		r.Post("/rewards/batches", s.adminRewardDraft)
		r.Post("/rewards/batches/{id}/confirm", s.adminRewardConfirm)
		r.Get("/openapi.json", s.adminAPIContract)
		r.Get("/settings", s.adminSettings)
		r.Get("/readiness", s.adminReadiness)
		r.Get("/capacity", s.adminCapacity)
		r.Get("/ai-readiness", s.adminAIReadiness)
		r.Post("/users/{id}/restriction", s.adminRestrictUser)
		r.Get("/users/{id}/records/{kind}", s.adminUserRecords)
		r.Post("/reports/{id}/resolve", s.adminResolveReport)
		r.Post("/videos/{id}/restore", s.adminRestoreVideo)
		r.Put("/settings/{key}", s.adminUpdateSetting)
		r.Get("/records/{kind}", s.adminRecords)
		r.Get("/runtime-policy", s.adminRuntimePolicy)
		r.Get("/runtime-policy/versions", s.adminRuntimePolicyVersions)
		r.Put("/runtime-policy", s.adminUpdateRuntimePolicy)
		r.Post("/runtime-policy/refresh-catalogues", s.adminRefreshRuntimeCatalogues)
		r.Post("/export/{kind}", s.adminExport)
		r.Post("/videos/{id}/block", func(w http.ResponseWriter, r *http.Request) {
			var b struct {
				Reason string `json:"reason"`
			}
			if !s.decode(w, r, &b) {
				return
			}
			if len(b.Reason) > 1000 {
				s.fail(w, ErrValidation)
				return
			}
			err := s.Store.BlockVideo(r.Context(), chi.URLParam(r, "id"), b.Reason, currentUser(r).ID)
			s.result(w, map[string]bool{"blocked": err == nil}, err)
		})
	})
}
func (s *Server) adminSettings(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Store.DB.QueryContext(r.Context(), `SELECT key,value,revision,updated_at FROM cloud_admin_settings ORDER BY key`)
	if err != nil {
		s.fail(w, err)
		return
	}
	defer rows.Close()
	out := []adminSetting{}
	for rows.Next() {
		var v adminSetting
		if err = rows.Scan(&v.Key, &v.Value, &v.Revision, &v.UpdatedAt); err != nil {
			s.fail(w, err)
			return
		}
		out = append(out, v)
	}
	s.result(w, out, rows.Err())
}
func (s *Server) adminUpdateSetting(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Value    json.RawMessage `json:"value"`
		Revision int64           `json:"revision"`
		Reason   string          `json:"reason"`
	}
	key := chi.URLParam(r, "key")
	if !s.decode(w, r, &b) {
		return
	}
	if !validAdminSetting(key, b.Value) || b.Revision < 1 || strings.TrimSpace(b.Reason) == "" || len(b.Reason) > 1000 {
		s.fail(w, ErrValidation)
		return
	}
	tx, err := s.Store.DB.BeginTx(r.Context(), nil)
	if err != nil {
		s.fail(w, err)
		return
	}
	defer tx.Rollback()
	var v adminSetting
	err = tx.QueryRowContext(r.Context(), `UPDATE cloud_admin_settings SET value=$2,revision=revision+1,updated_by=$3,updated_at=now() WHERE key=$1 AND revision=$4 RETURNING key,value,revision,updated_at`, key, []byte(b.Value), currentUser(r).ID, b.Revision).Scan(&v.Key, &v.Value, &v.Revision, &v.UpdatedAt)
	if err == sql.ErrNoRows {
		err = ErrConflict
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `INSERT INTO cloud_admin_audit(actor,action,resource_id,reason)VALUES($1,'update_setting',$2,$3)`, currentUser(r).ID, key, b.Reason)
	}
	if err == nil {
		err = tx.Commit()
	}
	s.result(w, v, err)
}

// Fixed query allowlist intentionally excludes credentials, prompts, private media keys and provider payloads.
var adminRecordQueries = map[string]string{
	"premium":  `SELECT user_id,status,paid_through,updated_at FROM cloud_premium_subscriptions ORDER BY updated_at DESC,user_id LIMIT 100`,
	"crypto":   `SELECT id,user_id,purpose,credits,price_cents,network,token,status,created_at FROM cloud_crypto_invoices ORDER BY created_at DESC,id DESC LIMIT 100`,
	"policies": `SELECT model,credits_per_second,max_cost_micros,version,enabled,created_at FROM cloud_video_policies ORDER BY created_at DESC,model LIMIT 100`,
	"usage":    `SELECT status,count(*) AS jobs,sum(credits) AS quoted_credits,max(created_at) AS latest_job FROM cloud_jobs WHERE created_at >= now()-interval '30 days' GROUP BY status ORDER BY status`,
	"budgets":  `SELECT day,reserved_micros FROM cloud_provider_budget_days ORDER BY day DESC LIMIT 31`,
	"revenue":  `SELECT 'stripe_credits' AS source,status,count(*) AS orders,sum(price_cents) AS gross_order_cents FROM cloud_purchases WHERE created_at>=now()-interval '30 days' GROUP BY status UNION ALL SELECT 'cryptorails',status,count(*),sum(price_cents) FROM cloud_crypto_invoices WHERE created_at>=now()-interval '30 days' GROUP BY status UNION ALL SELECT 'stripe_premium','invoiced',count(*),COALESCE(sum(gross_cents-refunded_cents),0) FROM cloud_premium_invoices WHERE created_at>=now()-interval '30 days'`,
	"audience": `WITH popular AS (SELECT id,title,views,watch_seconds,channel_id FROM cloud_videos WHERE status='published' ORDER BY views DESC,id LIMIT 100) SELECT v.id,v.title,v.views,v.watch_seconds,(SELECT count(*) FROM cloud_follows f WHERE f.channel_id=v.channel_id) AS channel_followers FROM popular v ORDER BY v.views DESC,v.id`,
	"users":    `SELECT u.id,u.name,u.email,u.verified,COALESCE(r.suspended,false) AS suspended,EXISTS(SELECT 1 FROM cloud_admin_roles a WHERE a.user_id=u.id) AS is_admin,u.created_at FROM cloud_users u LEFT JOIN cloud_user_restrictions r ON r.user_id=u.id ORDER BY u.created_at DESC,u.id DESC LIMIT 100`,
	"content":  `SELECT id,channel_id,title,visibility,audience,status,views,watch_seconds,created_at FROM cloud_videos ORDER BY created_at DESC,id DESC LIMIT 100`,
	"jobs":     `SELECT id,workspace_id,status,credits,attempt,created_at,updated_at FROM cloud_jobs ORDER BY created_at DESC,id DESC LIMIT 100`,
	"billing":  `SELECT id,workspace_id,pack_id,credits,price_cents,currency,status,created_at FROM cloud_purchases ORDER BY created_at DESC,id DESC LIMIT 100`,
	"reports":  `SELECT id,video_id,reason,status,created_at FROM cloud_reports WHERE status='open' ORDER BY created_at,id LIMIT 100`,
	"audit":    `SELECT id,actor,action,resource_id,reason,created_at FROM cloud_admin_audit ORDER BY created_at DESC,id DESC LIMIT 100`,
}

func (s *Server) adminRecords(w http.ResponseWriter, r *http.Request) {
	if s.adminPagedRecords(w, r, "") {
		return
	}
	kind := chi.URLParam(r, "kind")
	query, ok := adminRecordQueries[kind]
	if !ok {
		s.fail(w, ErrNotFound)
		return
	}
	if kind == "users" && r.Context().Value(adminRoleContext{}) == "viewer" {
		query = `SELECT id,name,verified,created_at FROM cloud_users ORDER BY created_at DESC,id DESC LIMIT 100`
	}
	rows, err := s.Store.DB.QueryContext(r.Context(), query)
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
	s.json(w, 200, map[string]any{"items": out, "limit": 100, "bounded": true})
}
