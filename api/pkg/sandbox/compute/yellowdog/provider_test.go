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
		APIKeyID:      "test-key",
		APISecret:     "test-secret",
		BaseURL:       f.srv.URL,
		Namespace:     "test-ns",
		DeploymentTag: "test-dep",
		WorkerTag:     "test-worker",
		TaskTimeout:   240 * time.Minute,
		HTTPClient:    f.srv.Client(),
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
	if got := p.Name(); got != "yellowdog" {
		t.Fatalf("Name() = %q, want %q", got, "yellowdog")
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
	if h.ProviderName != "yellowdog" {
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
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/work/requirements/wr-abc/transition/CANCELLED"):
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
		t.Fatal("expected rollback PUT to /work/requirements/wr-abc/transition/CANCELLED")
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
	if got.URL.Path != "/work/requirements/wr-xyz/transition/CANCELLED" {
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

func TestListReturnsAllPages(t *testing.T) {
	f := newFakeServer(t)
	page := 0
	f.handler = func(w http.ResponseWriter, r *http.Request) {
		page++
		if page == 1 {
			_, _ = w.Write([]byte(`{"items":[{"id":"wr-1","name":"a","status":"RUNNING"}],"nextSliceId":"cursor-2"}`))
		} else {
			// Confirm cursor was passed back.
			if got := r.URL.Query().Get("sliceReference"); got != "cursor-2" {
				t.Errorf("expected sliceReference=cursor-2, got %q", got)
			}
			_, _ = w.Write([]byte(`{"items":[{"id":"wr-2","name":"b","status":"COMPLETED"}]}`))
		}
	}
	p := f.provider(t)
	out, err := p.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 handles, got %d", len(out))
	}
	if out[0].State != compute.StateReady {
		t.Fatalf("expected StateReady for RUNNING WR, got %q", out[0].State)
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
		{"RUNNING", compute.StateReady, false},
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
