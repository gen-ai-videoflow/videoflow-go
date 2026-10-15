package cloud

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPipelineStoryValidation(t *testing.T) {
	var story PipelineStory
	if err := json.Unmarshal([]byte(`{"script":"A journey","scenes":[{"title":"Arrival","prompt":"Camera enters room"},{"title":"Exit","prompt":"Camera leaves room"}]}`), &story); err != nil {
		t.Fatal(err)
	}
	if err := validatePipelineStory(story, 2); err != nil {
		t.Fatal(err)
	}
	if err := validatePipelineStory(story, 3); err == nil {
		t.Fatal("wrong clip count accepted")
	}
	story.Scenes[0].Prompt = ""
	if err := validatePipelineStory(story, 2); err == nil {
		t.Fatal("empty prompt accepted")
	}
}
func TestAssemblyQuoteIsScopedAndFrozen(t *testing.T) {
	s, u := testPolicyStore(t)
	ctx := context.Background()
	p, err := s.CreateProject(ctx, u, "Assembly", "")
	if err != nil {
		t.Fatal(err)
	}
	p2, err := s.CreateProject(ctx, u, "Other", "")
	if err != nil {
		t.Fatal(err)
	}
	s.GrantCredits(ctx, u.WorkspaceID, "assembly-test", 50, "")
	q, err := s.CreateQuote(ctx, u, QuoteInput{ProjectID: p.ID, Prompt: "test", Duration: 5, AspectRatio: "16:9"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	j, err := s.SubmitJob(ctx, u, q.ID, "assembly-source-key")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.FinishJob(ctx, j.ID, "succeeded", "source.mp4", ""); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreateAssemblyQuote(ctx, u, p2.ID, []string{j.ID}, 10); err == nil {
		t.Fatal("cross-project source allowed")
	}
	made, err := s.CreateAssemblyQuote(ctx, u, p.ID, []string{j.ID}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if made.Credits != 10 {
		t.Fatal("assembly price incorrect")
	}
	if _, err = s.PipelineState(ctx, u, "missing"); err == nil {
		t.Fatal("missing pipeline readable")
	}
}

func TestPipelineDurableAdmissionAndAmbiguity(t *testing.T) {
	s, u := testPolicyStore(t)
	ctx := context.Background()
	p, err := s.CreateProject(ctx, u, "Pipeline", "")
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{StoryModel: "reviewed/story", StoryCredits: 2, StoryMaxCostMicros: 5000, ProviderMaxCostMicros: 25000}
	policy := VideoPolicy{Model: "reviewed/video", CreditsPerSecond: 1, ProviderMaxCostMicros: 10000, Version: "test", ExpiresAt: time.Now().Add(time.Hour), Resolution: "720p"}
	catalog := `[{"id":"reviewed/video","supported_durations":[5],"supported_resolutions":["720p"],"supported_aspect_ratios":["16:9"],"pricing_skus":{"duration_seconds_720p":"0.001"}}]`
	if _, err = s.DB.ExecContext(ctx, `INSERT INTO cloud_video_catalog(id,catalog,refreshed_at)VALUES(true,$1,now())ON CONFLICT(id)DO UPDATE SET catalog=cloud_video_catalog.catalog||excluded.catalog,refreshed_at=now()`, catalog); err != nil {
		t.Fatal(err)
	}
	q, err := s.CreatePipelineQuote(ctx, u, p.ID, PipelineQuoteInput{Idea: "A journey", Clips: 2, ClipDuration: 5, AspectRatio: "16:9"}, policy, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if q.Credits != 12 {
		t.Fatalf("credits=%d", q.Credits)
	}
	if err = s.GrantCredits(ctx, u.WorkspaceID, "pipeline-test", 50, ""); err != nil {
		t.Fatal(err)
	}
	j, err := s.SubmitJob(ctx, u, q.ID, "pipeline-submit-key")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.ExecContext(ctx, `UPDATE cloud_jobs SET status='running',attempt=1,lease_until=now()+interval '5 minutes' WHERE id=$1`, j.ID); err != nil {
		t.Fatal(err)
	}
	worker := &Worker{store: s, cfg: Config{ProviderDailyBudgetMicros: 1000000000000, ProviderMaxCostMicros: 25000}}
	if err = s.TryProviderBudget(ctx, j.ID, 25000, 1000000000000); err != nil {
		t.Fatal(err)
	}
	if err = worker.pipelineSubmission(ctx, j.ID, 1, q.ID, -1); err != nil {
		t.Fatal(err)
	}
	if err = worker.pipelineAmbiguous(ctx, j.ID, 1, "test uncertainty"); err != nil {
		t.Fatal(err)
	}
	state, err := s.PipelineState(ctx, u, j.ID)
	if err != nil || state.Status != "reconciliation_required" {
		t.Fatalf("ambiguous state %+v %v", state, err)
	}
	balance, err := s.Credits(ctx, u.WorkspaceID)
	if err != nil || balance.Reserved != 12 {
		t.Fatalf("lost reservation %+v %v", balance, err)
	}
	if err = worker.runPipeline(ctx, j.ID); err != nil {
		t.Fatal(err)
	}
	got, err := s.Job(ctx, u, j.ID)
	if err != nil || got.Status != "reconciliation_required" {
		t.Fatal("ambiguous pipeline automatically retried")
	}
	if _, err = s.DB.ExecContext(ctx, `UPDATE cloud_jobs SET status='running',attempt=2,lease_until=now()+interval '5 minutes' WHERE id=$1`, j.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.ExecContext(ctx, `INSERT INTO cloud_pipeline_clips(quote_id,position,prompt,title,status)VALUES($1,0,'test','test','submitting')`, q.ID); err != nil {
		t.Fatal(err)
	}
	if err = worker.pipelineRecoverClip(ctx, j.ID, 1, q.ID, 0, "stale-provider"); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale recovery accepted: %v", err)
	}
	if err = worker.pipelineAssemblyStage(ctx, j.ID, 1, q.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale assembly transition accepted: %v", err)
	}
	var clipStatus, clipProvider, stage string
	if err = s.DB.QueryRowContext(ctx, `SELECT status,provider_id FROM cloud_pipeline_clips WHERE quote_id=$1`, q.ID).Scan(&clipStatus, &clipProvider); err != nil || clipStatus != "submitting" || clipProvider != "" {
		t.Fatalf("stale recovery mutated state: %s %s %v", clipStatus, clipProvider, err)
	}
	if err = s.DB.QueryRowContext(ctx, `SELECT stage FROM cloud_pipeline_plans WHERE quote_id=$1`, q.ID).Scan(&stage); err != nil || stage != "story_submitting" {
		t.Fatalf("stale assembly mutated stage %s %v", stage, err)
	}
	if err = worker.pipelineSubmission(ctx, j.ID, 2, q.ID, -1); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate screenplay submission accepted: %v", err)
	}
	// Simulate a delayed pipeline crossing midnight. New paid stages need new
	// daily funding, while the original admission remains a durable prior-day fact.
	if _, err = s.DB.ExecContext(ctx, `INSERT INTO cloud_provider_budget_days(day)VALUES((now() AT TIME ZONE 'UTC')::date-1)ON CONFLICT DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.ExecContext(ctx, `UPDATE cloud_provider_admissions SET day=(now() AT TIME ZONE 'UTC')::date-1 WHERE job_id=$1`, j.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.ExecContext(ctx, `DELETE FROM cloud_pipeline_budget_days WHERE job_id=$1`, j.ID); err != nil {
		t.Fatal(err)
	}
	var before int64
	if err = s.DB.QueryRowContext(ctx, `SELECT reserved_micros FROM cloud_provider_budget_days WHERE day=(now() AT TIME ZONE 'UTC')::date`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = pipelineFence(ctx, tx, j.ID, 2); err != nil {
		t.Fatal(err)
	}
	if err = admitPipelineDayTx(ctx, tx, j.ID, before+24999, 25000); !errors.Is(err, ErrProviderBudget) {
		t.Fatalf("rollover bypassed daily ceiling: %v", err)
	}
	tx.Rollback()
	tx, err = s.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = pipelineFence(ctx, tx, j.ID, 2); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err = admitPipelineDayTx(ctx, tx, j.ID, before+25000, 25000); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var after int64
	if err = s.DB.QueryRowContext(ctx, `SELECT reserved_micros FROM cloud_provider_budget_days WHERE day=(now() AT TIME ZONE 'UTC')::date`).Scan(&after); err != nil || after != before+25000 {
		t.Fatalf("daily admission replay before=%d after=%d %v", before, after, err)
	}

}

func TestStoryProviderBudgetBound(t *testing.T) {
	limits := storyProviderLimits(10000)
	price := limits["max_price"].(map[string]any)
	if price["prompt"].(float64)*18432 > 10000 || price["request"] != 0 || limits["sort"] != "price" || limits["require_parameters"] != true {
		t.Fatalf("unbounded routing policy %+v", limits)
	}
}

func TestRealFFmpegAssemblyMixedAudioAndDuration(t *testing.T) {
	binaryPath := os.Getenv("PIPELINE_FFMPEG_TEST_BINARY")
	if binaryPath == "" {
		t.Skip("set PIPELINE_FFMPEG_TEST_BINARY to run real FFmpeg integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	dir := t.TempDir()
	silent := filepath.Join(dir, "silent.mp4")
	audible := filepath.Join(dir, "audible.mp4")
	for _, item := range []struct {
		path  string
		audio bool
	}{{silent, false}, {audible, true}} {
		args := []string{"-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "color=c=red:s=160x90:r=24"}
		if item.audio {
			args = append(args, "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000")
		}
		args = append(args, "-t", "1", "-c:v", "libx264", "-threads", "2", "-pix_fmt", "yuv420p")
		if item.audio {
			args = append(args, "-c:a", "aac")
		}
		args = append(args, item.path)
		if err := runFFmpeg(ctx, binaryPath, dir, args); err != nil {
			t.Fatal(err)
		}
	}
	first, err := os.ReadFile(silent)
	if err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(audible)
	if err != nil {
		t.Fatal(err)
	}
	var output []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			if strings.HasSuffix(r.URL.Path, "first.mp4") {
				w.Write(first)
			} else if strings.HasSuffix(r.URL.Path, "second.mp4") {
				w.Write(second)
			} else {
				http.NotFound(w, r)
			}
		} else if r.Method == "PUT" {
			output, _ = io.ReadAll(r.Body)
			w.WriteHeader(200)
		} else {
			w.WriteHeader(405)
		}
	}))
	defer server.Close()
	client := s3.NewFromConfig(aws.Config{Region: "ap-southeast-1", Credentials: credentials.NewStaticCredentialsProvider("test", "test", "")}, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(server.URL)
		o.UsePathStyle = true
		o.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
	})
	worker := &Worker{cfg: Config{OriginalsBucket: "originals", FFmpegPath: binaryPath}, media: &Media{s3: client}}
	key, err := worker.assemblePipeline(ctx, "workspace", "job", 1, "9:16", 1, []string{"first.mp4", "second.mp4"})
	if err != nil {
		t.Fatal(err)
	}
	if key == "" || len(output) == 0 {
		t.Fatal("assembly output missing")
	}
	audio, err := mp4HasAudio(output, 0)
	if err != nil || !audio {
		t.Fatalf("assembled audio: %v %v", audio, err)
	}
	duration, ok := testMP4Duration(output)
	if !ok || duration < 0.9 || duration > 1.2 {
		t.Fatalf("output duration %f, valid=%v", duration, ok)
	}
}
func testMP4Duration(data []byte) (float64, bool) {
	for len(data) >= 8 {
		size := int(binary.BigEndian.Uint32(data))
		if size < 8 || size > len(data) {
			return 0, false
		}
		body := data[8:size]
		kind := string(data[4:8])
		if kind == "moov" {
			return testMP4Duration(body)
		}
		if kind == "mvhd" && len(body) >= 20 && body[0] == 0 {
			scale := binary.BigEndian.Uint32(body[12:16])
			if scale == 0 {
				return 0, false
			}
			return float64(binary.BigEndian.Uint32(body[16:20])) / float64(scale), true
		}
		data = data[size:]
	}
	return 0, false
}

func TestScreenplayRequestEnforcesPriceAndOutputBounds(t *testing.T) {
	worker := &Worker{cfg: Config{OpenRouterKey: "test"}, storyHTTP: &http.Client{Transport: adminTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://openrouter.ai/api/v1/chat/completions" {
			t.Fatal("unexpected endpoint")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["max_tokens"] != float64(2048) {
			t.Fatal("unbounded text output")
		}
		provider := body["provider"].(map[string]any)
		if provider["sort"] != "price" || provider["require_parameters"] != true {
			t.Fatal("routing requirements missing")
		}
		prices := provider["max_price"].(map[string]any)
		if prices["prompt"].(float64)*18432 > 10000 || prices["request"] != float64(0) {
			t.Fatal("price bound missing")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"{\"script\":\"A journey\",\"scenes\":[{\"title\":\"Arrival\",\"prompt\":\"A traveler arrives\"}]}"}}]}`))}, nil
	})}}
	story, err := worker.generateStory(context.Background(), "reviewed/story", "A journey", 1, 5, 10000)
	if err != nil || validatePipelineStory(story, 1) != nil {
		t.Fatalf("screenplay %+v %v", story, err)
	}
}

func TestVideoSubmissionDayRolloverFenced(t *testing.T) {
	s, u := testPolicyStore(t)
	ctx := context.Background()
	p, err := s.CreateProject(ctx, u, "Day rollover", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.GrantCredits(ctx, u.WorkspaceID, "video-rollover", 20, ""); err != nil {
		t.Fatal(err)
	}
	q, err := s.CreateQuote(ctx, u, QuoteInput{ProjectID: p.ID, Prompt: "test", Duration: 5, AspectRatio: "16:9"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.ExecContext(ctx, `UPDATE cloud_quotes SET provider_max_cost_micros=50 WHERE id=$1`, q.ID); err != nil {
		t.Fatal(err)
	}
	j, err := s.SubmitJob(ctx, u, q.ID, "rollover-video-key")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.TryProviderBudget(ctx, j.ID, 50, 1000000000000); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.ExecContext(ctx, `INSERT INTO cloud_provider_budget_days(day)VALUES((now() AT TIME ZONE 'UTC')::date-1)ON CONFLICT DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.ExecContext(ctx, `UPDATE cloud_provider_admissions SET day=(now() AT TIME ZONE 'UTC')::date-1 WHERE job_id=$1`, j.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.ExecContext(ctx, `UPDATE cloud_jobs SET status='running',attempt=2,lease_until=now()+interval '5 minutes' WHERE id=$1`, j.ID); err != nil {
		t.Fatal(err)
	}
	var baseline int64
	if err = s.DB.QueryRowContext(ctx, `SELECT reserved_micros FROM cloud_provider_budget_days WHERE day=(now() AT TIME ZONE 'UTC')::date`).Scan(&baseline); err != nil {
		t.Fatal(err)
	}
	worker := &Worker{store: s, cfg: Config{ProviderDailyBudgetMicros: baseline + 49, ProviderMaxCostMicros: 50}}
	if err = worker.claimVideoSubmission(ctx, j.ID, 1); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale video submit accepted: %v", err)
	}
	if err = worker.claimVideoSubmission(ctx, j.ID, 2); !errors.Is(err, ErrProviderBudget) {
		t.Fatalf("video rollover bypassed ceiling: %v", err)
	}
	got, err := s.Job(ctx, u, j.ID)
	if err != nil || got.Status != "running" {
		t.Fatalf("failed claim mutated status: %+v %v", got, err)
	}
	worker.cfg.ProviderDailyBudgetMicros = baseline + 50
	if err = worker.claimVideoSubmission(ctx, j.ID, 2); err != nil {
		t.Fatal(err)
	}
	if err = worker.claimVideoSubmission(ctx, j.ID, 2); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate submit intent accepted: %v", err)
	}
	var after int64
	if err = s.DB.QueryRowContext(ctx, `SELECT reserved_micros FROM cloud_provider_budget_days WHERE day=(now() AT TIME ZONE 'UTC')::date`).Scan(&after); err != nil || after != baseline+50 {
		t.Fatalf("incorrect renewed budget %d -> %d %v", baseline, after, err)
	}
}
