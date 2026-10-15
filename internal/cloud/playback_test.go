package cloud

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPlaybackGrantCookiesAreScopedAndSingleUse(t *testing.T) {
	store, u := testPlatform(t)
	ctx := context.Background()
	store.GrantCredits(ctx, u.WorkspaceID, "media-seed", 100, "")
	p, _ := store.CreateProject(ctx, u, "Media", "")
	q, _ := store.CreateQuote(ctx, u, QuoteInput{ProjectID: p.ID, Prompt: "A film", Duration: 5, AspectRatio: "16:9"}, 1)
	j, e := store.SubmitJob(ctx, u, q.ID, "media-test-key")
	if e != nil {
		t.Fatal(e)
	}
	store.FinishJob(ctx, j.ID, "succeeded", "workspaces/test/output.mp4", "")
	v, e := store.CreateVideo(ctx, u, Video{JobID: j.ID, Title: "Film", Visibility: "public", Audience: "free"})
	if e != nil {
		t.Fatal(e)
	}
	manifest := "videos/" + v.ID + "/hls/master.m3u8"
	store.DB.ExecContext(ctx, `UPDATE cloud_videos SET status='published',manifest_key=$2 WHERE id=$1`, v.ID, manifest)
	key, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		t.Fatal(e)
	}
	encoded := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	cfg := Config{Region: "ap-southeast-1", CloudFrontDomain: "media.example.com", CloudFrontKeyPairID: "test-key", CloudFrontPrivateKey: string(encoded), PublicOrigin: "https://example.com"}
	app := NewServer(store, cfg)
	app.Media, e = NewMedia(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	req := httptest.NewRequest("POST", "/v1/videos/"+v.ID+"/playback-grant", strings.NewReader(`{}`))
	req.Header.Set("Origin", cfg.PublicOrigin)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	app.Router().ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("grant %d %s", w.Code, w.Body.String())
	}
	var grant struct {
		URL       string `json:"url"`
		Authorize string `json:"authorize_url"`
	}
	json.Unmarshal(w.Body.Bytes(), &grant)
	if grant.Authorize == "" {
		t.Fatal("missing media host authorization")
	}
	req = httptest.NewRequest("GET", grant.Authorize, nil)
	req.Header.Set("Origin", cfg.PublicOrigin)
	w = httptest.NewRecorder()
	app.Router().ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("authorize %d %s", w.Code, w.Body.String())
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 3 {
		t.Fatalf("cookies=%d", len(cookies))
	}
	for _, c := range cookies {
		if c.Path != "/videos/"+v.ID+"/hls/" || !c.HttpOnly || !c.Secure || c.Domain != "" {
			t.Fatalf("cookie scope %+v", c)
		}
	}
	w = httptest.NewRecorder()
	app.Router().ServeHTTP(w, req)
	if w.Code != 401 {
		t.Fatal("authorization token replayed")
	}
}
