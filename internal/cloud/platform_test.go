package cloud

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

func testPlatform(t *testing.T) (*Store, User) {
	t.Helper()
	dsn := os.Getenv("CLOUD_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set CLOUD_TEST_DATABASE_URL for PostgreSQL integration tests")
	}
	s, err := OpenStore(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err = s.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	u, err := s.Register(context.Background(), fmt.Sprintf("test-%d@example.com", time.Now().UnixNano()), "Test-password-123!", "Test Creator", fmt.Sprintf("creator%d", time.Now().UnixNano()))
	if err != nil {
		t.Fatal(err)
	}
	return s, u
}
func TestCreditReservationIsIdempotentAndCannotOverspend(t *testing.T) {
	s, u := testPlatform(t)
	ctx := context.Background()
	if err := s.GrantCredits(ctx, u.WorkspaceID, "purchase-test", 100, "Test purchase"); err != nil {
		t.Fatal(err)
	}
	if err := s.GrantCredits(ctx, u.WorkspaceID, "purchase-test", 100, "Test purchase"); err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProject(ctx, u, "Test", "Test")
	if err != nil {
		t.Fatal(err)
	}
	q, err := s.CreateQuote(ctx, u, QuoteInput{ProjectID: p.ID, Prompt: "A bird flying over the sea", Duration: 5, AspectRatio: "16:9"}, 10)
	if err != nil {
		t.Fatal(err)
	}
	j, err := s.SubmitJob(ctx, u, q.ID, "same-request")
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.SubmitJob(ctx, u, q.ID, "same-request")
	if err != nil || again.ID != j.ID {
		t.Fatalf("replay: %#v %v", again, err)
	}
	balance, err := s.Credits(ctx, u.WorkspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if balance.Available != 50 || balance.Reserved != 50 {
		t.Fatalf("balance %+v", balance)
	}
	_, err = s.SubmitJob(ctx, u, q.ID, "another-request")
	if err == nil {
		t.Fatal("quote must not be consumed twice")
	}
	if err = s.FinishJob(ctx, j.ID, "failed", "", "provider unavailable"); err != nil {
		t.Fatal(err)
	}
	if err = s.FinishJob(ctx, j.ID, "failed", "", "duplicate"); err != nil {
		t.Fatal(err)
	}
	balance, _ = s.Credits(ctx, u.WorkspaceID)
	if balance.Available != 100 || balance.Reserved != 0 {
		t.Fatalf("release %+v", balance)
	}
}
func TestProjectAndJobOwnership(t *testing.T) {
	s, u := testPlatform(t)
	_, other := testPlatform(t)
	ctx := context.Background()
	p, err := s.CreateProject(ctx, u, "Secret", "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.CreateQuote(ctx, other, QuoteInput{ProjectID: p.ID, Prompt: "Steal", Duration: 5, AspectRatio: "16:9"}, 10)
	if err == nil {
		t.Fatal("cross-workspace quote accepted")
	}
}
func TestStripeSignatureRejectsTamperingAndOldMessages(t *testing.T) {
	body := []byte(`{"type":"checkout.session.completed"}`)
	now := time.Unix(1800000000, 0)
	sig := stripeSignature(body, "secret", now.Unix())
	if !verifyStripeSignature(body, sig, "secret", now) {
		t.Fatal("valid signature rejected")
	}
	if verifyStripeSignature([]byte("tamper"), sig, "secret", now) {
		t.Fatal("tamper accepted")
	}
	if verifyStripeSignature(body, sig, "secret", now.Add(6*time.Minute)) {
		t.Fatal("expired accepted")
	}
}

func TestUnfundedRunningJobCanCancelButAdmittedJobCannot(t *testing.T) {
	s, u := testPlatform(t)
	ctx := context.Background()
	p, err := s.CreateProject(ctx, u, "Waiting budget", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.GrantCredits(ctx, u.WorkspaceID, "waiting-budget", 100, ""); err != nil {
		t.Fatal(err)
	}
	makeJob := func(key string) Job {
		t.Helper()
		q, e := s.CreateQuote(ctx, u, QuoteInput{ProjectID: p.ID, Prompt: "test", Duration: 5, AspectRatio: "16:9"}, 1)
		if e != nil {
			t.Fatal(e)
		}
		j, e := s.SubmitJob(ctx, u, q.ID, key)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = s.DB.ExecContext(ctx, `UPDATE cloud_jobs SET status='running' WHERE id=$1`, j.ID); e != nil {
			t.Fatal(e)
		}
		return j
	}
	j := makeJob("unfunded-cancel-key")
	got, err := s.Job(ctx, u, j.ID)
	if err != nil || !got.CanCancel {
		t.Fatalf("unfunded %+v %v", got, err)
	}
	if err = s.FinishJob(ctx, j.ID, "cancelled", "", "Cancelled before paid work"); err != nil {
		t.Fatal(err)
	}
	b, err := s.Credits(ctx, u.WorkspaceID)
	if err != nil || b.Reserved != 0 || b.Available != 100 {
		t.Fatalf("cancel balance %+v %v", b, err)
	}
	admitted := makeJob("admitted-cancel-key")
	if err = s.TryProviderBudget(ctx, admitted.ID, 1, 1<<60); err != nil {
		t.Fatal(err)
	}
	got, err = s.Job(ctx, u, admitted.ID)
	if err != nil || got.CanCancel {
		t.Fatalf("admitted %+v %v", got, err)
	}
	if err = s.FinishJob(ctx, admitted.ID, "cancelled", "", ""); err != ErrConflict {
		t.Fatalf("admitted cancellation %v", err)
	}
	b, err = s.Credits(ctx, u.WorkspaceID)
	if err != nil || b.Reserved != 5 {
		t.Fatalf("admitted reserve %+v %v", b, err)
	}
}
