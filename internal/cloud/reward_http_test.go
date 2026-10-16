package cloud

import (
	"context"
	"encoding/json"
	"github.com/go-chi/chi/v5"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRewardReadsArePrivateAndEnrollmentIsGated(t *testing.T) {
	store, u := testPlatform(t)
	_, token, err := store.Login(context.Background(), u.Email, "Test-password-123!")
	if err != nil {
		t.Fatal(err)
	}
	app := NewServer(store, Config{DevMode: true})
	defer app.Close()
	router := chi.NewRouter()
	router.Group(func(r chi.Router) {
		r.Use(app.requireUser)
		r.Get("/summary", app.rewardSummary)
		r.Get("/program", app.rewardProgram)
		r.Get("/videos/{id}", app.rewardVideoDetail)
		r.Post("/enroll", app.rewardEnroll)
	})
	call := func(method, path string, auth bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, nil)
		if auth {
			r.AddCookie(&http.Cookie{Name: "videoflow_session", Value: token})
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	if w := call("GET", "/summary", false); w.Code != 401 {
		t.Fatalf("anonymous summary %d", w.Code)
	}
	if w := call("GET", "/summary", true); w.Code != 200 {
		t.Fatalf("summary %d %s", w.Code, w.Body)
	}
	if w := call("GET", "/videos/not-owned", true); w.Code != 404 {
		t.Fatalf("ownership %d", w.Code)
	}
	if w := call("POST", "/enroll", true); w.Code == 200 {
		t.Fatal("disabled enrollment succeeded")
	}
	w := call("GET", "/program", true)
	var program struct {
		Enabled bool   `json:"enabled"`
		Policy  string `json:"policy_version"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &program); err != nil || program.Enabled || program.Policy != "pilot-v1" {
		t.Fatalf("program %s %v", w.Body, err)
	}
}
