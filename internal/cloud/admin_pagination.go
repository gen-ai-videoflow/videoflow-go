package cloud

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type adminListSpec struct{ columns, from, at, id, search, status, scope string }

var adminListSpecs = map[string]adminListSpec{
	"premium_payments": {`i.id,c.user_id,i.gross_cents,i.refunded_cents,i.currency,i.period_end,i.created_at`, `cloud_premium_invoices i JOIN cloud_premium_checkouts c ON c.id=i.checkout_id`, `i.created_at`, `i.id`, `lower(i.id) LIKE $1 OR lower(c.user_id) LIKE $1`, ``, ``},
	"users":            {`u.id,u.name,u.email,u.verified,COALESCE(r.suspended,false) AS suspended,EXISTS(SELECT 1 FROM cloud_admin_roles a WHERE a.user_id=u.id) AS is_admin,u.created_at`, `cloud_users u LEFT JOIN cloud_user_restrictions r ON r.user_id=u.id`, `u.created_at`, `u.id`, `lower(u.name) LIKE $1 OR lower(u.email) LIKE $1 OR lower(u.id) LIKE $1`, ``, ``},
	"content":          {`v.id,v.channel_id,v.title,v.visibility,v.audience,v.status,v.views,v.watch_seconds,v.created_at`, `cloud_videos v`, `v.created_at`, `v.id`, `lower(v.title) LIKE $1 OR lower(v.id) LIKE $1`, `v.status`, `v.channel_id IN(SELECT id FROM cloud_channels WHERE user_id=$7)`},
	"jobs":             {`j.id,j.workspace_id,j.status,j.credits,j.attempt,j.created_at,j.updated_at`, `cloud_jobs j`, `j.created_at`, `j.id`, `lower(j.id) LIKE $1 OR lower(j.workspace_id) LIKE $1`, `j.status`, `j.workspace_id IN(SELECT id FROM cloud_workspaces WHERE owner_id=$7)`},
	"reports":          {`r.id,r.video_id,r.reason,r.status,r.created_at`, `cloud_reports r`, `r.created_at`, `r.id`, `lower(r.id) LIKE $1 OR lower(r.video_id) LIKE $1`, `r.status`, ``},
	"audit":            {`a.id,a.actor,a.action,a.resource_id,a.reason,a.created_at`, `cloud_admin_audit a`, `a.created_at`, `a.id::text`, `lower(a.actor) LIKE $1 OR lower(a.action) LIKE $1 OR lower(a.resource_id) LIKE $1`, ``, ``},
	"billing":          {`b.id,b.workspace_id,b.pack_id,b.credits,b.price_cents,b.currency,b.status,b.created_at`, `cloud_purchases b`, `b.created_at`, `b.id`, `lower(b.id) LIKE $1 OR lower(b.workspace_id) LIKE $1`, `b.status`, ``},
	"crypto":           {`c.id,c.user_id,c.purpose,c.credits,c.price_cents,c.network,c.token,c.status,c.created_at`, `cloud_crypto_invoices c`, `c.created_at`, `c.id`, `lower(c.id) LIKE $1 OR lower(c.user_id) LIKE $1`, `c.status`, ``},
	"premium":          {`p.user_id,p.status,p.paid_through,p.updated_at`, `cloud_premium_subscriptions p`, `p.updated_at`, `p.user_id`, `lower(p.user_id) LIKE $1`, `p.status`, ``},
	"projects":         {`p.id,p.name,p.created_at`, `cloud_projects p`, `p.created_at`, `p.id`, `lower(p.name) LIKE $1 OR lower(p.id) LIKE $1`, ``, `p.workspace_id IN(SELECT id FROM cloud_workspaces WHERE owner_id=$7)`},
}

type adminPageCursor struct {
	Version int       `json:"v"`
	Kind    string    `json:"k"`
	Query   string    `json:"q"`
	Status  string    `json:"s"`
	User    string    `json:"u"`
	At      time.Time `json:"t"`
	ID      string    `json:"id"`
}
type adminPageRequest struct {
	Query, Status, User, Cursor string
	Limit                       int
}
type adminPage struct {
	Items      []map[string]any `json:"items"`
	Limit      int              `json:"limit"`
	NextCursor string           `json:"next_cursor,omitempty"`
	SearchMode string           `json:"search_mode"`
}

func adminPageInput(r *http.Request) (adminPageRequest, error) {
	q := r.URL.Query()
	p := adminPageRequest{Query: strings.ToLower(strings.TrimSpace(q.Get("q"))), Status: strings.TrimSpace(q.Get("status")), Cursor: q.Get("cursor"), Limit: 25}
	if raw := q.Get("limit"); raw != "" {
		v, e := strconv.Atoi(raw)
		if e != nil || v < 1 || v > 100 {
			return p, ErrValidation
		}
		p.Limit = v
	}
	if utf8.RuneCountInString(p.Query) > 120 || len(p.Status) > 40 || len(p.Cursor) > 2048 {
		return p, ErrValidation
	}
	return p, nil
}
func adminLikePrefix(q string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q) + "%"
}
func (s *Server) adminListPage(ctx context.Context, role, kind string, p adminPageRequest) (adminPage, error) {
	out := adminPage{Items: []map[string]any{}, Limit: p.Limit, SearchMode: "prefix"}
	spec, ok := adminListSpecs[kind]
	if !ok {
		return out, ErrNotFound
	}
	if p.User != "" && spec.scope == "" {
		return out, ErrValidation
	}
	if p.Limit < 1 || p.Limit > 1001 {
		return out, ErrValidation
	}
	var cursor adminPageCursor
	hasCursor := p.Cursor != ""
	if hasCursor {
		raw, e := base64.RawURLEncoding.DecodeString(p.Cursor)
		if e != nil || json.Unmarshal(raw, &cursor) != nil || cursor.Version != 1 || cursor.Kind != kind || cursor.Query != p.Query || cursor.Status != p.Status || cursor.User != p.User || cursor.At.IsZero() || len(cursor.ID) > 200 {
			return out, ErrValidation
		}
	}
	cols := spec.columns
	if kind == "users" && (role == "viewer" || normalizedAdminRole(role) == "moderation") {
		cols = `u.id,u.name,u.verified,u.created_at`
		spec.search = `lower(u.name) LIKE $1 OR lower(u.id) LIKE $1`
	}
	where := `($2='' OR (` + spec.search + `))`
	if spec.status != "" {
		where += ` AND ($3='' OR ` + spec.status + `=$3)`
	} else {
		where += ` AND $3=''`
	}
	where += ` AND (NOT $4 OR (` + spec.at + `,` + spec.id + `) < ($5::timestamptz,$6::text))`
	if p.User != "" {
		where += ` AND ` + spec.scope
	} else {
		where += ` AND $7::text=''`
	}
	query := `SELECT ` + cols + `,` + spec.at + ` AS _cursor_at,` + spec.id + ` AS _cursor_id FROM ` + spec.from + ` WHERE ` + where + ` ORDER BY ` + spec.at + ` DESC,` + spec.id + ` DESC LIMIT $8`
	at := cursor.At
	if at.IsZero() {
		at = time.Unix(0, 0)
	}
	rows, e := s.Store.DB.QueryContext(ctx, query, adminLikePrefix(p.Query), p.Query, p.Status, hasCursor, at, cursor.ID, p.User, p.Limit+1)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	columns, e := rows.Columns()
	if e != nil {
		return out, e
	}
	var lastAt time.Time
	lastID := ""
	for rows.Next() {
		values := make([]any, len(columns))
		ptrs := make([]any, len(columns))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if e = rows.Scan(ptrs...); e != nil {
			return out, e
		}
		if len(out.Items) == p.Limit {
			next := adminPageCursor{1, kind, p.Query, p.Status, p.User, lastAt, lastID}
			raw, _ := json.Marshal(next)
			out.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
			break
		}
		record := map[string]any{}
		for i, key := range columns {
			switch key {
			case "_cursor_at":
				lastAt, _ = values[i].(time.Time)
			case "_cursor_id":
				if b, ok := values[i].([]byte); ok {
					lastID = string(b)
				} else {
					lastID = fmt.Sprint(values[i])
				}
			default:
				if b, ok := values[i].([]byte); ok {
					record[key] = string(b)
				} else {
					record[key] = values[i]
				}
			}
		}
		out.Items = append(out.Items, record)
	}
	return out, rows.Err()
}
func (s *Server) adminPagedRecords(w http.ResponseWriter, r *http.Request, user string) bool {
	kind := chi.URLParam(r, "kind")
	if _, ok := adminListSpecs[kind]; !ok {
		return false
	}
	p, e := adminPageInput(r)
	if e != nil {
		s.fail(w, e)
		return true
	}
	p.User = user
	role, _ := r.Context().Value(adminRoleContext{}).(string)
	page, e := s.adminListPage(r.Context(), role, kind, p)
	s.result(w, page, e)
	return true
}
