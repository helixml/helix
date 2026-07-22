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
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if requests == 1 {
			if body["signing_token"] == nil || body["token"] == nil {
				t.Fatalf("first body=%v", body)
			}
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"message":{"signing_token":["is unknown"]}}`))
			return
		}
		if _, present := body["signing_token"]; present {
			t.Fatalf("fallback body=%v", body)
		}
		if body["token"] != "legacy" {
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
	hook, err := writeGitLabWebhook(context.Background(), client, true, 1, 0, "https://hook", "whsec_x", "legacy", []string{"Merge Request Hook"})
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 || hook.ID != 7 {
		t.Fatalf("requests=%d hook=%+v", requests, hook)
	}
}

func TestGitLabWebhookURLUsesProjectWebURL(t *testing.T) {
	got := gitLabWebhookURL("https://gitlab.example/group/project", 9)
	if got != "https://gitlab.example/group/project/-/hooks/9/edit" {
		t.Fatalf("url=%q", got)
	}
}
