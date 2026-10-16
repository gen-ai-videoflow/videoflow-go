package cloud

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
)

type oauthSettings struct{ Name, Issuer, ClientID, ClientSecret string }

func (s *Server) oauthSettings(name string) (oauthSettings, bool) {
	p := oauthSettings{Name: name}
	switch name {
	case "google":
		p.Issuer = "https://accounts.google.com"
		p.ClientID = s.Config.GoogleClientID
		p.ClientSecret = s.Config.GoogleClientSecret
	case "apple":
		p.Issuer = "https://appleid.apple.com"
		p.ClientID = s.Config.AppleClientID
		p.ClientSecret = s.Config.AppleClientSecret
	default:
		return p, false
	}
	_, err := authCipher(s.Config.AuthEncryptionKey)
	return p, p.ClientID != "" && p.ClientSecret != "" && err == nil && strings.HasPrefix(s.Config.PublicOrigin, "https://")
}

var oidcProviders sync.Map

func discoverOAuth(ctx context.Context, p oauthSettings) (*oidc.Provider, error) {
	if v, ok := oidcProviders.Load(p.Issuer); ok {
		return v.(*oidc.Provider), nil
	}
	provider, err := oidc.NewProvider(ctx, p.Issuer)
	if err == nil {
		oidcProviders.Store(p.Issuer, provider)
	}
	return provider, err
}
func (s *Server) oauthRoutes(r chi.Router) {
	r.Get("/auth/providers", s.authProviders)
	r.Post("/auth/oauth/{provider}/start", s.startOAuth)
	r.Get("/auth/oauth/{provider}/callback", s.oauthCallback)
	r.Post("/auth/oauth/{provider}/callback", s.oauthCallback)
}
func (s *Server) authProviders(w http.ResponseWriter, r *http.Request) {
	providers := []string{}
	for _, name := range []string{"google", "apple"} {
		if _, ok := s.oauthSettings(name); ok {
			providers = append(providers, name)
		}
	}
	linked := []string{}
	if u := s.optionalUser(r); u.ID != "" {
		rows, err := s.Store.DB.QueryContext(r.Context(), `SELECT provider FROM cloud_oauth_identities WHERE user_id=$1 ORDER BY provider`, u.ID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var name string
				if rows.Scan(&name) == nil {
					linked = append(linked, name)
				}
			}
		}
	}
	s.json(w, 200, map[string]any{"providers": providers, "linked": linked})
}
func (s *Server) oauthConfig(ctx context.Context, p oauthSettings) (*oidc.Provider, *oauth2.Config, error) {
	provider, err := discoverOAuth(ctx, p)
	if err != nil {
		return nil, nil, err
	}
	return provider, &oauth2.Config{ClientID: p.ClientID, ClientSecret: p.ClientSecret, Endpoint: provider.Endpoint(), RedirectURL: s.Config.PublicOrigin + "/api/v1/auth/oauth/" + p.Name + "/callback", Scopes: []string{oidc.ScopeOpenID, "email"}}, nil
}
func (s *Server) startOAuth(w http.ResponseWriter, r *http.Request) {
	if !s.rate(w, r, "oauth-start:"+s.peer(r), 15, 15*time.Minute, true) {
		return
	}
	name := chi.URLParam(r, "provider")
	p, ok := s.oauthSettings(name)
	if !ok {
		s.fail(w, ErrUnavailable)
		return
	}
	var in struct {
		Link     bool   `json:"link"`
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if !s.decode(w, r, &in) {
		return
	}
	var linkUser any
	var linkSession any
	if in.Link {
		u, err := s.oauthCurrentUser(r)
		if err != nil {
			s.fail(w, ErrUnauthorized)
			return
		}
		if err = s.reauth(r.Context(), r, u, in.Password, in.Code, false); err != nil {
			s.fail(w, err)
			return
		}
		linkUser = u.ID
		cookie, _ := sessionCookie(r)
		linkSession = tokenHash(cookie.Value)
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	_, cfg, err := s.oauthConfig(ctx, p)
	if err != nil {
		s.fail(w, ErrUnavailable)
		return
	}
	state, err := rawToken()
	if err != nil {
		s.fail(w, err)
		return
	}
	browser, err := rawToken()
	if err != nil {
		s.fail(w, err)
		return
	}
	nonce, err := rawToken()
	if err != nil {
		s.fail(w, err)
		return
	}
	verifier := oauth2.GenerateVerifier()
	cipher, err := s.encryptTOTP("oauth:"+state, verifier)
	if err != nil {
		s.fail(w, err)
		return
	}
	_, err = s.Store.DB.ExecContext(ctx, `INSERT INTO cloud_oauth_states(token_hash,provider,browser_hash,nonce_hash,verifier_ciphertext,link_user_id,link_session_hash)VALUES($1,$2,$3,$4,$5,$6,$7)`, tokenHash(state), name, tokenHash(browser), tokenHash(nonce), cipher, linkUser, linkSession)
	if err != nil {
		s.fail(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "6eek_oauth", Value: browser, Path: "/api/v1/auth/oauth/", HttpOnly: true, Secure: true, SameSite: http.SameSiteNoneMode, MaxAge: 300})
	options := []oauth2.AuthCodeOption{oidc.Nonce(nonce)}
	if name == "google" {
		options = append(options, oauth2.S256ChallengeOption(verifier))
	} else {
		options = append(options, oauth2.SetAuthURLParam("response_mode", "form_post"))
	}
	s.json(w, 200, map[string]string{"url": cfg.AuthCodeURL(state, options...)})
}
func (s *Server) oauthCurrentUser(r *http.Request) (User, error) {
	cookie, err := sessionCookie(r)
	if err != nil {
		return User{}, ErrUnauthorized
	}
	return s.Store.Authenticate(r.Context(), cookie.Value)
}

type oauthPending struct {
	NonceHash, Verifier string
	LinkUserID          sql.NullString
	LinkSessionHash     sql.NullString
}

func (s *Server) consumeOAuthState(ctx context.Context, state, browser, name string) (oauthPending, error) {
	var p oauthPending
	if len(state) != 43 || len(browser) != 43 {
		return p, ErrUnauthorized
	}
	err := s.Store.DB.QueryRowContext(ctx, `DELETE FROM cloud_oauth_states WHERE token_hash=$1 AND browser_hash=$2 AND provider=$3 AND expires_at>now() RETURNING nonce_hash,verifier_ciphertext,link_user_id,link_session_hash`, tokenHash(state), tokenHash(browser), name).Scan(&p.NonceHash, &p.Verifier, &p.LinkUserID, &p.LinkSessionHash)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrUnauthorized
	}
	return p, err
}

type oauthClaims struct {
	AuthorizedParty string          `json:"azp"`
	Email           string          `json:"email"`
	EmailVerified   json.RawMessage `json:"email_verified"`
	Nonce           string          `json:"nonce"`
	Name            string          `json:"name"`
}

func verifyOAuthToken(ctx context.Context, provider *oidc.Provider, p oauthSettings, raw, nonceHash string) (string, oauthClaims, error) {
	var claims oauthClaims
	token, err := provider.Verifier(&oidc.Config{ClientID: p.ClientID}).Verify(ctx, raw)
	if err != nil {
		return "", claims, ErrUnauthorized
	}
	if err = token.Claims(&claims); err != nil || token.Subject == "" || tokenHash(claims.Nonce) != nonceHash {
		return "", claims, ErrUnauthorized
	}
	verified := string(claims.EmailVerified) == "true" || string(claims.EmailVerified) == `"true"`
	if !verified || claims.Email == "" || (claims.AuthorizedParty != "" && claims.AuthorizedParty != p.ClientID) || (len(token.Audience) > 1 && claims.AuthorizedParty != p.ClientID) {
		return "", claims, ErrUnauthorized
	}
	return token.Subject, claims, nil
}
func (s *Server) oauthCallback(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	fail := func(reason string) {
		http.Redirect(w, r, s.Config.PublicOrigin+"/login?oauth_error="+url.QueryEscape(reason), http.StatusSeeOther)
	}
	if !s.rate(w, r, "oauth-callback:"+s.peer(r), 30, 15*time.Minute, true) {
		return
	}
	name := chi.URLParam(r, "provider")
	p, ok := s.oauthSettings(name)
	if !ok {
		fail("unavailable")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	if err := r.ParseForm(); err != nil {
		fail("invalid")
		return
	}
	cookie, err := oauthBrowserCookie(r)
	if err != nil {
		fail("expired")
		return
	}
	pending, err := s.consumeOAuthState(r.Context(), r.Form.Get("state"), cookie.Value, name)
	if err != nil {
		fail("expired")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "6eek_oauth", Path: "/api/v1/auth/oauth/", HttpOnly: true, Secure: true, SameSite: http.SameSiteNoneMode, MaxAge: -1})
	http.SetCookie(w, &http.Cookie{Name: "videoflow_oauth", Path: "/api/v1/auth/oauth/", HttpOnly: true, Secure: true, SameSite: http.SameSiteNoneMode, MaxAge: -1})
	if r.Form.Get("error") != "" || r.Form.Get("code") == "" {
		fail("cancelled")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	provider, cfg, err := s.oauthConfig(ctx, p)
	if err != nil {
		fail("unavailable")
		return
	}
	verifier, err := s.decryptTOTP("oauth:"+r.Form.Get("state"), pending.Verifier)
	if err != nil {
		fail("invalid")
		return
	}
	options := []oauth2.AuthCodeOption{}
	if name == "google" {
		options = append(options, oauth2.VerifierOption(verifier))
	}
	exchanged, err := cfg.Exchange(ctx, r.Form.Get("code"), options...)
	if err != nil {
		fail("invalid")
		return
	}
	raw, _ := exchanged.Extra("id_token").(string)
	subject, claims, err := verifyOAuthToken(ctx, provider, p, raw, pending.NonceHash)
	if err != nil {
		fail("invalid")
		return
	}
	var u User
	if pending.LinkUserID.Valid {
		valid, err := s.validOAuthLink(ctx, pending)
		if err != nil || !valid {
			fail("link_expired")
			return
		}
		err = s.Store.linkOAuthIdentity(ctx, pending.LinkUserID.String, name, subject)
		if err != nil {
			fail("already_linked")
			return
		}
		http.Redirect(w, r, s.Config.PublicOrigin+"/security?linked="+name, http.StatusSeeOther)
		return
	}
	u, err = s.Store.oauthIdentity(ctx, name, subject, claims)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			fail("account_exists")
		} else {
			fail("invalid")
		}
		return
	}
	session, challenge, err := s.Store.oauthSession(ctx, u)
	if err != nil {
		fail("invalid")
		return
	}
	if challenge != "" {
		http.Redirect(w, r, s.Config.PublicOrigin+"/magic-link?challenge="+url.QueryEscape(challenge), http.StatusSeeOther)
		return
	}
	s.cookie(w, session, 30*24*3600)
	http.Redirect(w, r, s.Config.PublicOrigin+"/discover", http.StatusSeeOther)
}
func (s *Store) linkOAuthIdentity(ctx context.Context, user, provider, subject string) error {
	// Never move an identity between accounts, including on conflicting verified emails.
	_, err := s.DB.ExecContext(ctx, `INSERT INTO cloud_oauth_identities(provider,subject,user_id)VALUES($1,$2,$3)`, provider, subject, user)
	return dbError(err)
}
func (s *Store) oauthIdentity(ctx context.Context, provider, subject string, claims oauthClaims) (User, error) {
	email := strings.ToLower(strings.TrimSpace(claims.Email))
	address, validationErr := mail.ParseAddress(email)
	if validationErr != nil || address.Address != email || len(email) > 254 || subject == "" || len(subject) > 255 {
		return User{}, ErrValidation
	}
	u, err := scanUser(s.DB.QueryRowContext(ctx, userSelect+` JOIN cloud_oauth_identities oi ON oi.user_id=u.id WHERE oi.provider=$1 AND oi.subject=$2`, provider, subject))
	if err == nil {
		return u, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return User{}, err
	}
	var exists bool
	if err = s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cloud_users WHERE email=$1)`, strings.ToLower(strings.TrimSpace(claims.Email))).Scan(&exists); err != nil {
		return User{}, err
	}
	if exists {
		return User{}, ErrConflict
	}
	password, err := rawToken()
	if err != nil {
		return User{}, err
	}
	handle := "user_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:20]
	name := strings.TrimSpace(claims.Name)
	if name == "" || len(name) > 100 {
		name = "6eek member"
	}
	hash, err := hashPassword(password)
	if err != nil {
		return User{}, err
	}
	u = User{ID: uuid.NewString(), Email: strings.ToLower(strings.TrimSpace(claims.Email)), Name: name, WorkspaceID: uuid.NewString(), ChannelID: uuid.NewString(), Handle: handle, Verified: true}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback()
	queries := []struct {
		q    string
		args []any
	}{
		{`INSERT INTO cloud_users(id,email,password_hash,name,verified)VALUES($1,$2,$3,$4,true)`, []any{u.ID, u.Email, hash, u.Name}},
		{`INSERT INTO cloud_workspaces(id,owner_id,name)VALUES($1,$2,$3)`, []any{u.WorkspaceID, u.ID, u.Name + "'s workspace"}},
		{`INSERT INTO cloud_channels(id,user_id,handle,name)VALUES($1,$2,$3,$4)`, []any{u.ChannelID, u.ID, u.Handle, u.Name}},
		{`INSERT INTO cloud_credit_accounts(workspace_id)VALUES($1)`, []any{u.WorkspaceID}},
		{`INSERT INTO cloud_oauth_identities(provider,subject,user_id)VALUES($1,$2,$3)`, []any{provider, subject, u.ID}},
	}
	for _, q := range queries {
		if _, err = tx.ExecContext(ctx, q.q, q.args...); err != nil {
			return User{}, dbError(err)
		}
	}
	return u, tx.Commit()
}
func (s *Store) oauthSession(ctx context.Context, u User) (string, string, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", "", err
	}
	defer tx.Rollback()
	var id string
	if err = tx.QueryRowContext(ctx, `SELECT id FROM cloud_users WHERE id=$1 AND NOT EXISTS(SELECT 1 FROM cloud_user_restrictions WHERE user_id=$1 AND suspended) FOR UPDATE`, u.ID).Scan(&id); err != nil {
		return "", "", ErrUnauthorized
	}
	var required bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cloud_totp WHERE user_id=$1 AND enabled) OR EXISTS(SELECT 1 FROM cloud_passkeys WHERE user_id=$1)`, u.ID).Scan(&required); err != nil {
		return "", "", err
	}
	raw, err := rawToken()
	if err != nil {
		return "", "", err
	}
	if required {
		_, err = tx.ExecContext(ctx, `INSERT INTO cloud_auth_challenges(token_hash,user_id,purpose)VALUES($1,$2,'password_mfa')`, tokenHash(raw), u.ID)
	} else {
		_, err = tx.ExecContext(ctx, `INSERT INTO cloud_sessions(token_hash,user_id)VALUES($1,$2)`, tokenHash(raw), u.ID)
	}
	if err != nil {
		return "", "", err
	}
	if err = tx.Commit(); err != nil {
		return "", "", err
	}
	if required {
		return "", raw, nil
	}
	return raw, "", nil
}

func (s *Server) validOAuthLink(ctx context.Context, pending oauthPending) (bool, error) {
	if !pending.LinkUserID.Valid || !pending.LinkSessionHash.Valid {
		return false, nil
	}
	var valid bool
	err := s.Store.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cloud_sessions ss WHERE ss.token_hash=$1 AND ss.user_id=$2 AND ss.expires_at>now() AND ss.last_seen_at>now()-interval '7 days' AND NOT EXISTS(SELECT 1 FROM cloud_user_restrictions ur WHERE ur.user_id=ss.user_id AND ur.suspended))`, pending.LinkSessionHash.String, pending.LinkUserID.String).Scan(&valid)
	return valid, err
}
