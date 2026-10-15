package cloud

import (
	"context"
	"github.com/google/uuid"
	"testing"
	"time"
)

func TestWatchHeartbeatsCannotInventElapsedTimeOrDuplicateViews(t *testing.T) {
	s, creator := testPlatform(t)
	_, viewer := testPlatform(t)
	ctx := context.Background()
	p, _ := s.CreateProject(ctx, creator, "View test", "")
	s.GrantCredits(ctx, creator.WorkspaceID, "watch-seed", 100, "")
	q, _ := s.CreateQuote(ctx, creator, QuoteInput{ProjectID: p.ID, Prompt: "A calm scene", Duration: 10, AspectRatio: "16:9"}, 1)
	j, e := s.SubmitJob(ctx, creator, q.ID, "watch-job-key")
	if e != nil {
		t.Fatal(e)
	}
	s.FinishJob(ctx, j.ID, "succeeded", "workspaces/test/output.mp4", "")
	v, e := s.CreateVideo(ctx, creator, Video{JobID: j.ID, Title: "Watch test", Visibility: "public", Audience: "free"})
	if e != nil {
		t.Fatal(e)
	}
	s.DB.ExecContext(ctx, `UPDATE cloud_videos SET status='published' WHERE id=$1`, v.ID)
	session, e := s.BeginWatch(ctx, viewer, v.ID, "viewer-test")
	if e != nil {
		t.Fatal(e)
	}
	id := uuid.NewString()
	n, e := s.RecordWatch(ctx, viewer, session, id, 15)
	if e != nil || n > 1 {
		t.Fatalf("invented elapsed time: %d %v", n, e)
	}
	s.DB.ExecContext(ctx, `UPDATE cloud_watch_sessions SET started_at=$2,last_event_at=$2 WHERE id=$1`, session, time.Now().Add(-20*time.Second))
	n, e = s.RecordWatch(ctx, viewer, session, id, 15)
	if e != nil || n != 0 {
		t.Fatalf("duplicate %d %v", n, e)
	}
	s.DB.ExecContext(ctx, `UPDATE cloud_watch_clocks SET last_at=now()-interval '20 seconds' WHERE user_id=$1`, viewer.ID)
	n, e = s.RecordWatch(ctx, viewer, session, uuid.NewString(), 15)
	if e != nil || n != 15 {
		t.Fatalf("valid heartbeat %d %v", n, e)
	}
	var views int
	s.DB.QueryRowContext(ctx, `SELECT views FROM cloud_videos WHERE id=$1`, v.ID).Scan(&views)
	if views != 1 {
		t.Fatalf("views=%d", views)
	}
	second, e := s.BeginWatch(ctx, viewer, v.ID, "viewer-test")
	if e != nil {
		t.Fatal(e)
	}
	s.DB.ExecContext(ctx, `UPDATE cloud_watch_sessions SET started_at=now()-interval '20 seconds',last_event_at=now()-interval '20 seconds' WHERE id=$1`, second)
	s.RecordWatch(ctx, viewer, second, uuid.NewString(), 15)
	s.DB.QueryRowContext(ctx, `SELECT views FROM cloud_videos WHERE id=$1`, v.ID).Scan(&views)
	if views != 1 {
		t.Fatal("same day view double counted")
	}
	_, other := testPlatform(t)
	if _, e = s.RecordWatch(ctx, other, session, uuid.NewString(), 15); e == nil {
		t.Fatal("cross-user watch accepted")
	}
}
