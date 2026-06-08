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
	"strings"
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

	// AllowInsecureBaseURL permits BaseURL with an http:// scheme.
	// Off by default - sending a yd-key auth header over cleartext
	// is a credential leak. Set true only for an httptest server in
	// unit tests.
	AllowInsecureBaseURL bool
}

// Provider implements compute.Provider against the YellowDog REST API.
// One Provider instance is created per Helix deployment and shared
// across goroutines (all methods are safe for concurrent use - all
// fields are set at NewProvider time and never mutated).
type Provider struct {
	cfg     Config
	creds   credentials
	httpc   *http.Client
	baseURL string
	retry   retryConfig
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
	// Reject CR/LF in credentials to foreclose header-injection. Risk
	// is theoretical (creds are operator-supplied, not user input) but
	// the check is trivial and prevents a class of mistakes if creds
	// ever start flowing through less-trusted paths.
	if strings.ContainsAny(cfg.APIKeyID, "\r\n") || strings.ContainsAny(cfg.APISecret, "\r\n") {
		return nil, errors.New("yellowdog: APIKeyID and APISecret must not contain CR or LF")
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
	// HTTPS is mandatory: the yd-key auth header is sent on every
	// request. Operators who genuinely need http:// (httptest in unit
	// tests, or a local mock during dev) must opt in explicitly via
	// AllowInsecureBaseURL.
	if !cfg.AllowInsecureBaseURL && !strings.HasPrefix(base, "https://") {
		return nil, fmt.Errorf("yellowdog: BaseURL must use https:// (got %q); set AllowInsecureBaseURL=true to override (tests only)", base)
	}
	httpc := cfg.HTTPClient
	if httpc == nil {
		// Build a private client that (a) bounds total request time
		// so a hung TCP connection from a broken upstream can't pin
		// a goroutine forever, and (b) refuses to follow redirects
		// so an upstream 302 cannot replay the yd-key Authorization
		// header against an attacker-influenced host.
		httpc = &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
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
// bash script on a worker matching cfg.WorkerTag, then returns a
// Handle in StateProvisioning. The script is responsible for bringing
// up the helix-sandbox container, which later registers back to the
// Helix control plane via WebSocket.
//
// Per the compute.Provider contract this is fire-and-forget with
// respect to *boot*: the WR status will be RUNNING long before the
// helix-sandbox process inside it has finished booting and
// registering. It is NOT fire-and-forget with respect to *submission*
// - we make two synchronous calls (POST WR, POST task) and one
// synchronous rollback (PUT cancel) on the second's failure. These
// are short JSON requests to the YD control plane (~hundreds of ms);
// the latency is bounded and caller error reporting is informative.
// If the upstream is slow this can block the caller for a few
// seconds, so HTTP handlers should run Provision behind a queue or
// in a request-scoped goroutine, not inline on the response path.
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
		state, _ := wrStatusToState(wr.Status)
		if state == "" {
			// Unknown YD status. Surface as Failed so the reconciler
			// notices the drift rather than silently treating it as
			// "still provisioning".
			state = compute.StateFailed
		}
		out = append(out, &compute.Handle{
			ProviderName: providerName,
			ProviderID:   wr.ID,
			State:        state,
			CreatedAt:    derefTime(wr.CreatedTime),
			Metadata: map[string]string{
				"yd.namespace":     wr.Namespace,
				"yd.work_req_name": wr.Name,
				"yd.status":        wr.Status,
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
	state, known := wrStatusToState(wr.Status)
	if !known {
		// Unknown YD status. Don't claim a State (leave it as-is from
		// the caller) and surface the drift loudly so we notice when
		// YD adds a new enum value we haven't taught the mapper.
		return fmt.Errorf("yellowdog: HealthCheck: WR %s in unknown YD status %q", h.ProviderID, wr.Status)
	}
	h.State = state
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
	// Three surprises confirmed live on 2026-06-05:
	//
	//   1. GET /work/requirements returns a flat JSON array, NOT the
	//      {items, nextSliceId} envelope the OpenAPI spec describes.
	//
	//   2. The `search` query parameter is silently ignored by the
	//      server - even garbage like `search=not-json` returns 200
	//      with the full unfiltered list. We still send it (in case
	//      it starts working someday) but cannot rely on it.
	//
	//   3. The API key's visibility scope is the only actual filter:
	//      we see WRs created by this key and no others. The result
	//      set is naturally small for a dedicated-key deployment.
	//
	// Because (2) is unsafe to assume, we re-filter client-side on
	// (namespace, tag) so a future YD account that contains WRs from
	// other tools using the same key won't be mistaken for ours.
	q := url.Values{}
	q.Set("search", fmt.Sprintf(`{"namespace":%q,"tag":%q}`, namespace, tag))
	var raw []workRequirement
	if err := doJSON(ctx, p.httpc, p.creds, p.baseURL, http.MethodGet, "/work/requirements", q, nil, &raw, p.retry); err != nil {
		return nil, err
	}
	out := raw[:0]
	for _, wr := range raw {
		if wr.Namespace == namespace && wr.Tag == tag {
			out = append(out, wr)
		}
	}
	return out, nil
}

func (p *Provider) cancelWorkRequirement(ctx context.Context, id string, force bool) error {
	// YD enforces a state machine: RUNNING/HELD -> CANCELLING ->
	// CANCELLED. We can only request the transition to CANCELLING;
	// the platform propagates to CANCELLED on its own once the
	// in-flight tasks drain (or are aborted, if abort=true).
	// Trying to PUT .../transition/CANCELLED directly returns
	// HTTP 403 InvalidWorkRequirementStatusException. Confirmed
	// live 2026-06-05.
	path := "/work/requirements/" + url.PathEscape(id) + "/transition/CANCELLING"
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
//
// COMPLETED maps to StateTerminated rather than StateReady because
// each WR runs exactly one task that brings up exactly one host:
// when YD reports COMPLETED, the task has finished and the host
// underneath it is gone. The "host is healthy and running" signal
// comes from RUNNING, not COMPLETED.
//
// Returns ("", false) for unknown statuses so the caller can decide
// whether to treat that as failure or schema drift. Silently
// defaulting to StateProvisioning hides real bugs - e.g. YD adding
// a STARTING enum value would have HealthCheck cheerfully report
// "still booting" forever.
func wrStatusToState(s string) (compute.State, bool) {
	switch s {
	case "RUNNING":
		return compute.StateReady, true
	case "HELD":
		return compute.StateProvisioning, true
	case "COMPLETED":
		return compute.StateTerminated, true
	case "FAILED":
		return compute.StateFailed, true
	case "CANCELLING":
		return compute.StateTerminating, true
	case "CANCELLED":
		return compute.StateTerminated, true
	default:
		return "", false
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
