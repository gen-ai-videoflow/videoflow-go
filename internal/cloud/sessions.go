package cloud

import (
	"github.com/go-chi/chi/v5"
	"net/http"
	"time"
)

type accountSession struct {
	ID         string    `json:"id"`
	CreatedAt  time.Time `json:"created_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	Current    bool      `json:"current"`
}

func (s *Server) listSessions(w http.ResponseWriter, r *http.Request) {
	cookie, e := sessionCookie(r)
	if e != nil {
		s.fail(w, ErrUnauthorized)
		return
	}
	rows, e := s.Store.DB.QueryContext(r.Context(), `SELECT id,created_at,last_seen_at,expires_at,token_hash=$2 FROM cloud_sessions WHERE user_id=$1 AND expires_at>now() AND last_seen_at>now()-interval '7 days' ORDER BY (token_hash=$2) DESC,last_seen_at DESC`, currentUser(r).ID, tokenHash(cookie.Value))
	if e != nil {
		s.fail(w, e)
		return
	}
	defer rows.Close()
	result := []accountSession{}
	for rows.Next() {
		var v accountSession
		if e = rows.Scan(&v.ID, &v.CreatedAt, &v.LastSeenAt, &v.ExpiresAt, &v.Current); e != nil {
			s.fail(w, e)
			return
		}
		result = append(result, v)
	}
	s.result(w, result, rows.Err())
}
func (s *Server) revokeSession(w http.ResponseWriter, r *http.Request) {
	cookie, e := sessionCookie(r)
	if e != nil {
		s.fail(w, ErrUnauthorized)
		return
	}
	// Never let this control sign out the session authorizing the operation.
	result, e := s.Store.DB.ExecContext(r.Context(), `DELETE FROM cloud_sessions WHERE id=$1 AND user_id=$2 AND token_hash<>$3`, chi.URLParam(r, "id"), currentUser(r).ID, tokenHash(cookie.Value))
	if e != nil {
		s.fail(w, e)
		return
	}
	n, e := result.RowsAffected()
	if e != nil {
		s.fail(w, e)
		return
	}
	if n == 0 {
		var current bool
		e = s.Store.DB.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM cloud_sessions WHERE id=$1 AND user_id=$2 AND token_hash=$3)`, chi.URLParam(r, "id"), currentUser(r).ID, tokenHash(cookie.Value)).Scan(&current)
		if e != nil {
			s.fail(w, e)
			return
		}
		if current {
			s.fail(w, ErrConflict)
		} else {
			s.fail(w, ErrNotFound)
		}
		return
	}
	s.json(w, 200, map[string]bool{"ok": true})
}
func (s *Server) revokeOtherSessions(w http.ResponseWriter, r *http.Request) {
	cookie, e := sessionCookie(r)
	if e != nil {
		s.fail(w, ErrUnauthorized)
		return
	}
	// Clear pending account-bound authentication challenges too, so a password
	// challenge created before revocation cannot establish another session later.
	tx, e := s.Store.DB.BeginTx(r.Context(), nil)
	if e != nil {
		s.fail(w, e)
		return
	}
	defer tx.Rollback()
	var locked string
	e = tx.QueryRowContext(r.Context(), `SELECT id FROM cloud_users WHERE id=$1 FOR UPDATE`, currentUser(r).ID).Scan(&locked)
	if e != nil {
		s.fail(w, e)
		return
	}
	_, e = tx.ExecContext(r.Context(), `DELETE FROM cloud_sessions WHERE user_id=$1 AND token_hash<>$2`, currentUser(r).ID, tokenHash(cookie.Value))
	if e == nil {
		_, e = tx.ExecContext(r.Context(), `DELETE FROM cloud_auth_challenges WHERE user_id=$1`, currentUser(r).ID)
	}
	if e == nil {
		e = tx.Commit()
	}
	s.result(w, map[string]bool{"ok": true}, e)
}
