package cloud

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"github.com/go-chi/chi/v5"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAdminPermissionMatrix(t *testing.T) {
	for _, tt := range []struct {
		role, method, path string
		allowed            bool
	}{
		{"support", "GET", "/v1/admin/records/users", true}, {"support", "GET", "/v1/admin/records/billing", false}, {"support", "POST", "/v1/admin/export/users", false},
		{"moderation", "POST", "/v1/admin/videos/id/block", true}, {"moderator", "POST", "/v1/admin/reports/id/resolve", true}, {"moderation", "PUT", "/v1/admin/settings/generation_enabled", false}, {"moderation", "GET", "/v1/admin/records/crypto", false},
		{"finance", "GET", "/v1/admin/records/billing", true}, {"finance", "POST", "/v1/admin/export/crypto", true}, {"finance", "GET", "/v1/admin/records/users", false}, {"finance", "POST", "/v1/admin/videos/id/block", false},
		{"operations", "GET", "/v1/admin/capacity", true}, {"operations", "PUT", "/v1/admin/settings/publishing_enabled", true}, {"operations", "PUT", "/v1/admin/runtime-policy", false}, {"operations", "GET", "/v1/admin/runtime-policy", true},
		{"viewer", "GET", "/v1/admin/records/users", true}, {"viewer", "POST", "/v1/admin/export/users", false}, {"unknown", "GET", "/v1/admin/me", false}, {"unknown", "GET", "/v1/admin/records/users", false},
	} {
		if got := adminAllowed(tt.role, tt.method, tt.path); got != tt.allowed {
			t.Fatalf("%s %s %s=%v", tt.role, tt.method, tt.path, got)
		}
	}
}
func TestAdminCSVFormulaEscaping(t *testing.T) {
	for _, tt := range []struct {
		value    any
		expected string
	}{{"=HYPERLINK(\"bad\")", "'=HYPERLINK(\"bad\")"}, {"  +SUM(1,2)", "'  +SUM(1,2)"}, {"\t@SUM(A1)", "'\t@SUM(A1)"}, {"-cmd|abc", "'-cmd|abc"}, {"林安", "林安"}, {"\ufeff=SUM(A1)", "'\ufeff=SUM(A1)"}, {"Safe, quoted", "Safe, quoted"}, {int64(-20), "-20"}, {nil, ""}} {
		if got := adminCSVCell(tt.value); got != tt.expected {
			t.Fatalf("%v=%q expected%q", tt.value, got, tt.expected)
		}
	}
}
func TestAdminPaginationSearchAndExport(t *testing.T) {
	store, u := testPolicyStore(t)
	ctx := context.Background()
	prefix := "pager" + strings.ReplaceAll(u.ID, "-", "")
	created := time.Now().UTC().Add(-time.Hour)
	for i := 0; i < 7; i++ {
		name := prefix + string(rune('a'+i))
		_, err := store.DB.ExecContext(ctx, `INSERT INTO cloud_users(id,email,password_hash,name,created_at)VALUES($1,$2,'not-a-login',$3,$4)`, name, name+"@example.test", name, created)
		if err != nil {
			t.Fatal(err)
		}
	}
	server := NewServer(store, Config{DevMode: true})
	defer server.Close()
	for kind := range adminListSpecs {
		if _, err := server.adminListPage(ctx, "owner", kind, adminPageRequest{Limit: 1}); err != nil {
			t.Fatalf("list %s %v", kind, err)
		}
	}
	p := adminPageRequest{Query: prefix, Limit: 3}
	seen := map[string]bool{}
	pages := 0
	for {
		page, err := server.adminListPage(ctx, "owner", "users", p)
		if err != nil {
			t.Fatal(err)
		}
		pages++
		for _, row := range page.Items {
			id := row["id"].(string)
			if seen[id] {
				t.Fatal("duplicate cursor row")
			}
			seen[id] = true
		}
		if page.NextCursor == "" {
			break
		}
		p.Cursor = page.NextCursor
		if pages > 4 {
			t.Fatal("cursor loop")
		}
	}
	if len(seen) != 7 || pages != 3 {
		t.Fatalf("seen%d pages%d", len(seen), pages)
	}
	page, err := server.adminListPage(ctx, "owner", "users", adminPageRequest{Query: prefix, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = server.adminListPage(ctx, "owner", "users", adminPageRequest{Query: "different", Limit: 2, Cursor: page.NextCursor}); err != ErrValidation {
		t.Fatalf("cursor filter binding=%v", err)
	}
	wildcard, err := server.adminListPage(ctx, "owner", "users", adminPageRequest{Query: prefix + "%", Limit: 25})
	if err != nil || len(wildcard.Items) != 0 {
		t.Fatalf("LIKE wildcard escaped=%d %v", len(wildcard.Items), err)
	}
	viewer, err := server.adminListPage(ctx, "viewer", "users", adminPageRequest{Query: prefix, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := viewer.Items[0]["email"]; ok {
		t.Fatal("viewer got email")
	}
	_, token, err := store.Login(ctx, u.Email, "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{`INSERT INTO cloud_admin_roles(user_id,role,granted_by)VALUES($1,'owner','test')`, `INSERT INTO cloud_totp(user_id,secret_ciphertext,enabled)VALUES($1,'test-only',true)`} {
		if _, err = store.DB.ExecContext(ctx, q, u.ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = store.DB.ExecContext(ctx, `UPDATE cloud_sessions SET mfa_verified_at=now() WHERE token_hash=$1`, tokenHash(token)); err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	server.adminConsoleRoutes(router)
	req := httptest.NewRequest("POST", "/admin/export/users?q="+prefix, strings.NewReader(`{"reason":"Verify scoped CSV export"}`))
	req.AddCookie(&http.Cookie{Name: "videoflow_session", Value: token})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("export%d %s", rec.Code, rec.Body.String())
	}
	records, err := csv.NewReader(strings.NewReader(rec.Body.String())).ReadAll()
	if err != nil || len(records) != 8 {
		t.Fatalf("CSV records%d %v", len(records), err)
	}
	var audits int
	if err = store.DB.QueryRowContext(ctx, `SELECT count(*) FROM cloud_admin_audit WHERE actor=$1 AND action IN('admin_export_started','admin_export_completed')`, u.ID).Scan(&audits); err != nil || audits != 2 {
		t.Fatalf("export audit%d %v", audits, err)
	}
	req = httptest.NewRequest("GET", "/admin/records/users?limit=2&q="+prefix, nil)
	req.AddCookie(&http.Cookie{Name: "videoflow_session", Value: token})
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	var response adminPage
	if err = json.Unmarshal(rec.Body.Bytes(), &response); err != nil || len(response.Items) != 2 || response.NextCursor == "" {
		t.Fatalf("HTTP page%d %+v %v", rec.Code, response, err)
	}
	bulk := prefix + "bulk"
	if _, err = store.DB.ExecContext(ctx, `INSERT INTO cloud_users(id,email,password_hash,name,created_at)SELECT $1||lpad(g::text,4,'0'),$1||g::text||'@example.test','not-a-login',$1||g::text,$2 FROM generate_series(1,1005)g`, bulk, created); err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest("POST", "/admin/export/users?q="+bulk, strings.NewReader(`{"reason":"Verify hard export cap"}`))
	req.AddCookie(&http.Cookie{Name: "videoflow_session", Value: token})
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	capped, err := csv.NewReader(strings.NewReader(rec.Body.String())).ReadAll()
	if rec.Code != 200 || err != nil || len(capped) != 1001 || rec.Header().Get("X-Export-Limit") != "1000" {
		t.Fatalf("CSV cap status%d rows%d %v", rec.Code, len(capped), err)
	}
	var capAudit string
	if err = store.DB.QueryRowContext(ctx, `SELECT reason FROM cloud_admin_audit WHERE actor=$1 AND action='admin_export_completed' ORDER BY id DESC LIMIT 1`, u.ID).Scan(&capAudit); err != nil || !strings.Contains(capAudit, "rows=1000; complete=false") {
		t.Fatalf("cap audit%q %v", capAudit, err)
	}

}
