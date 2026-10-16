package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"testing"

	"github.com/google/uuid"
)

func analyticsVideo(t *testing.T, s *Store, u User, title, format string) string {
	t.Helper()
	id := uuid.NewString()
	asset := uuid.NewString()
	ctx := context.Background()
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO cloud_media_assets(id,workspace_id,origin,object_key,status,width,height,duration_ms,measured)VALUES($1,$2,'upload',$3,'ready',1280,720,20000,true)`, asset, u.WorkspaceID, "workspaces/"+u.WorkspaceID+"/"+asset+".mp4"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO cloud_videos(id,channel_id,source_asset_id,title,visibility,audience,status,format,origin,published_at,views,watch_seconds)VALUES($1,$2,$3,$4,'public','free','published',$5,'upload',now(),999,999999)`, id, u.ChannelID, asset, title, format); err != nil {
		t.Fatal(err)
	}
	return id
}
func TestCreatorDetailedAnalyticsPrivacyAndServerCounters(t *testing.T) {
	s, creator := testPlatform(t)
	ctx := context.Background()
	first := analyticsVideo(t, s, creator, "Analytics wide", "standard")
	small := analyticsVideo(t, s, creator, "Small audience", "standard")
	for i := 0; i < 5; i++ {
		_, viewer := testPlatform(t)
		session := uuid.NewString()
		event := uuid.NewString()
		if _, err := s.DB.ExecContext(ctx, `INSERT INTO cloud_watch_sessions(id,video_id,user_id,viewer_hash,seconds,view_counted)VALUES($1,$2,$3,$4,20,true)`, session, first, viewer.ID, tokenHash(viewer.ID)); err != nil {
			t.Fatal(err)
		}
		if _, err := s.DB.ExecContext(ctx, `INSERT INTO cloud_watch_events(id,user_id,video_id,seconds,session_id)VALUES($1,$2,$3,10,$4)`, event, viewer.ID, first, session); err != nil {
			t.Fatal(err)
		}
		if _, err := s.DB.ExecContext(ctx, `INSERT INTO cloud_watch_events(id,user_id,video_id,seconds,session_id)VALUES($1,$2,$3,10,$4)`, uuid.NewString(), viewer.ID, first, session); err != nil {
			t.Fatal(err)
		}
		if _, err := s.DB.ExecContext(ctx, `INSERT INTO cloud_daily_viewers(video_id,viewer_hash,day)VALUES($1,$2,(now() AT TIME ZONE 'UTC')::date)`, first, tokenHash(viewer.ID)); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			session = uuid.NewString()
			if _, err := s.DB.ExecContext(ctx, `INSERT INTO cloud_watch_sessions(id,video_id,user_id,viewer_hash,seconds,view_counted)VALUES($1,$2,$3,$4,20,true)`, session, small, viewer.ID, tokenHash(viewer.ID)); err != nil {
				t.Fatal(err)
			}
			s.DB.ExecContext(ctx, `INSERT INTO cloud_watch_events(id,user_id,video_id,seconds,session_id)VALUES($1,$2,$3,10,$4)`, uuid.NewString(), viewer.ID, small, session)
			s.DB.ExecContext(ctx, `INSERT INTO cloud_watch_events(id,user_id,video_id,seconds,session_id)VALUES($1,$2,$3,10,$4)`, uuid.NewString(), viewer.ID, small, session)
			s.DB.ExecContext(ctx, `INSERT INTO cloud_daily_viewers(video_id,viewer_hash,day)VALUES($1,$2,(now() AT TIME ZONE 'UTC')::date)`, small, tokenHash(viewer.ID))
		}
	}
	detail, err := s.CreatorAnalyticsDetail(ctx, creator, 7)
	if err != nil {
		t.Fatal(err)
	}
	if detail.QualifiedViews != 6 || detail.WatchSeconds != 120 || detail.Sessions != 6 || detail.UniqueViewers == nil || *detail.UniqueViewers != 5 || len(detail.Daily) != 7 {
		t.Fatalf("unexpected aggregates %+v", detail)
	}
	if len(detail.Videos) != 2 || detail.Videos[0].QualifiedViews != 5 || detail.Videos[0].CompletionPercent == nil || *detail.Videos[0].CompletionPercent != 100 || detail.Videos[0].Reach100 == nil || *detail.Videos[0].Reach100 != 100 {
		t.Fatalf("video rates %+v", detail.Videos)
	}
	if detail.Videos[1].UniqueViewers != nil || detail.Videos[1].CompletionPercent != nil || detail.Videos[1].AverageWatchSeconds != nil {
		t.Fatalf("small audience not suppressed %+v", detail.Videos[1])
	}
	_, foreign := testPlatform(t)
	other, err := s.CreatorAnalyticsDetail(ctx, foreign, 7)
	if err != nil || other.WatchSeconds != 0 || len(other.Videos) != 0 {
		t.Fatalf("foreign analytics leaked %+v %v", other, err)
	}
	if _, err = s.CreatorAnalyticsDetail(ctx, creator, 365); err != ErrValidation {
		t.Fatal("unbounded range accepted")
	}
}
func TestFeedRankingConsentPrivacyAndCursorBinding(t *testing.T) {
	server, viewer, token := securityTestServer(t)
	s := server.Store
	ctx := context.Background()
	_, preferredCreator := testPlatform(t)
	_, otherCreator := testPlatform(t)
	liked := analyticsVideo(t, s, preferredCreator, "Ranking liked "+viewer.ID, "standard")
	popular := analyticsVideo(t, s, otherCreator, "Ranking popular "+viewer.ID, "standard")
	hidden := analyticsVideo(t, s, otherCreator, "Ranking hidden "+viewer.ID, "standard")
	if _, err := s.DB.ExecContext(ctx, `UPDATE cloud_videos SET watch_seconds=CASE WHEN id=$1 THEN 100000 ELSE 0 END,views=0,published_at=now()-interval '1 hour' WHERE id IN ($1,$2,$3)`, popular, liked, hidden); err != nil {
		t.Fatal(err)
	}
	s.DB.ExecContext(ctx, `INSERT INTO cloud_video_progress(user_id,video_id,position_ms)VALUES($1,$2,10000)`, viewer.ID, liked)
	feed := func(rank string) []Video {
		t.Helper()
		w := securityRequest(server, "GET", "/feed?format=standard&rank="+rank+"&q="+url.QueryEscape(viewer.ID), token, nil)
		var out struct {
			Items []Video `json:"items"`
		}
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil {
			t.Fatalf("rank %s %d %s", rank, w.Code, w.Body.String())
		}
		return out.Items
	}
	if items := feed("popular"); len(items) != 3 || items[0].ID != popular {
		t.Fatalf("popular order %+v", items)
	}
	if items := feed("for-you"); len(items) != 3 || items[0].ID != liked {
		t.Fatalf("history ranking %+v", items)
	}
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO cloud_video_preferences(user_id,settings)VALUES($1,'{"history_enabled":false}')`, viewer.ID); err != nil {
		t.Fatal(err)
	}
	if items := feed("for-you"); len(items) != 3 || items[0].ID != popular {
		t.Fatalf("disabled history used %+v", items)
	}
	s.DB.ExecContext(ctx, `INSERT INTO cloud_video_likes(user_id,video_id)VALUES($1,$2)`, viewer.ID, liked)
	if items := feed("for-you"); len(items) != 3 || items[0].ID != liked {
		t.Fatalf("explicit like ignored %+v", items)
	}
	s.DB.ExecContext(ctx, `UPDATE cloud_videos SET visibility='private' WHERE id=$1`, hidden)
	if items := feed("popular"); len(items) != 2 {
		t.Fatalf("private video leaked %+v", items)
	}
	s.DB.ExecContext(ctx, `INSERT INTO cloud_viewer_preferences(user_id,channel_id,hidden)VALUES($1,$2,true)`, viewer.ID, otherCreator.ChannelID)
	if items := feed("popular"); len(items) != 1 || items[0].ID != liked {
		t.Fatalf("hidden channel leaked %+v", items)
	}

	if _, err := s.DB.ExecContext(ctx, `INSERT INTO cloud_follows(user_id,channel_id)VALUES($1,$2)`, viewer.ID, preferredCreator.ChannelID); err != nil {
		t.Fatal(err)
	}
	if items := feed("following"); len(items) != 1 || items[0].ID != liked {
		t.Fatalf("following leaked unrelated creators %+v", items)
	}
	shortPreferred := analyticsVideo(t, s, preferredCreator, "Short ranking "+viewer.ID, "short")
	analyticsVideo(t, s, otherCreator, "Short other "+viewer.ID, "short")
	wShort := securityRequest(server, "GET", "/feed?format=short&rank=for-you&q="+url.QueryEscape(viewer.ID), token, nil)
	var shortResult struct {
		Items []Video `json:"items"`
	}
	if wShort.Code != 200 || json.Unmarshal(wShort.Body.Bytes(), &shortResult) != nil || len(shortResult.Items) != 1 || shortResult.Items[0].ID != shortPreferred {
		t.Fatalf("short ranking/privacy mismatch %d %s", wShort.Code, wShort.Body.String())
	}
	if _, err := s.DB.ExecContext(ctx, `UPDATE cloud_videos SET audience='members' WHERE id=$1`, liked); err != nil {
		t.Fatal(err)
	}
	premium, err := s.Video(ctx, liked, viewer.ID)
	if err != nil {
		t.Fatal(err)
	}
	if s.CanWatch(ctx, viewer, premium) {
		t.Fatal("ranking inputs bypassed Premium playback")
	}
	if w := securityRequest(server, "GET", "/feed?format=standard&rank=unbounded", token, nil); w.Code != 400 {
		t.Fatal("unknown ranking accepted")
	}
	// A feed cursor may not be reused for a different ranking or viewer.

	if w := securityRequest(server, "GET", fmt.Sprintf("/feed?rank=popular&cursor=%s", "eyJhdCI6IjIwMjYtMDEtMDFUMDA6MDA6MDBaIiwiaWQiOiJ4IiwiY29udGV4dCI6Indyb25nIn0"), token, nil); w.Code != 400 {
		t.Fatal("mismatched ranking cursor accepted")
	}
}
