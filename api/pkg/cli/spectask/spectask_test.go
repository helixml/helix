package spectask

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spf13/cobra"
)

func TestSpecTaskLifecycleCommandsDoNotExposeAgentFlag(t *testing.T) {
	commands := map[string]*cobra.Command{
		"create": newCreateCommand(),
		"start":  newStartCommand(),
		"update": newUpdateCommand(),
		"e2e":    newE2ECommand(),
	}
	for name, command := range commands {
		t.Run(name, func(t *testing.T) {
			if flag := command.Flags().Lookup("agent"); flag != nil {
				t.Fatalf("SpecTask command %q still exposes legacy --agent", name)
			}
		})
	}
}

func TestScreenshotAcceptsSupportedEndpointFormats(t *testing.T) {
	tests := []struct {
		name    string
		body    []byte
		wantOK  bool
		wantErr string
	}{
		{name: "png", body: append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 32)...), wantOK: true},
		{name: "jpeg", body: append([]byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00}, make([]byte, 32)...), wantOK: true},
		{name: "invalid", body: []byte("not an image"), wantErr: "not a supported screenshot image"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(tt.body)
			}))
			defer server.Close()

			result := testScreenshot(server.URL, "test-token", "ses_test", 5)
			if result.Passed != tt.wantOK {
				t.Fatalf("Passed=%v, want %v (error=%q)", result.Passed, tt.wantOK, result.Error)
			}
			if tt.wantErr != "" && !bytes.Contains([]byte(result.Error), []byte(tt.wantErr)) {
				t.Fatalf("error %q does not contain %q", result.Error, tt.wantErr)
			}
		})
	}
}

func TestCreateSpecTaskPayloadContainsNoAppIdentity(t *testing.T) {
	payloads := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		payloads <- payload
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"spt_test","project_id":"prj_test","status":"backlog"}`))
	}))
	defer server.Close()

	task, err := createSpecTask(server.URL, "test-token", "test", "prompt", "prj_test", "headless-ubuntu")
	if err != nil {
		t.Fatalf("createSpecTask returned an error: %v", err)
	}
	if task.ID != "spt_test" {
		t.Fatalf("unexpected task ID %q", task.ID)
	}

	payload := <-payloads
	for _, field := range []string{"app_id", "helix_app_id", "code_agent_overrides"} {
		if _, exists := payload[field]; exists {
			t.Fatalf("legacy field %q was sent in task creation payload: %#v", field, payload)
		}
	}
	if payload["project_id"] != "prj_test" {
		t.Fatalf("project_id missing from payload: %#v", payload)
	}
}
