package cloud

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"github.com/go-chi/chi/v5"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"net/http"
	"strings"
	"time"
)

var ErrMFARequired = errors.New("additional authentication required")

func authCipher(key string) (cipher.AEAD, error) {
	b, e := base64.StdEncoding.DecodeString(key)
	if e != nil || len(b) != 32 {
		return nil, ErrUnavailable
	}
	block, e := aes.NewCipher(b)
	if e != nil {
		return nil, e
	}
	return cipher.NewGCM(block)
}
func (s *Server) encryptTOTP(user, secret string) (string, error) {
	a, e := authCipher(s.Config.AuthEncryptionKey)
	if e != nil {
		return "", e
	}
	nonce := make([]byte, a.NonceSize())
	if _, e = rand.Read(nonce); e != nil {
		return "", e
	}
	return base64.RawStdEncoding.EncodeToString(a.Seal(nonce, nonce, []byte(secret), []byte(user))), nil
}
func (s *Server) decryptTOTP(user, encoded string) (string, error) {
	a, e := authCipher(s.Config.AuthEncryptionKey)
	if e != nil {
		return "", e
	}
	b, e := base64.RawStdEncoding.DecodeString(encoded)
	if e != nil || len(b) < a.NonceSize() {
		return "", ErrUnauthorized
	}
	plain, e := a.Open(nil, b[:a.NonceSize()], b[a.NonceSize():], []byte(user))
	if e != nil {
		return "", ErrUnauthorized
	}
	return string(plain), nil
}
func (s *Store) mfaRequired(ctx context.Context, user string) (bool, error) {
	var required bool
	e := s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cloud_totp WHERE user_id=$1 AND enabled) OR EXISTS(SELECT 1 FROM cloud_passkeys WHERE user_id=$1)`, user).Scan(&required)
	return required, e
}
func (s *Store) newSecuritySession(ctx context.Context, user string, passkey bool) (string, error) {
	token, e := rawToken()
	if e != nil {
		return "", e
	}
	_, e = s.DB.ExecContext(ctx, `INSERT INTO cloud_sessions(token_hash,user_id,passkey_verified_at)VALUES($1,$2,CASE WHEN $3 THEN now() ELSE NULL END)`, tokenHash(token), user, passkey)
	return token, e
}
func (s *Server) securityRoutes(r chi.Router) {
	r.Post("/auth/mfa", s.finishPasswordMFA)
	r.Post("/auth/passkeys/options", s.passkeyLoginOptions)
	r.Post("/auth/passkeys/verify", s.passkeyLoginVerify)
	r.Group(func(r chi.Router) {
		r.Use(s.requireUser, s.mutationLimits)
		r.Get("/security", s.securityStatus)
		r.Post("/security/totp/begin", s.totpBegin)
		r.Post("/security/totp/confirm", s.totpConfirm)
		r.Post("/security/totp/disable", s.totpDisable)
		r.Post("/security/passkeys/register/options", s.passkeyRegisterOptions)
		r.Post("/security/passkeys/register/verify", s.passkeyRegisterVerify)
		r.Delete("/security/passkeys/{id}", s.passkeyDelete)
	})
}
func (s *Server) securityStatus(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	var enabled bool
	var recovery int
	e := s.Store.DB.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM cloud_totp WHERE user_id=$1 AND enabled),(SELECT count(*) FROM cloud_recovery_codes WHERE user_id=$1)`, u.ID).Scan(&enabled, &recovery)
	if e != nil {
		s.fail(w, e)
		return
	}
	keys := []map[string]any{}
	rows, e := s.Store.DB.QueryContext(r.Context(), `SELECT id,name,created_at FROM cloud_passkeys WHERE user_id=$1 ORDER BY created_at`, u.ID)
	if e != nil {
		s.fail(w, e)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id, name string
		var created time.Time
		if e = rows.Scan(&id, &name, &created); e != nil {
			s.fail(w, e)
			return
		}
		keys = append(keys, map[string]any{"id": id, "name": name, "created_at": created})
	}
	if e = rows.Err(); e != nil {
		s.fail(w, e)
		return
	}
	s.json(w, 200, map[string]any{"totp_available": s.Config.AuthEncryptionKey != "", "totp_enabled": enabled, "mfa_required": enabled || len(keys) > 0, "recovery_codes_remaining": recovery, "passkeys": keys})
}
func (s *Server) checkFreshPassword(ctx context.Context, u User, password string) error {
	if len(password) > 1024 {
		return ErrUnauthorized
	}
	checked, e := s.Store.verifyPassword(ctx, u.Email, password)
	if e != nil || checked.ID != u.ID {
		return ErrUnauthorized
	}
	return nil
}

// Locked factor verification consumes a TOTP time step or recovery code atomically.
func (s *Server) verifyFactorTx(ctx context.Context, tx *sql.Tx, user, code string, allowRecovery bool) error {
	var encrypted string
	var last int64
	e := tx.QueryRowContext(ctx, `SELECT secret_ciphertext,last_step FROM cloud_totp WHERE user_id=$1 AND enabled FOR UPDATE`, user).Scan(&encrypted, &last)
	if e != nil {
		return ErrUnauthorized
	}
	secret, e := s.decryptTOTP(user, encrypted)
	if e != nil {
		return e
	}
	now := time.Now()
	for delta := -1; delta <= 1; delta++ {
		at := now.Add(time.Duration(delta) * 30 * time.Second)
		step := at.Unix() / 30
		if step <= last {
			continue
		}
		valid, _ := totp.ValidateCustom(code, secret, at, totp.ValidateOpts{Period: 30, Skew: 0, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1})
		if valid {
			_, e = tx.ExecContext(ctx, `UPDATE cloud_totp SET last_step=$2 WHERE user_id=$1`, user, step)
			return e
		}
	}
	if allowRecovery && len(code) >= 16 && len(code) <= 128 {
		res, e := tx.ExecContext(ctx, `DELETE FROM cloud_recovery_codes WHERE user_id=$1 AND code_hash=$2`, user, tokenHash(strings.ToUpper(strings.TrimSpace(code))))
		if e != nil {
			return e
		}
		n, _ := res.RowsAffected()
		if n == 1 {
			return nil
		}
	}
	return ErrUnauthorized
}
func (s *Server) reauth(ctx context.Context, r *http.Request, u User, password, code string, passkeySensitive bool) error {
	if e := s.checkFreshPassword(ctx, u, password); e != nil {
		return e
	}
	var totpEnabled, keys bool
	e := s.Store.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cloud_totp WHERE user_id=$1 AND enabled),EXISTS(SELECT 1 FROM cloud_passkeys WHERE user_id=$1)`, u.ID).Scan(&totpEnabled, &keys)
	if e != nil {
		return e
	}
	if keys {
		fresh := false
		cookie, cookieErr := r.Cookie("videoflow_session")
		if cookieErr == nil {
			e = s.Store.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cloud_sessions WHERE token_hash=$1 AND user_id=$2 AND passkey_verified_at>now()-interval '5 minutes')`, tokenHash(cookie.Value), u.ID).Scan(&fresh)
			if e != nil {
				return e
			}
		}
		// A recently verified UV passkey supplies the strong factor by itself.
		if fresh {
			return nil
		}
		if passkeySensitive || !totpEnabled {
			return ErrUnauthorized
		}
	}
	if totpEnabled {
		tx, e := s.Store.DB.BeginTx(ctx, nil)
		if e != nil {
			return e
		}
		defer tx.Rollback()
		if e = s.verifyFactorTx(ctx, tx, u.ID, code, false); e != nil {
			return e
		}
		return tx.Commit()
	}
	return nil
}
func (s *Server) totpBegin(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if !s.rate(w, r, "totp-begin:"+u.ID, 5, time.Hour, true) {
		return
	}
	var b struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if !s.decode(w, r, &b) {
		return
	}
	if _, e := authCipher(s.Config.AuthEncryptionKey); e != nil {
		s.fail(w, ErrUnavailable)
		return
	}
	if e := s.reauth(r.Context(), r, u, b.Password, b.Code, false); e != nil {
		s.fail(w, e)
		return
	}
	key, e := totp.Generate(totp.GenerateOpts{Issuer: "VideoFlow", AccountName: u.Email, SecretSize: 32})
	if e != nil {
		s.fail(w, e)
		return
	}
	encrypted, e := s.encryptTOTP(u.ID, key.Secret())
	if e != nil {
		s.fail(w, e)
		return
	}
	// Existing MFA is never overwritten by an unconfirmed enrollment.
	res, e := s.Store.DB.ExecContext(r.Context(), `INSERT INTO cloud_totp(user_id,secret_ciphertext)VALUES($1,$2) ON CONFLICT(user_id)DO UPDATE SET secret_ciphertext=$2,last_step=-1,pending_expires_at=now()+interval '10 minutes' WHERE NOT cloud_totp.enabled`, u.ID, encrypted)
	if e != nil {
		s.fail(w, e)
		return
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		s.fail(w, ErrConflict)
		return
	}
	s.json(w, 200, map[string]string{"secret": key.Secret(), "otpauth_url": key.URL()})
}
func (s *Server) totpConfirm(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if !s.rate(w, r, "totp-confirm:"+u.ID, 5, time.Minute, true) {
		return
	}
	var b struct {
		Code string `json:"code"`
	}
	if !s.decode(w, r, &b) {
		return
	}
	tx, e := s.Store.DB.BeginTx(r.Context(), nil)
	if e != nil {
		s.fail(w, e)
		return
	}
	defer tx.Rollback()
	var locked string
	if e = tx.QueryRowContext(r.Context(), `SELECT id FROM cloud_users WHERE id=$1 FOR UPDATE`, u.ID).Scan(&locked); e != nil {
		s.fail(w, e)
		return
	}
	var encrypted string
	e = tx.QueryRowContext(r.Context(), `SELECT secret_ciphertext FROM cloud_totp WHERE user_id=$1 AND NOT enabled AND pending_expires_at>now() FOR UPDATE`, u.ID).Scan(&encrypted)
	if e != nil {
		s.fail(w, ErrUnauthorized)
		return
	}
	secret, e := s.decryptTOTP(u.ID, encrypted)
	if e != nil {
		s.fail(w, e)
		return
	}
	matchedStep := int64(-1)
	now := time.Now()
	for delta := -1; delta <= 1; delta++ {
		at := now.Add(time.Duration(delta) * 30 * time.Second)
		valid, _ := totp.ValidateCustom(b.Code, secret, at, totp.ValidateOpts{Period: 30, Skew: 0, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1})
		if valid {
			matchedStep = at.Unix() / 30
			break
		}
	}
	if matchedStep < 0 {
		s.fail(w, ErrUnauthorized)
		return
	}
	codes := []string{}
	for i := 0; i < 10; i++ {
		raw, e := rawToken()
		if e != nil {
			s.fail(w, e)
			return
		}
		code := strings.ToUpper(raw[:20])
		codes = append(codes, code)
		if _, e = tx.ExecContext(r.Context(), `INSERT INTO cloud_recovery_codes(user_id,code_hash)VALUES($1,$2)`, u.ID, tokenHash(code)); e != nil {
			s.fail(w, e)
			return
		}
	}
	_, e = tx.ExecContext(r.Context(), `UPDATE cloud_totp SET enabled=true,last_step=$2 WHERE user_id=$1`, u.ID, matchedStep)
	if e == nil {
		_, e = tx.ExecContext(r.Context(), `DELETE FROM cloud_sessions WHERE user_id=$1`, u.ID)
	}
	if e == nil {
		_, e = tx.ExecContext(r.Context(), `DELETE FROM cloud_auth_challenges WHERE user_id=$1`, u.ID)
	}
	if e == nil {
		e = tx.Commit()
	}
	if e != nil {
		s.fail(w, e)
		return
	}
	token, e := s.Store.newSecuritySession(r.Context(), u.ID, false)
	if e != nil {
		s.fail(w, e)
		return
	}
	s.cookie(w, token, 30*24*3600)
	s.json(w, 200, map[string]any{"recovery_codes": codes})
}
func (s *Server) totpDisable(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if !s.rate(w, r, "totp-disable:"+u.ID, 5, time.Minute, true) {
		return
	}
	var b struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if !s.decode(w, r, &b) {
		return
	}
	if e := s.reauth(r.Context(), r, u, b.Password, b.Code, false); e != nil {
		s.fail(w, e)
		return
	}
	tx, e := s.Store.DB.BeginTx(r.Context(), nil)
	if e != nil {
		s.fail(w, e)
		return
	}
	defer tx.Rollback()
	for _, q := range []string{`DELETE FROM cloud_totp WHERE user_id=$1`, `DELETE FROM cloud_recovery_codes WHERE user_id=$1`, `DELETE FROM cloud_auth_challenges WHERE user_id=$1`, `DELETE FROM cloud_sessions WHERE user_id=$1`} {
		if _, e = tx.ExecContext(r.Context(), q, u.ID); e != nil {
			s.fail(w, e)
			return
		}
	}
	if e = tx.Commit(); e != nil {
		s.fail(w, e)
		return
	}
	token, e := s.Store.newSecuritySession(r.Context(), u.ID, false)
	if e != nil {
		s.fail(w, e)
		return
	}
	s.cookie(w, token, 30*24*3600)
	s.json(w, 200, map[string]bool{"ok": true})
}
func (s *Server) finishPasswordMFA(w http.ResponseWriter, r *http.Request) {
	if !s.rate(w, r, "mfa-ip:"+s.peer(r), 20, time.Minute, true) {
		return
	}
	var b struct {
		Challenge string `json:"challenge"`
		Code      string `json:"code"`
	}
	if !s.decode(w, r, &b) {
		return
	}
	if len(b.Challenge) != 43 || len(b.Code) > 128 {
		s.fail(w, ErrUnauthorized)
		return
	}
	if !s.rate(w, r, "mfa-challenge:"+tokenHash(b.Challenge), 5, time.Minute, true) {
		return
	}
	tx, e := s.Store.DB.BeginTx(r.Context(), nil)
	if e != nil {
		s.fail(w, e)
		return
	}
	defer tx.Rollback()
	var user string
	e = tx.QueryRowContext(r.Context(), `SELECT user_id FROM cloud_auth_challenges WHERE token_hash=$1 AND purpose='password_mfa' AND expires_at>now()`, tokenHash(b.Challenge)).Scan(&user)
	if e != nil {
		s.fail(w, ErrUnauthorized)
		return
	}
	var locked string
	if e = tx.QueryRowContext(r.Context(), `SELECT id FROM cloud_users WHERE id=$1 FOR UPDATE`, user).Scan(&locked); e != nil {
		s.fail(w, ErrUnauthorized)
		return
	}
	e = tx.QueryRowContext(r.Context(), `DELETE FROM cloud_auth_challenges WHERE token_hash=$1 AND purpose='password_mfa' AND expires_at>now() RETURNING user_id`, tokenHash(b.Challenge)).Scan(&user)
	if e != nil {
		s.fail(w, ErrUnauthorized)
		return
	}
	if e = s.verifyFactorTx(r.Context(), tx, user, b.Code, true); e != nil {
		s.fail(w, e)
		return
	}
	token, e := rawToken()
	if e == nil {
		_, e = tx.ExecContext(r.Context(), `INSERT INTO cloud_sessions(token_hash,user_id)VALUES($1,$2)`, tokenHash(token), user)
	}
	if e == nil {
		e = tx.Commit()
	}
	if e != nil {
		s.fail(w, e)
		return
	}
	u, e := scanUser(s.Store.DB.QueryRowContext(r.Context(), userSelect+" WHERE u.id=$1", user))
	if e != nil {
		s.fail(w, e)
		return
	}
	s.cookie(w, token, 30*24*3600)
	s.json(w, 200, u)
}
