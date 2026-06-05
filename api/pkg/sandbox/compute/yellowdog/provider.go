// Package yellowdog implements compute.Provider against the YellowDog
// REST API.
//
// The package surface is deliberately tiny: only the Provider type and
// its Config are exported. YellowDog's domain vocabulary (work
// requirements, task groups, worker pools) does not leak out of this
// package - callers interact via the compute.Provider abstraction
// (Provision / Deprovision / List / HealthCheck) and never see a YD
// primitive directly.
//
// Authentication uses the YD API key pair (header format
// "Authorization: yd-key <id>:<secret>"). Credentials are loaded
// operator-side via Config and never logged.
//
// Wire format details are in http.go; the domain logic for how a
// Helix sandbox host maps to a YD work requirement is in provider.go.
package yellowdog

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/sandbox/compute"
)

// providerName is the value returned by Provider.Name(). Stable across
// releases - SandboxInstance rows persist this string in their
// Provider column, so renaming it is a data migration.
const providerName = "yellowdog"

// Config carries operator-supplied configuration for the YellowDog
// provider. Loaded from env/secret-store at Helix startup; not loaded
// from any user request.
type Config struct {
	// APIKeyID and APISecret are the YD account credentials. Both
	// required; NewProvider rejects empty values.
	APIKeyID  string
	APISecret string

	// BaseURL overrides the production API endpoint. Empty falls back
	// to https://portal.yellowdog.co/api. Tests inject an httptest URL.
	BaseURL string

	// Namespace is the YD namespace all our work requirements live in.
	// Required. Matches the Helix deployment's allocated namespace
	// (e.g. "helix-prod", "helix-staging").
	Namespace string

	// DeploymentTag identifies WRs originated by this Helix install,
	// used by List to filter out unrelated WRs in the same namespace.
	// Required. Pick something stable per deployment (e.g. the Helix
	// install ID).
	DeploymentTag string

	// WorkerTag is the tag the operator's worker pool advertises.
	// Tasks include this in their RunSpecification so the scheduler
	// only assigns them to workers from the matching pool. Required.
	WorkerTag string

	// TaskTimeout bounds individual task runtime. The platform aborts
	// the task and records TaskError type=TIMED_OUT when exceeded.
	// Zero means "use platform default". The YD API expresses this
	// as ISO 8601 minutes - we translate from time.Duration here.
	TaskTimeout time.Duration

	// TaskType is the worker task-type handler name (registered on
	// each worker by install-agent.sh in the POC). Defaults to "bash"
	// if empty.
	TaskType string

	// TaskArguments is the positional argument vector passed to each
	// task. Empty arguments default to a one-line shell that runs the
	// helix-sandbox image - operators MUST override this in real
	// deployments to pass HELIX_URL, RUNNER_TOKEN, image tag etc.
	// Treated as opaque by this package; the script template/upload
	// flow is out of scope here.
	TaskArguments []string

	// HTTPClient overrides the default *http.Client. Tests inject
	// an httptest-backed client. Nil falls back to http.DefaultClient.
	HTTPClient *http.Client

	// MaxRetries caps retry attempts for idempotent requests (GET,
	// PUT, DELETE). POST is never retried. Zero or one disables retry.
	MaxRetries int

	// InitialBackoff is the first sleep before retry. Subsequent
	// sleeps double up to a 10s ceiling, with ±25% jitter.
	InitialBackoff time.Duration
}

// Provider implements compute.Provider against the YellowDog REST API.
// One Provider instance is created per Helix deployment and shared
// across goroutines (all methods are safe for concurrent use).
type Provider struct {
	cfg     Config
	creds   credentials
	httpc   *http.Client
	baseURL string
	retry   retryConfig

	mu sync.RWMutex // guards future mutable state (none today)
}

// Compile-time conformance check. If compute.Provider drifts and
// Provider stops satisfying it, the build fails here rather than at
// the call site.
var _ compute.Provider = (*Provider)(nil)

// NewProvider validates cfg and returns a ready Provider.
func NewProvider(cfg Config) (*Provider, error) {
	creds := credentials{keyID: cfg.APIKeyID, secret: cfg.APISecret}
	if !creds.valid() {
		return nil, errors.New("yellowdog: APIKeyID and APISecret are required")
	}
	if cfg.Namespace == "" {
		return nil, errors.New("yellowdog: Namespace is required")
	}
	if cfg.DeploymentTag == "" {
		return nil, errors.New("yellowdog: DeploymentTag is required")
	}
	if cfg.WorkerTag == "" {
		return nil, errors.New("yellowdog: WorkerTag is required")
	}
	if cfg.TaskType == "" {
		cfg.TaskType = "bash"
	}
	base := cfg.BaseURL
	if base == "" {
		base = defaultBaseURL
	}
	httpc := cfg.HTTPClient
	if httpc == nil {
		httpc = http.DefaultClient
	}
	return &Provider{
		cfg:     cfg,
		creds:   creds,
		httpc:   httpc,
		baseURL: base,
		retry: retryConfig{
			maxAttempts:    cfg.MaxRetries,
			initialBackoff: cfg.InitialBackoff,
		},
	}, nil
}

// Name returns "yellowdog". Persisted on every SandboxInstance row this
// Provider creates.
func (p *Provider) Name() string { return providerName }

// Provision submits a YD work requirement that runs the configured
// bash script on a worker matching cfg.WorkerTag, then returns
// immediately with a Handle in StateProvisioning. The script is
// responsible for bringing up the helix-sandbox container, which
// later registers back to the Helix control plane via WebSocket.
//
// Per the compute.Provider contract this is fire-and-forget: the WR
// status will be RUNNING long before the helix-sandbox process inside
// it has finished booting and registering.
func (p *Provider) Provision(ctx context.Context, spec compute.Spec) (*compute.Handle, error) {
	// One TG with one task: the simplest mapping of "one Provision call
	// = one new sandbox host". Multi-task batches are out of scope.
	tgName := fmt.Sprintf("tg-%d", time.Now().UnixNano())
	wrName := fmt.Sprintf("helix-%d", time.Now().UnixNano())

	wr := workRequirement{
		Namespace: p.cfg.Namespace,
		Name:      wrName,
		Tag:       p.cfg.DeploymentTag,
		TaskGroups: []taskGroup{{
			Name: tgName,
			RunSpecification: map[string]any{
				"workerTags": []string{p.cfg.WorkerTag},
				"taskTypes":  []string{p.cfg.TaskType},
			},
		}},
	}
	created, err := p.submitWorkRequirement(ctx, &wr)
	if err != nil {
		return nil, fmt.Errorf("yellowdog: Provision: submit WR: %w", err)
	}
	if len(created.TaskGroups) == 0 || created.TaskGroups[0].ID == "" {
		return nil, fmt.Errorf("yellowdog: Provision: WR %s returned with no task groups", created.ID)
	}

	tg := created.TaskGroups[0]
	task := taskPayload{
		Name:        fmt.Sprintf("task-%d", time.Now().UnixNano()),
		TaskType:    p.cfg.TaskType,
		Arguments:   p.cfg.TaskArguments,
		Environment: mergeLabels(spec.Labels, nil),
		Timeout:     isoMinutes(p.cfg.TaskTimeout),
	}
	if err := p.addTask(ctx, tg.ID, &task); err != nil {
		// Roll back the WR so we don't leak a stuck empty task group.
		// Best effort - if cancel fails, the reaper will see an orphan
		// WR with no tasks and clean it up later.
		_ = p.cancelWorkRequirement(ctx, created.ID, true)
		return nil, fmt.Errorf("yellowdog: Provision: add task to TG %s: %w", tg.ID, err)
	}

	h := &compute.Handle{
		ProviderName: providerName,
		ProviderID:   created.ID,
		State:        compute.StateProvisioning,
		CreatedAt:    time.Now(),
		Metadata: map[string]string{
			"yd.namespace":     p.cfg.Namespace,
			"yd.work_req_name": created.Name,
			"yd.task_group_id": tg.ID,
		},
	}
	return h, nil
}

// Deprovision cancels the WR identified by handle. Force=true asks the
// platform to abort in-flight tasks rather than draining them.
// Idempotent: returns nil if the WR is already in a terminal state or
// doesn't exist (per the compute.Provider contract).
func (p *Provider) Deprovision(ctx context.Context, h *compute.Handle, opts compute.DeprovisionOpts) error {
	if h == nil || h.ProviderID == "" {
		return errors.New("yellowdog: Deprovision: handle.ProviderID required")
	}
	err := p.cancelWorkRequirement(ctx, h.ProviderID, opts.Force)
	if err == nil {
		h.State = compute.StateTerminating
		return nil
	}
	if isNotFound(err) {
		// Already gone - idempotent success.
		h.State = compute.StateTerminated
		return nil
	}
	return fmt.Errorf("yellowdog: Deprovision: %w", err)
}

// List returns Handles for every WR this provider owns - i.e. WRs in
// the configured namespace tagged with the deployment tag. Used by the
// reconciler to detect drift between Helix's view and YD's view.
func (p *Provider) List(ctx context.Context) ([]*compute.Handle, error) {
	wrs, err := p.searchWorkRequirements(ctx, p.cfg.Namespace, p.cfg.DeploymentTag)
	if err != nil {
		return nil, fmt.Errorf("yellowdog: List: %w", err)
	}
	out := make([]*compute.Handle, 0, len(wrs))
	for _, wr := range wrs {
		out = append(out, &compute.Handle{
			ProviderName: providerName,
			ProviderID:   wr.ID,
			State:        wrStatusToState(wr.Status),
			CreatedAt:    derefTime(wr.CreatedTime),
			Metadata: map[string]string{
				"yd.namespace":     wr.Namespace,
				"yd.work_req_name": wr.Name,
			},
		})
	}
	return out, nil
}

// HealthCheck fetches the WR's current status, updates handle.State, and
// returns a non-nil error if the WR is in a failure/terminal state.
func (p *Provider) HealthCheck(ctx context.Context, h *compute.Handle) error {
	if h == nil || h.ProviderID == "" {
		return errors.New("yellowdog: HealthCheck: handle.ProviderID required")
	}
	wr, err := p.getWorkRequirement(ctx, h.ProviderID)
	if err != nil {
		if isNotFound(err) {
			h.State = compute.StateTerminated
			return fmt.Errorf("yellowdog: HealthCheck: WR %s not found", h.ProviderID)
		}
		return fmt.Errorf("yellowdog: HealthCheck: %w", err)
	}
	h.State = wrStatusToState(wr.Status)
	switch h.State {
	case compute.StateFailed:
		return fmt.Errorf("yellowdog: HealthCheck: WR %s in FAILED state", h.ProviderID)
	case compute.StateTerminated:
		return fmt.Errorf("yellowdog: HealthCheck: WR %s terminated", h.ProviderID)
	}
	return nil
}

// --- Internal types -----------------------------------------------------
//
// These mirror just the YD JSON fields this package needs. They are
// unexported on purpose: YD's vocabulary stays inside this package.

type workRequirement struct {
	ID                string      `json:"id,omitempty"`
	Namespace         string      `json:"namespace"`
	Name              string      `json:"name"`
	Tag               string      `json:"tag,omitempty"`
	TaskGroups        []taskGroup `json:"taskGroups,omitempty"`
	Status            string      `json:"status,omitempty"`
	CreatedTime       *time.Time  `json:"createdTime,omitempty"`
	StatusChangedTime *time.Time  `json:"statusChangedTime,omitempty"`
}

type taskGroup struct {
	ID               string         `json:"id,omitempty"`
	Name             string         `json:"name"`
	RunSpecification map[string]any `json:"runSpecification,omitempty"`
	Status           string         `json:"status,omitempty"`
}

type taskPayload struct {
	Name        string            `json:"name"`
	TaskType    string            `json:"taskType"`
	Arguments   []string          `json:"arguments,omitempty"`
	Environment map[string]string `json:"environment,omitempty"`
	Timeout     string            `json:"timeout,omitempty"`
}

// pageOf is the standard YD pagination envelope.
type pageOf[T any] struct {
	Items       []T    `json:"items"`
	NextSliceID string `json:"nextSliceId,omitempty"`
}

// --- HTTP helper methods (one per YD call we make) ----------------------

func (p *Provider) submitWorkRequirement(ctx context.Context, wr *workRequirement) (*workRequirement, error) {
	var out workRequirement
	if err := doJSON(ctx, p.httpc, p.creds, p.baseURL, http.MethodPost, "/work/requirements", nil, wr, &out, p.retry); err != nil {
		return nil, err
	}
	return &out, nil
}

func (p *Provider) addTask(ctx context.Context, taskGroupID string, t *taskPayload) error {
	path := "/work/taskGroups/" + url.PathEscape(taskGroupID) + "/tasks"
	// The endpoint accepts an array of tasks; we only ever post one.
	body := []taskPayload{*t}
	return doJSON(ctx, p.httpc, p.creds, p.baseURL, http.MethodPost, path, nil, body, nil, p.retry)
}

func (p *Provider) getWorkRequirement(ctx context.Context, id string) (*workRequirement, error) {
	var out workRequirement
	path := "/work/requirements/" + url.PathEscape(id)
	if err := doJSON(ctx, p.httpc, p.creds, p.baseURL, http.MethodGet, path, nil, nil, &out, p.retry); err != nil {
		return nil, err
	}
	return &out, nil
}

func (p *Provider) searchWorkRequirements(ctx context.Context, namespace, tag string) ([]workRequirement, error) {
	// Walk pagination - reconciliation needs the complete set.
	var all []workRequirement
	var slice string
	for {
		q := url.Values{}
		q.Set("search", fmt.Sprintf(`{"namespace":%q,"tag":%q}`, namespace, tag))
		if slice != "" {
			q.Set("sliceReference", slice)
		}
		var page pageOf[workRequirement]
		if err := doJSON(ctx, p.httpc, p.creds, p.baseURL, http.MethodGet, "/work/requirements", q, nil, &page, p.retry); err != nil {
			return nil, err
		}
		all = append(all, page.Items...)
		if page.NextSliceID == "" {
			break
		}
		slice = page.NextSliceID
	}
	return all, nil
}

func (p *Provider) cancelWorkRequirement(ctx context.Context, id string, force bool) error {
	path := "/work/requirements/" + url.PathEscape(id) + "/transition/CANCELLED"
	var query url.Values
	if force {
		query = url.Values{"abort": []string{"true"}}
	}
	return doJSON(ctx, p.httpc, p.creds, p.baseURL, http.MethodPut, path, query, nil, nil, p.retry)
}

// --- Helpers ------------------------------------------------------------

// wrStatusToState maps the YD WorkRequirement.Status enum to the
// compute.State enum. The mapping is intentionally lossy: the YD enum
// has finer-grained distinctions than compute.State exposes, and we
// collapse the differences (e.g. CANCELLING and CANCELLED both
// become StateTerminating/StateTerminated).
func wrStatusToState(s string) compute.State {
	switch s {
	case "RUNNING":
		return compute.StateReady
	case "HELD":
		return compute.StateProvisioning
	case "COMPLETED":
		return compute.StateTerminated
	case "FAILED":
		return compute.StateFailed
	case "CANCELLING":
		return compute.StateTerminating
	case "CANCELLED":
		return compute.StateTerminated
	default:
		return compute.StateProvisioning
	}
}

func derefTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

// isoMinutes formats a duration as a YD-compatible ISO 8601 minute
// expression. Empty string for zero (signals "use platform default").
// Rounds up to the next whole minute - sub-minute timeouts make no
// sense for compute provisioning.
func isoMinutes(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	mins := int(d / time.Minute)
	if d%time.Minute > 0 {
		mins++
	}
	return fmt.Sprintf("PT%dM", mins)
}

func mergeLabels(a, b map[string]string) map[string]string {
	if len(a) == 0 && len(b) == 0 {
		return nil
	}
	out := make(map[string]string, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}
