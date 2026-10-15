package cloud

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCryptoPremiumExpiryAndVisibilityDenyPlayback(t *testing.T) {
	store, creator := testPlatform(t)
	_, viewer := testPlatform(t)
	ctx := context.Background()
	if e := store.GrantCredits(ctx, creator.WorkspaceID, "premium-playback", 100, ""); e != nil {
		t.Fatal(e)
	}
	project, e := store.CreateProject(ctx, creator, "Protected", "")
	if e != nil {
		t.Fatal(e)
	}
	quote, e := store.CreateQuote(ctx, creator, QuoteInput{ProjectID: project.ID, Prompt: "A movie", Duration: 5, AspectRatio: "16:9"}, 1)
	if e != nil {
		t.Fatal(e)
	}
	job, e := store.SubmitJob(ctx, creator, quote.ID, "protected-playback")
	if e != nil {
		t.Fatal(e)
	}
	if e = store.FinishJob(ctx, job.ID, "succeeded", "workspaces/test/protected.mp4", ""); e != nil {
		t.Fatal(e)
	}
	video, e := store.CreateVideo(ctx, creator, Video{JobID: job.ID, Title: "Premium film", Visibility: "public", Audience: "members"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = store.DB.ExecContext(ctx, `UPDATE cloud_videos SET status='published',manifest_key=$2 WHERE id=$1`, video.ID, "videos/"+video.ID+"/hls/master.m3u8"); e != nil {
		t.Fatal(e)
	}
	if _, e = store.DB.ExecContext(ctx, `INSERT INTO cloud_crypto_premium(user_id,paid_through)VALUES($1,now()+interval '30 days')`, viewer.ID); e != nil {
		t.Fatal(e)
	}
	_, session, e := store.Login(ctx, viewer.Email, "Test-password-123!")
	if e != nil {
		t.Fatal(e)
	}
	app := NewServer(store, Config{DevMode: true, PublicOrigin: "http://localhost:5173", CloudFrontDomain: "media.example.com"})
	app.Media = &Media{}
	request := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/v1/videos/"+video.ID+"/playback-grant", strings.NewReader(`{}`))
		req.Header.Set("Origin", app.Config.PublicOrigin)
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "videoflow_session", Value: session})
		w := httptest.NewRecorder()
		app.Router().ServeHTTP(w, req)
		return w
	}
	w := request()
	if w.Code != 200 {
		t.Fatalf("active Premium denied %d %s", w.Code, w.Body.String())
	}
	var grant struct {
		Authorize string `json:"authorize_url"`
	}
	if e = json.Unmarshal(w.Body.Bytes(), &grant); e != nil || grant.Authorize == "" {
		t.Fatal("missing grant", e)
	}
	if _, e = store.DB.ExecContext(ctx, `UPDATE cloud_crypto_premium SET paid_through=now()-interval '1 second' WHERE user_id=$1`, viewer.ID); e != nil {
		t.Fatal(e)
	}
	w = request()
	if w.Code != 401 {
		t.Fatalf("expired Premium grant %d %s", w.Code, w.Body.String())
	}
	req := httptest.NewRequest("GET", grant.Authorize, nil)
	w = httptest.NewRecorder()
	app.Router().ServeHTTP(w, req)
	if w.Code != 401 || len(w.Result().Cookies()) != 0 {
		t.Fatalf("pre-expiry token survived expiry %d", w.Code)
	}
	if _, e = store.DB.ExecContext(ctx, `UPDATE cloud_crypto_premium SET paid_through=now()+interval '30 days' WHERE user_id=$1`, viewer.ID); e != nil {
		t.Fatal(e)
	}
	for _, test := range []struct{ name, status, visibility string }{{"private", "published", "private"}, {"blocked", "blocked", "public"}, {"deleted", "deleted", "public"}} {
		t.Run(test.name, func(t *testing.T) {
			if _, e := store.DB.ExecContext(ctx, `UPDATE cloud_videos SET status=$2,visibility=$3 WHERE id=$1`, video.ID, test.status, test.visibility); e != nil {
				t.Fatal(e)
			}
			w := request()
			if w.Code != 404 {
				t.Fatalf("protected state returned %d %s", w.Code, w.Body.String())
			}
			if store.CanWatch(ctx, viewer, Video{ChannelID: creator.ChannelID, Status: test.status, Visibility: test.visibility, Audience: "members"}) {
				t.Fatal("Premium bypassed visibility/moderation")
			}
			if test.status != "published" && store.CanWatch(ctx, creator, Video{ChannelID: creator.ChannelID, Status: test.status, Visibility: test.visibility, Audience: "members"}) {
				t.Fatal("creator bypassed moderation")
			}
		})
	}
	var grants int
	if e = store.DB.QueryRowContext(ctx, `SELECT count(*) FROM cloud_playback_grants WHERE video_id=$1`, video.ID).Scan(&grants); e != nil || grants != 0 {
		t.Fatalf("denied requests created grants %d %v", grants, e)
	}
}
