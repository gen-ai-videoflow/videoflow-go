package cloud

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func TestVideoUploadValidatedPublicationAndDownload(t *testing.T) {
	binary := os.Getenv("MEDIA_FFPROBE_TEST_BINARY")
	ffmpeg := os.Getenv("PIPELINE_FFMPEG_TEST_BINARY")
	if binary == "" || ffmpeg == "" {
		t.Skip("fixture binaries required")
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "source.mp4")
	if e := runFFmpeg(context.Background(), ffmpeg, dir, []string{"-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "color=c=blue:s=90x160:d=3", "-c:v", "libx264", "-threads", "1", "-pix_fmt", "yuv420p", file}); e != nil {
		t.Fatal(e)
	}
	data, e := os.ReadFile(file)
	if e != nil {
		t.Fatal(e)
	}
	checksum := sha256.Sum256(data)
	var mu sync.Mutex
	objects := map[string][]byte{}
	var part []byte
	multipartOpen := false
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		key := strings.TrimPrefix(r.URL.Path, "/")
		w.Header().Set("Content-Type", "application/xml")
		if r.Method == "POST" && r.URL.Query().Has("uploads") {
			part = nil
			multipartOpen = true
			fmt.Fprint(w, `<InitiateMultipartUploadResult><UploadId>fixture-upload</UploadId></InitiateMultipartUploadResult>`)
			return
		}
		if r.Method == "PUT" && r.URL.Query().Get("partNumber") != "" {
			part, _ = io.ReadAll(r.Body)
			w.Header().Set("ETag", `"part-1"`)
			return
		}
		if r.Method == "GET" && r.URL.Query().Has("uploads") {
			fmt.Fprint(w, `<ListMultipartUploadsResult><IsTruncated>false</IsTruncated></ListMultipartUploadsResult>`)
			return
		}
		if r.Method == "GET" && r.URL.Query().Get("uploadId") != "" {
			if !multipartOpen {
				w.WriteHeader(404)
				fmt.Fprint(w, `<Error><Code>NoSuchUpload</Code></Error>`)
				return
			}
			fmt.Fprintf(w, `<ListPartsResult><IsTruncated>false</IsTruncated><Part><PartNumber>1</PartNumber><ETag>"part-1"</ETag><Size>%d</Size></Part></ListPartsResult>`, len(part))
			return
		}
		if r.Method == "POST" && r.URL.Query().Get("uploadId") != "" {
			objects[key] = append([]byte(nil), part...)
			fmt.Fprint(w, `<CompleteMultipartUploadResult><ETag>"complete"</ETag></CompleteMultipartUploadResult>`)
			return
		}
		if r.Method == "PUT" {
			if copyFrom := r.Header.Get("X-Amz-Copy-Source"); copyFrom != "" {
				source, _ := url.PathUnescape(strings.TrimPrefix(copyFrom, "/"))
				objects[key] = append([]byte(nil), objects[source]...)
				fmt.Fprint(w, `<CopyObjectResult><ETag>"copy"</ETag></CopyObjectResult>`)
			} else {
				objects[key], _ = io.ReadAll(r.Body)
			}
			return
		}
		if r.Method == "DELETE" {
			if r.URL.Query().Get("uploadId") != "" {
				multipartOpen = false
				w.WriteHeader(204)
				return
			}
			delete(objects, key)
			w.WriteHeader(204)
			return
		}
		b, ok := objects[key]
		if !ok {
			w.WriteHeader(404)
			fmt.Fprint(w, `<Error><Code>NoSuchKey</Code></Error>`)
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(b)))
		if r.Method == "GET" {
			w.Write(b)
		}
	}))
	defer storage.Close()
	s, u, token := securityTestServer(t)
	ctx := context.Background()
	s.Store.DB.ExecContext(ctx, `UPDATE cloud_users SET verified=true WHERE id=$1`, u.ID)
	s.Config.VideoUploadsEnabled = true
	s.Config.UploadsBucket = "uploads"
	s.Config.OriginalsBucket = "originals"
	s.Config.PlaybackBucket = "playback"
	s.Config.MediaProbePath = binary
	s.Config.FFmpegPath = ffmpeg
	client := s3.NewFromConfig(aws.Config{Region: "local", Credentials: credentials.NewStaticCredentialsProvider("test", "test", "")}, func(o *s3.Options) { o.BaseEndpoint = aws.String(storage.URL); o.UsePathStyle = true })
	s.Media = &Media{s3: client, presign: s3.NewPresignClient(client)}
	w := securityRequest(s, "POST", "/video-uploads", token, map[string]any{"filename": "story.mp4", "content_type": "video/mp4", "bytes": len(data), "checksum": hex.EncodeToString(checksum[:]), "rights_confirmed": true, "idempotency_key": "fixture-flow-key"})
	if w.Code != 200 {
		t.Fatalf("init %d %s", w.Code, w.Body.String())
	}
	var upload VideoUpload
	json.Unmarshal(w.Body.Bytes(), &upload)
	w = securityRequest(s, "POST", "/video-uploads/"+upload.ID+"/parts", token, map[string]int{"part_number": 1})
	var grant map[string]any
	json.Unmarshal(w.Body.Bytes(), &grant)
	put, _ := http.NewRequest("PUT", grant["url"].(string), bytes.NewReader(data))
	response, e := http.DefaultClient.Do(put)
	if e != nil {
		t.Fatal(e)
	}
	response.Body.Close()
	w = securityRequest(s, "POST", "/video-uploads/"+upload.ID+"/complete", token, map[string]any{})
	if w.Code != 200 {
		t.Fatalf("complete %d %s", w.Code, w.Body.String())
	}
	worker := &Worker{store: s.Store, cfg: s.Config, media: s.Media}
	if e = worker.validateVideoUpload(ctx, upload.ID); e != nil {
		t.Fatal(e)
	}
	a, e := s.Store.ownedAsset(ctx, u, upload.AssetID)
	if e != nil || a.Status != "ready" || !a.Measured || a.Width != 90 || a.Height != 160 {
		t.Fatalf("asset %+v %v", a, e)
	}
	w = securityRequest(s, "POST", "/publications", token, map[string]any{"source_asset_id": a.ID, "title": "Uploaded Short", "visibility": "public", "audience": "free", "format": "short", "ai_generated": false, "rights_confirmed": true})
	if w.Code != 200 {
		t.Fatalf("publish draft %d %s", w.Code, w.Body.String())
	}
	var video Video
	json.Unmarshal(w.Body.Bytes(), &video)
	if video.JobID != "" || video.Origin != "upload" || video.AIGenerated {
		t.Fatalf("fake generation provenance %+v", video)
	}
	w = securityRequest(s, "GET", "/media-assets/"+a.ID+"/download", token, nil)
	if w.Code != 200 {
		t.Fatalf("download %d %s", w.Code, w.Body.String())
	}
	_, _, foreign := securityTestServer(t)
	if w = securityRequest(s, "GET", "/media-assets/"+a.ID+"/download", foreign, nil); w.Code != 404 {
		t.Fatal("foreign download")
	}
	// A different checksum rejects the same bytes and retains quota until cleanup.
	w = securityRequest(s, "POST", "/video-uploads", token, map[string]any{"filename": "invalid.mp4", "content_type": "video/mp4", "bytes": len(data), "checksum": strings.Repeat("0", 64), "rights_confirmed": true, "idempotency_key": "invalid-fixture-key"})
	if w.Code != 200 {
		t.Fatalf("invalid init %s", w.Body.String())
	}
	var invalid VideoUpload
	json.Unmarshal(w.Body.Bytes(), &invalid)
	w = securityRequest(s, "POST", "/video-uploads/"+invalid.ID+"/parts", token, map[string]int{"part_number": 1})
	json.Unmarshal(w.Body.Bytes(), &grant)
	put, _ = http.NewRequest("PUT", grant["url"].(string), bytes.NewReader(data))
	response, e = http.DefaultClient.Do(put)
	if e != nil {
		t.Fatal(e)
	}
	response.Body.Close()
	w = securityRequest(s, "POST", "/video-uploads/"+invalid.ID+"/complete", token, map[string]any{})
	if w.Code != 200 {
		t.Fatalf("invalid complete %d %s", w.Code, w.Body.String())
	}
	if e = worker.validateVideoUpload(ctx, invalid.ID); e != nil {
		t.Fatal(e)
	}
	rejected, e := s.Store.ownedAsset(ctx, u, invalid.AssetID)
	if e != nil || rejected.Status != "rejected" {
		t.Fatalf("checksum accepted %+v %v", rejected, e)
	}
	var released bool
	s.Store.DB.QueryRowContext(ctx, `SELECT quota_released FROM cloud_video_uploads WHERE id=$1`, invalid.ID).Scan(&released)
	if released {
		t.Fatal("quota released before cleanup")
	}
	if e = worker.cleanupVideoUploads(ctx); e != nil {
		t.Fatal(e)
	}
	s.Store.DB.QueryRowContext(ctx, `SELECT quota_released FROM cloud_video_uploads WHERE id=$1`, invalid.ID).Scan(&released)
	if !released {
		t.Fatal("cleanup did not release quota")
	}

}
