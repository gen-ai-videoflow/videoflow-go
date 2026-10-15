package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestVideoUploadGatesOwnershipAndSignedParts(t *testing.T) {
	s, u, token := securityTestServer(t)
	body := map[string]any{"filename": "story.mp4", "content_type": "video/mp4", "bytes": 1024, "checksum": strings.Repeat("a", 64), "rights_confirmed": true, "idempotency_key": "upload-test-key"}
	if w := securityRequest(s, "POST", "/video-uploads", token, body); w.Code != 503 {
		t.Fatalf("disabled upload %d", w.Code)
	}
	_, e := s.Store.DB.ExecContext(context.Background(), `UPDATE cloud_users SET verified=true WHERE id=$1`, u.ID)
	if e != nil {
		t.Fatal(e)
	}
	s.Config.VideoUploadsEnabled = true
	s.Config.UploadsBucket = "uploads"
	s.Config.OriginalsBucket = "originals"
	client := s3.NewFromConfig(aws.Config{Region: "local", Credentials: credentials.NewStaticCredentialsProvider("test", "test", ""), HTTPClient: &http.Client{Transport: commerceRoundTrip(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/xml"}}, Body: io.NopCloser(bytes.NewBufferString(`<InitiateMultipartUploadResult><Bucket>uploads</Bucket><Key>x</Key><UploadId>storage-id</UploadId></InitiateMultipartUploadResult>`))}, nil
	})}}, func(o *s3.Options) { o.BaseEndpoint = aws.String("http://localhost:4566"); o.UsePathStyle = true })
	s.Media = &Media{s3: client, presign: s3.NewPresignClient(client)}
	w := securityRequest(s, "POST", "/video-uploads", token, body)
	if w.Code != 200 {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	var upload map[string]any
	json.Unmarshal(w.Body.Bytes(), &upload)
	id, _ := upload["id"].(string)
	if id == "" {
		t.Fatal(upload)
	}
	again := securityRequest(s, "POST", "/video-uploads", token, body)
	var repeated map[string]any
	json.Unmarshal(again.Body.Bytes(), &repeated)
	if repeated["id"] != id {
		t.Fatal("upload idempotency", repeated)
	}
	_, _, foreign := securityTestServer(t)
	if w = securityRequest(s, "GET", "/video-uploads/"+id, foreign, nil); w.Code != 404 {
		t.Fatalf("foreign upload %d", w.Code)
	}
	w = securityRequest(s, "POST", "/video-uploads/"+id+"/parts", token, map[string]int{"part_number": 1})
	if w.Code != 200 {
		t.Fatalf("part %d %s", w.Code, w.Body.String())
	}
	var part map[string]any
	json.Unmarshal(w.Body.Bytes(), &part)
	url, _ := part["url"].(string)
	if !strings.Contains(url, "content-length") {
		t.Fatalf("part length is not signed")
	}
	if w = securityRequest(s, "POST", "/video-uploads/"+id+"/parts", token, map[string]int{"part_number": 2}); w.Code != 400 {
		t.Fatal("unbounded part")
	}
}

func TestLegacyUploadCountsVideoQuota(t *testing.T) {
	s, u, _ := securityTestServer(t)
	ctx := context.Background()
	asset, id := uuid.NewString(), uuid.NewString()
	_, e := s.Store.DB.ExecContext(ctx, `INSERT INTO cloud_media_assets(id,workspace_id,origin,object_key,status) VALUES($1,$2,'upload',$3,'ready')`, asset, u.WorkspaceID, "workspaces/"+u.WorkspaceID+"/videos/"+id+"/source.mp4")
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.Store.DB.ExecContext(ctx, `INSERT INTO cloud_video_uploads(id,asset_id,workspace_id,object_key,bytes,checksum,filename,content_type,state,idempotency_key,fingerprint) VALUES($1,$2,$3,$4,$5,'a','x.mp4','video/mp4','ready',$1,'a')`, id, asset, u.WorkspaceID, "workspaces/"+u.WorkspaceID+"/video-uploads/"+id+"/source.mp4", MaxUploadBytes)
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 9; i++ {
		if e = s.Store.ReserveUpload(ctx, u.WorkspaceID, "workspaces/"+u.WorkspaceID+"/uploads/"+uuid.NewString()+".mp4", MaxUploadBytes); e != nil {
			t.Fatal(e)
		}
	}
	if e = s.Store.ReserveUpload(ctx, u.WorkspaceID, "workspaces/"+u.WorkspaceID+"/uploads/"+uuid.NewString()+".mp4", 1); e != ErrConflict {
		t.Fatalf("combined quota accepted: %v", e)
	}
}

func TestMultipartCreationSerializesCancellation(t *testing.T) {
	s, u, token := securityTestServer(t)
	_, e := s.Store.DB.ExecContext(context.Background(), `UPDATE cloud_users SET verified=true WHERE id=$1`, u.ID)
	if e != nil {
		t.Fatal(e)
	}
	s.Config.VideoUploadsEnabled = true
	s.Config.UploadsBucket = "uploads"
	s.Config.OriginalsBucket = "originals"
	started, release := make(chan struct{}), make(chan struct{})
	client := s3.NewFromConfig(aws.Config{Region: "local", Credentials: credentials.NewStaticCredentialsProvider("test", "test", ""), HTTPClient: &http.Client{Transport: commerceRoundTrip(func(r *http.Request) (*http.Response, error) {
		close(started)
		<-release
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`<InitiateMultipartUploadResult><UploadId>storage-id</UploadId></InitiateMultipartUploadResult>`))}, nil
	})}}, func(o *s3.Options) { o.BaseEndpoint = aws.String("http://localhost:4566"); o.UsePathStyle = true })
	s.Media = &Media{s3: client, presign: s3.NewPresignClient(client)}
	done := make(chan int, 1)
	go func() {
		done <- securityRequest(s, "POST", "/video-uploads", token, map[string]any{"filename": "x.mp4", "content_type": "video/mp4", "bytes": 1024, "checksum": strings.Repeat("a", 64), "rights_confirmed": true, "idempotency_key": "race-upload-key"}).Code
	}()
	<-started
	var id string
	if e = s.Store.DB.QueryRow(`SELECT id FROM cloud_video_uploads WHERE workspace_id=$1`, u.WorkspaceID).Scan(&id); e != nil {
		t.Fatal(e)
	}
	cancelled := make(chan int, 1)
	go func() { cancelled <- securityRequest(s, "DELETE", "/video-uploads/"+id, token, nil).Code }()
	select {
	case code := <-cancelled:
		close(release)
		<-done
		t.Fatalf("cancellation escaped creation lock: %d", code)
	case <-time.After(150 * time.Millisecond):
	}
	close(release)
	if code := <-done; code != 200 {
		t.Fatalf("create %d", code)
	}
	if code := <-cancelled; code != 200 {
		t.Fatalf("cancel %d", code)
	}
	var state, storage string
	if e = s.Store.DB.QueryRow(`SELECT state,storage_upload_id FROM cloud_video_uploads WHERE id=$1`, id).Scan(&state, &storage); e != nil {
		t.Fatal(e)
	}
	if state != "aborting" || storage != "storage-id" {
		t.Fatalf("lost remote upload: %s %s", state, storage)
	}
}

func TestUncertainMultipartCreationRetainsQuota(t *testing.T) {
	s, u, _ := securityTestServer(t)
	ctx := context.Background()
	asset, id := uuid.NewString(), uuid.NewString()
	_, e := s.Store.DB.ExecContext(ctx, `INSERT INTO cloud_media_assets(id,workspace_id,origin,object_key,status)VALUES($1,$2,'upload',$3,'validating')`, asset, u.WorkspaceID, "workspaces/"+u.WorkspaceID+"/videos/"+id+"/source.mp4")
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.Store.DB.ExecContext(ctx, `INSERT INTO cloud_video_uploads(id,asset_id,workspace_id,object_key,bytes,checksum,filename,content_type,state,idempotency_key,fingerprint)VALUES($1,$2,$3,$4,1024,'a','x.mp4','video/mp4','aborting',$1,'a')`, id, asset, u.WorkspaceID, "workspaces/"+u.WorkspaceID+"/video-uploads/"+id+"/source.mp4")
	if e != nil {
		t.Fatal(e)
	}
	calls := 0
	client := s3.NewFromConfig(aws.Config{Region: "local", Credentials: credentials.NewStaticCredentialsProvider("test", "test", ""), HTTPClient: &http.Client{Transport: commerceRoundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "GET" || !r.URL.Query().Has("uploads") {
			t.Errorf("unexpected cleanup mutation %s", r.Method)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`<ListMultipartUploadsResult><IsTruncated>false</IsTruncated></ListMultipartUploadsResult>`))}, nil
	})}}, func(o *s3.Options) { o.BaseEndpoint = aws.String("http://localhost:4566"); o.UsePathStyle = true })
	worker := &Worker{store: s.Store, media: &Media{s3: client}, cfg: Config{UploadsBucket: "uploads", OriginalsBucket: "originals"}}
	if e = worker.cleanupOneVideoUpload(ctx, id); e != ErrUnavailable {
		t.Fatalf("unresolved outcome: %v", e)
	}
	var released bool
	if e = s.Store.DB.QueryRow(`SELECT quota_released FROM cloud_video_uploads WHERE id=$1`, id).Scan(&released); e != nil {
		t.Fatal(e)
	}
	if released || calls != 1 {
		t.Fatalf("uncertain creation released quota: %v, calls=%d", released, calls)
	}
}
