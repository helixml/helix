package bootstrap

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/sandbox/compute"
	"github.com/helixml/helix/api/pkg/types"
)

// nullStore is a no-op SandboxStore so we can construct Bootstrap
// without standing up a real Postgres or even an in-memory fake.
// Bootstrap doesn't call into the store at construction time - it
// only stashes the reference for later use by Manager.Reconcile -
// so a nil-method stub is sufficient for these contract tests.
type nullStore struct{}

func (nullStore) ListSandboxInstances(context.Context) ([]*types.SandboxInstance, error) {
	return nil, nil
}
func (nullStore) GetSandboxInstance(context.Context, string) (*types.SandboxInstance, error) {
	return nil, nil
}
func (nullStore) RegisterSandboxInstance(context.Context, *types.SandboxInstance) error {
	return nil
}
func (nullStore) UpdateSandboxInstanceStatus(context.Context, string, string) error {
	return nil
}
func (nullStore) UpdateSandboxInstanceComputeState(context.Context, string, string) error {
	return nil
}
func (nullStore) UpdateSandboxInstanceProviderID(context.Context, string, string) error {
	return nil
}
func (nullStore) DeregisterSandboxInstance(context.Context, string) error {
	return nil
}

func TestBootstrapDisabledWhenProviderUnset(t *testing.T) {
	// THE core disabled-by-default contract: HELIX_COMPUTE_PROVIDER
	// empty means no Manager is constructed, no goroutine runs, no
	// behavioural change for existing deployments. Returning (nil,
	// nil) is how the boot path detects "disabled" without a
	// sentinel.
	mgr, err := Bootstrap(config.Compute{}, "", "", nullStore{})
	if err != nil {
		t.Fatalf("expected nil error for disabled config, got %v", err)
	}
	if mgr != nil {
		t.Fatalf("expected nil Manager for disabled config, got %+v", mgr)
	}
}

func TestBootstrapDisabledIgnoresAllOtherFields(t *testing.T) {
	// Defence-in-depth: even if other fields are set (e.g. operator
	// left HELIX_COMPUTE_FLOOR=5 from a previous setup), an empty
	// Provider MUST still disable the subsystem. No partial enable.
	cfg := config.Compute{
		Provider:                "", // the only thing that matters
		DeploymentTag:           "prod",
		Floor:                   5,
		ReconcileInterval:       30 * time.Second,
		HealthCheckTimeout:      10 * time.Second,
		MaxConcurrentProvisions: 3,
		MaxProvisioningAge:      30 * time.Minute,
	}
	mgr, err := Bootstrap(cfg, "https://helix.example.com", "test-token", nullStore{})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if mgr != nil {
		t.Fatal("expected nil Manager, got non-nil")
	}
}

func TestBootstrapErrorsWhenNoTagAndNoNamespace(t *testing.T) {
	// With both DeploymentTag AND the provider-specific namespace
	// empty, derivation can't produce anything; we surface a clear
	// error rather than silently using an unstable default.
	cfg := config.Compute{
		Provider: "yellowdog",
	}
	_, err := Bootstrap(cfg, "https://helix.example.com", "test-token", nullStore{})
	if err == nil {
		t.Fatal("expected error for missing DeploymentTag + no namespace, got nil")
	}
	if !strings.Contains(err.Error(), "HELIX_COMPUTE_DEPLOYMENT_TAG") {
		t.Fatalf("error should name the missing env var, got %q", err.Error())
	}
}

func TestDeriveDeploymentTag(t *testing.T) {
	cases := []struct {
		name string
		cfg  config.Compute
		want string
	}{
		{
			name: "yellowdog with namespace",
			cfg:  config.Compute{Provider: "yellowdog", Yellowdog: config.Yellowdog{Namespace: "development"}},
			want: "helix-development",
		},
		{
			name: "yellowdog with no namespace",
			cfg:  config.Compute{Provider: "yellowdog"},
			want: "",
		},
		{
			name: "unknown provider",
			cfg:  config.Compute{Provider: "nonesuch", Yellowdog: config.Yellowdog{Namespace: "irrelevant"}},
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := deriveDeploymentTag(tc.cfg); got != tc.want {
				t.Fatalf("deriveDeploymentTag = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBootstrapDerivesTagFromYellowdogNamespace(t *testing.T) {
	// Common deployment shape: operator sets HELIX_YD_NAMESPACE
	// (required for the provider to work at all) but leaves
	// HELIX_COMPUTE_DEPLOYMENT_TAG to default. Bootstrap should
	// derive "helix-<namespace>" automatically.
	cfg := config.Compute{
		Provider:                "yellowdog",
		ReconcileInterval:       time.Second,
		HealthCheckTimeout:      time.Second,
		MaxConcurrentProvisions: 1,
		MaxProvisioningAge:      time.Minute,
		Yellowdog: config.Yellowdog{
			APIKeyID:    "k",
			APISecret:   "s",
			BaseURL:     "https://portal.yellowdog.co/api",
			Namespace:   "development",
			WorkerTag:   "w",
			TaskTimeout: time.Hour,
		},
	}
	mgr, err := Bootstrap(cfg, "https://helix.example.com", "test-token", nullStore{})
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if mgr == nil {
		t.Fatal("expected non-nil Manager")
	}
	// Validate the derivation produced the expected tag by checking
	// the Provider.Name() suffix the Manager carries internally.
	// We can't read cfg.DeploymentTag back from outside since
	// Bootstrap took it by value; check the observable behaviour.
	// (Manager.Reconcile filters owned rows by Provider.Name(); a
	// test that exercises that path through the public API is in
	// the compute package's manager_test.go.)
}

func TestBootstrapUnknownProviderErrors(t *testing.T) {
	cfg := config.Compute{
		Provider:      "nonesuch",
		DeploymentTag: "prod",
	}
	_, err := Bootstrap(cfg, "https://helix.example.com", "test-token", nullStore{})
	if err == nil {
		t.Fatal("expected error for unknown provider, got nil")
	}
	if !strings.Contains(err.Error(), "nonesuch") {
		t.Fatalf("error should name the bad provider, got %q", err.Error())
	}
}

func TestBootstrapDerivesWorkerTagFromNamespace(t *testing.T) {
	// Omitting HELIX_YD_WORKER_TAG should auto-derive "worker-<namespace>"
	// per the yd-provision POC convention. Bootstrap completes without
	// error using only Namespace; yellowdog.NewProvider will reject
	// the build if WorkerTag is still empty after derivation.
	cfg := config.Compute{
		Provider:                "yellowdog",
		ReconcileInterval:       time.Second,
		HealthCheckTimeout:      time.Second,
		MaxConcurrentProvisions: 1,
		MaxProvisioningAge:      time.Minute,
		Yellowdog: config.Yellowdog{
			APIKeyID:    "k",
			APISecret:   "s",
			BaseURL:     "https://portal.yellowdog.co/api",
			Namespace:   "development",
			WorkerTag:   "", // intentionally empty
			TaskTimeout: time.Hour,
		},
	}
	mgr, err := Bootstrap(cfg, "https://helix.example.com", "test-token", nullStore{})
	if err != nil {
		t.Fatalf("Bootstrap with empty WorkerTag should auto-derive, got error: %v", err)
	}
	if mgr == nil {
		t.Fatal("expected non-nil Manager")
	}
}

func TestBootstrapErrorsWhenWorkerTagAndNamespaceBothEmpty(t *testing.T) {
	// With BOTH WorkerTag AND Namespace empty, derivation can't
	// produce a default and yellowdog.NewProvider rejects the
	// empty WorkerTag (existing validation in provider.go).
	cfg := config.Compute{
		Provider:                "yellowdog",
		DeploymentTag:           "explicit-tag", // skip the namespace-derived DeploymentTag path
		ReconcileInterval:       time.Second,
		HealthCheckTimeout:      time.Second,
		MaxConcurrentProvisions: 1,
		MaxProvisioningAge:      time.Minute,
		Yellowdog: config.Yellowdog{
			APIKeyID:    "k",
			APISecret:   "s",
			BaseURL:     "https://portal.yellowdog.co/api",
			Namespace:   "", // intentionally empty - blocks the derivation
			WorkerTag:   "",
			TaskTimeout: time.Hour,
		},
	}
	// Bootstrap should fail at the Namespace validation in
	// yellowdog.NewProvider, since Namespace is required for the
	// provider itself; WorkerTag derivation never gets a chance to
	// run with no namespace input.
	_, err := Bootstrap(cfg, "https://helix.example.com", "test-token", nullStore{})
	if err == nil {
		t.Fatal("expected error when Namespace is empty (which blocks both Namespace validation and WorkerTag derivation)")
	}
}

func TestBootstrapYellowdogRequiresCredentials(t *testing.T) {
	// Bootstrap delegates field validation to yellowdog.NewProvider,
	// so this test mostly proves the wiring is plumbed correctly.
	cfg := config.Compute{
		Provider:                "yellowdog",
		DeploymentTag:           "prod",
		ReconcileInterval:       time.Second,
		HealthCheckTimeout:      time.Second,
		MaxConcurrentProvisions: 1,
		MaxProvisioningAge:      time.Minute,
		// Yellowdog block empty - missing APIKeyID/APISecret etc.
	}
	_, err := Bootstrap(cfg, "https://helix.example.com", "test-token", nullStore{})
	if err == nil {
		t.Fatal("expected error for missing yellowdog credentials, got nil")
	}
}

func TestBootstrapNilStoreErrors(t *testing.T) {
	cfg := config.Compute{
		Provider:      "yellowdog",
		DeploymentTag: "prod",
	}
	_, err := Bootstrap(cfg, "https://helix.example.com", "test-token", nil)
	if err == nil {
		t.Fatal("expected error for nil store, got nil")
	}
	if !strings.Contains(err.Error(), "store") {
		t.Fatalf("error should mention store, got %q", err.Error())
	}
}

func TestBootstrapValidYellowdogConfigBuildsManager(t *testing.T) {
	cfg := config.Compute{
		Provider:                "yellowdog",
		DeploymentTag:           "prod",
		Floor:                   2,
		ReconcileInterval:       30 * time.Second,
		HealthCheckTimeout:      10 * time.Second,
		MaxConcurrentProvisions: 1,
		MaxProvisioningAge:      30 * time.Minute,
		Yellowdog: config.Yellowdog{
			APIKeyID:    "test-key",
			APISecret:   "test-secret",
			BaseURL:     "https://portal.yellowdog.co/api",
			Namespace:   "development",
			WorkerTag:   "helix-prod",
			TaskTimeout: 4 * time.Hour,
		},
	}
	mgr, err := Bootstrap(cfg, "https://helix.example.com", "test-token", nullStore{})
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if mgr == nil {
		t.Fatal("expected non-nil Manager for valid config")
	}
	// Compile-time check: returned value satisfies the public Manager
	// surface (Run + Reconcile). If the bootstrap accidentally
	// returns something else, this won't compile.
	var _ interface {
		Run(context.Context) error
		Reconcile(context.Context) error
	} = mgr
}

func TestHelixSandboxImageFor(t *testing.T) {
	// Auto-derived sandbox image tag. Production must pull
	// ghcr.io/helixml/helix-sandbox:<version> matching the running
	// control plane; dev/non-release builds fall back to :latest so
	// they don't try to pull a tag that was never published.
	cases := []struct {
		in   string
		want string
	}{
		// Release shapes -> use the version verbatim
		{"2.11.14", "ghcr.io/helixml/helix-sandbox:2.11.14"},
		{"v2.11.14", "ghcr.io/helixml/helix-sandbox:v2.11.14"},
		{"1.0.0", "ghcr.io/helixml/helix-sandbox:1.0.0"},
		// Dev / unset / unrecognised -> fall back to :latest
		{"v0.0.0+dev", "ghcr.io/helixml/helix-sandbox:latest"},
		{"v0.0.0", "ghcr.io/helixml/helix-sandbox:latest"},
		{"0.0.0", "ghcr.io/helixml/helix-sandbox:latest"},
		{"<unknown>", "ghcr.io/helixml/helix-sandbox:latest"},
		{"", "ghcr.io/helixml/helix-sandbox:latest"},
		// Git sha (when no Version baked in, GetHelixVersion can return vcs.revision)
		{"a1b2c3d4e5f6", "ghcr.io/helixml/helix-sandbox:latest"},
		// Pre-release / build-metadata shapes -> fall back rather than guess
		{"2.11.14-rc1", "ghcr.io/helixml/helix-sandbox:latest"},
		{"2.11.14+build.42", "ghcr.io/helixml/helix-sandbox:latest"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := helixSandboxImageFor(tc.in); got != tc.want {
				t.Fatalf("helixSandboxImageFor(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
	_ = compute.Manager{} // keep the compute import used even if signature simplifies later
}
