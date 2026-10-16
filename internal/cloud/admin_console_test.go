package cloud

import (
	"context"
	"github.com/go-chi/chi/v5"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestAdminSettingAllowlist(t *testing.T) {
	for _, tt := range []struct {
		key, value string
		ok         bool
	}{{"generation_enabled", "true", true}, {"publishing_enabled", "false", true}, {"announcement", `"Hello"`, true}, {"announcement", `true`, false}, {"announcement", `null`, false}, {"generation_enabled", `"true"`, false}, {"generation_enabled", `null`, false}, {"OPENROUTER_API_KEY", `"secret"`, false}} {
		if got := validAdminSetting(tt.key, []byte(tt.value)); got != tt.ok {
			t.Fatalf("%s=%s got %v", tt.key, tt.value, got)
		}
	}
}
func TestAdminConsoleRoleAndSettings(t *testing.T) {
	store, u := testPolicyStore(t)
	ctx := context.Background()
	_, token, err := store.Login(ctx, u.Email, "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	s := NewServer(store, Config{DevMode: true})
	defer s.Close()
	router := chi.NewRouter()
	s.adminConsoleRoutes(router)
	request := func(method, path, body string, authenticated bool) int {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if authenticated {
			req.AddCookie(&http.Cookie{Name: "videoflow_session", Value: token})
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec.Code
	}
	if got := request("GET", "/admin/me", "", false); got != 401 {
		t.Fatalf("anonymous=%d", got)
	}
	if got := request("GET", "/admin/records/users", "", true); got != 403 {
		t.Fatalf("ordinary user=%d", got)
	}
	if _, err = store.DB.ExecContext(ctx, `INSERT INTO cloud_admin_roles(user_id,role,granted_by)VALUES($1,'viewer','test')`, u.ID); err != nil {
		t.Fatal(err)
	}
	if got := request("GET", "/admin/records/users", "", true); got != 403 {
		t.Fatalf("admin without MFA=%d", got)
	}
	if _, err = store.DB.ExecContext(ctx, `INSERT INTO cloud_totp(user_id,secret_ciphertext,enabled)VALUES($1,'test-only',true)`, u.ID); err != nil {
		t.Fatal(err)
	}
	if got := request("GET", "/admin/records/users", "", true); got != 403 {
		t.Fatalf("enrolled without verified session=%d", got)
	}
	if _, err = store.DB.ExecContext(ctx, `UPDATE cloud_sessions SET mfa_verified_at=now() WHERE token_hash=$1`, tokenHash(token)); err != nil {
		t.Fatal(err)
	}
	if got := request("GET", "/admin/records/users", "", true); got != 200 {
		t.Fatalf("viewer=%d", got)
	}
	if got := request("PUT", "/admin/settings/announcement", `{"value":"test","revision":1,"reason":"test"}`, true); got != 403 {
		t.Fatalf("viewer mutation=%d", got)
	}
	if _, err = store.DB.ExecContext(ctx, `UPDATE cloud_admin_roles SET role='owner' WHERE user_id=$1`, u.ID); err != nil {
		t.Fatal(err)
	}
	for kind := range adminRecordQueries {
		if got := request("GET", "/admin/records/"+kind, "", true); got != 200 {
			t.Fatalf("records %s=%d", kind, got)
		}
	}
	var rev int64
	if err = store.DB.QueryRowContext(ctx, `SELECT revision FROM cloud_admin_settings WHERE key='announcement'`).Scan(&rev); err != nil {
		t.Fatal(err)
	}
	// Independent key revision avoids cross-test assumptions.
	body := `{"value":"Admin console test","revision":` + strconv.FormatInt(rev, 10) + `,"reason":"Verify audited controls"}`
	if got := request("PUT", "/admin/settings/announcement", body, true); got != 200 {
		t.Fatalf("owner mutation=%d", got)
	}
	if got := request("PUT", "/admin/settings/announcement", body, true); got != 409 {
		t.Fatalf("stale update=%d", got)
	}
	var count int
	if err = store.DB.QueryRowContext(ctx, `SELECT count(*) FROM cloud_admin_audit WHERE actor=$1 AND action='update_setting'`, u.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("audit=%d %v", count, err)
	}
	if got := request("GET", "/admin/records/password_hash", "", true); got != 404 {
		t.Fatalf("query allowlist=%d", got)
	}
	// Only owner can restrict ordinary users; administration and self cannot be locked out by this endpoint.
	_, target := testPolicyStore(t)
	if got := request("POST", "/admin/users/"+u.ID+"/restriction", `{"suspended":true,"reason":"test"}`, true); got != 400 {
		t.Fatalf("self suspension=%d", got)
	}
	if got := request("POST", "/admin/users/"+target.ID+"/restriction", `{"suspended":true,"reason":"Policy violation"}`, true); got != 200 {
		t.Fatalf("suspension=%d", got)
	}
	// A new cookie issued by another auth path still cannot regain authenticated access.
	bypass, err := store.newSecuritySession(ctx, target.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Authenticate(ctx, bypass); err != ErrUnauthorized {
		t.Fatalf("suspended session bypass=%v", err)
	}
	if _, _, err = store.Login(ctx, target.Email, "correct horse battery staple"); err != ErrInvalidCredentials {
		t.Fatalf("suspended password login=%v", err)
	}
	if restricted, err := s.adminUserRestricted(ctx, target.ID); err != nil || !restricted {
		t.Fatalf("restriction not stored %v %v", restricted, err)
	}
	if _, err = store.DB.ExecContext(ctx, `UPDATE cloud_sessions SET mfa_verified_at=now()-interval '16 minutes' WHERE token_hash=$1`, tokenHash(token)); err != nil {
		t.Fatal(err)
	}
	if got := request("GET", "/admin/records/jobs", "", true); got != 200 {
		t.Fatalf("read session=%d", got)
	}
	if got := request("PUT", "/admin/settings/announcement", body, true); got != 403 {
		t.Fatalf("stale MFA mutation=%d", got)
	}

}
