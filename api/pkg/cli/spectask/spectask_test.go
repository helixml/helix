package spectask

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/helixml/helix/api/pkg/types"
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

	task, err := createSpecTask(server.URL, "test-token", types.CreateTaskRequest{
		Name: "test", Prompt: "prompt", ProjectID: "prj_test",
		SandboxRuntime: types.SandboxRuntimeHeadlessUbuntu, JustDoItMode: true, ConfineAgentKey: true,
	})
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
	if payload["just_do_it_mode"] != true {
		t.Fatalf("just_do_it_mode missing from payload: %#v", payload)
	}
	if payload["confine_agent_key"] != true {
		t.Fatalf("confine_agent_key missing from payload: %#v", payload)
	}
}

func TestStartAcceptsDirectImplementationFlags(t *testing.T) {
	command := newStartCommand()
	for _, name := range []string{"just-do-it", "auto-start"} {
		if command.Flags().Lookup(name) == nil {
			t.Fatalf("start command is missing --%s", name)
		}
	}
}

func TestStopSpecTaskNoOpsWhenTaskHasNoSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/spec-tasks/spt_test/stop-agent" {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"spt_test","planning_session_id":""}`))
	}))
	defer server.Close()

	if err := stopSpecTask(server.URL, "test-token", "spt_test"); err != nil {
		t.Fatalf("stopSpecTask returned an error: %v", err)
	}
}

func TestQueueSessionMessageReturnsImmediately(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/sessions/ses_test/messages" {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"prompt_id":"prm_test"}`))
	}))
	defer server.Close()

	output, err := queueSessionMessage(server.URL, "test-token", "ses_test", "collect files")
	if err != nil {
		t.Fatalf("queueSessionMessage returned an error: %v", err)
	}
	if !output.Delivered || output.PromptID != "prm_test" {
		t.Fatalf("unexpected output: %#v", output)
	}
}

func TestSendWaitTimeoutIsSuccessfulDelivery(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		<-release
	}))
	defer server.Close()

	output, err := sendSessionMessageAndWait(server.URL, "test-token", "ses_test", "collect files", 10*time.Millisecond)
	if err != nil {
		close(release)
		t.Fatalf("timeout should be a successful delivery outcome: %v", err)
	}
	close(release)
	if !output.Delivered || !output.StillRunning {
		t.Fatalf("unexpected output: %#v", output)
	}
}

func TestCreateWithAttachSendsAttachmentsInCreateRequest(t *testing.T) {
	dir := t.TempDir()
	attachPath := filepath.Join(dir, "scope.json")
	content := []byte(`{"scope":"x"}`)
	if err := os.WriteFile(attachPath, content, 0o600); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var requests []string
	var created map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		requests = append(requests, r.Method+" "+r.URL.Path)
		if r.URL.Path == "/api/v1/spec-tasks/from-prompt" {
			_ = json.NewDecoder(r.Body).Decode(&created)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"spt_test","project_id":"prj_test","status":"backlog"}`))
	}))
	defer server.Close()
	t.Setenv("HELIX_URL", server.URL)
	t.Setenv("HELIX_API_KEY", "test-token")

	command := newCreateCommand()
	command.SetArgs([]string{"--project", "prj_test", "--prompt", "do it", "--auto-start", "--attach", attachPath, "--quiet"})
	if err := command.Execute(); err != nil {
		t.Fatalf("create failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 1 || requests[0] != "POST /api/v1/spec-tasks/from-prompt" {
		t.Fatalf("expected a single create request, got %v", requests)
	}
	attachments, _ := created["attachments"].([]any)
	if len(attachments) != 1 {
		t.Fatalf("create request has no inline attachment: %#v", created)
	}
	first, _ := attachments[0].(map[string]any)
	if first["name"] != "scope.json" || first["content_base64"] != base64.StdEncoding.EncodeToString(content) {
		t.Fatalf("unexpected inline attachment: %#v", first)
	}
}

func TestCreateRequestTimeoutOutlivesIngestion(t *testing.T) {
	if got := createRequestTimeout(0); got != 60*time.Second {
		t.Fatalf("no attachments: got %v", got)
	}
	if got := createRequestTimeout(1); got <= types.SpecTaskInlineAttachmentIngestionTimeout {
		t.Fatalf("with attachments timeout %v must exceed server ingestion %v", got, types.SpecTaskInlineAttachmentIngestionTimeout)
	}
}

func TestCreateWithAttachRejectsBadFilesWithoutRequest(t *testing.T) {
	dir := t.TempDir()
	sub1, sub2 := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	for _, d := range []string{sub1, sub2} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "log.txt"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cases := map[string][]string{
		"missing file":  {filepath.Join(dir, "nope.txt")},
		"same basename": {filepath.Join(sub1, "log.txt"), filepath.Join(sub2, "log.txt")},
	}
	for name, attach := range cases {
		t.Run(name, func(t *testing.T) {
			var hits int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&hits, 1)
			}))
			defer server.Close()
			t.Setenv("HELIX_URL", server.URL)
			t.Setenv("HELIX_API_KEY", "test-token")

			args := []string{"--project", "prj_test", "--prompt", "do it", "--quiet"}
			for _, a := range attach {
				args = append(args, "--attach", a)
			}
			command := newCreateCommand()
			command.SetArgs(args)
			if err := command.Execute(); err == nil {
				t.Fatal("expected an error")
			}
			if n := atomic.LoadInt32(&hits); n != 0 {
				t.Fatalf("expected no HTTP request, got %d", n)
			}
		})
	}
}

func TestBuildInlineAttachmentsRejectsOversizeFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.bin")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(types.SpecTaskAttachmentMaxBytes + 1); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if _, err := buildInlineAttachments([]string{path}); err == nil {
		t.Fatal("expected oversize error")
	}
}
