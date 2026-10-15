package cloud

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
)

func maintenanceMedia(status func(*http.Request) int) *Media {
	client := s3.NewFromConfig(aws.Config{Region: "ap-southeast-1", Credentials: credentials.NewStaticCredentialsProvider("test", "test", ""), HTTPClient: &http.Client{Transport: commerceRoundTrip(func(r *http.Request) (*http.Response, error) {
		code := status(r)
		body := ""
		if code >= 400 {
			body = "<Error><Code>InternalError</Code></Error>"
		}
		return &http.Response{StatusCode: code, Header: http.Header{"Content-Type": []string{"application/xml"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}, RetryMaxAttempts: 1}, func(o *s3.Options) { o.BaseEndpoint = aws.String("https://local.test"); o.UsePathStyle = true })
	return &Media{s3: client}
}
func TestMaintenanceDoesNotReleaseUploadQuotaOnStorageError(t *testing.T) {
	store, u := testPlatform(t)
	key := "workspaces/" + u.WorkspaceID + "/uploads/" + uuid.NewString() + "/x.png"
	ctx := context.Background()
	if e := store.ReserveUpload(ctx, u.WorkspaceID, key, 100); e != nil {
		t.Fatal(e)
	}
	store.DB.ExecContext(ctx, `UPDATE cloud_upload_reservations SET created_at=now()-interval '2 days',expires_at=now()-interval '1 day' WHERE object_key=$1`, key)
	worker := &Worker{store: store, cfg: Config{UploadsBucket: "uploads"}, media: maintenanceMedia(func(r *http.Request) int { return 500 })}
	if e := worker.cleanupExpiredUploads(ctx); e == nil {
		t.Fatal("storage failure ignored")
	}
	var deleted bool
	store.DB.QueryRowContext(ctx, `SELECT deleted_at IS NOT NULL FROM cloud_upload_reservations WHERE object_key=$1`, key).Scan(&deleted)
	if deleted {
		t.Fatal("quota released without confirmed deletion")
	}
	exists := true
	deletes := 0
	worker.media = maintenanceMedia(func(r *http.Request) int {
		if r.Method == "DELETE" {
			exists = false
			deletes++
			return 204
		}
		if exists {
			return 200
		}
		return 404
	})
	if e := worker.cleanupExpiredUploads(ctx); e != nil {
		t.Fatal(e)
	}
	store.DB.QueryRowContext(ctx, `SELECT deleted_at IS NOT NULL FROM cloud_upload_reservations WHERE object_key=$1`, key).Scan(&deleted)
	if !deleted || deletes == 0 {
		t.Fatal("existing object was not deleted and confirmed")
	}
}
func TestMaintenanceArchivesBeforeDeletingRawWatchEvents(t *testing.T) {
	store, u := testPlatform(t)
	ctx := context.Background()
	p, _ := store.CreateProject(ctx, u, "Archive", "")
	store.GrantCredits(ctx, u.WorkspaceID, "archive-seed", 100, "")
	q, _ := store.CreateQuote(ctx, u, QuoteInput{ProjectID: p.ID, Prompt: "A lake", Duration: 5, AspectRatio: "16:9"}, 1)
	j, e := store.SubmitJob(ctx, u, q.ID, "archive-test-key")
	if e != nil {
		t.Fatal(e)
	}
	store.FinishJob(ctx, j.ID, "succeeded", "workspaces/test/a.mp4", "")
	v, e := store.CreateVideo(ctx, u, Video{JobID: j.ID, Title: "Archive", Visibility: "public", Audience: "free"})
	if e != nil {
		t.Fatal(e)
	}
	id := uuid.NewString()
	_, e = store.DB.ExecContext(ctx, `INSERT INTO cloud_watch_events(id,user_id,video_id,seconds,created_at)VALUES($1,$2,$3,10,$4)`, id, u.ID, v.ID, time.Now().Add(-92*24*time.Hour))
	if e != nil {
		t.Fatal(e)
	}
	worker := &Worker{store: store, cfg: Config{AnalyticsBucket: "analytics"}, media: maintenanceMedia(func(r *http.Request) int { return 500 })}
	if e = worker.archiveExpiredWatchEvents(ctx); e == nil {
		t.Fatal("archive failure ignored")
	}
	var exists bool
	store.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cloud_watch_events WHERE id=$1)`, id).Scan(&exists)
	if !exists {
		t.Fatal("raw record lost before durable archive")
	}
	var payload, key string
	worker.media = maintenanceMedia(func(r *http.Request) int {
		body, _ := io.ReadAll(r.Body)
		payload = string(body)
		key = r.URL.Path
		return 200
	})
	if e = worker.archiveExpiredWatchEvents(ctx); e != nil {
		t.Fatal(e)
	}
	store.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cloud_watch_events WHERE id=$1)`, id).Scan(&exists)
	if exists {
		t.Fatal("archived raw record retained")
	}
	if strings.Contains(payload, u.ID) || strings.Contains(payload, u.Email) || !strings.Contains(key, "watch/") {
		t.Fatalf("archive identity/path issue: %s %s", payload, key)
	}
}

func TestPrivacyControlsPreserveFinancialFactsAndRevokeSessions(t *testing.T) {
	store, u := testPlatform(t)
	ctx := context.Background()
	server := NewServer(store, Config{DevMode: true})
	_, token, e := store.Login(ctx, u.Email, "Test-password-123!")
	if e != nil {
		t.Fatal(e)
	}
	_, e = store.DB.ExecContext(ctx, `INSERT INTO cloud_viewer_preferences(user_id,channel_id)VALUES($1,$2)`, u.ID, u.ChannelID)
	if e != nil {
		t.Fatal(e)
	}
	request := httptest.NewRequest(http.MethodDelete, "/viewer/history", nil).WithContext(context.WithValue(ctx, userContext{}, u))
	response := httptest.NewRecorder()
	server.clearHistory(response, request)
	if response.Code != 200 {
		t.Fatalf("clear: %d %s", response.Code, response.Body.String())
	}
	var count int
	store.DB.QueryRowContext(ctx, `SELECT count(*) FROM cloud_viewer_preferences WHERE user_id=$1`, u.ID).Scan(&count)
	if count != 0 {
		t.Fatal("preferences retained")
	}
	request = httptest.NewRequest(http.MethodPost, "/account/deletion", nil).WithContext(context.WithValue(ctx, userContext{}, u))
	response = httptest.NewRecorder()
	server.requestAccountDeletion(response, request)
	if response.Code != 202 {
		t.Fatalf("request: %d %s", response.Code, response.Body.String())
	}
	if _, e = store.Authenticate(ctx, token); e == nil {
		t.Fatal("session survived deletion request")
	}
	store.DB.QueryRowContext(ctx, `SELECT count(*) FROM cloud_account_deletion_requests WHERE user_id=$1`, u.ID).Scan(&count)
	if count != 1 {
		t.Fatal("deletion request missing")
	}
	store.DB.QueryRowContext(ctx, `SELECT count(*) FROM cloud_credit_accounts WHERE workspace_id=$1`, u.WorkspaceID).Scan(&count)
	if count != 1 {
		t.Fatal("financial account deleted")
	}
}
