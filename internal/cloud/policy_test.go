package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"os"
	"strings"
	"sync"
	"testing"
)

func TestVideoPriceSelection(t *testing.T) {
	raw := `[{"id":"cheap-free","supported_durations":[5],"supported_resolutions":["720p","1080p"],"supported_aspect_ratios":["16:9"],"pricing_skus":{"per-video-second":"0"}},{"id":"eligible","supported_durations":[5],"supported_resolutions":["720p","1080p"],"supported_aspect_ratios":["16:9"],"pricing_skus":{"per-video-second":"0.025","per-video-second-1080p":"0.1"}},{"id":"wrong-duration","supported_durations":[6],"supported_resolutions":["720p","1080p"],"supported_aspect_ratios":["16:9"],"pricing_skus":{"per-video-second":"0.001"}}]`
	var models []VideoCatalogModel
	if err := json.Unmarshal([]byte(raw), &models); err != nil {
		t.Fatal(err)
	}
	if _, err := videoPriceMicros(models[0], 5, "16:9", "720p"); err == nil {
		t.Fatal("free model eligible")
	}
	cost, err := videoPriceMicros(models[1], 5, "16:9", "720p")
	if err != nil || cost != 125000 {
		t.Fatalf("cost=%d err=%v", cost, err)
	}
	if _, err = videoPriceMicros(models[2], 5, "16:9", "720p"); err == nil {
		t.Fatal("unsupported duration eligible")
	}
	models[1].PricingSKUs["mystery-audio"] = "0.2"
	if _, err = videoPriceMicros(models[1], 5, "16:9", "720p"); err == nil {
		t.Fatal("unknown extra pricing eligible")
	}
}

func TestProviderBudgetConcurrentAdmission(t *testing.T) {
	s, u := testPolicyStore(t)
	ctx := context.Background()
	p, err := s.CreateProject(ctx, u, "Budget", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.GrantCredits(ctx, u.WorkspaceID, "budget-test", 100, ""); err != nil {
		t.Fatal(err)
	}
	jobs := []string{}
	for _, key := range []string{"budget-key-one", "budget-key-two"} {
		q, e := s.CreateQuote(ctx, u, QuoteInput{ProjectID: p.ID, Prompt: "Budget test", Duration: 5, AspectRatio: "16:9"}, 1)
		if e != nil {
			t.Fatal(e)
		}
		j, e := s.SubmitJob(ctx, u, q.ID, key)
		if e != nil {
			t.Fatal(e)
		}
		jobs = append(jobs, j.ID)
	}
	var baseline int64
	err = s.DB.QueryRowContext(ctx, `SELECT COALESCE((SELECT reserved_micros FROM cloud_provider_budget_days WHERE day=(now() AT TIME ZONE 'UTC')::date),0)`).Scan(&baseline)
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, id := range jobs {
		wg.Add(1)
		go func(id string) { defer wg.Done(); results <- s.TryProviderBudget(ctx, id, 100, baseline+100) }(id)
	}
	wg.Wait()
	close(results)
	accepted, rejected := 0, 0
	for e := range results {
		if e == nil {
			accepted++
		} else if errors.Is(e, ErrProviderBudget) {
			rejected++
		} else {
			t.Fatal(e)
		}
	}
	if accepted != 1 || rejected != 1 {
		t.Fatalf("accepted=%d rejected=%d", accepted, rejected)
	}
	var admitted string
	if err = s.DB.QueryRowContext(ctx, `SELECT job_id FROM cloud_provider_admissions WHERE job_id IN ($1,$2)`, jobs[0], jobs[1]).Scan(&admitted); err != nil {
		t.Fatal(err)
	}
	if err = s.TryProviderBudget(ctx, admitted, 100, baseline+100); err != nil {
		t.Fatalf("admission replay: %v", err)
	}
	if err = s.TryProviderBudget(ctx, admitted, 101, baseline+200); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed admission accepted: %v", err)
	}
	if err = s.TryProviderBudget(ctx, jobs[0], 0, 100); !errors.Is(err, ErrProviderBudget) {
		t.Fatalf("zero budget enabled: %v", err)
	}
}

func testPolicyStore(t *testing.T) (*Store, User) {
	t.Helper()
	dsn := os.Getenv("CLOUD_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("CLOUDFLOW_TEST_DATABASE_URL")
	}
	if dsn == "" {
		t.Skip("PostgreSQL integration database not configured")
	}
	ctx := context.Background()
	s, err := OpenStore(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err = s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")
	u, err := s.Register(ctx, suffix+"@example.com", "correct horse battery staple", "Policy test", "p"+suffix)
	if err != nil {
		t.Fatal(err)
	}
	return s, u
}

// Faithful public catalog subset captured 2026-10-15: no credentials or URLs.
func TestLiveCatalogPricingFamilies(t *testing.T) {
	const fixture = `[{"id":"x-ai/grok-imagine-video-1.5-lite","supported_durations":[1,2,3,4,5,6,7,8,9,10,11,12,13,14,15],"supported_resolutions":["480p","720p","1080p"],"supported_aspect_ratios":["16:9","9:16","1:1","4:3","3:4","3:2","2:3"],"pricing_skus":{"cents_per_image_input":"1","cents_per_video_output_second_480p":"2","cents_per_video_output_second_720p":"3","cents_per_video_output_second_1080p":"14"}},{"id":"alibaba/wan-3.0","supported_durations":[2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20,21,22,23,24,25,26,27,28,29,30],"supported_resolutions":["480p","720p","1080p"],"supported_aspect_ratios":["16:9","4:3","1:1","3:4","9:16"],"pricing_skus":{"duration_seconds_480p":"0.05","duration_seconds_720p":"0.1","duration_seconds_1080p":"0.2"}},{"id":"runway/gen-4.5","supported_durations":[2,3,4,5,6,7,8,9,10],"supported_resolutions":["720p"],"supported_aspect_ratios":["16:9","9:16"],"pricing_skus":{"cents_per_second_output":"12"}},{"id":"google/veo-3.1-lite","supported_durations":[8,4,6],"supported_resolutions":["720p","1080p"],"supported_aspect_ratios":["16:9","9:16"],"pricing_skus":{"duration_seconds_with_audio":"0.08","duration_seconds_without_audio":"0.05","duration_seconds_with_audio_720p":"0.05","duration_seconds_without_audio_720p":"0.03"}}]`
	var models []VideoCatalogModel
	if err := json.Unmarshal([]byte(fixture), &models); err != nil {
		t.Fatal(err)
	}
	for _, m := range models {
		switch m.ID {
		case "x-ai/grok-imagine-video-1.5-lite":
			cost, err := videoPriceMicros(m, 5, "16:9", "720p")
			if err != nil || cost != 150000 {
				t.Fatalf("grok720 %d %v", cost, err)
			}
			cost, err = videoPriceMicros(m, 5, "16:9", "480p")
			if err != nil || cost != 100000 {
				t.Fatalf("grok480 %d %v", cost, err)
			}
		case "alibaba/wan-3.0":
			cost, err := videoPriceMicros(m, 5, "16:9", "720p")
			if err != nil || cost != 500000 {
				t.Fatalf("wan %d %v", cost, err)
			}
		case "runway/gen-4.5":
			cost, err := videoPriceMicros(m, 5, "16:9", "720p")
			if err != nil || cost != 600000 {
				t.Fatalf("runway %d %v", cost, err)
			}
		case "google/veo-3.1-lite":
			if _, err := videoPriceMicros(m, 4, "16:9", "720p"); err == nil {
				t.Fatal("ambiguous audio pricing eligible")
			}
		}
	}
}

func TestCurrentApprovalListExcludesPreviouslyApprovedModel(t *testing.T) {
	s, _ := testPolicyStore(t)
	ctx := context.Background()
	suffix := uuid.NewString()
	old := "test/old-" + suffix
	current := "test/current-" + suffix
	models := []VideoCatalogModel{{ID: old, Durations: []int{5}, Resolutions: []string{"720p"}, AspectRatios: []string{"16:9"}, PricingSKUs: map[string]string{"duration_seconds_720p": "0.01"}}, {ID: current, Durations: []int{5}, Resolutions: []string{"720p"}, AspectRatios: []string{"16:9"}, PricingSKUs: map[string]string{"duration_seconds_720p": "0.02"}}}
	raw, _ := json.Marshal(models)
	if _, e := s.DB.ExecContext(ctx, `INSERT INTO cloud_video_catalog(id,catalog,refreshed_at)VALUES(true,$1,now())ON CONFLICT(id)DO UPDATE SET catalog=excluded.catalog,refreshed_at=now()`, string(raw)); e != nil {
		t.Fatal(e)
	}
	if e := s.ConfigureVideoPolicy(ctx, old, 10, 1000000); e != nil {
		t.Fatal(e)
	}
	if e := s.ConfigureVideoPolicy(ctx, current, 10, 1000000); e != nil {
		t.Fatal(e)
	}
	p, e := s.ActiveVideoPolicy(ctx, 5, "16:9", current)
	if e != nil || p.Model != current {
		t.Fatalf("old approval selected %+v %v", p, e)
	}
}
