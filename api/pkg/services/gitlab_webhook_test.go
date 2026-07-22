package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	gitlabclient "github.com/helixml/helix/api/pkg/agent/skill/gitlab"
)

func TestWriteGitLabWebhookFallsBackWhenSigningTokenUnsupported(t *testing.T) {
	for _, test := range []struct {
		name   string
		create bool
		hookID int
		method string
		path   string
		events []string
		want   map[string]bool
	}{
		{
			name:   "create",
			create: true,
			method: http.MethodPost,
			path:   "/api/v4/projects/1/hooks",
			events: []string{"Merge Request Hook"},
			want: map[string]bool{
				"merge_requests_events": true,
				"note_events":           false,
				"pipeline_events":       false,
				"push_events":           false,
			},
		},
		{
			name:   "update",
			hookID: 7,
			method: http.MethodPut,
			path:   "/api/v4/projects/1/hooks/7",
			events: []string{"Note Hook"},
			want: map[string]bool{
				"merge_requests_events": false,
				"note_events":           true,
				"pipeline_events":       false,
				"push_events":           false,
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.Method != test.method || r.URL.Path != test.path {
					t.Fatalf("request=%s %s", r.Method, r.URL.Path)
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if body["token"] != "legacy" || body["url"] != "https://hook" {
					t.Fatalf("body=%v", body)
				}
				for event, want := range test.want {
					if got, ok := body[event].(bool); !ok || got != want {
						t.Fatalf("%s=%v want=%v body=%v", event, body[event], want, body)
					}
				}
				if requests == 1 {
					if body["signing_token"] != "whsec_x" {
						t.Fatalf("first body=%v", body)
					}
					w.WriteHeader(http.StatusBadRequest)
					_, _ = w.Write([]byte(`{"message":{"signing_token":["is unknown"]}}`))
					return
				}
				if _, present := body["signing_token"]; present {
					t.Fatalf("fallback body=%v", body)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":7,"url":"https://hook"}`))
			}))
			defer server.Close()

			client, err := gitlabclient.NewClientWithPAT(server.URL, "pat")
			if err != nil {
				t.Fatal(err)
			}
			hook, err := writeGitLabWebhook(context.Background(), client, test.create, 1, test.hookID, "https://hook", "whsec_x", "legacy", test.events)
			if err != nil {
				t.Fatal(err)
			}
			if requests != 2 || hook.ID != 7 {
				t.Fatalf("requests=%d hook=%+v", requests, hook)
			}
		})
	}
}

func TestGitLabWebhookURLUsesProjectWebURL(t *testing.T) {
	got := gitLabWebhookURL("https://gitlab.example/group/project", 9)
	if got != "https://gitlab.example/group/project/-/hooks/9/edit" {
		t.Fatalf("url=%q", got)
	}
}
