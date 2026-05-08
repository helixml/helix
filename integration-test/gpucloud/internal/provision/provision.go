// Package provision is the harness's GPU-cloud abstraction. The
// Provisioner interface lets us swap implementations: real cloud APIs in
// CI, a dry-run that logs intent for local development.
//
// The harness uses two real providers:
//   - Hot Aisle (admin.hotaisle.app/api/docs/) for AMD MI300X — the only
//     self-serve real-VM cloud with on-demand MI300X stock as of Apr 2026.
//   - Verda (api.verda.com/v1/docs, formerly DataCrunch) for NVIDIA L40S
//     and A100 — KVM VMs, full root, cheapest on-demand pricing among
//     self-serve providers we evaluated.
//
// Each provisioned instance runs a cloud-init that:
//   1. Installs nvidia-container-toolkit (NVIDIA) or sets up ROCm + AMD
//      device permissions (AMD).
//   2. Pulls the helix-sandbox image.
//   3. Boots it with HELIX_API_URL + RUNNER_TOKEN + SANDBOX_INSTANCE_ID
//      pointing at the test API.
//
// Both providers also auto-terminate at 35 min via their own
// inactivity/lifecycle settings as a belt-and-braces budget guard.
package provision

import (
	"context"
	"fmt"
	"time"
)

// Provider names a cloud-side implementation. Matrix entries pick one.
type Provider string

const (
	ProviderHotAisle Provider = "hotaisle"
	ProviderVerda    Provider = "verda"
	ProviderDryRun   Provider = "dryrun"
)

// PodSpec is what callers ask for. Field meanings depend on the provider:
//
//   - Hot Aisle: InstanceType selects the VM SKU (e.g. "1xMI300X",
//     "8xMI300X"). GPUCount is informational; the SKU determines GPU
//     allocation.
//   - Verda: InstanceType is the Verda type slug (e.g. "1A100.22V",
//     "4L40S"). Region is the Verda region code.
//
// The harness keeps the field set small and provider-agnostic;
// per-provider quirks live in the implementation.
type PodSpec struct {
	EntryID      string // matrix entry ID — used as the sandbox instance ID
	Provider     Provider
	InstanceType string // provider-specific SKU/type slug
	GPUCount     int
	Region       string
	ImageRef     string // helix-sandbox image to pull from cloud-init
}

// Pod is what callers receive.
type Pod struct {
	ID           string   // provider-specific instance ID — used for teardown
	URL          string   // public URL of the running sandbox (for the harness to hit)
	Provider     Provider // echoed for reporting + correct teardown dispatch
	InstanceType string   // echoed for reporting
	GPUCount     int
	Started      time.Time
}

// Provisioner abstracts a single cloud provider.
type Provisioner interface {
	Provision(ctx context.Context, spec PodSpec) (*Pod, error)
	Teardown(ctx context.Context, podID string) error
	TodaySpentUSD(ctx context.Context) (float64, error)
}

// Multi dispatches a PodSpec to the right per-provider Provisioner based
// on spec.Provider. Teardown uses pod.Provider since the pod ID alone
// doesn't say which API to call.
type Multi struct {
	byProvider map[Provider]Provisioner
}

// NewMulti returns a dispatcher. Pass nil for a provider you don't want
// to enable — Provision will reject entries that target it.
func NewMulti(impls map[Provider]Provisioner) *Multi {
	return &Multi{byProvider: impls}
}

func (m *Multi) Provision(ctx context.Context, spec PodSpec) (*Pod, error) {
	p, ok := m.byProvider[spec.Provider]
	if !ok {
		return nil, fmt.Errorf("provider %q not configured (set its API key env var)", spec.Provider)
	}
	pod, err := p.Provision(ctx, spec)
	if err != nil {
		return nil, err
	}
	pod.Provider = spec.Provider
	return pod, nil
}

// TeardownByPod dispatches teardown using the pod's Provider field.
// Callers should prefer this over the bare Teardown method since they
// already have the Pod in hand.
func (m *Multi) TeardownByPod(ctx context.Context, pod *Pod) error {
	p, ok := m.byProvider[pod.Provider]
	if !ok {
		return fmt.Errorf("provider %q not configured for teardown of %s", pod.Provider, pod.ID)
	}
	return p.Teardown(ctx, pod.ID)
}

// Teardown on the Multi is unsafe — we don't know which provider owns
// the ID. It exists only to satisfy the Provisioner interface for code
// paths that already have a Pod handle and use TeardownByPod.
func (m *Multi) Teardown(_ context.Context, podID string) error {
	return fmt.Errorf("Multi.Teardown(%s) requires a Pod; use TeardownByPod", podID)
}

// TodaySpentUSD sums spend across all configured providers.
func (m *Multi) TodaySpentUSD(ctx context.Context) (float64, error) {
	var total float64
	for name, p := range m.byProvider {
		spent, err := p.TodaySpentUSD(ctx)
		if err != nil {
			return total, fmt.Errorf("billing for %s: %w", name, err)
		}
		total += spent
	}
	return total, nil
}

// --- dry-run ---

type dryRun struct{}

// NewDryRun returns a Provisioner that logs intent without calling any cloud.
func NewDryRun() Provisioner { return &dryRun{} }

func (d *dryRun) Provision(_ context.Context, spec PodSpec) (*Pod, error) {
	return &Pod{
		ID:           fmt.Sprintf("dryrun-%s", spec.EntryID),
		URL:          "http://dryrun.invalid",
		Provider:     ProviderDryRun,
		InstanceType: spec.InstanceType,
		GPUCount:     spec.GPUCount,
		Started:      time.Now(),
	}, nil
}

func (d *dryRun) Teardown(_ context.Context, _ string) error      { return nil }
func (d *dryRun) TodaySpentUSD(_ context.Context) (float64, error) { return 0, nil }
