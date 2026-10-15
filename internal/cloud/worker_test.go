package cloud

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"os"
	"strings"
	"testing"
)

func TestEncodingEventEnvelope(t *testing.T) {
	raw := `{"source":"aws.mediaconvert","detail":{"jobId":"job-1","status":"COMPLETE","userMetadata":{"video_id":"video-1"}}}`
	for _, body := range []string{raw, `{"Message":` + string(mustJSON(raw)) + `}`} {
		e, err := parseEncodingEvent([]byte(body))
		if err != nil || e.Detail.JobID != "job-1" || e.Detail.UserMetadata["video_id"] != "video-1" {
			t.Fatalf("event=%+v err=%v", e, err)
		}
	}
	if _, err := parseEncodingEvent([]byte(`{"detail":{"status":"COMPLETE"}}`)); err == nil {
		t.Fatal("accepted missing encoding identity")
	}
}
func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }
func TestMediaKeyValidation(t *testing.T) {
	for _, key := range []string{"../private", "/absolute", "a/../secret", "https://evil.test/x", "a\\b", ""} {
		if validMediaKey(key) {
			t.Fatalf("accepted %q", key)
		}
	}
	if !validMediaKey("workspaces/abc/output.mp4") {
		t.Fatal("valid key rejected")
	}
}

func TestEncodingCompletionIsCorrelatedAndIdempotent(t *testing.T) {
	dsn := os.Getenv("CLOUD_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("PostgreSQL integration database not configured")
	}
	ctx := context.Background()
	store, err := OpenStore(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err = store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")
	u, err := store.Register(ctx, suffix+"@example.com", "correct horse battery staple", "Encoding test", "e"+suffix)
	if err != nil {
		t.Fatal(err)
	}
	p, err := store.CreateProject(ctx, u, "Encoding test", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = store.GrantCredits(ctx, u.WorkspaceID, "test", 100, ""); err != nil {
		t.Fatal(err)
	}
	q, err := store.CreateQuote(ctx, u, QuoteInput{ProjectID: p.ID, Prompt: "test", Duration: 5, AspectRatio: "16:9"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	j, err := store.SubmitJob(ctx, u, q.ID, "encoding-test-key")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.DB.ExecContext(ctx, `UPDATE cloud_jobs SET attempt=2,lease_until=now()+interval '5 minutes',status='running' WHERE id=$1`, j.ID); err != nil {
		t.Fatal(err)
	}
	if err = store.FinishJobAttempt(ctx, j.ID, 1, "succeeded", "output.mp4", ""); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale settlement accepted: %v", err)
	}
	balance, err := store.Credits(ctx, u.WorkspaceID)
	if err != nil || balance.Reserved != 5 {
		t.Fatalf("stale worker changed reservation: %+v %v", balance, err)
	}
	if _, err = store.DB.ExecContext(ctx, `UPDATE cloud_jobs SET lease_until=now()-interval '1 second' WHERE id=$1`, j.ID); err != nil {
		t.Fatal(err)
	}
	if err = store.FinishJobAttempt(ctx, j.ID, 2, "succeeded", "output.mp4", ""); !errors.Is(err, ErrConflict) {
		t.Fatalf("expired worker accepted: %v", err)
	}
	if _, err = store.DB.ExecContext(ctx, `UPDATE cloud_jobs SET lease_until=now()+interval '5 minutes' WHERE id=$1`, j.ID); err != nil {
		t.Fatal(err)
	}
	if err = store.FinishJobAttempt(ctx, j.ID, 2, "succeeded", "output.mp4", ""); err != nil {
		t.Fatal(err)
	}
	id := uuid.NewString()
	_, err = store.DB.ExecContext(ctx, `INSERT INTO cloud_videos(id,channel_id,job_id,title,visibility,audience,status,encoding_id) VALUES($1,$2,$3,'Test','public','free','processing','encoder-test')`, id, u.ChannelID, j.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		for _, statement := range []string{`DELETE FROM cloud_videos WHERE id=$1`, `DELETE FROM cloud_job_events WHERE job_id=$1`, `DELETE FROM cloud_outbox WHERE aggregate_id=$1`, `DELETE FROM cloud_jobs WHERE id=$1`, `DELETE FROM cloud_quotes WHERE id=$1`, `DELETE FROM cloud_credit_entries WHERE workspace_id=$1`, `DELETE FROM cloud_projects WHERE id=$1`, `DELETE FROM cloud_users WHERE id=$1`} {
			arg := id
			switch {
			case strings.Contains(statement, "cloud_job_events"), strings.Contains(statement, "cloud_outbox"), strings.Contains(statement, "cloud_jobs"):
				arg = j.ID
			case strings.Contains(statement, "cloud_quotes"):
				arg = q.ID
			case strings.Contains(statement, "cloud_credit_entries"):
				arg = u.WorkspaceID
			case strings.Contains(statement, "cloud_projects"):
				arg = p.ID
			case strings.Contains(statement, "cloud_users"):
				arg = u.ID
			}
			store.DB.ExecContext(ctx, statement, arg)
		}
	}()
	worker := &Worker{store: store}
	event := encodingEvent{}
	event.Detail.UserMetadata = map[string]string{"video_id": id}
	event.Detail.JobID = "wrong"
	event.Detail.Status = "COMPLETE"
	if err = worker.encodingComplete(ctx, event); err != nil {
		t.Fatal(err)
	}
	var status string
	store.DB.QueryRowContext(ctx, `SELECT status FROM cloud_videos WHERE id=$1`, id).Scan(&status)
	if status != "processing" {
		t.Fatal("uncorrelated event changed publication")
	}
	event.Detail.JobID = "encoder-test"
	for i := 0; i < 2; i++ {
		if err = worker.encodingComplete(ctx, event); err != nil {
			t.Fatal(err)
		}
	}
	store.DB.QueryRowContext(ctx, `SELECT status FROM cloud_videos WHERE id=$1`, id).Scan(&status)
	if status != "published" {
		t.Fatalf("completion status=%s", status)
	}
	if _, err = store.DB.ExecContext(ctx, `UPDATE cloud_videos SET status='encoding_reconciliation_required',encoding_id=$2 WHERE id=$1`, id, "submitting:"+id); err != nil {
		t.Fatal(err)
	}
	if err = worker.encodingComplete(ctx, event); err != nil {
		t.Fatal(err)
	}
	if err = store.DB.QueryRowContext(ctx, `SELECT status FROM cloud_videos WHERE id=$1`, id).Scan(&status); err != nil || status != "published" {
		t.Fatalf("ambiguous submission not reconciled: %s %v", status, err)
	}

}

func TestMP4AudioInspection(t *testing.T) {
	atom := func(kind string, body []byte) []byte {
		b := make([]byte, len(body)+8)
		binary.BigEndian.PutUint32(b, uint32(len(b)))
		copy(b[4:8], kind)
		copy(b[8:], body)
		return b
	}
	for _, kind := range []string{"soun", "vide"} {
		handler := make([]byte, 12)
		copy(handler[8:], kind)
		data := atom("moov", atom("trak", atom("mdia", atom("hdlr", handler))))
		got, err := mp4HasAudio(data, 0)
		if err != nil || got != (kind == "soun") {
			t.Fatalf("%s: %v %v", kind, got, err)
		}
	}
	if _, err := mp4HasAudio([]byte{0, 0, 0, 99, 'm', 'o', 'o', 'v'}, 0); err == nil {
		t.Fatal("accepted truncated MP4")
	}
}
