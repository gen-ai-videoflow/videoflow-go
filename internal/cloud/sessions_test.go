package cloud

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestSessionManagementOwnershipAndRevocation(t *testing.T) {
	s, u, current := securityTestServer(t)
	ctx := context.Background()
	other, e := s.Store.newSecuritySession(ctx, u.ID, false)
	if e != nil {
		t.Fatal(e)
	}
	expired, e := s.Store.newSecuritySession(ctx, u.ID, false)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Store.DB.ExecContext(ctx, `UPDATE cloud_sessions SET expires_at=now()-interval '1 second' WHERE token_hash=$1`, tokenHash(expired)); e != nil {
		t.Fatal(e)
	}
	idle, e := s.Store.newSecuritySession(ctx, u.ID, false)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Store.DB.ExecContext(ctx, `UPDATE cloud_sessions SET last_seen_at=now()-interval '8 days' WHERE token_hash=$1`, tokenHash(idle)); e != nil {
		t.Fatal(e)
	}
	outsider, e := s.Store.Register(ctx, "sessions-outsider-"+u.ID+"@example.test", "Test-password-123!", "Outsider", "out_"+u.ID[:8])
	if e != nil {
		t.Fatal(e)
	}
	foreign, e := s.Store.newSecuritySession(ctx, outsider.ID, false)
	if e != nil {
		t.Fatal(e)
	}
	if w := securityRequest(s, "GET", "/security/sessions", "", nil); w.Code != 401 {
		t.Fatalf("guest %d", w.Code)
	}
	w := securityRequest(s, "GET", "/security/sessions", current, nil)
	if w.Code != 200 {
		t.Fatalf("list %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), current) || strings.Contains(w.Body.String(), tokenHash(current)) {
		t.Fatal("session credential leak")
	}
	var rows []struct {
		ID      string `json:"id"`
		Current bool   `json:"current"`
	}
	json.Unmarshal(w.Body.Bytes(), &rows)
	if len(rows) != 2 {
		t.Fatalf("sessions: %s", w.Body.String())
	}
	var target, currentID string
	for _, v := range rows {
		if v.Current {
			currentID = v.ID
		} else {
			target = v.ID
		}
	}
	if target == "" || currentID == "" {
		t.Fatal("current indication missing")
	}
	var foreignID string
	s.Store.DB.QueryRowContext(ctx, `SELECT id FROM cloud_sessions WHERE token_hash=$1`, tokenHash(foreign)).Scan(&foreignID)
	if w = securityRequest(s, "DELETE", "/security/sessions/"+foreignID, current, nil); w.Code != 404 {
		t.Fatalf("foreign revoke %d", w.Code)
	}
	if _, e = s.Store.Authenticate(ctx, foreign); e != nil {
		t.Fatal("foreign session revoked")
	}
	if w = securityRequest(s, "DELETE", "/security/sessions/"+currentID, current, nil); w.Code != 409 {
		t.Fatalf("current revoke %d", w.Code)
	}
	if w = securityRequest(s, "DELETE", "/security/sessions/"+target, current, nil); w.Code != 200 {
		t.Fatalf("revoke %d %s", w.Code, w.Body.String())
	}
	if _, e = s.Store.Authenticate(ctx, other); e == nil {
		t.Fatal("revoked session valid")
	}
	other, e = s.Store.newSecuritySession(ctx, u.ID, false)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Store.DB.ExecContext(ctx, `INSERT INTO cloud_auth_challenges(token_hash,user_id,purpose)VALUES($1,$2,'password_mfa')`, "session-test-"+u.ID, u.ID); e != nil {
		t.Fatal(e)
	}
	if w = securityRequest(s, "POST", "/security/sessions/revoke-others", current, map[string]string{}); w.Code != 200 {
		t.Fatalf("revoke others %d", w.Code)
	}
	var challenges int
	if e = s.Store.DB.QueryRowContext(ctx, `SELECT count(*) FROM cloud_auth_challenges WHERE user_id=$1`, u.ID).Scan(&challenges); e != nil || challenges != 0 {
		t.Fatalf("pending challenges remain: %d %v", challenges, e)
	}

	if _, e = s.Store.Authenticate(ctx, other); e == nil {
		t.Fatal("other session survived")
	}
	if _, e = s.Store.Authenticate(ctx, current); e != nil {
		t.Fatal("current lost")
	}
}

func TestSessionMaintenanceRemovesExpiredAndIdleOnly(t *testing.T) {
	s, u, current := securityTestServer(t)
	ctx := context.Background()
	expired, e := s.Store.newSecuritySession(ctx, u.ID, false)
	if e != nil {
		t.Fatal(e)
	}
	idle, e := s.Store.newSecuritySession(ctx, u.ID, false)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Store.DB.ExecContext(ctx, `UPDATE cloud_sessions SET expires_at=now()-interval '1 minute' WHERE token_hash=$1`, tokenHash(expired)); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Store.DB.ExecContext(ctx, `UPDATE cloud_sessions SET last_seen_at=now()-interval '8 days' WHERE token_hash=$1`, tokenHash(idle)); e != nil {
		t.Fatal(e)
	}
	worker := &Worker{store: s.Store}
	if e = worker.cleanupExpiredOperationalRows(ctx); e != nil {
		t.Fatal(e)
	}
	var count int
	if e = s.Store.DB.QueryRowContext(ctx, `SELECT count(*) FROM cloud_sessions WHERE token_hash IN ($1,$2)`, tokenHash(expired), tokenHash(idle)).Scan(&count); e != nil || count != 0 {
		t.Fatalf("expired rows remain: %d %v", count, e)
	}
	if _, e = s.Store.Authenticate(ctx, current); e != nil {
		t.Fatal("maintenance removed active session")
	}
}
