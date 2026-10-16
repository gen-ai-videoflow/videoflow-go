package cloud

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func Test6eekSessionDomainValidation(t *testing.T) {
	for _, domain := range []string{".6eek.com", "6eek.com"} {
		if e := validateSessionConfig(Config{PublicOrigin: "https://studio.6eek.com", SessionCookieDomain: domain, BrowserOrigins: []string{"https://admin.6eek.com"}}); e != nil {
			t.Fatal(e)
		}
	}
	for _, domain := range []string{".com", ".evil.com", ".admin.6eek.com", ".6eek.com.evil.com", "*.6eek.com"} {
		if e := validateSessionConfig(Config{PublicOrigin: "https://6eek.com", SessionCookieDomain: domain}); e == nil {
			t.Fatal("unsafe cookie domain", domain)
		}
	}
	if e := validateSessionConfig(Config{PublicOrigin: "http://localhost:5173", DevMode: true, SessionCookieDomain: "localhost"}); e == nil {
		t.Fatal("insecure shared cookie accepted")
	}
	if e := validateSessionConfig(Config{PublicOrigin: "https://6eek.com", BrowserOrigins: []string{"https://*.6eek.com"}}); e == nil {
		t.Fatal("wildcard browser origin")
	}
}
func Test6eekSessionCookieAttributesAndClearScopes(t *testing.T) {
	app := NewServer(nil, Config{PublicOrigin: "https://6eek.com", SessionCookieDomain: ".6eek.com", BrowserOrigins: []string{"https://admin.6eek.com"}})
	defer app.Close()
	w := httptest.NewRecorder()
	app.cookie(w, "session-token", 100)
	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatal(cookies)
	}
	c := cookies[0]
	if c.Name != "6eek_session" || c.Domain != "6eek.com" || !c.Secure || !c.HttpOnly || c.Path != "/" || c.SameSite != http.SameSiteLaxMode {
		t.Fatalf("cookie attributes %+v", c)
	}
	w = httptest.NewRecorder()
	app.cookie(w, "", -1)
	cookies = w.Result().Cookies()
	if len(cookies) != 4 {
		t.Fatalf("did not clear both names and scopes %+v", cookies)
	}
	for _, c := range cookies {
		if c.MaxAge >= 0 || !c.HttpOnly || !c.Secure {
			t.Fatalf("uncleared cookie %+v", c)
		}
	}
	dev := NewServer(nil, Config{DevMode: true, PublicOrigin: "http://localhost:5173"})
	defer dev.Close()
	w = httptest.NewRecorder()
	dev.cookie(w, "token", 100)
	c = w.Result().Cookies()[0]
	if c.Domain != "" || c.Secure {
		t.Fatal("development cookie must stay host-only", c)
	}
}
func Test6eekLegacySessionFallbackAndLogoutRevocation(t *testing.T) {
	s, u := testPlatform(t)
	ctx := context.Background()
	_, old, e := s.Login(ctx, u.Email, "Test-password-123!")
	if e != nil {
		t.Fatal(e)
	}
	_, newToken, e := s.Login(ctx, u.Email, "Test-password-123!")
	if e != nil {
		t.Fatal(e)
	}
	app := NewServer(s, Config{PublicOrigin: "https://6eek.com", SessionCookieDomain: ".6eek.com", BrowserOrigins: []string{"https://admin.6eek.com"}})
	defer app.Close()
	for name, token := range map[string]string{legacySessionCookieName: old, sessionCookieName: newToken} {
		r := httptest.NewRequest("GET", "https://api.6eek.com/v1/auth/me", nil)
		r.AddCookie(&http.Cookie{Name: name, Value: token})
		if got := app.optionalUser(r); got.ID != u.ID {
			t.Fatal("session compatibility", name)
		}
	}
	r := httptest.NewRequest("POST", "https://api.6eek.com/v1/auth/logout", strings.NewReader(`{}`))
	r.AddCookie(&http.Cookie{Name: legacySessionCookieName, Value: old})
	r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: newToken})
	r.Header.Set("Origin", "https://admin.6eek.com")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	app.security(http.HandlerFunc(app.logout)).ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("admin-origin logout %d %s", w.Code, w.Body)
	}
	if len(w.Result().Cookies()) != 4 {
		t.Fatal("logout did not clear host/domain aliases")
	}
	for _, token := range []string{old, newToken} {
		if _, e = s.Authenticate(ctx, token); e == nil {
			t.Fatal("logout left a session active")
		}
	}
}
func Test6eekCSRFRejectsUntrustedOrigins(t *testing.T) {
	app := NewServer(nil, Config{PublicOrigin: "https://6eek.com", SessionCookieDomain: ".6eek.com", BrowserOrigins: []string{"https://admin.6eek.com"}})
	defer app.Close()
	calls := 0
	handler := app.security(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(200) }))
	for _, origin := range []string{"https://evil.com", "https://admin.6eek.com.evil.com", "null", ""} {
		r := httptest.NewRequest("POST", "https://api.6eek.com/v1/auth/logout", strings.NewReader(`{}`))
		r.Header.Set("Origin", origin)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code == 200 || w.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Fatal("untrusted CSRF origin allowed", origin, w.Code)
		}
	}
	if calls != 0 {
		t.Fatal("untrusted mutation reached handler")
	}
	for _, origin := range []string{"https://6eek.com", "https://admin.6eek.com"} {
		r := httptest.NewRequest("POST", "https://api.6eek.com/v1/auth/logout", strings.NewReader(`{}`))
		r.Header.Set("Origin", origin)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 200 || w.Header().Get("Access-Control-Allow-Origin") != origin {
			t.Fatal("approved origin rejected", origin, w.Code)
		}
	}
}
