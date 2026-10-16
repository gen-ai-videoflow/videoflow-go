package desktop

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDesktopRoutes(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("6eek UI"), 0600); err != nil {
		t.Fatal(err)
	}
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/settings/providers" && r.URL.Path != "/storage/video.mp4" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte("API"))
	})
	handler := Handler(api, root, "127.0.0.1:8000")
	for _, tc := range []struct {
		path string
		code int
		body string
	}{
		{"/", 200, "6eek UI"}, {"/settings", 200, "6eek UI"},
		{"/characters", 200, "6eek UI"}, {"/api/settings/providers", 200, "API"},
		{"/storage/video.mp4", 200, "API"}, {"/api/missing", 404, "404 page not found\n"},
		{"/_app/missing.js", 404, "404 page not found\n"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1:8000"+tc.path, nil))
			if w.Code != tc.code || w.Body.String() != tc.body {
				t.Fatalf("got %d %q", w.Code, w.Body.String())
			}
		})
	}
}

func TestDesktopRejectsForeignBrowserRequests(t *testing.T) {
	calls := 0
	handler := Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(202) }), t.TempDir(), "127.0.0.1:8000")
	for _, tc := range []struct {
		host, origin string
		want         int
	}{
		{"127.0.0.1:8000", "http://unrelated.example", 403},
		{"rebound.example:8000", "", 403},
		{"127.0.0.1:8000", "null", 403},
		{"127.0.0.1:8000", "http://127.0.0.1:8000", 202},
		{"127.0.0.1:8000", "", 202},
	} {
		r := httptest.NewRequest("POST", "http://"+tc.host+"/api/videos/generate", nil)
		if tc.origin != "" {
			r.Header.Set("Origin", tc.origin)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("host=%s origin=%s: %d", tc.host, tc.origin, w.Code)
		}
	}
	if calls != 2 {
		t.Fatalf("foreign request dispatched: %d calls", calls)
	}
}
