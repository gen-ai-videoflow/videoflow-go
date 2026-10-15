package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"testing"
)

func TestViewingPreferencesAndActions(t *testing.T) {
	s, _, token := securityTestServer(t)
	w := securityRequest(s, "GET", "/feed?format=short", token, nil)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	var feed map[string]any
	if json.Unmarshal(w.Body.Bytes(), &feed) != nil || feed["items"] == nil {
		t.Fatalf("feed contract %s", w.Body.String())
	}
	for _, path := range []string{"/feed?format=bad", "/feed?format=short&cursor=not-a-cursor"} {
		if w = securityRequest(s, "GET", path, token, nil); w.Code != 400 {
			t.Fatalf("invalid feed %d", w.Code)
		}
	}
	if w = securityRequest(s, "GET", "/viewer/saved", "", nil); w.Code != 401 {
		t.Fatalf("guest library %d", w.Code)
	}
	if w = securityRequest(s, "PUT", "/videos/missing/save", token, map[string]bool{}); w.Code != 404 {
		t.Fatalf("missing save %d", w.Code)
	}
	if w = securityRequest(s, "PUT", "/videos/missing/progress", token, map[string]int{"position_ms": 10}); w.Code != 404 {
		t.Fatalf("missing progress %d", w.Code)
	}
}

func TestViewingPaginationActionsAndHistoryPrivacy(t *testing.T) {
	s, u, token := securityTestServer(t)
	ctx := context.Background()
	_, e := s.Store.DB.ExecContext(ctx, `INSERT INTO cloud_media_assets(id,workspace_id,origin,object_key,status,width,height,duration_ms,measured) SELECT $1||'-asset-'||n,$2,'upload','workspaces/'||$2||'/fixture-'||n||'.mp4','ready',90,160,5000,true FROM generate_series(1,21)n`, u.ID, u.WorkspaceID)
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.Store.DB.ExecContext(ctx, `INSERT INTO cloud_videos(id,channel_id,source_asset_id,title,visibility,audience,status,format,origin,published_at) SELECT $1||'-video-'||n,$2,$1||'-asset-'||n,$3,'public','free','published','short','upload',now()-n*interval '1 minute' FROM generate_series(1,21)n`, u.ID, u.ChannelID, "pagination "+u.Handle)
	if e != nil {
		t.Fatal(e)
	}

	creator := securityRequest(s, "GET", "/channel/videos?format=short", token, nil)
	var own struct {
		Items  []Video `json:"items"`
		Cursor string  `json:"next_cursor"`
	}
	if json.Unmarshal(creator.Body.Bytes(), &own) != nil || creator.Code != 200 || len(own.Items) != 20 || own.Cursor == "" {
		t.Fatalf("creator pagination %d %s", creator.Code, creator.Body.String())
	}
	nextOwn := securityRequest(s, "GET", "/channel/videos?format=short&cursor="+url.QueryEscape(own.Cursor), token, nil)
	var finalOwn struct {
		Items []Video `json:"items"`
	}
	if json.Unmarshal(nextOwn.Body.Bytes(), &finalOwn) != nil || len(finalOwn.Items) != 1 {
		t.Fatalf("creator page two %s", nextOwn.Body.String())
	}
	if got := securityRequest(s, "GET", "/channel/videos?status=draft&format=short&cursor="+url.QueryEscape(own.Cursor), token, nil); got.Code != 400 {
		t.Fatal("creator cursor crossed filter")
	}
	if got := securityRequest(s, "GET", "/channel/videos", "", nil); got.Code != 401 {
		t.Fatal("guest creator list")
	}
	_, _, catalogueForeign := securityTestServer(t)
	other := securityRequest(s, "GET", "/channel/videos", catalogueForeign, nil)
	if json.Unmarshal(other.Body.Bytes(), &finalOwn) != nil || len(finalOwn.Items) != 0 {
		t.Fatal("creator catalogue crossed ownership")
	}
	feedPath := "/feed?" + url.Values{"format": {"short"}, "q": {"pagination " + u.Handle}}.Encode()
	w := securityRequest(s, "GET", feedPath, token, nil)
	var feed struct {
		Items  []Video `json:"items"`
		Cursor string  `json:"next_cursor"`
	}
	if json.Unmarshal(w.Body.Bytes(), &feed) != nil || len(feed.Items) != 20 || feed.Cursor == "" {
		t.Fatalf("page one %d %s", w.Code, w.Body.String())
	}
	first := feed.Items[0].ID
	w = securityRequest(s, "GET", feedPath+"&cursor="+url.QueryEscape(feed.Cursor), token, nil)
	json.Unmarshal(w.Body.Bytes(), &feed)
	if len(feed.Items) != 1 || feed.Items[0].ID == first {
		t.Fatalf("page two %s", w.Body.String())
	}
	for _, action := range []string{"save", "like"} {
		for i := 0; i < 2; i++ {
			if w = securityRequest(s, "PUT", "/videos/"+first+"/"+action, token, map[string]any{}); w.Code != 200 {
				t.Fatalf("action %s %d", action, w.Code)
			}
		}
	}
	w = securityRequest(s, "GET", "/videos/"+first+"/actions", token, nil)
	var actions map[string]any
	json.Unmarshal(w.Body.Bytes(), &actions)
	if actions["saved"] != true || actions["likes"] != float64(1) {
		t.Fatal(actions)
	}
	if w = securityRequest(s, "PUT", "/videos/"+first+"/progress", token, map[string]int{"position_ms": 1000}); w.Code != 200 {
		t.Fatal(w.Code)
	}
	securityRequest(s, "PATCH", "/settings/preferences", token, map[string]bool{"history_enabled": false})
	securityRequest(s, "PUT", "/videos/"+first+"/progress", token, map[string]int{"position_ms": 2000})
	var position int64
	s.Store.DB.QueryRowContext(ctx, `SELECT position_ms FROM cloud_video_progress WHERE user_id=$1 AND video_id=$2`, u.ID, first).Scan(&position)
	if position != 1000 {
		t.Fatal("paused history wrote progress", position)
	}
	securityRequest(s, "DELETE", "/viewer/history", token, map[string]any{})
	var count int
	s.Store.DB.QueryRowContext(ctx, `SELECT count(*) FROM cloud_video_progress WHERE user_id=$1`, u.ID).Scan(&count)
	if count != 0 {
		t.Fatal("clear history kept progress")
	}
	_, _, foreign := securityTestServer(t)
	w = securityRequest(s, "GET", "/videos/"+first+"/actions", foreign, nil)
	json.Unmarshal(w.Body.Bytes(), &actions)
	if actions["saved"] != false || actions["position_ms"] != float64(0) {
		t.Fatal("personal state leak", actions)
	}
}

func TestShortQualifyingViewThreshold(t *testing.T) {
	for _, tc := range []struct {
		format   string
		duration int64
		measured bool
		want     int
	}{{"short", 3000, true, 3}, {"short", 5000, true, 4}, {"short", 10000, true, 8}, {"short", 180000, true, 10}, {"short", 5000, false, 10}, {"standard", 5000, true, 10}} {
		if got := qualifyingViewSeconds(tc.format, tc.duration, tc.measured); got != tc.want {
			t.Fatalf("%+v got %d", tc, got)
		}
	}
}

func TestShortWatchAccountingIsDurationAwareAndDeduplicated(t *testing.T) {
	s, creator, _ := securityTestServer(t)
	ctx := context.Background()
	_, viewer := testPlatform(t)
	asset := creator.ID + "-short-asset"
	video := creator.ID + "-short-video"
	_, e := s.Store.DB.ExecContext(ctx, `INSERT INTO cloud_media_assets(id,workspace_id,origin,object_key,status,width,height,duration_ms,measured)VALUES($1,$2,'upload','workspaces/test/fixture.mp4','ready',90,160,5000,true)`, asset, creator.WorkspaceID)
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.Store.DB.ExecContext(ctx, `INSERT INTO cloud_videos(id,channel_id,source_asset_id,title,visibility,audience,status,format,origin,published_at)VALUES($1,$2,$3,'Short fixture','public','free','published','short','upload',now())`, video, creator.ChannelID, asset)
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 2; i++ {
		session, e := s.Store.BeginWatch(ctx, viewer, video, viewer.ID)
		if e != nil {
			t.Fatal(e)
		}
		s.Store.DB.ExecContext(ctx, `UPDATE cloud_watch_sessions SET last_event_at=now()-interval '5 seconds' WHERE id=$1`, session)
		s.Store.DB.ExecContext(ctx, `UPDATE cloud_watch_clocks SET last_at=now()-interval '5 seconds' WHERE user_id=$1`, viewer.ID)
		if _, e = s.Store.RecordWatch(ctx, viewer, session, fmt.Sprintf("short-event-%s-%d", video, i), 4); e != nil {
			t.Fatal(e)
		}
	}
	var views int
	s.Store.DB.QueryRowContext(ctx, `SELECT views FROM cloud_videos WHERE id=$1`, video).Scan(&views)
	if views != 1 {
		t.Fatalf("qualified short views %d", views)
	}
}
