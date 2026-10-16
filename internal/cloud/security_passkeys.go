package cloud

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/go-chi/chi/v5"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type passkeyUser struct {
	User
	Credentials []webauthn.Credential
}

func (u passkeyUser) WebAuthnID() []byte                         { return []byte(u.ID) }
func (u passkeyUser) WebAuthnName() string                       { return u.Email }
func (u passkeyUser) WebAuthnDisplayName() string                { return u.Name }
func (u passkeyUser) WebAuthnCredentials() []webauthn.Credential { return u.Credentials }
func (s *Server) webAuthn() (*webauthn.WebAuthn, error) {
	if validateSessionConfig(s.Config) != nil {
		return nil, ErrUnavailable
	}
	u, e := url.Parse(s.Config.PublicOrigin)
	if e != nil || u.Hostname() == "" || u.Scheme != "https" && !(s.Config.DevMode && u.Hostname() == "localhost") {
		return nil, ErrUnavailable
	}
	return webauthn.New(&webauthn.Config{RPID: u.Hostname(), RPDisplayName: "6eek", RPOrigins: append([]string{s.Config.PublicOrigin}, s.Config.BrowserOrigins...), AuthenticatorSelection: protocol.AuthenticatorSelection{UserVerification: protocol.VerificationRequired, ResidentKey: protocol.ResidentKeyRequirementRequired}})
}
func (s *Server) loadPasskeyUser(ctx context.Context, user User) (passkeyUser, error) {
	u := passkeyUser{User: user, Credentials: []webauthn.Credential{}}
	rows, e := s.Store.DB.QueryContext(ctx, `SELECT credential FROM cloud_passkeys WHERE user_id=$1`, user.ID)
	if e != nil {
		return u, e
	}
	defer rows.Close()
	for rows.Next() {
		var b []byte
		var cred webauthn.Credential
		if e = rows.Scan(&b); e != nil {
			return u, e
		}
		if e = json.Unmarshal(b, &cred); e != nil {
			return u, e
		}
		u.Credentials = append(u.Credentials, cred)
	}
	return u, rows.Err()
}
func (s *Server) savePasskeyChallenge(ctx context.Context, user any, purpose string, session *webauthn.SessionData) (string, error) {
	raw, e := rawToken()
	if e != nil {
		return "", e
	}
	b, e := json.Marshal(session)
	if e != nil {
		return "", e
	}
	_, e = s.Store.DB.ExecContext(ctx, `INSERT INTO cloud_auth_challenges(token_hash,user_id,purpose,session_data)VALUES($1,$2,$3,$4)`, tokenHash(raw), user, purpose, b)
	return raw, e
}
func (s *Server) consumePasskeyChallenge(ctx context.Context, raw, purpose, user string) (webauthn.SessionData, error) {
	var data webauthn.SessionData
	if len(raw) != 43 {
		return data, ErrUnauthorized
	}
	var b []byte
	e := s.Store.DB.QueryRowContext(ctx, `DELETE FROM cloud_auth_challenges WHERE token_hash=$1 AND purpose=$2 AND expires_at>now() AND (user_id=$3 OR ($3='' AND user_id IS NULL)) RETURNING session_data`, tokenHash(raw), purpose, user).Scan(&b)
	if e != nil {
		return data, ErrUnauthorized
	}
	if json.Unmarshal(b, &data) != nil {
		return data, ErrUnauthorized
	}
	return data, nil
}
func (s *Server) passkeyRegisterOptions(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if !s.rate(w, r, "passkey-register:"+u.ID, 5, time.Minute, true) {
		return
	}
	var b struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if !s.decode(w, r, &b) {
		return
	}
	if e := s.reauth(r.Context(), r, u, b.Password, b.Code, true); e != nil {
		s.fail(w, e)
		return
	}
	wa, e := s.webAuthn()
	if e != nil {
		s.fail(w, e)
		return
	}
	user, e := s.loadPasskeyUser(r.Context(), u)
	if e != nil {
		s.fail(w, e)
		return
	}
	if len(user.Credentials) >= 10 {
		s.fail(w, ErrConflict)
		return
	}
	options, session, e := wa.BeginRegistration(user, webauthn.WithExclusions(webauthn.Credentials(user.Credentials).CredentialDescriptors()))
	if e != nil {
		s.fail(w, ErrUnavailable)
		return
	}
	challenge, e := s.savePasskeyChallenge(r.Context(), u.ID, "passkey_register", session)
	if e != nil {
		s.fail(w, e)
		return
	}
	s.json(w, 200, map[string]any{"challenge": challenge, "options": options.Response})
}
func (s *Server) passkeyRegisterVerify(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if !s.rate(w, r, "passkey-register-verify:"+u.ID, 5, time.Minute, true) {
		return
	}
	var b struct {
		Challenge  string          `json:"challenge"`
		Credential json.RawMessage `json:"credential"`
		Name       string          `json:"name"`
	}
	if !s.decode(w, r, &b) {
		return
	}
	b.Name = strings.TrimSpace(b.Name)
	if b.Name == "" || len(b.Name) > 80 {
		s.fail(w, ErrValidation)
		return
	}
	session, e := s.consumePasskeyChallenge(r.Context(), b.Challenge, "passkey_register", u.ID)
	if e != nil {
		s.fail(w, e)
		return
	}
	wa, e := s.webAuthn()
	if e != nil {
		s.fail(w, e)
		return
	}
	user, e := s.loadPasskeyUser(r.Context(), u)
	if e != nil {
		s.fail(w, e)
		return
	}
	parsed, e := protocol.ParseCredentialCreationResponseBody(bytes.NewReader(b.Credential))
	if e != nil {
		s.fail(w, ErrUnauthorized)
		return
	}
	cred, e := wa.CreateCredential(user, session, parsed)
	if e != nil || !cred.Flags.UserVerified {
		s.fail(w, ErrUnauthorized)
		return
	}
	data, e := json.Marshal(cred)
	if e != nil {
		s.fail(w, e)
		return
	}
	tx, e := s.Store.DB.BeginTx(r.Context(), nil)
	if e != nil {
		s.fail(w, e)
		return
	}
	defer tx.Rollback()
	var id string
	e = tx.QueryRowContext(r.Context(), `SELECT id FROM cloud_users WHERE id=$1 FOR UPDATE`, u.ID).Scan(&id)
	if e != nil {
		s.fail(w, e)
		return
	}
	_, e = tx.ExecContext(r.Context(), `INSERT INTO cloud_passkeys(id,user_id,credential,name)VALUES($1,$2,$3,$4)`, base64.RawURLEncoding.EncodeToString(cred.ID), u.ID, data, b.Name)
	if e == nil {
		_, e = tx.ExecContext(r.Context(), `DELETE FROM cloud_sessions WHERE user_id=$1`, u.ID)
	}
	if e == nil {
		_, e = tx.ExecContext(r.Context(), `DELETE FROM cloud_auth_challenges WHERE user_id=$1`, u.ID)
	}
	token, tokenErr := rawToken()
	if e == nil {
		e = tokenErr
	}
	if e == nil {
		_, e = tx.ExecContext(r.Context(), `INSERT INTO cloud_sessions(token_hash,user_id,passkey_verified_at,mfa_verified_at)VALUES($1,$2,now(),now())`, tokenHash(token), u.ID)
	}
	if e == nil {
		e = tx.Commit()
	}
	if e != nil {
		s.fail(w, dbError(e))
		return
	}
	s.cookie(w, token, 30*24*3600)
	s.json(w, 200, map[string]bool{"ok": true})
}
func (s *Server) passkeyLoginOptions(w http.ResponseWriter, r *http.Request) {
	var b struct{}
	if !s.decode(w, r, &b) {
		return
	}
	if !s.rate(w, r, "passkey-options:"+s.peer(r), 20, time.Minute, true) {
		return
	}
	wa, e := s.webAuthn()
	if e != nil {
		s.fail(w, e)
		return
	}
	options, session, e := wa.BeginDiscoverableLogin(webauthn.WithUserVerification(protocol.VerificationRequired))
	if e != nil {
		s.fail(w, ErrUnavailable)
		return
	}
	challenge, e := s.savePasskeyChallenge(r.Context(), nil, "passkey_login", session)
	if e != nil {
		s.fail(w, e)
		return
	}
	s.json(w, 200, map[string]any{"challenge": challenge, "options": options.Response})
}
func (s *Server) passkeyLoginVerify(w http.ResponseWriter, r *http.Request) {
	if !s.rate(w, r, "passkey-verify:"+s.peer(r), 20, time.Minute, true) {
		return
	}
	var b struct {
		Challenge  string          `json:"challenge"`
		Credential json.RawMessage `json:"credential"`
	}
	if !s.decode(w, r, &b) {
		return
	}
	session, e := s.consumePasskeyChallenge(r.Context(), b.Challenge, "passkey_login", "")
	if e != nil {
		s.fail(w, e)
		return
	}
	parsed, e := protocol.ParseCredentialRequestResponseBody(bytes.NewReader(b.Credential))
	if e != nil {
		s.fail(w, ErrUnauthorized)
		return
	}
	wa, e := s.webAuthn()
	if e != nil {
		s.fail(w, e)
		return
	}
	tx, e := s.Store.DB.BeginTx(r.Context(), nil)
	if e != nil {
		s.fail(w, e)
		return
	}
	defer tx.Rollback()
	var owner string
	var stored []byte
	id := base64.RawURLEncoding.EncodeToString(parsed.RawID)
	e = tx.QueryRowContext(r.Context(), `SELECT user_id,credential FROM cloud_passkeys WHERE id=$1 FOR UPDATE`, id).Scan(&owner, &stored)
	if e != nil {
		s.fail(w, ErrUnauthorized)
		return
	}
	var old webauthn.Credential
	if json.Unmarshal(stored, &old) != nil {
		s.fail(w, ErrUnauthorized)
		return
	}
	u, e := scanUser(tx.QueryRowContext(r.Context(), userSelect+" WHERE u.id=$1", owner))
	if e != nil {
		s.fail(w, ErrUnauthorized)
		return
	}
	cred, e := wa.ValidateDiscoverableLogin(func(rawID, userHandle []byte) (webauthn.User, error) {
		if !bytes.Equal(rawID, old.ID) || !bytes.Equal(userHandle, []byte(owner)) {
			return nil, ErrUnauthorized
		}
		return passkeyUser{User: u, Credentials: []webauthn.Credential{old}}, nil
	}, session, parsed)
	if e != nil || cred.Authenticator.CloneWarning || !cred.Flags.UserVerified {
		s.fail(w, ErrUnauthorized)
		return
	}
	data, e := json.Marshal(cred)
	if e != nil {
		s.fail(w, e)
		return
	}
	_, e = tx.ExecContext(r.Context(), `UPDATE cloud_passkeys SET credential=$2 WHERE id=$1`, id, data)
	token, tokenErr := rawToken()
	if e == nil {
		e = tokenErr
	}
	if e == nil {
		_, e = tx.ExecContext(r.Context(), `INSERT INTO cloud_sessions(token_hash,user_id,passkey_verified_at,mfa_verified_at)VALUES($1,$2,now(),now())`, tokenHash(token), u.ID)
	}
	if e == nil {
		e = tx.Commit()
	}
	if e != nil {
		s.fail(w, e)
		return
	}
	s.cookie(w, token, 30*24*3600)
	s.json(w, 200, u)
}
func (s *Server) passkeyDelete(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if !s.rate(w, r, "passkey-delete:"+u.ID, 5, time.Minute, true) {
		return
	}
	var b struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if !s.decode(w, r, &b) {
		return
	}
	if e := s.reauth(r.Context(), r, u, b.Password, b.Code, true); e != nil {
		s.fail(w, e)
		return
	}
	tx, e := s.Store.DB.BeginTx(r.Context(), nil)
	if e != nil {
		s.fail(w, e)
		return
	}
	defer tx.Rollback()
	res, e := tx.ExecContext(r.Context(), `DELETE FROM cloud_passkeys WHERE id=$1 AND user_id=$2`, chi.URLParam(r, "id"), u.ID)
	if e != nil {
		s.fail(w, e)
		return
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		s.fail(w, ErrNotFound)
		return
	}
	for _, q := range []string{`DELETE FROM cloud_sessions WHERE user_id=$1`, `DELETE FROM cloud_auth_challenges WHERE user_id=$1`} {
		if _, e = tx.ExecContext(r.Context(), q, u.ID); e != nil {
			s.fail(w, e)
			return
		}
	}
	token, e := rawToken()
	if e == nil {
		_, e = tx.ExecContext(r.Context(), `INSERT INTO cloud_sessions(token_hash,user_id,passkey_verified_at,mfa_verified_at)VALUES($1,$2,now(),now())`, tokenHash(token), u.ID)
	}
	if e == nil {
		e = tx.Commit()
	}
	if e != nil {
		s.fail(w, e)
		return
	}
	s.cookie(w, token, 30*24*3600)
	s.json(w, 200, map[string]bool{"ok": true})
}
