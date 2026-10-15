package cloud

import (
	"encoding/json"
	"sync"
	"testing"
)

func TestPlaybackPreferencesIsolationAndValidation(t *testing.T) {
	s, _, token := securityTestServer(t)
	if w := securityRequest(s, "GET", "/settings/preferences", "", nil); w.Code != 401 {
		t.Fatalf("guest %d", w.Code)
	}
	w := securityRequest(s, "GET", "/settings/preferences", token, nil)
	if w.Code != 200 {
		t.Fatalf("defaults %d", w.Code)
	}
	var p map[string]any
	json.Unmarshal(w.Body.Bytes(), &p)
	if p["autoplay_next"] != false || p["history_enabled"] != true {
		t.Fatalf("defaults %v", p)
	}
	w = securityRequest(s, "PATCH", "/settings/preferences", token, map[string]any{"captions_enabled": true, "history_enabled": false, "caption_language": "en", "reduce_motion": "reduce"})
	if w.Code != 200 {
		t.Fatalf("save %d %s", w.Code, w.Body.String())
	}
	w = securityRequest(s, "GET", "/settings/preferences", token, nil)
	json.Unmarshal(w.Body.Bytes(), &p)
	if p["captions_enabled"] != true || p["history_enabled"] != false {
		t.Fatal(p)
	}
	for _, body := range []map[string]any{{"reduce_motion": "fast"}, {"user_id": "foreign"}, {"caption_language": "../bad"}} {
		if w = securityRequest(s, "PATCH", "/settings/preferences", token, body); w.Code != 400 {
			t.Fatalf("invalid accepted %d", w.Code)
		}
	}
	other, _, otherToken := securityTestServer(t)
	w = securityRequest(other, "GET", "/settings/preferences", otherToken, nil)
	json.Unmarshal(w.Body.Bytes(), &p)
	if p["history_enabled"] != true || p["captions_enabled"] != false {
		t.Fatal("cross-account preferences", p)
	}
}

func TestPreferencesConcurrentPartialUpdates(t *testing.T) {
	s, _, token := securityTestServer(t)
	var wg sync.WaitGroup
	statuses := make(chan int, 2)
	for _, patch := range []map[string]bool{{"data_saving": true}, {"history_enabled": false}} {
		wg.Add(1)
		go func(body map[string]bool) {
			defer wg.Done()
			statuses <- securityRequest(s, "PATCH", "/settings/preferences", token, body).Code
		}(patch)
	}
	wg.Wait()
	close(statuses)
	for status := range statuses {
		if status != 200 {
			t.Fatal(status)
		}
	}
	w := securityRequest(s, "GET", "/settings/preferences", token, nil)
	var p PlaybackPreferences
	json.Unmarshal(w.Body.Bytes(), &p)
	if !p.DataSaving || p.HistoryEnabled {
		t.Fatalf("lost partial update %+v", p)
	}
}

func TestDeletionRequiresFreshCredentials(t *testing.T) {
	s, _, token := securityTestServer(t)
	for _, body := range []map[string]string{{}, {"password": "wrong-password"}} {
		if w := securityRequest(s, "POST", "/account/deletion", token, body); w.Code != 401 {
			t.Fatalf("deletion without reauth %d", w.Code)
		}
	}
	if w := securityRequest(s, "POST", "/account/deletion", token, map[string]string{"password": "Test-password-123!"}); w.Code != 202 {
		t.Fatalf("deletion %d %s", w.Code, w.Body.String())
	}
	if w := securityRequest(s, "GET", "/me", token, nil); w.Code != 401 {
		t.Fatal("session survived deletion")
	}
}
