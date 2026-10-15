package cloud

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestPasswordHash(t *testing.T) {
	hash, err := hashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if hash == "correct horse battery staple" || !checkPassword(hash, "correct horse battery staple") || checkPassword(hash, "wrong") {
		t.Fatal("password hash verification failed")
	}
	if checkPassword("$argon2id$invalid", "anything") {
		t.Fatal("accepted corrupt hash")
	}
}

func TestIdentityLifecycle(t *testing.T) {
	dsn := os.Getenv("CLOUDFLOW_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("CLOUD_TEST_DATABASE_URL")
	}
	if dsn == "" {
		t.Skip("PostgreSQL integration database not configured")
	}
	ctx := context.Background()
	s, err := OpenStore(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	suffix := time.Now().Format("150405000000000")
	email := "identity-" + suffix + "@example.com"
	u, err := s.Register(ctx, email, "correct horse battery staple", "Tester", "tester"+suffix)
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.ExecContext(ctx, "DELETE FROM cloud_users WHERE id=$1", u.ID)
	var balance int64
	if err = s.DB.QueryRowContext(ctx, "SELECT balance FROM cloud_credit_accounts WHERE workspace_id=$1", u.WorkspaceID).Scan(&balance); err != nil || balance != 0 {
		t.Fatalf("initial balance=%d: %v", balance, err)
	}
	if _, _, err = s.Login(ctx, email, "wrong"); err == nil {
		t.Fatal("accepted bad password")
	}
	_, token, err := s.Login(ctx, email, "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Authenticate(ctx, token)
	if err != nil || got.ID != u.ID {
		t.Fatalf("authentication: %v", err)
	}
	// Idle and absolute lifetimes are independently enforced.
	if _, err = s.DB.ExecContext(ctx, "UPDATE cloud_sessions SET last_seen_at=now()-interval '8 days' WHERE token_hash=$1", tokenHash(token)); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Authenticate(ctx, token); err == nil {
		t.Fatal("idle-expired token valid")
	}
	_, token, err = s.Login(ctx, email, "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.ExecContext(ctx, "UPDATE cloud_sessions SET expires_at=now()-interval '1 second' WHERE token_hash=$1", tokenHash(token)); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Authenticate(ctx, token); err == nil {
		t.Fatal("absolute-expired token valid")
	}
	_, token, err = s.Login(ctx, email, "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	verify, err := s.IssueAccountToken(ctx, email, "verify_email")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.VerifyEmail(ctx, verify); err != nil {
		t.Fatal(err)
	}
	if err = s.VerifyEmail(ctx, verify); err == nil {
		t.Fatal("verification token reused")
	}
	got, err = s.Authenticate(ctx, token)
	if err != nil || !got.Verified {
		t.Fatalf("verification not persisted: %v", err)
	}
	reset, err := s.IssueAccountToken(ctx, email, "reset_password")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ResetPassword(ctx, reset, "new correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Authenticate(ctx, token); err == nil {
		t.Fatal("reset did not revoke session")
	}
	if _, _, err = s.Login(ctx, email, "correct horse battery staple"); err == nil {
		t.Fatal("old password valid after reset")
	}
	_, token, err = s.Login(ctx, email, "new correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Logout(ctx, token); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Authenticate(ctx, token); err == nil {
		t.Fatal("revoked token valid")
	}
}
