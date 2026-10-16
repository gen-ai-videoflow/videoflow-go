package cloud

import (
	"context"
	"database/sql"
	"errors"
	"github.com/go-chi/chi/v5"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

var magicMailSlots = make(chan struct{}, 8)

func (s *Server) magicLoginRoutes(r chi.Router) {
	r.Post("/auth/magic-link", s.requestMagicLogin)
	r.Post("/auth/magic-link/verify", s.verifyMagicLogin)
}
func (s *Server) requestMagicLogin(w http.ResponseWriter, r *http.Request) {
	if !s.rate(w, r, "magic-ip:"+s.peer(r), 5, time.Hour, true) {
		return
	}
	var in struct {
		Email string `json:"email"`
	}
	if !s.decode(w, r, &in) {
		return
	}
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	if !s.rate(w, r, "magic-email:"+tokenHash(in.Email), 3, time.Hour, true) {
		return
	}
	if !s.Config.DevMode && s.Config.MailFrom == "" {
		s.fail(w, ErrUnavailable)
		return
	}
	slotOwned := false
	if !s.Config.DevMode {
		select {
		case magicMailSlots <- struct{}{}:
			slotOwned = true
		default:
			s.fail(w, ErrUnavailable)
			return
		}
	}
	defer func() {
		if slotOwned {
			<-magicMailSlots
		}
	}()
	token, err := s.Store.IssueAccountToken(r.Context(), in.Email, "magic_login")
	if err != nil && !errors.Is(err, ErrNotFound) {
		s.fail(w, err)
		return
	}
	result := map[string]any{"message": "If this address has an account, a sign-in link will arrive shortly. It expires in 15 minutes."}
	if s.Config.DevMode {
		result["development_token"] = token
	} else if token != "" {
		// Delivery latency must not reveal whether an account exists. Bound the
		// detached send independently of the HTTP request; no secret is logged.
		slotOwned = false
		go func(email, raw string) {
			defer func() { <-magicMailSlots }()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if sendErr := s.sendAccountEmail(ctx, email, raw, "magic_login"); sendErr != nil {
				slog.Error("magic sign-in delivery failed", "error", sendErr)
			}
		}(in.Email, token)
	}
	s.json(w, 200, result)
}
func (s *Store) consumeMagicLogin(ctx context.Context, token string) (User, string, string, error) {
	if len(token) != 43 {
		return User{}, "", "", ErrUnauthorized
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return User{}, "", "", err
	}
	defer tx.Rollback()
	var id string
	err = tx.QueryRowContext(ctx, `SELECT user_id FROM cloud_tokens WHERE token_hash=$1 AND purpose='magic_login' AND expires_at>now()`, tokenHash(token)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, "", "", ErrUnauthorized
	}
	if err != nil {
		return User{}, "", "", err
	}
	var lock string
	if err = tx.QueryRowContext(ctx, `SELECT id FROM cloud_users WHERE id=$1 FOR UPDATE`, id).Scan(&lock); err != nil {
		return User{}, "", "", err
	}
	err = tx.QueryRowContext(ctx, `DELETE FROM cloud_tokens WHERE token_hash=$1 AND purpose='magic_login' AND expires_at>now() RETURNING user_id`, tokenHash(token)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, "", "", ErrUnauthorized
	}
	if err != nil {
		return User{}, "", "", err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE cloud_users SET verified=true WHERE id=$1`, id); err != nil {
		return User{}, "", "", err
	}
	u, err := scanUser(tx.QueryRowContext(ctx, userSelect+" WHERE u.id=$1", id))
	if err != nil {
		return User{}, "", "", err
	}
	var required bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cloud_totp WHERE user_id=$1 AND enabled) OR EXISTS(SELECT 1 FROM cloud_passkeys WHERE user_id=$1)`, id).Scan(&required)
	if err != nil {
		return User{}, "", "", err
	}
	raw, err := rawToken()
	if err != nil {
		return User{}, "", "", err
	}
	if required {
		_, err = tx.ExecContext(ctx, `INSERT INTO cloud_auth_challenges(token_hash,user_id,purpose)VALUES($1,$2,'password_mfa')`, tokenHash(raw), id)
	} else {
		_, err = tx.ExecContext(ctx, `INSERT INTO cloud_sessions(token_hash,user_id)VALUES($1,$2)`, tokenHash(raw), id)
	}
	if err != nil {
		return User{}, "", "", err
	}
	if err = tx.Commit(); err != nil {
		return User{}, "", "", err
	}
	if required {
		return u, "", raw, nil
	}
	return u, raw, "", nil
}
func (s *Server) verifyMagicLogin(w http.ResponseWriter, r *http.Request) {
	if !s.rate(w, r, "magic-verify:"+s.peer(r), 10, 15*time.Minute, true) {
		return
	}
	var in struct {
		Token string `json:"token"`
	}
	if !s.decode(w, r, &in) {
		return
	}
	u, token, challenge, err := s.Store.consumeMagicLogin(r.Context(), in.Token)
	if err != nil {
		s.fail(w, err)
		return
	}
	if challenge != "" {
		methods := []string{}
		var totp, keys bool
		if err = s.Store.DB.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM cloud_totp WHERE user_id=$1 AND enabled),EXISTS(SELECT 1 FROM cloud_passkeys WHERE user_id=$1)`, u.ID).Scan(&totp, &keys); err != nil {
			s.fail(w, err)
			return
		}
		if totp {
			methods = append(methods, "totp", "recovery")
		}
		if keys {
			methods = append(methods, "passkey")
		}
		s.json(w, 202, map[string]any{"mfa_required": true, "challenge": challenge, "methods": methods})
		return
	}
	s.cookie(w, token, 30*24*3600)
	s.json(w, 200, u)
}
