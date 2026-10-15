package cloud

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProfileURLValidation(t *testing.T) {
	for _, u := range []string{"javascript:alert(1)", "data:image/png;base64,abc", "http://example.com/a.png", "https://user:pass@example.com/a.png", "https://example.com/picture.SVG?x=1"} {
		if validProfileURL(u, true) {
			t.Errorf("accepted %q", u)
		}
	}
	for _, u := range []string{"", "https://example.com/avatar.png?size=200"} {
		if !validProfileURL(u, true) {
			t.Errorf("rejected %q", u)
		}
	}
}
func TestProfilePatchOwnershipAndPublicCounts(t *testing.T) {
	store, u := testPlatform(t)
	ctx := context.Background()
	server := &Server{Store: store}
	patch := func(body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPatch, "/profile", strings.NewReader(body)).WithContext(context.WithValue(ctx, userContext{}, u))
		server.patchProfile(w, r)
		return w
	}
	if w := patch(`{"name":"Edited","bio":"About","social_links":[{"label":"Portfolio","url":"https://example.com"}]}`); w.Code != 200 {
		t.Fatalf("patch %d %s", w.Code, w.Body.String())
	}
	if w := patch(`{"featured_video_id":"foreign-video"}`); w.Code < 400 {
		t.Fatalf("foreign pin accepted: %s", w.Body.String())
	}
	if w := patch(`{"avatar_url":"data:image/svg+xml,evil"}`); w.Code < 400 {
		t.Fatal("unsafe image accepted")
	}
	c, e := server.richChannelData(ctx, u.Handle)
	if e != nil {
		t.Fatal(e)
	}
	if c["name"] != "Edited" || c["bio"] != "About" {
		t.Fatalf("profile %#v", c)
	}
	if c["public_video_count"] != int64(0) || c["followers"] != int64(0) || c["public_views"] != int64(0) {
		t.Fatalf("fabricated counts %#v", c)
	}
	if _, ok := c["email"]; ok {
		t.Fatal("public email leak")
	}
}
func TestProfileHidesPrivateAndUnlistedVideos(t *testing.T) {
	store, u := testPlatform(t)
	ctx := context.Background()
	server := &Server{Store: store}
	project, e := store.CreateProject(ctx, u, "Profile visibility", "")
	if e != nil {
		t.Fatal(e)
	}
	if e = store.GrantCredits(ctx, u.WorkspaceID, "profile-seed", 100, ""); e != nil {
		t.Fatal(e)
	}
	var published string
	for _, visibility := range []string{"public", "private", "unlisted"} {
		q, err := store.CreateQuote(ctx, u, QuoteInput{ProjectID: project.ID, Prompt: "A calm scene", Duration: 10, AspectRatio: "16:9"}, 1)
		if err != nil {
			t.Fatal(err)
		}
		j, err := store.SubmitJob(ctx, u, q.ID, "profile-"+visibility)
		if err != nil {
			t.Fatal(err)
		}
		if err = store.FinishJob(ctx, j.ID, "succeeded", "workspaces/test/output.mp4", ""); err != nil {
			t.Fatal(err)
		}
		v, err := store.CreateVideo(ctx, u, Video{JobID: j.ID, Title: visibility, Visibility: visibility, Audience: "free"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = store.DB.ExecContext(ctx, `UPDATE cloud_videos SET status='published',views=7 WHERE id=$1`, v.ID); err != nil {
			t.Fatal(err)
		}
		if visibility == "public" {
			published = v.ID
		} else {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPatch, "/profile", strings.NewReader(`{"featured_video_id":"`+v.ID+`"}`)).WithContext(context.WithValue(ctx, userContext{}, u))
			server.patchProfile(w, r)
			if w.Code < 400 {
				t.Fatal("nonpublic pin accepted")
			}
		}
	}
	c, e := server.richChannelData(ctx, u.Handle)
	if e != nil {
		t.Fatal(e)
	}
	if c["public_video_count"] != int64(1) || c["public_views"] != int64(7) {
		t.Fatalf("private aggregate leak: %#v", c)
	}
	videos, e := store.Videos(ctx, u.ChannelID, false)
	if e != nil || len(videos) != 1 || videos[0].ID != published {
		t.Fatalf("profile video leak: %#v %v", videos, e)
	}
	// A video made private after it was pinned must disappear from the public pin.
	if _, e = store.DB.ExecContext(ctx, `UPDATE cloud_channels SET featured_video_id=$2 WHERE id=$1`, u.ChannelID, published); e != nil {
		t.Fatal(e)
	}
	// Featured order follows the profile selection rather than creation time.
	if _, e = store.DB.ExecContext(ctx, `UPDATE cloud_videos SET created_at=now()-interval '90 days' WHERE id=$1`, published); e != nil {
		t.Fatal(e)
	}
	if _, e = store.DB.ExecContext(ctx, `UPDATE cloud_videos SET visibility='public' WHERE channel_id=$1 AND visibility='unlisted'`, u.ChannelID); e != nil {
		t.Fatal(e)
	}
	videos, e = store.Videos(ctx, u.ChannelID, false)
	if e != nil || len(videos) != 2 || videos[0].ID != published {
		t.Fatalf("feature order: %#v %v", videos, e)
	}
	if _, e = store.DB.ExecContext(ctx, `UPDATE cloud_videos SET visibility='private' WHERE id=$1`, published); e != nil {
		t.Fatal(e)
	}
	c, e = server.richChannelData(ctx, u.Handle)
	if e != nil || c["featured_video_id"] != "" {
		t.Fatalf("stale private pin leaked: %#v %v", c, e)
	}
}

func TestProfileRoutesRequireSessionAndSaveOwner(t *testing.T) {
	s, u, token := securityTestServer(t)
	if w := securityRequest(s, "GET", "/profile", "", nil); w.Code != 401 {
		t.Fatalf("guest profile %d", w.Code)
	}
	if w := securityRequest(s, "PATCH", "/profile", token, map[string]string{"name": "Viewer profile", "bio": "Real viewer"}); w.Code != 200 {
		t.Fatalf("save %d %s", w.Code, w.Body.String())
	}
	w := securityRequest(s, "GET", "/profile", token, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Real viewer") {
		t.Fatalf("read %d %s", w.Code, w.Body.String())
	}
	w = securityRequest(s, "GET", "/channels/"+u.Handle, "", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Real viewer") || strings.Contains(w.Body.String(), u.Email) {
		t.Fatalf("public profile %d %s", w.Code, w.Body.String())
	}
}

func TestPublicProfilePreservesFollowingAcrossRequests(t *testing.T) {
	s, _, token := securityTestServer(t)
	_, creator := testPlatform(t)
	path := "/channels/" + creator.Handle
	if w := securityRequest(s, "GET", path, token, nil); w.Code != 200 || !strings.Contains(w.Body.String(), `"is_following":false`) {
		t.Fatalf("initial profile %d %s", w.Code, w.Body.String())
	}
	if w := securityRequest(s, "POST", "/channels/"+creator.ChannelID+"/follow", token, map[string]any{}); w.Code != 200 {
		t.Fatalf("follow %d %s", w.Code, w.Body.String())
	}
	for i := 0; i < 2; i++ {
		w := securityRequest(s, "GET", path, token, nil)
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"is_following":true`) || !strings.Contains(w.Body.String(), `"followers":1`) {
			t.Fatalf("reload profile %d %s", w.Code, w.Body.String())
		}
	}
	if w := securityRequest(s, "GET", path, "", nil); w.Code != 200 || !strings.Contains(w.Body.String(), `"is_following":false`) {
		t.Fatalf("guest profile %d %s", w.Code, w.Body.String())
	}
}
