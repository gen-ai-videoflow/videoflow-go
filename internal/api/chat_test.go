package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestChatReturnsRunnableCanonicalActions(t *testing.T) {
	for _, tc := range []struct{ input, want string }{{"add_scene", "generate_script"}, {"create_script", "generate_script"}, {"generate_shots", "generate_shots"}, {"none", "unknown"}, {"invented_action", "unknown"}} {
		t.Run(tc.input, func(t *testing.T) {
			s := routerServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/models" {
					w.Write([]byte(`{"data":[{"id":"openai/gpt-4o-mini","architecture":{"output_modalities":["text"]}}]}`))
					return
				}
				content, _ := json.Marshal(map[string]string{"reply": "Scene idea", "action": tc.input})
				json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": string(content)}}}})
			})
			result := call(t, s, "POST", "/chat", `{"message":"Write a new scene"}`)
			if result.Code != 200 {
				t.Fatalf("%d %s", result.Code, result.Body.String())
			}
			var body struct{ Intent struct{ Action string } }
			json.Unmarshal(result.Body.Bytes(), &body)
			if body.Intent.Action != tc.want {
				t.Fatalf("dead card action %q; want %q", body.Intent.Action, tc.want)
			}
		})
	}
}
