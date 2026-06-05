// Package compute defines the abstraction Helix uses to bring sandbox
// hosts (helix-sandbox containers running on cloud GPUs) into and out
// of existence on demand.
//
// This is a different layer from the parent api/pkg/sandbox package,
// which orchestrates the lifecycle of inner desktop sessions on top
// of hosts that already exist. The compute package answers "where
// does a host come from in the first place" - typically a cloud
// upstream like YellowDog or GCP - whereas sandbox.Controller answers
// "given a pool of running hosts, where does a user's session land
// and how is it managed."
//
// Helix has historically assumed hosts "just show up" by connecting
// via WebSocket (RevDial). That model still works for on-prem deploys
// and operator-driven bootstraps, but doesn't cover the cloud case
// where Helix itself needs to bring compute online when sessions
// arrive and tear it down when they go idle.
//
// A Provider implementation knows how to talk to one upstream
// compute system and translates Helix's "I need a sandbox host"
// intent into that system's primitives. The first concrete
// implementation lives at api/pkg/sandbox/compute/yellowdog and
// targets the YellowDog REST API.
package compute

import (
	"context"
	"time"
)

// Provider provisions, observes, and reaps sandbox hosts on some
// upstream system. Implementations are typically constructed once
// per Helix deployment (or once per organisation, when credentials
// are org-scoped) and shared across goroutines.
//
// All methods take a context for cancellation. Most are short
// requests against the upstream provider's API; Provision is
// fire-and-forget (see its docstring).
type Provider interface {
	// Name returns the short identifier of this provider implementation,
	// e.g. "yellowdog", "gcp", "lambda". Stable across releases; used
	// as the value of SandboxInstance.Provider when a host is
	// registered.
	Name() string

	// Provision requests a new sandbox host matching spec.
	//
	// The call is fire-and-forget: it returns as soon as the upstream
	// has accepted the request, NOT when the host has finished booting
	// and registered. Provisioning a cloud GPU typically takes 10+
	// minutes; blocking that long is unacceptable for callers like an
	// HTTP handler responding to a user starting a session.
	//
	// The returned Handle carries the upstream's opaque ID (e.g. a
	// YellowDog work-requirement YDID) and is in StateProvisioning.
	// Callers observe progress via HealthCheck and via the host
	// registering through the existing WebSocket path (which writes
	// the SandboxInstance row separately).
	Provision(ctx context.Context, spec Spec) (*Handle, error)

	// Deprovision asks the upstream to tear down the host identified
	// by handle. Defaults to graceful (the upstream sends SIGTERM /
	// abort signals; the host's bash-script trap stops the
	// helix-sandbox container with a grace period; cloud resources
	// reap after that). Pass DeprovisionOpts.Force to skip the grace
	// period.
	//
	// Returns nil even if the handle was already torn down (idempotent).
	// Returns an error if the upstream is unreachable or the request
	// is rejected.
	Deprovision(ctx context.Context, handle *Handle, opts DeprovisionOpts) error

	// List returns all host handles currently known to this provider.
	// Used for reconciliation: comparing what Helix thinks is running
	// against what the upstream says exists. Pagination, if relevant
	// for the upstream, is hidden inside the implementation.
	List(ctx context.Context) ([]*Handle, error)

	// HealthCheck queries the upstream for the current state of one
	// handle and updates handle.State accordingly. Returns nil if the
	// underlying resource exists and is in a non-failure state.
	// Returns a non-nil error if the resource is missing, failed, or
	// the provider is unreachable.
	//
	// Cheap to call - implementations should issue at most one upstream
	// API call. The dispatcher / reconciler is expected to poll this
	// per handle on a low-frequency schedule (every 30-60 seconds).
	HealthCheck(ctx context.Context, handle *Handle) error
}

// Spec describes what kind of sandbox host the caller wants. Provider
// implementations translate these into upstream-specific primitives
// (YellowDog work requirements, GCP instance templates, etc.).
//
// Fields are intentionally narrow: anything that requires provider-
// specific knowledge (instance type, image family, region preference)
// is configured operator-side on the provider itself, not in Spec.
// The provider's defaults apply unless a field here overrides them.
//
// Free-form hints that don't fit the common fields go in Labels.
// Implementations may interpret known label keys; unknown keys are
// ignored.
type Spec struct {
	// GPUVendor selects the accelerator family the host needs.
	// Values: "nvidia", "amd", "intel", or "" for any.
	GPUVendor string

	// Image is the helix-sandbox container image to run.
	// E.g. "ghcr.io/helixml/helix-sandbox:v2.11.14" or a SHA tag.
	// Empty means the provider chooses its configured default.
	Image string

	// MaxSandboxes caps how many concurrent inner desktop containers
	// hydra will spawn on this host. 0 means the provider's default.
	MaxSandboxes int

	// Labels carries provider-specific hints not modelled above.
	// Examples: "yellowdog.compute_source_template_id",
	// "yellowdog.worker_tag", "region_preference".
	Labels map[string]string
}

// Handle is the provider-side reference to a single sandbox host's
// compute. It survives across Helix process restarts (the opaque
// ProviderID is what we ask the upstream about during reconciliation)
// and is the link between Helix's SandboxInstance row and whatever
// the upstream system calls the resource that hosts it.
//
// At Provision time, the handle has ProviderID set and SandboxID
// empty. The SandboxID is filled in later, when the host phones home
// via WebSocket and the heartbeat handler matches its
// SANDBOX_INSTANCE_ID env var to one of the provisioning handles.
type Handle struct {
	// ProviderName is the Name() of the Provider that owns this
	// handle. Lets persistence layers look up the right impl when
	// loading handles from storage.
	ProviderName string

	// ProviderID is the upstream system's opaque identifier for the
	// host. For YellowDog, this is the work-requirement YDID. The
	// provider treats this as its primary key.
	ProviderID string

	// SandboxID is the Helix SandboxInstance.ID of the host that
	// this handle backs. Empty until the host registers. Once set,
	// it stays set even after the host deregisters (so we can do
	// post-mortems).
	SandboxID string

	// State is the provider's view of the host's lifecycle.
	State State

	// CreatedAt is when Provision was called for this handle.
	CreatedAt time.Time

	// Metadata is provider-specific opaque data for reconciliation,
	// debugging, and admin display. Examples: YellowDog worker-pool ID,
	// EC2 instance ID, region, public IP. Persisted with the handle.
	Metadata map[string]string
}

// State enumerates the lifecycle stages of a provisioned sandbox host.
// String values are persisted directly in SandboxInstance.ComputeState
// (varchar column), so renaming or adding a value here is a schema
// change - keep the literals stable.
type State string

const (
	// StateProvisioning means Provision was called and the upstream
	// accepted the request, but the host has not yet registered.
	StateProvisioning State = "provisioning"

	// StateReady means the host has registered with the control
	// plane and is serving sessions.
	StateReady State = "ready"

	// StateTerminating means Deprovision was called and graceful
	// teardown is in flight.
	StateTerminating State = "terminating"

	// StateTerminated means the host is fully gone, upstream
	// resources reaped.
	StateTerminated State = "terminated"

	// StateFailed means provisioning or running failed in a way that
	// won't recover without operator intervention. The handle's
	// Metadata typically includes the failure reason.
	StateFailed State = "failed"
)

// DeprovisionOpts tunes Deprovision behaviour.
type DeprovisionOpts struct {
	// Force skips the graceful shutdown grace period and asks the
	// upstream to terminate the host immediately. Use sparingly;
	// in-flight session work is lost.
	Force bool

	// Reason is a free-form note attached to the deprovision event
	// for audit / admin display. Optional.
	Reason string
}
