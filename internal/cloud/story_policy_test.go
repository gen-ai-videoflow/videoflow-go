package cloud

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestCheapestStoryPolicyUnderstandsPricingAndBounds(t *testing.T) {
	raw := `[{"id":"approved/a","context_length":32768,"architecture":{"input_modalities":["text"],"output_modalities":["text"]},"supported_parameters":["max_tokens","temperature"],"pricing":{"prompt":"0.000002","completion":"0.000004","request":"0"}},{"id":"approved/b","context_length":32768,"architecture":{"input_modalities":["text"],"output_modalities":["text"]},"supported_parameters":["max_tokens","temperature"],"pricing":{"prompt":"0.000001","completion":"0.000002"}}]`
	var models []StoryCatalogModel
	if err := json.Unmarshal([]byte(raw), &models); err != nil {
		t.Fatal(err)
	}
	id, err := cheapestStoryModel(models, "approved/a, approved/b", "雨后的城市", 100000)
	if err != nil || id != "approved/b" {
		t.Fatalf("expected cheaper approved model got %q %v", id, err)
	}
	if _, err = cheapestStoryModel(models, "unapproved/model", "idea", 100000); err == nil {
		t.Fatal("unapproved model selected")
	}
	if _, err = cheapestStoryModel(models, "approved/a,approved/b", "idea", 1); err == nil {
		t.Fatal("budget overflow accepted")
	}
	for _, change := range []func(*StoryCatalogModel){func(m *StoryCatalogModel) { m.Pricing["completion"] = "unknown" }, func(m *StoryCatalogModel) { m.Pricing["request"] = "1" }, func(m *StoryCatalogModel) { m.ContextLength = 10 }, func(m *StoryCatalogModel) { m.SupportedParameters = nil }, func(m *StoryCatalogModel) { m.Pricing["prompt"] = "0" }} {
		var fresh []StoryCatalogModel
		json.Unmarshal([]byte(raw), &fresh)
		change(&fresh[1])
		id, err = cheapestStoryModel(fresh, "approved/b", "idea", 100000)
		if err == nil {
			t.Fatalf("unsupported pricing/capability selected %s", id)
		}
	}
}

func TestCheapestStoryCatalogRefreshIsBoundedAndCached(t *testing.T) {
	store, _ := testPlatform(t)
	store.DB.Exec(`DELETE FROM cloud_story_catalog`)
	server := NewServer(store, Config{OpenRouterKey: "test", StoryModel: "reviewed/story", StoryMaxCostMicros: 100000})
	defer server.Close()
	calls := 0
	server.HTTP = &http.Client{Transport: commerceRoundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.String() != "https://openrouter.ai/api/v1/models" {
			t.Fatal("wrong catalog endpoint")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"data":[{"id":"reviewed/story","context_length":32768,"architecture":{"input_modalities":["text"],"output_modalities":["text"]},"supported_parameters":["max_tokens","temperature"],"pricing":{"prompt":"0.000001","completion":"0.000002"}}]}`))}, nil
	})}
	for _, idea := range []string{"An English screenplay", "一段中文剧本"} {
		id, err := server.selectStoryModel(context.Background(), idea)
		if err != nil || id != "reviewed/story" {
			t.Fatalf("selected %q %v", id, err)
		}
	}
	if calls != 1 {
		t.Fatalf("catalog refreshed %d times", calls)
	}
}
