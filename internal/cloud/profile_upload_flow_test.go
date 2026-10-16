package cloud

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func TestProfileImageUploadPublishOwnershipAndCleanup(t *testing.T) {
	server, u, token := securityTestServer(t)
	ctx := context.Background()
	server.Config.OriginalsBucket = "originals"
	var mu sync.Mutex
	objects := map[string][]byte{}
	deleted := map[string]bool{}
	client := s3.NewFromConfig(aws.Config{Region: "local", Credentials: credentials.NewStaticCredentialsProvider("test", "test", ""), RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired, HTTPClient: &http.Client{Transport: commerceRoundTrip(func(r *http.Request) (*http.Response, error) {
		mu.Lock()
		defer mu.Unlock()
		header := http.Header{}
		var data []byte
		status := 200
		switch r.Method {
		case http.MethodPut:
			var err error
			data, err = io.ReadAll(r.Body)
			if err != nil {
				return nil, err
			}
			objects[r.URL.Path] = data
			header.Set("ETag", `"test-etag"`)
			data = nil
		case http.MethodGet:
			var ok bool
			data, ok = objects[r.URL.Path]
			if !ok {
				status = 404
			}
			header.Set("Content-Type", "image/jpeg")
		case http.MethodDelete:
			deleted[r.URL.Path] = true
			delete(objects, r.URL.Path)
			status = 204
		default:
			return nil, fmt.Errorf("unexpected S3 method %s", r.Method)
		}
		header.Set("Content-Length", strconv.Itoa(len(data)))
		return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(bytes.NewReader(data)), ContentLength: int64(len(data))}, nil
	})}}, func(o *s3.Options) { o.BaseEndpoint = aws.String("http://localhost:4566"); o.UsePathStyle = true })
	server.Media = &Media{s3: client}
	var source bytes.Buffer
	if err := png.Encode(&source, image.NewRGBA(image.Rect(0, 0, 128, 128))); err != nil {
		t.Fatal(err)
	}
	upload := func() string {
		t.Helper()
		w := securityRequest(server, "POST", "/profile/images", token, map[string]string{"kind": "avatar", "data": base64.StdEncoding.EncodeToString(source.Bytes())})
		if w.Code != 201 {
			t.Fatalf("upload %d %s", w.Code, w.Body.String())
		}
		var out map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out["url"]
	}
	imageURL := upload()
	id := imageURL[strings.LastIndex(imageURL, "/")+1:]
	if w := securityRequest(server, "GET", "/profile-images/"+id, "", nil); w.Code != 404 {
		t.Fatalf("unpublished image accessible %d", w.Code)
	}
	if w := securityRequest(server, "PATCH", "/profile", token, map[string]string{"avatar_url": imageURL}); w.Code != 200 {
		t.Fatalf("publish profile %d %s", w.Code, w.Body.String())
	}
	public := securityRequest(server, "GET", "/profile-images/"+id, "", nil)
	if public.Code != 200 || public.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("public image %d %s", public.Code, public.Body.String())
	}
	decoded, format, err := image.Decode(bytes.NewReader(public.Body.Bytes()))
	if err != nil || format != "jpeg" || decoded.Bounds().Dx() != 128 {
		t.Fatalf("public image invalid %s %v", format, err)
	}
	_, _, foreignToken := securityTestServer(t)
	if w := securityRequest(server, "PATCH", "/profile", foreignToken, map[string]string{"avatar_url": imageURL}); w.Code != 400 {
		t.Fatalf("foreign image attached %d %s", w.Code, w.Body.String())
	}
	orphanURL := upload()
	orphanID := orphanURL[strings.LastIndex(orphanURL, "/")+1:]
	if _, err = server.Store.DB.ExecContext(ctx, `UPDATE cloud_profile_images SET created_at=now()-interval '25 hours' WHERE id IN ($1,$2)`, id, orphanID); err != nil {
		t.Fatal(err)
	}
	if count, err := server.CleanupProfileImages(ctx); err != nil || count != 1 {
		t.Fatalf("cleanup count=%d error=%v", count, err)
	}
	if w := securityRequest(server, "GET", "/profile-images/"+id, "", nil); w.Code != 200 {
		t.Fatal("cleanup removed published image")
	}
	if w := securityRequest(server, "PATCH", "/profile", token, map[string]string{"banner_url": orphanURL}); w.Code != 400 {
		t.Fatalf("deleted orphan republished %d", w.Code)
	}
	// A cleanup candidate concurrently published while cleanup waits on the
	// channel lock must be rechecked and retained, rather than deleting live media.
	racingURL := upload()
	racingID := racingURL[strings.LastIndex(racingURL, "/")+1:]
	if _, err = server.Store.DB.ExecContext(ctx, `UPDATE cloud_profile_images SET created_at=now()-interval '25 hours' WHERE id=$1`, racingID); err != nil {
		t.Fatal(err)
	}
	tx, err := server.Store.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT id FROM cloud_channels WHERE id=$1 FOR UPDATE`, u.ChannelID); err != nil {
		t.Fatal(err)
	}
	type result struct {
		count int
		err   error
	}
	done := make(chan result, 1)
	cleanupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	go func() { n, e := server.CleanupProfileImages(cleanupCtx); done <- result{n, e} }()
	waiting := false
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if err = server.Store.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE 'SELECT c.id FROM cloud_channels c JOIN cloud_profile_images%')`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !waiting {
		t.Fatal("cleanup did not reach channel lock")
	}
	if _, err = tx.ExecContext(ctx, `UPDATE cloud_channels SET banner_url=$2 WHERE id=$1`, u.ChannelID, racingURL); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	outcome := <-done
	if outcome.err != nil || outcome.count != 0 {
		t.Fatalf("cleanup deleted newly published image: %+v", outcome)
	}
	if w := securityRequest(server, "GET", "/profile-images/"+racingID, "", nil); w.Code != 200 {
		t.Fatalf("racing published image lost %d", w.Code)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(deleted) != 1 {
		t.Fatalf("expected only one orphan storage delete, got%d", len(deleted))
	}
}
