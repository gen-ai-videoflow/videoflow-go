package cloud

import (
	"encoding/json"
	"github.com/go-chi/chi/v5"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestLiveRouteOpenAPIParity(t *testing.T) {
	data, e := os.ReadFile("../../api/openapi.json")
	if e != nil {
		t.Fatal(e)
	}
	var contract struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if e = json.Unmarshal(data, &contract); e != nil {
		t.Fatal(e)
	}
	normalize := func(path string) string { return regexp.MustCompile(`\{[^}]+\}`).ReplaceAllString(path, "{}") }
	documented := map[string]bool{}
	for p, methods := range contract.Paths {
		for method := range methods {
			documented[strings.ToUpper(method)+" "+normalize(p)] = true
		}
	}
	s := NewServer(nil, Config{DevMode: true})
	defer s.Close()
	live := map[string]bool{}
	e = chi.Walk(s.routes(), func(method, path string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
		if !strings.HasPrefix(path, "/v1/") {
			return nil
		}
		path = strings.TrimPrefix(path, "/v1")
		for _, expanded := range expandContractRoute(path) {
			key := method + " " + normalize(expanded)
			live[key] = true
			if !documented[key] {
				t.Errorf("live route missing OpenAPI: %s %s", method, expanded)
			}
		}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	for key := range documented {
		if !live[key] {
			t.Errorf("documented route missing live handler: %s", key)
		}
	}
}

func expandContractRoute(path string) []string {
	re := regexp.MustCompile(`\{[^}:]+:([^}]+)\}`)
	match := re.FindStringSubmatchIndex(path)
	if match == nil {
		return []string{path}
	}
	out := []string{}
	for _, part := range strings.Split(path[match[2]:match[3]], "|") {
		out = append(out, expandContractRoute(path[:match[0]]+part+path[match[1]:])...)
	}
	return out
}
