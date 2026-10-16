package cloud

import (
	"fmt"
	"golang.org/x/net/publicsuffix"
	"net"
	"net/http"
	"net/url"
	"strings"
)

const sessionCookieName = "6eek_session"
const legacySessionCookieName = "videoflow_session"

func sessionCookie(r *http.Request) (*http.Cookie, error) {
	if c, e := r.Cookie(sessionCookieName); e == nil && c.Value != "" {
		return c, nil
	}
	return r.Cookie(legacySessionCookieName)
}
func validateSessionConfig(c Config) error {
	u, e := url.Parse(c.PublicOrigin)
	if e != nil || u.Hostname() == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && !(c.DevMode && u.Scheme == "http")) {
		return fmt.Errorf("invalid public origin for session settings")
	}
	if c.SessionCookieDomain != "" {
		domain := strings.TrimPrefix(strings.ToLower(c.SessionCookieDomain), ".")
		registered, e := publicsuffix.EffectiveTLDPlusOne(strings.ToLower(u.Hostname()))
		if e != nil || net.ParseIP(u.Hostname()) != nil || u.Scheme != "https" || domain != registered || strings.ContainsAny(domain, "/:* ") {
			return fmt.Errorf("SESSION_COOKIE_DOMAIN must match the PUBLIC_ORIGIN registrable domain and use HTTPS")
		}
	}
	if len(c.BrowserOrigins) > 10 {
		return fmt.Errorf("BROWSER_ORIGINS permits at most 10 explicit origins")
	}
	for _, origin := range c.BrowserOrigins {
		v, e := url.Parse(origin)
		if e != nil || v.Hostname() == "" || v.User != nil || v.Path != "" || v.RawQuery != "" || v.Fragment != "" || strings.Contains(origin, "*") || (v.Scheme != "https" && !(c.DevMode && v.Scheme == "http")) {
			return fmt.Errorf("BROWSER_ORIGINS must contain explicit HTTPS origins")
		}
	}
	return nil
}
func (s *Server) allowedBrowserOrigin(origin string) bool {
	if origin == "" {
		return s.Config.DevMode && s.Config.PublicOrigin == ""
	}
	if origin == s.Config.PublicOrigin {
		return true
	}
	for _, allowed := range s.Config.BrowserOrigins {
		if origin == allowed {
			return true
		}
	}
	return false
}
func (s *Server) sessionDomain() string {
	if s.Config.SessionCookieDomain == "" || validateSessionConfig(s.Config) != nil {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(s.Config.SessionCookieDomain), ".")
}
func (s *Server) clearSessionCookies(w http.ResponseWriter) {
	domains := []string{""}
	if d := s.sessionDomain(); d != "" {
		domains = append(domains, d)
	}
	for _, name := range []string{sessionCookieName, legacySessionCookieName} {
		for _, domain := range domains {
			http.SetCookie(w, &http.Cookie{Name: name, Path: "/", Domain: domain, MaxAge: -1, HttpOnly: true, Secure: !s.Config.DevMode, SameSite: http.SameSiteLaxMode})
		}
	}
}

func oauthBrowserCookie(r *http.Request) (*http.Cookie, error) {
	if c, e := r.Cookie("6eek_oauth"); e == nil && c.Value != "" {
		return c, nil
	}
	return r.Cookie("videoflow_oauth")
}
