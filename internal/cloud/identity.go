package cloud

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/argon2"
	"net/mail"
	"regexp"
	"strings"
	"time"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUnauthorized       = errors.New("unauthorized")
	ErrConflict           = errors.New("already exists")
	ErrValidation         = errors.New("invalid input")
)

type User struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	Name        string `json:"name"`
	WorkspaceID string `json:"workspace_id"`
	ChannelID   string `json:"channel_id"`
	Handle      string `json:"handle"`
	Verified    bool   `json:"verified"`
}

var handlePattern = regexp.MustCompile(`^[a-z0-9_]{3,40}$`)

func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, 3, 64*1024, 2, 32)
	return fmt.Sprintf("$argon2id$v=19$m=65536,t=3,p=2$%s$%s", base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash)), nil
}
func checkPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false
	}
	var memory, iterations uint32
	var parallel uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallel); err != nil || memory < 8192 || memory > 262144 || iterations < 1 || iterations > 10 || parallel < 1 || parallel > 8 {
		return false
	}
	salt, e := base64.RawStdEncoding.DecodeString(parts[4])
	if e != nil || len(salt) < 16 {
		return false
	}
	expected, e := base64.RawStdEncoding.DecodeString(parts[5])
	if e != nil || len(expected) != 32 {
		return false
	}
	actual := argon2.IDKey([]byte(password), salt, iterations, memory, parallel, 32)
	return subtle.ConstantTimeCompare(actual, expected) == 1
}
func dbError(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		return ErrConflict
	}
	return err
}

const userSelect = `SELECT u.id,u.email,u.name,w.id,c.id,c.handle,u.verified FROM cloud_users u JOIN cloud_workspaces w ON w.owner_id=u.id JOIN cloud_channels c ON c.user_id=u.id `

func scanUser(row interface{ Scan(...any) error }) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Email, &u.Name, &u.WorkspaceID, &u.ChannelID, &u.Handle, &u.Verified)
	return u, err
}
func (s *Store) Register(ctx context.Context, email, password, name, handle string) (User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	name = strings.TrimSpace(name)
	handle = strings.ToLower(strings.TrimSpace(handle))
	a, e := mail.ParseAddress(email)
	if e != nil || a.Address != email || len(email) > 254 || len(password) < 12 || len(password) > 1024 || name == "" || len(name) > 100 || !handlePattern.MatchString(handle) {
		return User{}, ErrValidation
	}
	hash, err := hashPassword(password)
	if err != nil {
		return User{}, err
	}
	u := User{ID: uuid.NewString(), Email: email, Name: name, WorkspaceID: uuid.NewString(), ChannelID: uuid.NewString(), Handle: handle}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback()
	queries := []struct {
		q    string
		args []any
	}{
		{"INSERT INTO cloud_users(id,email,password_hash,name) VALUES($1,$2,$3,$4)", []any{u.ID, email, hash, name}},
		{"INSERT INTO cloud_workspaces(id,owner_id,name) VALUES($1,$2,$3)", []any{u.WorkspaceID, u.ID, name + "'s workspace"}},
		{"INSERT INTO cloud_channels(id,user_id,handle,name) VALUES($1,$2,$3,$4)", []any{u.ChannelID, u.ID, handle, name}},
		{"INSERT INTO cloud_credit_accounts(workspace_id) VALUES($1)", []any{u.WorkspaceID}}}
	for _, q := range queries {
		if _, err = tx.ExecContext(ctx, q.q, q.args...); err != nil {
			return User{}, dbError(err)
		}
	}
	return u, tx.Commit()
}
func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
func rawToken() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b), err
}
func (s *Store) verifyPassword(ctx context.Context, email, password string) (User, error) {
	var id, hash string
	err := s.DB.QueryRowContext(ctx, "SELECT id,password_hash FROM cloud_users WHERE email=$1", strings.ToLower(strings.TrimSpace(email))).Scan(&id, &hash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, ErrInvalidCredentials
		}
		return User{}, err
	}
	if !checkPassword(hash, password) {
		return User{}, ErrInvalidCredentials
	}
	return scanUser(s.DB.QueryRowContext(ctx, userSelect+" WHERE u.id=$1", id))
}
func (s *Store) Login(ctx context.Context, email, password string) (User, string, error) {
	u, token, challenge, e := s.beginSecurityLogin(ctx, email, password)
	if e != nil {
		return User{}, "", e
	}
	if challenge != "" {
		return User{}, "", ErrMFARequired
	}
	return u, token, nil
}
func (s *Store) beginSecurityLogin(ctx context.Context, email, password string) (User, string, string, error) {
	if len(password) > 1024 {
		return User{}, "", "", ErrInvalidCredentials
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return User{}, "", "", e
	}
	defer tx.Rollback()
	var id, hash string
	e = tx.QueryRowContext(ctx, `SELECT id,password_hash FROM cloud_users WHERE email=$1 FOR UPDATE`, strings.ToLower(strings.TrimSpace(email))).Scan(&id, &hash)
	if e != nil {
		if errors.Is(e, sql.ErrNoRows) {
			return User{}, "", "", ErrInvalidCredentials
		}
		return User{}, "", "", e
	}
	if !checkPassword(hash, password) {
		return User{}, "", "", ErrInvalidCredentials
	}
	u, e := scanUser(tx.QueryRowContext(ctx, userSelect+" WHERE u.id=$1", id))
	if e != nil {
		return User{}, "", "", e
	}
	var required bool
	e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cloud_totp WHERE user_id=$1 AND enabled) OR EXISTS(SELECT 1 FROM cloud_passkeys WHERE user_id=$1)`, id).Scan(&required)
	if e != nil {
		return User{}, "", "", e
	}
	raw, e := rawToken()
	if e != nil {
		return User{}, "", "", e
	}
	if required {
		_, e = tx.ExecContext(ctx, `INSERT INTO cloud_auth_challenges(token_hash,user_id,purpose)VALUES($1,$2,'password_mfa')`, tokenHash(raw), id)
	} else {
		_, e = tx.ExecContext(ctx, `INSERT INTO cloud_sessions(token_hash,user_id)VALUES($1,$2)`, tokenHash(raw), id)
	}
	if e != nil {
		return User{}, "", "", e
	}
	if e = tx.Commit(); e != nil {
		return User{}, "", "", e
	}
	if required {
		return u, "", raw, nil
	}
	return u, raw, "", nil
}
func (s *Store) Authenticate(ctx context.Context, token string) (User, error) {
	if len(token) != 43 {
		return User{}, ErrUnauthorized
	}
	var id string
	err := s.DB.QueryRowContext(ctx, `UPDATE cloud_sessions SET last_seen_at=now() WHERE token_hash=$1 AND expires_at>now() AND last_seen_at>now()-interval '7 days' RETURNING user_id`, tokenHash(token)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrUnauthorized
	}
	if err != nil {
		return User{}, err
	}
	return scanUser(s.DB.QueryRowContext(ctx, userSelect+" WHERE u.id=$1", id))
}
func (s *Store) Logout(ctx context.Context, token string) error {
	_, err := s.DB.ExecContext(ctx, "DELETE FROM cloud_sessions WHERE token_hash=$1", tokenHash(token))
	return err
}

// IssueAccountToken returns a single-use token for delivery through trusted email infrastructure.
func (s *Store) IssueAccountToken(ctx context.Context, email, purpose string) (string, error) {
	if purpose != "verify_email" && purpose != "reset_password" {
		return "", ErrValidation
	}
	token, err := rawToken()
	if err != nil {
		return "", err
	}
	ttl := time.Hour
	if purpose == "verify_email" {
		ttl = 24 * time.Hour
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var id string
	if err = tx.QueryRowContext(ctx, "SELECT id FROM cloud_users WHERE email=$1 FOR UPDATE", strings.ToLower(strings.TrimSpace(email))).Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM cloud_tokens WHERE user_id=$1 AND purpose=$2", id, purpose); err != nil {
		return "", err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO cloud_tokens(token_hash,user_id,purpose,expires_at) VALUES($1,$2,$3,$4)", tokenHash(token), id, purpose, time.Now().Add(ttl)); err != nil {
		return "", err
	}
	return token, tx.Commit()
}
func (s *Store) VerifyEmail(ctx context.Context, token string) error {
	return s.consumeAccountToken(ctx, token, "verify_email", "")
}
func (s *Store) ResetPassword(ctx context.Context, token, password string) error {
	if len(password) < 12 || len(password) > 1024 {
		return ErrValidation
	}
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	return s.consumeAccountToken(ctx, token, "reset_password", hash)
}
func (s *Store) consumeAccountToken(ctx context.Context, token, purpose, hash string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var id string
	err = tx.QueryRowContext(ctx, "DELETE FROM cloud_tokens WHERE token_hash=$1 AND purpose=$2 AND expires_at>now() RETURNING user_id", tokenHash(token), purpose).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrUnauthorized
	}
	if err != nil {
		return err
	}
	if purpose == "verify_email" {
		_, err = tx.ExecContext(ctx, "UPDATE cloud_users SET verified=true WHERE id=$1", id)
	} else {
		_, err = tx.ExecContext(ctx, "UPDATE cloud_users SET password_hash=$2 WHERE id=$1", id, hash)
		if err == nil {
			_, err = tx.ExecContext(ctx, "DELETE FROM cloud_sessions WHERE user_id=$1", id)
			if err == nil {
				_, err = tx.ExecContext(ctx, "DELETE FROM cloud_auth_challenges WHERE user_id=$1", id)
			}
		}
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}
