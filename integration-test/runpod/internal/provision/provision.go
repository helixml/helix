// Package provision is the harness's RunPod abstraction. The Provisioner
// interface lets us swap implementations: real RunPod calls in production,
// a dry-run that logs intent for local development, future hyperscaler
// providers (Lambda, Crusoe, AWS spot) when we add them.
//
// The real RunPod implementation talks to the v2 REST API:
//   POST /v2/pod                              -> create
//   GET  /v2/pod/{id}                         -> status
//   POST /v2/pod/{id}/stop                    -> teardown
//   GET  /v2/billing/usage?start=YYYY-MM-DD   -> daily spend
//
// Each provisioned pod runs a cloud-init that:
//   1. Installs nvidia-container-toolkit (or AMD equivalent).
//   2. Pulls the helix-sandbox image.
//   3. Boots it with HELIX_API_URL + RUNNER_TOKEN + SANDBOX_INSTANCE_ID
//      pointing at the test API.
//
// Pods auto-terminate at 35 min via RunPod's `--termination-time` so a
// stuck harness doesn't leak GPU spend.
package provision

import (
	"context"
	"fmt"
	"time"
)

// PodSpec is what callers ask for.
type PodSpec struct {
	EntryID  string // matrix entry ID — used as the sandbox instance ID
	GPUType  string // RunPod GPU type string (e.g. "NVIDIA H100 80GB HBM3")
	GPUCount int
	Region   string
	ImageRef string // helix-sandbox image to pull
}

// Pod is what callers receive.
type Pod struct {
	ID       string    // RunPod pod ID — used for teardown
	URL      string    // public URL of the running sandbox (for the harness to hit)
	GPUType  string    // echoed back for reporting
	GPUCount int
	Started  time.Time
}

// Provisioner abstracts the cloud provider. NewRunPodProvisioner returns
// the real one; NewDryRun returns a no-op for local planning.
type Provisioner interface {
	Provision(ctx context.Context, spec PodSpec) (*Pod, error)
	Teardown(ctx context.Context, podID string) error
	TodaySpentUSD(ctx context.Context) (float64, error)
}

// --- dry-run ---

type dryRun struct{}

// NewDryRun returns a Provisioner that logs intent without calling RunPod.
func NewDryRun() Provisioner { return &dryRun{} }

func (d *dryRun) Provision(_ context.Context, spec PodSpec) (*Pod, error) {
	return &Pod{
		ID:       fmt.Sprintf("dryrun-%s", spec.EntryID),
		URL:      "http://dryrun.invalid",
		GPUType:  spec.GPUType,
		GPUCount: spec.GPUCount,
		Started:  time.Now(),
	}, nil
}

func (d *dryRun) Teardown(_ context.Context, _ string) error      { return nil }
func (d *dryRun) TodaySpentUSD(_ context.Context) (float64, error) { return 0, nil }
