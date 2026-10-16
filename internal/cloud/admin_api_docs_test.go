package cloud

import (
	"context"
	"github.com/go-chi/chi/v5"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAdminAPIContractRequiresVerifiedAdministrator(t *testing.T) {
	store, u := testPlatform(t)
	ctx := context.Background()
	_, token, err := store.Login(ctx, u.Email, "Test-password-123!")
	if err != nil {
		t.Fatal(err)
	}
	app := NewServer(store, Config{DevMode: true})
	defer app.Close()
	router := chi.NewRouter()
	app.adminConsoleRoutes(router)
	call := func(auth bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/admin/openapi.json", nil)
		if auth {
			r.AddCookie(&http.Cookie{Name: "videoflow_session", Value: token})
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	if w := call(false); w.Code != 401 {
		t.Fatalf("anonymous %d", w.Code)
	}
	if w := call(true); w.Code != 403 {
		t.Fatalf("ordinary user %d", w.Code)
	}
	if _, err = store.DB.Exec(`INSERT INTO cloud_admin_roles(user_id,role,granted_by)VALUES($1,'owner','test')`, u.ID); err != nil {
		t.Fatal(err)
	}
	if w := call(true); w.Code != 403 {
		t.Fatalf("without MFA %d", w.Code)
	}
	store.DB.Exec(`INSERT INTO cloud_totp(user_id,secret_ciphertext,enabled)VALUES($1,'test-only',true)`, u.ID)
	store.DB.Exec(`UPDATE cloud_sessions SET mfa_verified_at=now() WHERE token_hash=$1`, tokenHash(token))
	w := call(true)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"openapi"`) || !strings.Contains(w.Body.String(), `/creator/rewards/summary`) {
		t.Fatalf("contract %d %s", w.Code, w.Body)
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("private contract can be cached")
	}
	for _, role := range []string{"support", "viewer", "moderation"} {
		if adminAllowed(role, "GET", "/v1/admin/openapi.json") {
			t.Fatalf("%s docs access", role)
		}
	}
}
