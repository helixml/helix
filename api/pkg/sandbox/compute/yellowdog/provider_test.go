package yellowdog

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/sandbox/compute"
)

// fakeServer is a tiny scriptable httptest server. Handler receives
// every request; the test inspects path/method and writes the desired
// response. Keeps each test self-contained without a router framework.
type fakeServer struct {
	t        *testing.T
	srv      *httptest.Server
	handler  http.HandlerFunc
	requests []*http.Request
	bodies   [][]byte
}

func newFakeServer(t *testing.T) *fakeServer {
	t.Helper()
	f := &fakeServer{t: t}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := readAllBody(r)
		f.requests = append(f.requests, r)
		f.bodies = append(f.bodies, body)
		// Re-attach body so the handler can read it again if it wants to.
		if f.handler == nil {
			http.Error(w, "fake: no handler set", http.StatusTeapot)
			return
		}
		f.handler(w, r)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func readAllBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	buf := make([]byte, 0, 1024)
	chunk := make([]byte, 512)
	for {
		n, err := r.Body.Read(chunk)
		if n > 0 {
			buf = append(buf, chunk[:n]...)
		}
		if err != nil {
			break
		}
	}
	return buf, nil
}

func (f *fakeServer) provider(t *testing.T) *Provider {
	t.Helper()
	cfg := Config{
		APIKeyID:             "test-key",
		APISecret:            "test-secret",
		BaseURL:              f.srv.URL,
		Namespace:            "test-ns",
		DeploymentTag:        "test-dep",
		WorkerTag:            "test-worker",
		TaskTimeout:          240 * time.Minute,
		HTTPClient:           f.srv.Client(),
		AllowInsecureBaseURL: true, // httptest serves http://
	}
	p, err := NewProvider(cfg)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	return p
}

func TestNewProviderValidates(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		want string
	}{
		{"missing key", Config{APISecret: "s", Namespace: "n", DeploymentTag: "d", WorkerTag: "w"}, "APIKeyID and APISecret"},
		{"missing secret", Config{APIKeyID: "k", Namespace: "n", DeploymentTag: "d", WorkerTag: "w"}, "APIKeyID and APISecret"},
		{"missing namespace", Config{APIKeyID: "k", APISecret: "s", DeploymentTag: "d", WorkerTag: "w"}, "Namespace"},
		{"missing deployment tag", Config{APIKeyID: "k", APISecret: "s", Namespace: "n", WorkerTag: "w"}, "DeploymentTag"},
		{"missing worker tag", Config{APIKeyID: "k", APISecret: "s", Namespace: "n", DeploymentTag: "d"}, "WorkerTag"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewProvider(tc.cfg)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.want)
			}
		})
	}
}

func TestNewProviderDefaults(t *testing.T) {
	p, err := NewProvider(Config{
		APIKeyID:      "k",
		APISecret:     "s",
		Namespace:     "n",
		DeploymentTag: "d",
		WorkerTag:     "w",
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if p.baseURL != defaultBaseURL {
		t.Fatalf("expected default baseURL, got %q", p.baseURL)
	}
	if p.cfg.TaskType != "bash" {
		t.Fatalf("expected TaskType default 'bash', got %q", p.cfg.TaskType)
	}
}

func TestProviderName(t *testing.T) {
	f := newFakeServer(t)
	p := f.provider(t)
	// Provider.Name() composes the static kind with cfg.DeploymentTag
	// so two Helix installations sharing a Postgres but using
	// different deployment tags do not see each other's rows as owned.
	// The fakeServer provider sets DeploymentTag="test-dep".
	if got := p.Name(); got != "yellowdog-test-dep" {
		t.Fatalf("Name() = %q, want %q", got, "yellowdog-test-dep")
	}
}

func TestTaskBodyShape_BashInline(t *testing.T) {
	// Validates the YD task wiring: TaskType=bash so YD's default
	// /bin/bash handler is invoked with our arguments. We use the
	// "bash -c <body> argv..." form so the embedded script body
	// runs with $1, $2, $3 as helix_url, runner_token, helix_image.
	// SANDBOX_INSTANCE_ID flows via the task Environment because
	// the script reads $SANDBOX_INSTANCE_ID from its env.
	f := newFakeServer(t)
	var taskBody []byte
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/work/requirements":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"wr-1","name":"x","namespace":"test-ns","taskGroups":[{"id":"tg-1","name":"tg1"}]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/work/taskGroups/tg-1/tasks":
			taskBody = f.bodies[len(f.bodies)-1]
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`[{"id":"task-1"}]`))
		default:
			http.Error(w, "unexpected", http.StatusBadRequest)
		}
	}
	cfg := Config{
		APIKeyID:             "k",
		APISecret:            "s",
		BaseURL:              f.srv.URL,
		Namespace:            "test-ns",
		DeploymentTag:        "test-dep",
		WorkerTag:            "test-worker",
		HTTPClient:           f.srv.Client(),
		AllowInsecureBaseURL: true,
		HelixURL:             "https://helix.example.com",
		RunnerToken:          "secret-token-xyz",
		HelixImage:           "ghcr.io/helixml/helix-sandbox:test-tag",
	}
	p, err := NewProvider(cfg)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	_, err = p.Provision(context.Background(), compute.Spec{
		Labels: map[string]string{"helix.sandbox_id": "sbx_test123"},
	})
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}
	if taskBody == nil {
		t.Fatal("task POST body was never captured")
	}
	var tasks []taskPayload
	if err := json.Unmarshal(taskBody, &tasks); err != nil {
		t.Fatalf("decode task body: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	task := tasks[0]

	// TaskType must be "bash" - we no longer use the docker task
	// type because YD agent registration is one-shot and adding
	// docker via post-install userdata is fragile (see
	// design/2026-06-09-yd-bash-script-alternatives.md).
	if task.TaskType != "bash" {
		t.Fatalf("TaskType = %q, want bash", task.TaskType)
	}

	// Argument shape: -c, <embedded body>, "yd-inline" ($0),
	// helix_url ($1), runner_token ($2), helix_image ($3).
	if len(task.Arguments) != 6 {
		t.Fatalf("expected 6 arguments [-c body $0 $1 $2 $3], got %d: %v", len(task.Arguments), task.Arguments)
	}
	if task.Arguments[0] != "-c" {
		t.Fatalf("Arguments[0] = %q, want -c", task.Arguments[0])
	}
	// Arguments[1] is the embedded script body. Spot-check that it
	// references the positional args we'll pass.
	if !strings.Contains(task.Arguments[1], "HELIX_URL=\"${1") {
		t.Fatalf("embedded script body doesn't reference $1 as expected")
	}
	if task.Arguments[2] != "yd-inline" {
		t.Fatalf("Arguments[2] = %q, want yd-inline ($0)", task.Arguments[2])
	}
	if task.Arguments[3] != "https://helix.example.com" {
		t.Fatalf("Arguments[3] = %q, want helix URL ($1)", task.Arguments[3])
	}
	if task.Arguments[4] != "secret-token-xyz" {
		t.Fatalf("Arguments[4] = %q, want runner token ($2)", task.Arguments[4])
	}
	if task.Arguments[5] != "ghcr.io/helixml/helix-sandbox:test-tag" {
		t.Fatalf("Arguments[5] = %q, want image ($3)", task.Arguments[5])
	}

	// SANDBOX_INSTANCE_ID must be in the task Environment - the
	// embedded script reads $SANDBOX_INSTANCE_ID from its process
	// env to name the container + propagate to helix-sandbox.
	if got := task.Environment["SANDBOX_INSTANCE_ID"]; got != "sbx_test123" {
		t.Fatalf("Environment[SANDBOX_INSTANCE_ID] = %q, want sbx_test123", got)
	}
}

func TestTaskArgumentsEmptyWhenConfigIncomplete(t *testing.T) {
	// Defensive: if operator forgets HelixURL/RunnerToken/HelixImage,
	// taskArguments returns nil so the task fails immediately on the
	// worker rather than silently submitting bogus arguments.
	cfg := Config{
		APIKeyID: "k", APISecret: "s",
		Namespace:     "n",
		DeploymentTag: "d",
		WorkerTag:     "w",
		// HelixURL, RunnerToken, HelixImage all empty
	}
	p, err := NewProvider(cfg)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if got := p.taskArguments(); got != nil {
		t.Fatalf("expected nil arguments when config is incomplete, got %v", got)
	}
}

func TestProviderNameDifferentDeploymentTags(t *testing.T) {
	// Cross-tenancy regression guard: two Providers with the same
	// kind but different deployment tags MUST report different
	// Name()s, otherwise both would see each other's rows.
	cfgA := Config{
		APIKeyID: "k", APISecret: "s",
		Namespace:     "n",
		DeploymentTag: "prod",
		WorkerTag:     "w",
	}
	cfgB := cfgA
	cfgB.DeploymentTag = "staging"

	a, err := NewProvider(cfgA)
	if err != nil {
		t.Fatalf("NewProvider A: %v", err)
	}
	b, err := NewProvider(cfgB)
	if err != nil {
		t.Fatalf("NewProvider B: %v", err)
	}
	if a.Name() == b.Name() {
		t.Fatalf("same Name() across different deployment tags: %q", a.Name())
	}
	if a.Name() != "yellowdog-prod" {
		t.Fatalf("expected yellowdog-prod, got %q", a.Name())
	}
	if b.Name() != "yellowdog-staging" {
		t.Fatalf("expected yellowdog-staging, got %q", b.Name())
	}
}

func TestProvisionSubmitsWRThenAddsTask(t *testing.T) {
	f := newFakeServer(t)
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/work/requirements":
			// Echo back with assigned IDs.
			var got workRequirement
			_ = json.Unmarshal(f.bodies[len(f.bodies)-1], &got)
			got.ID = "wr-123"
			if len(got.TaskGroups) > 0 {
				got.TaskGroups[0].ID = "tg-456"
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(got)
		case r.Method == http.MethodPost && r.URL.Path == "/work/taskGroups/tg-456/tasks":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`[{"id":"task-789"}]`))
		default:
			http.Error(w, "unexpected: "+r.Method+" "+r.URL.Path, http.StatusBadRequest)
		}
	}

	p := f.provider(t)
	h, err := p.Provision(context.Background(), compute.Spec{
		GPUVendor: "nvidia",
		Labels:    map[string]string{"helix.deployment": "test"},
	})
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}
	if h.ProviderName != "yellowdog-test-dep" {
		t.Fatalf("wrong ProviderName: %q", h.ProviderName)
	}
	if h.ProviderID != "wr-123" {
		t.Fatalf("wrong ProviderID: %q", h.ProviderID)
	}
	if h.State != compute.StateProvisioning {
		t.Fatalf("wrong State: %q", h.State)
	}
	if h.Metadata["yd.task_group_id"] != "tg-456" {
		t.Fatalf("missing task_group_id metadata")
	}
	if len(f.requests) != 2 {
		t.Fatalf("expected 2 requests (POST WR + POST task), got %d", len(f.requests))
	}
}

func TestProvisionWRSubmitFailureReturnsError(t *testing.T) {
	f := newFakeServer(t)
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"status":500,"title":"server error"}`))
	}
	p := f.provider(t)
	_, err := p.Provision(context.Background(), compute.Spec{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "submit WR") {
		t.Fatalf("expected 'submit WR' in error, got %q", err.Error())
	}
}

func TestProvisionTaskAddFailureCancelsWR(t *testing.T) {
	f := newFakeServer(t)
	cancelled := false
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/work/requirements":
			_, _ = w.Write([]byte(`{"id":"wr-abc","namespace":"test-ns","name":"x","taskGroups":[{"id":"tg-xyz","name":"tg1"}]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/work/taskGroups/tg-xyz/tasks":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"status":400,"title":"bad task"}`))
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/work/requirements/wr-abc/transition/CANCELLING"):
			cancelled = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		default:
			http.Error(w, "unexpected: "+r.Method+" "+r.URL.Path, http.StatusBadRequest)
		}
	}
	p := f.provider(t)
	_, err := p.Provision(context.Background(), compute.Spec{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !cancelled {
		t.Fatal("expected rollback PUT to /work/requirements/wr-abc/transition/CANCELLING")
	}
}

func TestDeprovisionForceAddsAbortQuery(t *testing.T) {
	f := newFakeServer(t)
	var got *http.Request
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		got = r
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}
	p := f.provider(t)
	h := &compute.Handle{ProviderID: "wr-xyz"}
	if err := p.Deprovision(context.Background(), h, compute.DeprovisionOpts{Force: true}); err != nil {
		t.Fatalf("Deprovision: %v", err)
	}
	if got.Method != http.MethodPut {
		t.Fatalf("wrong method: %s", got.Method)
	}
	if got.URL.Path != "/work/requirements/wr-xyz/transition/CANCELLING" {
		t.Fatalf("wrong path: %s", got.URL.Path)
	}
	if got.URL.Query().Get("abort") != "true" {
		t.Fatalf("expected abort=true, got query %q", got.URL.RawQuery)
	}
	if h.State != compute.StateTerminating {
		t.Fatalf("state not updated to Terminating: %q", h.State)
	}
}

func TestDeprovisionNotFoundIsIdempotent(t *testing.T) {
	f := newFakeServer(t)
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"status":404,"title":"not found"}`))
	}
	p := f.provider(t)
	h := &compute.Handle{ProviderID: "wr-gone"}
	if err := p.Deprovision(context.Background(), h, compute.DeprovisionOpts{}); err != nil {
		t.Fatalf("expected nil for 404, got %v", err)
	}
	if h.State != compute.StateTerminated {
		t.Fatalf("expected StateTerminated, got %q", h.State)
	}
}

func TestDeprovisionRejectsNilHandle(t *testing.T) {
	f := newFakeServer(t)
	p := f.provider(t)
	if err := p.Deprovision(context.Background(), nil, compute.DeprovisionOpts{}); err == nil {
		t.Fatal("expected error for nil handle")
	}
	if err := p.Deprovision(context.Background(), &compute.Handle{}, compute.DeprovisionOpts{}); err == nil {
		t.Fatal("expected error for empty ProviderID")
	}
}

func TestListDecodesFlatArrayAndFiltersClientSide(t *testing.T) {
	// GET /work/requirements returns a flat JSON array, NOT a paged
	// {items, nextSliceId} envelope. The server-side `search` param
	// is silently ignored - we still send it but cannot rely on it,
	// so we re-filter client-side on (namespace, tag). Both confirmed
	// against live YD on 2026-06-05.
	f := newFakeServer(t)
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("search"); got != `{"namespace":"test-ns","tag":"test-dep"}` {
			t.Errorf("unexpected search query: %q", got)
		}
		// Return a mix: two that match the provider's (ns, tag) and
		// two that don't. The client-side filter must keep only the
		// matching ones.
		_, _ = w.Write([]byte(`[
			{"id":"ours-1","namespace":"test-ns","tag":"test-dep","name":"a","status":"RUNNING"},
			{"id":"foreign-1","namespace":"other-ns","tag":"test-dep","name":"x","status":"RUNNING"},
			{"id":"foreign-2","namespace":"test-ns","tag":"other-tag","name":"y","status":"RUNNING"},
			{"id":"ours-2","namespace":"test-ns","tag":"test-dep","name":"b","status":"COMPLETED"}
		]`))
	}
	p := f.provider(t)
	out, err := p.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 filtered handles, got %d", len(out))
	}
	ids := []string{out[0].ProviderID, out[1].ProviderID}
	for _, got := range ids {
		if got != "ours-1" && got != "ours-2" {
			t.Fatalf("client-side filter let through foreign WR: %q", got)
		}
	}
	if out[0].State != compute.StateProvisioning {
		t.Fatalf("expected StateProvisioning for RUNNING WR (host has not registered yet), got %q", out[0].State)
	}
	if out[1].State != compute.StateTerminated {
		t.Fatalf("expected StateTerminated for COMPLETED WR, got %q", out[1].State)
	}
}

func TestHealthCheckMapsStatusToState(t *testing.T) {
	cases := []struct {
		ydStatus string
		want     compute.State
		wantErr  bool
	}{
		// RUNNING does NOT mean "host is up and serving". YD reports
		// RUNNING as soon as the WR is accepted by the scheduler -
		// the task may still be queued, the EC2 may still be booting,
		// helix-sandbox may still be pulling its image. The only
		// signal that the host is actually live is its WebSocket
		// registration, which the auto-register bridge handles.
		{"RUNNING", compute.StateProvisioning, false},
		{"HELD", compute.StateProvisioning, false},
		{"COMPLETED", compute.StateTerminated, true},
		{"FAILED", compute.StateFailed, true},
		{"CANCELLING", compute.StateTerminating, false},
		{"CANCELLED", compute.StateTerminated, true},
	}
	for _, tc := range cases {
		t.Run(tc.ydStatus, func(t *testing.T) {
			f := newFakeServer(t)
			f.handler = func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprintf(w, `{"id":"wr-1","status":%q}`, tc.ydStatus)
			}
			p := f.provider(t)
			h := &compute.Handle{ProviderID: "wr-1"}
			err := p.HealthCheck(context.Background(), h)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error for %q, got nil", tc.ydStatus)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.ydStatus, err)
			}
			if h.State != tc.want {
				t.Fatalf("state for %q: got %q, want %q", tc.ydStatus, h.State, tc.want)
			}
		})
	}
}

func TestHealthCheckUnknownStatusReturnsError(t *testing.T) {
	f := newFakeServer(t)
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		// Imagine YD adds STARTING in a future release.
		_, _ = w.Write([]byte(`{"id":"wr-1","status":"STARTING"}`))
	}
	p := f.provider(t)
	priorState := compute.State("priorState")
	h := &compute.Handle{ProviderID: "wr-1", State: priorState}
	err := p.HealthCheck(context.Background(), h)
	if err == nil {
		t.Fatal("expected error for unknown YD status")
	}
	if !strings.Contains(err.Error(), "unknown YD status") {
		t.Fatalf("expected error mentioning unknown YD status, got %q", err.Error())
	}
	// Caller's State must NOT be silently overwritten.
	if h.State != priorState {
		t.Fatalf("State should remain unchanged on unknown status; got %q", h.State)
	}
}

func TestListUnknownStatusSurfacesAsFailed(t *testing.T) {
	f := newFakeServer(t)
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		// Namespace + tag must match the fakeServer-provider config
		// for the client-side filter to retain this row.
		_, _ = w.Write([]byte(`[{"id":"wr-1","namespace":"test-ns","tag":"test-dep","status":"WEIRD"}]`))
	}
	p := f.provider(t)
	out, err := p.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 handle, got %d", len(out))
	}
	if out[0].State != compute.StateFailed {
		t.Fatalf("expected StateFailed for unknown status, got %q", out[0].State)
	}
	if out[0].Metadata["yd.status"] != "WEIRD" {
		t.Fatalf("expected yd.status metadata=WEIRD, got %q", out[0].Metadata["yd.status"])
	}
}

func TestHealthCheckNotFoundReturnsError(t *testing.T) {
	f := newFakeServer(t)
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"status":404}`))
	}
	p := f.provider(t)
	h := &compute.Handle{ProviderID: "wr-gone"}
	err := p.HealthCheck(context.Background(), h)
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if h.State != compute.StateTerminated {
		t.Fatalf("expected StateTerminated, got %q", h.State)
	}
}

func TestAuthHeaderRedactedInStringer(t *testing.T) {
	c := credentials{keyID: "key12345-abc", secret: "supersecret"}
	s := fmt.Sprintf("%v", c)
	if strings.Contains(s, "supersecret") {
		t.Fatalf("secret leaked: %q", s)
	}
	if !strings.Contains(s, "<redacted>") {
		t.Fatalf("expected <redacted> marker: %q", s)
	}
}

func TestNewProviderRejectsInsecureBaseURLByDefault(t *testing.T) {
	_, err := NewProvider(Config{
		APIKeyID:      "k",
		APISecret:     "s",
		Namespace:     "n",
		DeploymentTag: "d",
		WorkerTag:     "w",
		BaseURL:       "http://yd.example.com/api",
	})
	if err == nil {
		t.Fatal("expected error for http:// BaseURL without AllowInsecureBaseURL")
	}
	if !strings.Contains(err.Error(), "https://") {
		t.Fatalf("error should mention https://, got %q", err.Error())
	}
}

func TestNewProviderAllowsInsecureWhenExplicitlyOptedIn(t *testing.T) {
	p, err := NewProvider(Config{
		APIKeyID:             "k",
		APISecret:            "s",
		Namespace:            "n",
		DeploymentTag:        "d",
		WorkerTag:            "w",
		BaseURL:              "http://yd.example.com/api",
		AllowInsecureBaseURL: true,
	})
	if err != nil {
		t.Fatalf("expected success with AllowInsecureBaseURL=true: %v", err)
	}
	if p.baseURL != "http://yd.example.com/api" {
		t.Fatalf("baseURL not preserved: %q", p.baseURL)
	}
}

func TestNewProviderRejectsCRLFInCredentials(t *testing.T) {
	bad := []struct {
		name      string
		key, sec  string
		wantToken string
	}{
		{"newline in key", "k\nbad", "secret", "CR or LF"},
		{"carriage return in secret", "k", "se\rcret", "CR or LF"},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewProvider(Config{
				APIKeyID:      tc.key,
				APISecret:     tc.sec,
				Namespace:     "n",
				DeploymentTag: "d",
				WorkerTag:     "w",
			})
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.wantToken) {
				t.Fatalf("error %q missing %q", err.Error(), tc.wantToken)
			}
		})
	}
}

func TestNewProviderDefaultHTTPClientHasTimeoutAndBlocksRedirects(t *testing.T) {
	// Build a Provider without explicitly setting HTTPClient so we
	// exercise the default-construction path.
	p, err := NewProvider(Config{
		APIKeyID:      "k",
		APISecret:     "s",
		Namespace:     "n",
		DeploymentTag: "d",
		WorkerTag:     "w",
	})
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if p.httpc == http.DefaultClient {
		t.Fatal("default HTTPClient must NOT be http.DefaultClient (no timeout)")
	}
	if p.httpc.Timeout == 0 {
		t.Fatal("default HTTPClient must have a non-zero Timeout")
	}
	if p.httpc.CheckRedirect == nil {
		t.Fatal("default HTTPClient must have a CheckRedirect that blocks redirects")
	}
	// The CheckRedirect should return http.ErrUseLastResponse to
	// short-circuit the redirect without forwarding the auth header.
	if got := p.httpc.CheckRedirect(nil, nil); got != http.ErrUseLastResponse {
		t.Fatalf("expected http.ErrUseLastResponse, got %v", got)
	}
}

func TestRetryOn500ThenSuccess(t *testing.T) {
	f := newFakeServer(t)
	calls := 0
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"status":500,"title":"flake"}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"wr-1","status":"RUNNING"}`))
	}
	cfg := Config{
		APIKeyID:             "k",
		APISecret:            "s",
		BaseURL:              f.srv.URL,
		Namespace:            "n",
		DeploymentTag:        "d",
		WorkerTag:            "w",
		HTTPClient:           f.srv.Client(),
		AllowInsecureBaseURL: true,
		MaxRetries:           2,
		InitialBackoff:       1 * time.Millisecond,
	}
	p, err := NewProvider(cfg)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	// GET is idempotent, so retry applies.
	h := &compute.Handle{ProviderID: "wr-1"}
	if err := p.HealthCheck(context.Background(), h); err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls (1 fail + 1 retry success), got %d", calls)
	}
	if h.State != compute.StateProvisioning {
		t.Fatalf("expected StateProvisioning after retry (RUNNING means WR accepted, not host up), got %q", h.State)
	}
}

func TestPOSTIsNotRetried(t *testing.T) {
	f := newFakeServer(t)
	calls := 0
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"status":500,"title":"flake"}`))
	}
	cfg := Config{
		APIKeyID:             "k",
		APISecret:            "s",
		BaseURL:              f.srv.URL,
		Namespace:            "n",
		DeploymentTag:        "d",
		WorkerTag:            "w",
		HTTPClient:           f.srv.Client(),
		AllowInsecureBaseURL: true,
		MaxRetries:           5,
		InitialBackoff:       1 * time.Millisecond,
	}
	p, err := NewProvider(cfg)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	_, err = p.Provision(context.Background(), compute.Spec{})
	if err == nil {
		t.Fatal("expected error")
	}
	// POST is NOT in the idempotent set, so it must be attempted exactly once.
	if calls != 1 {
		t.Fatalf("expected exactly 1 POST attempt (no retry), got %d", calls)
	}
}

func TestIsoMinutesFormatsDurations(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{0, ""},
		{30 * time.Second, "PT1M"},     // rounds up
		{4 * time.Hour, "PT240M"},      // matches the POC config.toml
		{2*time.Hour + 30*time.Second, "PT121M"},
	}
	for _, tc := range cases {
		if got := isoMinutes(tc.in); got != tc.want {
			t.Errorf("isoMinutes(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
