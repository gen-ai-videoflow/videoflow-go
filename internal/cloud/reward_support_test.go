package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSupportProgramValidation(t *testing.T) {
	p := RewardSupportProgram{ID: "x", Title: "Original work", Criteria: "One verified original publication", BudgetCents: 100, GrantCredits: 30, ServiceCostCents: 10, CreatorCap: 1, GrantCap: 10, ExpiryDays: 30, MinimumPublications: 1}
	if !p.valid() {
		t.Fatal("valid program rejected")
	}
	p.GrantCredits = 1000001
	if p.valid() {
		t.Fatal("unbounded credits")
	}
	p.GrantCredits = 30
	p.ServiceCostCents = 101
	if p.valid() {
		t.Fatal("underfunded program")
	}
}
func supportFixture(t *testing.T) (*Store, User, string, string) {
	t.Helper()
	s, u := testPlatform(t)
	ctx := context.Background()
	f := RewardFunding{ID: uuid.NewString(), Reference: uuid.NewString(), Kind: "support", Cents: 100, EvidenceReference: "settled:fixture", ProposedBy: "a", ReconciledBy: "b"}
	if e := s.RecordRewardFunding(ctx, f); e != nil {
		t.Fatal(e)
	}
	p := RewardSupportProgram{ID: uuid.NewString(), Title: "Original work", Criteria: "One verified original publication; no spending required", SourceID: f.ID, BudgetCents: 100, GrantCredits: 30, ServiceCostCents: 10, CreatorCap: 1, GrantCap: 10, ExpiryDays: 30, MinimumPublications: 1}
	if e := s.ProposeRewardSupport(ctx, p, "a"); e != nil {
		t.Fatal(e)
	}
	if _, e := s.ClaimRewardSupport(ctx, u, p.ID, "original-work"); e == nil {
		t.Fatal("disabled claim accepted")
	}
	if e := s.ApproveRewardSupport(ctx, p.ID, "a", "self"); e == nil {
		t.Fatal("self program approval")
	}
	if e := s.ApproveRewardSupport(ctx, p.ID, "b", "service-cost estimate reviewed"); e != nil {
		t.Fatal(e)
	}
	if _, e := s.DB.Exec(`INSERT INTO cloud_videos(id,channel_id,title,status,visibility,audience)VALUES($1,$2,'Original','published','public','free')`, uuid.NewString(), u.ChannelID); e != nil {
		t.Fatal(e)
	}
	id, e := s.ClaimRewardSupport(ctx, u, p.ID, "Original publication verified by independent reviewer")
	if e != nil {
		t.Fatal(e)
	}
	if e = s.ReviewRewardSupport(ctx, id, u.ID, "self", true); e == nil {
		t.Fatal("owner grant review accepted")
	}
	if e = s.ReviewRewardSupport(ctx, id, "reviewer", "Verified original work", true); e != nil {
		t.Fatal(e)
	}
	return s, u, p.ID, id
}
func TestSupportMixedCreditsAndExpiryReservation(t *testing.T) {
	s, u, program, grant := supportFixture(t)
	ctx := context.Background()
	if _, e := s.ClaimRewardSupport(ctx, u, program, "duplicate"); e == nil {
		t.Fatal("creator cap bypass")
	}
	if e := s.GrantCredits(ctx, u.WorkspaceID, "purchase", 100, "purchased"); e != nil {
		t.Fatal(e)
	}
	p, e := s.CreateProject(ctx, u, "Test", "")
	if e != nil {
		t.Fatal(e)
	}
	q, e := s.CreateQuote(ctx, u, QuoteInput{ProjectID: p.ID, Prompt: "A bird over the sea", Duration: 5, AspectRatio: "16:9"}, 10)
	if e != nil {
		t.Fatal(e)
	}
	j, e := s.SubmitJob(ctx, u, q.ID, "support-mixed-job")
	if e != nil {
		t.Fatal(e)
	}
	var promo int64
	if e = s.DB.QueryRow(`SELECT promotional_credits FROM cloud_jobs WHERE id=$1`, j.ID).Scan(&promo); e != nil || promo != 30 {
		t.Fatalf("promo %d %v", promo, e)
	}
	b, e := s.Credits(ctx, u.WorkspaceID)
	if e != nil || b.Reserved != 20 || b.Available != 80 {
		t.Fatalf("purchased balance %+v %v", b, e)
	}
	// Move database clock boundary by inserting an already-expired fixture is
	// forbidden by immutable terms; expire this lot via a transaction-local trigger
	// disable solely in isolated test DB, preserving the reserved obligation.
	if _, e = s.DB.Exec(`ALTER TABLE cloud_promotional_credits DISABLE TRIGGER cloud_promo_terms_immutable; UPDATE cloud_promotional_credits SET expires_at=now()-interval '1 second' WHERE id='` + grant + `'; ALTER TABLE cloud_promotional_credits ENABLE TRIGGER cloud_promo_terms_immutable`); e != nil {
		t.Fatal(e)
	}
	if e = s.FinishJob(ctx, j.ID, "succeeded", "output", ""); e != nil {
		t.Fatal(e)
	}
	if e = s.FinishJob(ctx, j.ID, "succeeded", "output", ""); e != nil {
		t.Fatal(e)
	}
	var consumed, reserved int64
	if e = s.DB.QueryRow(`SELECT consumed,reserved FROM cloud_promotional_credits WHERE id=$1`, grant).Scan(&consumed, &reserved); e != nil || consumed != 30 || reserved != 0 {
		t.Fatalf("promo consumed %d reserved %d %v", consumed, reserved, e)
	}
	b, e = s.Credits(ctx, u.WorkspaceID)
	if e != nil || b.Available != 80 || b.Reserved != 0 {
		t.Fatalf("settled %+v %v", b, e)
	}
	var debit int64
	if e = s.DB.QueryRow(`SELECT amount FROM cloud_credit_entries WHERE reference=$1`, "job:"+j.ID).Scan(&debit); e != nil || debit != -20 {
		t.Fatalf("purchase-only posting %d %v", debit, e)
	}
}
func TestSupportFailureReturnsOnlyUnexpiredPromoAndRestrictions(t *testing.T) {
	s, u, _, grant := supportFixture(t)
	ctx := context.Background()
	p, e := s.CreateProject(ctx, u, "Test", "")
	if e != nil {
		t.Fatal(e)
	}
	quote := func() Quote {
		q, e := s.CreateQuote(ctx, u, QuoteInput{ProjectID: p.ID, Prompt: "Original bird above sea", Duration: 5, AspectRatio: "16:9"}, 6)
		if e != nil {
			t.Fatal(e)
		}
		return q
	}
	q := quote()
	if _, e = s.DB.Exec(`UPDATE cloud_credit_accounts SET restricted=true WHERE workspace_id=$1`, u.WorkspaceID); e != nil {
		t.Fatal(e)
	}
	if _, e = s.SubmitJob(ctx, u, q.ID, "restricted-promo"); e == nil {
		t.Fatal("restriction bypass")
	}
	s.DB.Exec(`UPDATE cloud_credit_accounts SET restricted=false WHERE workspace_id=$1`, u.WorkspaceID)
	j, e := s.SubmitJob(ctx, u, q.ID, "unrestricted-promo")
	if e != nil {
		t.Fatal(e)
	}
	if e = s.FinishJob(ctx, j.ID, "failed", "", "failed"); e != nil {
		t.Fatal(e)
	}
	var n int64
	s.DB.QueryRow(`SELECT credits-consumed-reserved FROM cloud_promotional_credits WHERE id=$1`, grant).Scan(&n)
	if n != 30 {
		t.Fatalf("refund %d", n)
	}
	if _, e = s.DB.Exec(`ALTER TABLE cloud_promotional_credits DISABLE TRIGGER cloud_promo_terms_immutable; UPDATE cloud_promotional_credits SET expires_at=now()-interval '1 second' WHERE id='` + grant + `'; ALTER TABLE cloud_promotional_credits ENABLE TRIGGER cloud_promo_terms_immutable`); e != nil {
		t.Fatal(e)
	}
	q = quote()
	if _, e = s.SubmitJob(ctx, u, q.ID, "expired-promo"); e == nil {
		t.Fatal("expired promo spent")
	}
}
func TestAppealsOwnershipResolutionAndNotificationDedup(t *testing.T) {
	s, u := testPlatform(t)
	ctx := context.Background()
	if _, e := s.SubmitRewardAppeal(ctx, u.ID, "unknown", "allocation", "evidence"); e == nil {
		t.Fatal("nonowner allocation accepted")
	}
	id, e := s.SubmitRewardAppeal(ctx, u.ID, "", "hold", "Please review this hold")
	if e != nil {
		t.Fatal(e)
	}
	if e = s.ResolveRewardAppeal(ctx, id, u.ID, "self", true); e == nil {
		t.Fatal("self resolution")
	}
	if e = s.ResolveRewardAppeal(ctx, id, "reviewer", "Review completed; separate funded correction required", true); e != nil {
		t.Fatal(e)
	}
	if e = s.ResolveRewardAppeal(ctx, id, "reviewer", "duplicate", true); e == nil {
		t.Fatal("resolution rewrite")
	}
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- s.RecordRewardNotification(ctx, u.ID, "same-event", "confirmation", "Confirmed reward available in details")
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	var count int
	s.DB.QueryRow(`SELECT count(*) FROM cloud_reward_notifications WHERE user_id=$1 AND dedup_key='same-event'`, u.ID).Scan(&count)
	if count != 1 {
		t.Fatal("duplicate notification", count)
	}
	var attempts int
	var status string
	s.DB.QueryRow(`SELECT d.status,d.attempts FROM cloud_reward_notification_deliveries d JOIN cloud_reward_notifications n ON n.id=d.notification_id WHERE n.user_id=$1 AND n.dedup_key='same-event'`, u.ID).Scan(&status, &attempts)
	if status != "unsupported" || attempts != 0 {
		t.Fatal(status, attempts)
	}
}

func TestSupportConcurrentGrantAndInsufficientServiceBudget(t *testing.T) {
	s, u, program, _ := supportFixture(t)
	ctx := context.Background()
	var source string
	if e := s.DB.QueryRow(`SELECT source_id FROM cloud_reward_support_programs WHERE id=$1`, program).Scan(&source); e != nil {
		t.Fatal(e)
	}
	p := RewardSupportProgram{ID: uuid.NewString(), Title: "Independent milestone", Criteria: "Reviewed original publication", SourceID: source, BudgetCents: 100, GrantCredits: 10, ServiceCostCents: 10, CreatorCap: 1, GrantCap: 10, ExpiryDays: 2, MinimumPublications: 1}
	if e := s.ProposeRewardSupport(ctx, p, "a"); e != nil {
		t.Fatal(e)
	}
	if e := s.ApproveRewardSupport(ctx, p.ID, "b", "Approved bounded service estimate"); e != nil {
		t.Fatal(e)
	}
	claim, e := s.ClaimRewardSupport(ctx, u, p.ID, "Original work reference")
	if e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	errors := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errors <- s.ReviewRewardSupport(ctx, claim, "reviewer", "verified", true) }()
	}
	wg.Wait()
	close(errors)
	success := 0
	for e := range errors {
		if e == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("concurrent grants %d", success)
	}
	var reserved int64
	s.DB.QueryRow(`SELECT reserved_cents FROM cloud_reward_sources WHERE id=$1`, source).Scan(&reserved)
	if reserved != 20 {
		t.Fatalf("full expected service cost not reserved %d", reserved)
	}
	// Another independently reviewed milestone shares the exhausted source; no
	// partial grant, program count, credit lot or notification may commit.
	p.ID = uuid.NewString()
	if e = s.ProposeRewardSupport(ctx, p, "a"); e != nil {
		t.Fatal(e)
	}
	if e = s.ApproveRewardSupport(ctx, p.ID, "b", "reviewed"); e != nil {
		t.Fatal(e)
	}
	claim, e = s.ClaimRewardSupport(ctx, u, p.ID, "Original work reference")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.DB.Exec(`UPDATE cloud_reward_sources SET reserved_cents=cents WHERE id=$1`, source); e != nil {
		t.Fatal(e)
	}
	if e = s.ReviewRewardSupport(ctx, claim, "reviewer", "verified", true); e != ErrInsufficientCredits {
		t.Fatalf("source budget bypass %v", e)
	}
	var count int
	s.DB.QueryRow(`SELECT count(*) FROM cloud_promotional_credits WHERE id=$1`, claim).Scan(&count)
	if count != 0 {
		t.Fatal("unfunded lot")
	}
	var state string
	s.DB.QueryRow(`SELECT status FROM cloud_reward_support_claims WHERE id=$1`, claim).Scan(&state)
	if state != "pending" {
		t.Fatal("failed grant changed claim", state)
	}
}

func TestSupportCreatorProjectionsArePrivate(t *testing.T) {
	s, u, _, _ := supportFixture(t)
	app := NewServer(s, Config{DevMode: true})
	defer app.Close()
	for _, handler := range []func(http.ResponseWriter, *http.Request){app.rewardSupportOverview, app.rewardAppealList, app.rewardNotificationList} {
		r := httptest.NewRequest("GET", "/", nil)
		r = r.WithContext(context.WithValue(r.Context(), userContext{}, u))
		w := httptest.NewRecorder()
		handler(w, r)
		if w.Code != 200 {
			t.Fatalf("projection %d %s", w.Code, w.Body)
		}
		var out map[string]any
		if e := json.Unmarshal(w.Body.Bytes(), &out); e != nil {
			t.Fatal(e)
		}
		for _, secret := range []string{"source_id", "service_cost_cents", "proposed_by", "reviewed_by", "reviewer"} {
			if strings.Contains(w.Body.String(), `"`+secret+`"`) {
				t.Fatalf("private field exposed: %s", secret)
			}
		}
	}
}

func TestSupportRouteRegistrationAndAdminMFA(t *testing.T) {
	s, u := testPlatform(t)
	ctx := context.Background()
	_, token, e := s.Login(ctx, u.Email, "Test-password-123!")
	if e != nil {
		t.Fatal(e)
	}
	app := NewServer(s, Config{DevMode: true})
	defer app.Close()
	router := app.Router()
	call := func(path string, auth bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		if auth {
			r.AddCookie(&http.Cookie{Name: "videoflow_session", Value: token})
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	for _, path := range []string{"/v1/creator/rewards/support", "/v1/creator/rewards/appeals", "/v1/creator/rewards/notifications", "/v1/creator/rewards/corrections"} {
		if w := call(path, false); w.Code != 401 {
			t.Fatalf("anonymous %s %d", path, w.Code)
		}
		if w := call(path, true); w.Code != 200 {
			t.Fatalf("creator route %s %d %s", path, w.Code, w.Body)
		}
	}
	for _, path := range []string{"/v1/admin/rewards/support", "/v1/admin/rewards/appeals", "/v1/admin/rewards/corrections"} {
		if w := call(path, true); w.Code != 403 {
			t.Fatalf("nonadmin %s %d", path, w.Code)
		}
	}
	if _, e = s.DB.Exec(`INSERT INTO cloud_admin_roles(user_id,role,granted_by)VALUES($1,'finance','test')`, u.ID); e != nil {
		t.Fatal(e)
	}
	if w := call("/v1/admin/rewards/support", true); w.Code != 403 {
		t.Fatal("MFA bypass", w.Code)
	}
	if _, e = s.DB.Exec(`INSERT INTO cloud_totp(user_id,secret_ciphertext,enabled)VALUES($1,'fixture',true)`, u.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = s.DB.Exec(`UPDATE cloud_sessions SET mfa_verified_at=now() WHERE token_hash=$1`, tokenHash(token)); e != nil {
		t.Fatal(e)
	}
	for _, path := range []string{"/v1/admin/rewards/support", "/v1/admin/rewards/appeals", "/v1/admin/rewards/corrections"} {
		if w := call(path, true); w.Code != 200 {
			t.Fatalf("finance %s %d %s", path, w.Code, w.Body)
		}
	}
}
func TestSupportAdministrationPermissionBoundaries(t *testing.T) {
	for _, path := range []string{"/v1/admin/rewards/support", "/v1/admin/rewards/support/program/approve", "/v1/admin/rewards/support/claims/claim/review", "/v1/admin/rewards/appeals/appeal/resolve", "/v1/admin/rewards/corrections", "/v1/admin/rewards/corrections/correction/approve"} {
		if !adminAllowed("finance", "POST", path) {
			t.Fatal("finance mutation denied", path)
		}
		for _, role := range []string{"viewer", "support", "operations", "moderation"} {
			if adminAllowed(role, "POST", path) {
				t.Fatal("role can mutate support", role, path)
			}
		}
	}
}

func TestSupportConfirmationVestingAndHoldNotifications(t *testing.T) {
	s, u := testPlatform(t)
	ctx := context.Background()
	if _, e := s.DB.Exec(`UPDATE cloud_reward_policies SET active=true WHERE version='pilot-v1'`); e != nil {
		t.Fatal(e)
	}
	defer s.DB.Exec(`UPDATE cloud_reward_policies SET active=false WHERE version='pilot-v1'`)
	if e := s.EnrollRewards(ctx, u.ID, "pilot-v1"); e != nil {
		t.Fatal(e)
	}
	video := uuid.NewString()
	if _, e := s.DB.Exec(`INSERT INTO cloud_videos(id,channel_id,title,status,visibility,audience,published_at)VALUES($1,$2,'Original','published','public','free',now()-interval '2 days')`, video, u.ChannelID); e != nil {
		t.Fatal(e)
	}
	f := RewardFunding{ID: uuid.NewString(), Reference: uuid.NewString(), Kind: "generation", Cents: 100, EvidenceReference: "settled:notification-fixture", ProposedBy: "a", ReconciledBy: "b"}
	if e := s.RecordRewardFunding(ctx, f); e != nil {
		t.Fatal(e)
	}
	batch := uuid.NewString()
	if e := s.DraftRewardBatch(ctx, RewardBatchDraft{ID: batch, Day: time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02"), SourceID: f.ID, PolicyVersion: "pilot-v1", EvidenceReference: "qualified:notification", ProposedBy: "a", BudgetCents: 100, Weights: map[string]int64{video: 1}}); e != nil {
		t.Fatal(e)
	}
	if e := s.ConfirmRewardBatch(ctx, batch, "b"); e != nil {
		t.Fatal(e)
	}
	if e := s.ConfirmRewardBatch(ctx, batch, "b"); e != nil {
		t.Fatal(e)
	}
	var confirmed time.Time
	s.DB.QueryRow(`SELECT confirmed_at FROM cloud_reward_batches WHERE id=$1`, batch).Scan(&confirmed)
	if e := s.ReleaseRewardVesting(ctx, u.ID, confirmed.Add(30*24*time.Hour)); e != nil {
		t.Fatal(e)
	}
	if e := s.ReleaseRewardVesting(ctx, u.ID, confirmed.Add(30*24*time.Hour)); e != nil {
		t.Fatal(e)
	}
	for _, kind := range []string{"confirmation", "release"} {
		var n int
		if e := s.DB.QueryRow(`SELECT count(*) FROM cloud_reward_notifications WHERE user_id=$1 AND kind=$2`, u.ID, kind).Scan(&n); e != nil || n != 1 {
			t.Fatalf("%s notifications %d %v", kind, n, e)
		}
	}
	app := NewServer(s, Config{DevMode: true})
	defer app.Close()
	for _, held := range []bool{true, false} {
		r := httptest.NewRequest("POST", "/", strings.NewReader(fmt.Sprintf(`{"held":%t,"reason":"Internal review details"}`, held)))
		route := chi.NewRouteContext()
		route.URLParams.Add("id", u.ID)
		r = r.WithContext(context.WithValue(context.WithValue(r.Context(), userContext{}, User{ID: "reviewer"}), chi.RouteCtxKey, route))
		w := httptest.NewRecorder()
		app.adminRewardHold(w, r)
		if w.Code != 200 {
			t.Fatalf("hold notification %d %s", w.Code, w.Body)
		}
	}
	var count int
	s.DB.QueryRow(`SELECT count(*) FROM cloud_reward_notifications WHERE user_id=$1 AND kind='hold'`, u.ID).Scan(&count)
	if count != 1 {
		t.Fatal("missing hold notification", count)
	}
	var leaked bool
	s.DB.QueryRow(`SELECT EXISTS(SELECT 1 FROM cloud_reward_notifications WHERE user_id=$1 AND message LIKE '%Internal review%')`, u.ID).Scan(&leaked)
	if leaked {
		t.Fatal("private hold reason leaked in notification")
	}
}
