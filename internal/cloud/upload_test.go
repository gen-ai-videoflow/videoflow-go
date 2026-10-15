package cloud

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"sync"
	"testing"
)

func TestUploadPostEnforcesSize(t *testing.T) {
	client := s3.NewFromConfig(aws.Config{Region: "ap-southeast-1", Credentials: credentials.NewStaticCredentialsProvider("test", "test", "")})
	m := &Media{cfg: Config{UploadsBucket: "uploads"}, s3: client, presign: s3.NewPresignClient(client)}
	grant, err := m.UploadPostGrant(context.Background(), "workspace", "video.mp4", "video/mp4", 1234)
	if err != nil {
		t.Fatal(err)
	}
	fields := grant["fields"].(map[string]string)
	data, err := base64.StdEncoding.DecodeString(fields["policy"])
	if err != nil {
		t.Fatal(err)
	}
	var policy struct {
		Conditions []json.RawMessage `json:"conditions"`
	}
	if err = json.Unmarshal(data, &policy); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, condition := range policy.Conditions {
		var c []any
		if json.Unmarshal(condition, &c) == nil && len(c) == 3 && c[0] == "content-length-range" {
			found = c[1] == float64(1) && c[2] == float64(1234)
		}
	}
	if !found {
		t.Fatalf("missing size condition: %s", data)
	}
	if fields["Content-Type"] != "video/mp4" {
		t.Fatal("missing signed content type")
	}
	if _, err = m.UploadPostGrant(context.Background(), "workspace", "video.mp4", "video/mp4", MaxUploadBytes+1); err == nil {
		t.Fatal("oversize upload accepted")
	}
}
func TestUploadReservationQuota(t *testing.T) {
	s, u := testPolicyStore(t)
	ctx := context.Background()
	for i := 0; i < 9; i++ {
		if err := s.ReserveUpload(ctx, u.WorkspaceID, u.WorkspaceID+"/key"+string(rune('a'+i)), MaxUploadBytes); err != nil {
			t.Fatal(err)
		}
	}
	var group sync.WaitGroup
	results := make(chan error, 2)
	for _, suffix := range []string{"race-a", "race-b"} {
		group.Add(1)
		go func(suffix string) {
			defer group.Done()
			results <- s.ReserveUpload(ctx, u.WorkspaceID, u.WorkspaceID+"/"+suffix, MaxUploadBytes)
		}(suffix)
	}
	group.Wait()
	close(results)
	accepted := 0
	rejected := 0
	for err := range results {
		if err == nil {
			accepted++
		} else if errors.Is(err, ErrConflict) {
			rejected++
		} else {
			t.Fatal(err)
		}
	}
	if accepted != 1 || rejected != 1 {
		t.Fatalf("concurrent quota accepted=%d rejected=%d", accepted, rejected)
	}
	if err := s.ReserveUpload(ctx, u.WorkspaceID, u.WorkspaceID+"/overflow", 1); err == nil {
		t.Fatal("exceeded workspace quota")
	}
}

func TestRejectedPublicationCannotBecomeReadyFromManifest(t *testing.T) {
	s, u := testPolicyStore(t)
	ctx := context.Background()
	p, err := s.CreateProject(ctx, u, "Moderation test", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.GrantCredits(ctx, u.WorkspaceID, "test-publish", 10, ""); err != nil {
		t.Fatal(err)
	}
	q, err := s.CreateQuote(ctx, u, QuoteInput{ProjectID: p.ID, Prompt: "test", Duration: 5, AspectRatio: "16:9"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	j, err := s.SubmitJob(ctx, u, q.ID, "publish-guard-key")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.FinishJob(ctx, j.ID, "succeeded", "output.mp4", ""); err != nil {
		t.Fatal(err)
	}
	v, err := s.CreateVideo(ctx, u, Video{JobID: j.ID, Title: "Test", Visibility: "public", Audience: "free"})
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"encoding_failed", "encoding_reconciliation_required", "draft"} {
		if _, err = s.DB.ExecContext(ctx, `UPDATE cloud_videos SET status=$2,manifest_key='unready.m3u8' WHERE id=$1`, v.ID, status); err != nil {
			t.Fatal(err)
		}
		if err = s.PublishVideo(ctx, u, v.ID); err == nil {
			t.Fatalf("published incomplete %s", status)
		}
	}
	for _, status := range []string{"blocked", "deleted"} {
		v.Status = status
		if s.CanWatch(ctx, u, v) {
			t.Fatalf("owner can watch %s", status)
		}
	}
	if _, err = s.IssueAccountToken(ctx, "missing-"+u.Email, "reset_password"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing account error=%v", err)
	}
}
