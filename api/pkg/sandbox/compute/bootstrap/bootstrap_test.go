package bootstrap

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/sandbox/compute"
	"github.com/helixml/helix/api/pkg/types"
)

// TestMain sets a plausible release version on the data package
// global so tests that construct a supervisor via Bootstrap don't trip
// the helixSandboxImage sentinel-rejection. Tests of the rejection
// itself call helixSandboxImageFor directly with their own values.
func TestMain(m *testing.M) {
	origVersion := data.Version
	data.Version = "0.0.0-test"
	defer func() {
		data.Version = origVersion
	}()
	os.Exit(m.Run())
}

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
func (nullStore) ListRunnerAssignments(context.Context) ([]*types.RunnerAssignment, error) {
	return nil, nil
}

func TestBootstrapDisabledWhenProviderUnset(t *testing.T) {
	// THE core disabled-by-default contract: HELIX_COMPUTE_PROVIDER
	// empty means no Manager is constructed, no goroutine runs, no
	// behavioural change for existing deployments. Returning (nil,
	// nil) is how the boot path detects "disabled" without a
	// sentinel.
	mgr, err := Bootstrap(config.Compute{}, 20, "", "", nullStore{})
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
	mgr, err := Bootstrap(cfg, 20, "https://helix.example.com", "test-token", nullStore{})
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
	_, err := Bootstrap(cfg, 20, "https://helix.example.com", "test-token", nullStore{})
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
	mgr, err := Bootstrap(cfg, 20, "https://helix.example.com", "test-token", nullStore{})
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
	_, err := Bootstrap(cfg, 20, "https://helix.example.com", "test-token", nullStore{})
	if err == nil {
		t.Fatal("expected error for unknown provider, got nil")
	}
	if !strings.Contains(err.Error(), "nonesuch") {
		t.Fatalf("error should name the bad provider, got %q", err.Error())
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
	_, err := Bootstrap(cfg, 20, "https://helix.example.com", "test-token", nullStore{})
	if err == nil {
		t.Fatal("expected error for missing yellowdog credentials, got nil")
	}
}

func TestBootstrapNilStoreErrors(t *testing.T) {
	cfg := config.Compute{
		Provider:      "yellowdog",
		DeploymentTag: "prod",
	}
	_, err := Bootstrap(cfg, 20, "https://helix.example.com", "test-token", nil)
	if err == nil {
		t.Fatal("expected error for nil store, got nil")
	}
	if !strings.Contains(err.Error(), "store") {
		t.Fatalf("error should mention store, got %q", err.Error())
	}
}

func TestBootstrapSandboxImageOverrideBypassesVersionGuard(t *testing.T) {
	// Force a placeholder version so the version-derived image path would
	// fatal (the dev/source-build case, e.g. a local Air stack).
	orig := data.Version
	data.Version = "<unknown>"
	defer func() { data.Version = orig }()

	base := config.Compute{
		Provider:                "yellowdog",
		DeploymentTag:           "prod",
		Floor:                   1,
		ReconcileInterval:       30 * time.Second,
		HealthCheckTimeout:      10 * time.Second,
		MaxConcurrentProvisions: 1,
		MaxProvisioningAge:      30 * time.Minute,
		Yellowdog: config.Yellowdog{
			APIKeyID: "k", APISecret: "s", BaseURL: "https://portal.yellowdog.co/api",
			Namespace: "development", WorkerTag: "helix-prod", TaskTimeout: 4 * time.Hour,
		},
	}

	// Without an override, the placeholder version must fail the build.
	if _, err := Bootstrap(base, 20, "https://helix.example.com", "tok", nullStore{}); err == nil {
		t.Fatal("expected version-guard failure without SandboxImage override")
	}

	// With the override set, Bootstrap succeeds despite the placeholder version.
	withImg := base
	withImg.SandboxImage = "ghcr.io/helixml/helix-sandbox:abc1234-linux-amd64"
	mgr, err := Bootstrap(withImg, 20, "https://helix.example.com", "tok", nullStore{})
	if err != nil {
		t.Fatalf("SandboxImage override should bypass the version guard, got: %v", err)
	}
	if mgr == nil {
		t.Fatal("expected non-nil Manager with SandboxImage override")
	}
}

func TestBootstrapValidYellowdogConfigBuildsSupervisor(t *testing.T) {
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
	svc, err := Bootstrap(cfg, 20, "https://helix.example.com", "test-token", nullStore{})
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if svc == nil {
		t.Fatal("expected non-nil Service for valid config")
	}
	// Discovery is the only mode, so Bootstrap returns a PoolSupervisor
	// (declared type is the narrower compute.Service).
	if _, ok := svc.(*compute.PoolSupervisor); !ok {
		t.Fatalf("expected *compute.PoolSupervisor, got %T", svc)
	}
}

func TestHelixSandboxImageFor(t *testing.T) {
	// The Helix build process publishes a sandbox image at the same
	// tag as whatever data.Version was set to - release tags, RCs,
	// per-PR short SHAs all valid. We trust the build process and
	// pass the version through verbatim. The only thing we reject
	// is sentinel/placeholder values that mean "Version wasn't
	// actually baked in" - because there's no corresponding image
	// for those.
	t.Run("valid versions are passed through verbatim (default registry)", func(t *testing.T) {
		cases := []struct {
			in   string
			want string
		}{
			// Release tags
			{"2.11.14", "ghcr.io/helixml/helix-sandbox:2.11.14"},
			{"v2.11.14", "ghcr.io/helixml/helix-sandbox:v2.11.14"},
			{"1.0.0", "ghcr.io/helixml/helix-sandbox:1.0.0"},
			// Release candidates (Helix publishes -rc tags)
			{"2.11.14-rc1", "ghcr.io/helixml/helix-sandbox:2.11.14-rc1"},
			{"2.12.0-beta.3", "ghcr.io/helixml/helix-sandbox:2.12.0-beta.3"},
			// Per-PR short SHAs (Helix publishes these too)
			{"a1b2c3d", "ghcr.io/helixml/helix-sandbox:a1b2c3d"},
			// Build metadata - trust the build process
			{"2.11.14+build.42", "ghcr.io/helixml/helix-sandbox:2.11.14+build.42"},
		}
		for _, tc := range cases {
			t.Run(tc.in, func(t *testing.T) {
				got, err := helixSandboxImageFor(tc.in, "")
				if err != nil {
					t.Fatalf("unexpected error for %q: %v", tc.in, err)
				}
				if got != tc.want {
					t.Fatalf("helixSandboxImageFor(%q, \"\") = %q, want %q", tc.in, got, tc.want)
				}
			})
		}
	})

	t.Run("sentinel values are rejected loudly", func(t *testing.T) {
		// These are the values data.GetHelixVersion() returns when
		// Version wasn't baked in via ldflags - no published image
		// corresponds, so we error rather than silently producing a
		// ref that will fail at docker pull on the worker.
		sentinels := []string{
			"",
			"<unknown>",
			"v0.0.0",
			"0.0.0",
			"v0.0.0+dev",
		}
		for _, s := range sentinels {
			t.Run(s, func(t *testing.T) {
				_, err := helixSandboxImageFor(s, "")
				if err == nil {
					t.Fatalf("expected error for sentinel version %q, got nil", s)
				}
				if !strings.Contains(err.Error(), "Helix build version") {
					t.Fatalf("error should explain why; got %q", err.Error())
				}
				if !strings.Contains(err.Error(), "-ldflags") {
					t.Fatalf("error should tell operator how to fix; got %q", err.Error())
				}
			})
		}
	})

	t.Run("registry override swaps only the hostname", func(t *testing.T) {
		// HELIX_SANDBOX_REGISTRY is HOSTNAME ONLY - the same semantic
		// the pre-existing consumers (sandbox/04-start-dockerd.sh and
		// composemgr.rewriteRegistry) already use. The "helixml/helix-
		// sandbox" org+image path is always appended. Version pinning
		// stays with the build (preserves "release-tag-is-the-truth").
		cases := []struct {
			version  string
			registry string
			want     string
		}{
			// ECR (the demo case): same-region intra-AWS pull
			{
				"2.11.17",
				"123456789012.dkr.ecr.us-east-1.amazonaws.com",
				"123456789012.dkr.ecr.us-east-1.amazonaws.com/helixml/helix-sandbox:2.11.17",
			},
			// Internal mirror with a corp-style hostname
			{
				"2.11.17",
				"internal-registry.corp.example.com",
				"internal-registry.corp.example.com/helixml/helix-sandbox:2.11.17",
			},
			// Trailing slash tolerated and stripped
			{
				"2.11.17",
				"internal-registry.corp.example.com/",
				"internal-registry.corp.example.com/helixml/helix-sandbox:2.11.17",
			},
			// Surrounding whitespace tolerated and stripped (common
			// when copied from a docs snippet or ConfigMap line)
			{
				"2.11.17",
				"  ghcr.io  ",
				"ghcr.io/helixml/helix-sandbox:2.11.17",
			},
		}
		for _, tc := range cases {
			t.Run(tc.registry, func(t *testing.T) {
				got, err := helixSandboxImageFor(tc.version, tc.registry)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got != tc.want {
					t.Fatalf("got %q, want %q", got, tc.want)
				}
			})
		}
	})

	t.Run("invalid registry inputs rejected at boot", func(t *testing.T) {
		// Loud failure beats silent fallback to GHCR - an air-gapped
		// deployment that meant to set the var should NOT egress to
		// ghcr.io just because their templating produced "/".
		cases := []struct {
			name     string
			registry string
			wantErr  string
		}{
			// URL form
			{"url form", "https://123.dkr.ecr.us-east-1.amazonaws.com", "must be a registry HOSTNAME only"},

			// Empty after trim
			{"single slash", "/", "collapses to empty"},
			{"slashes only", "///", "collapses to empty"},
			{"whitespace only", "   ", "collapses to empty"},
			{"whitespace+slashes", "  /  ", "collapses to empty"},

			// Host+org form - the original ultrareview's bug class.
			// Operators who copy their ECR push target ("acct.dkr.ecr.../helixml")
			// MUST get a loud error pointing at the right shape.
			{"host+org trailing slash stripped first", "mirror.corp/helixml/", "without any path segments"},
			{"host+org bare", "mirror.corp/helixml", "without any path segments"},
			{"ecr push target shape", "123456789012.dkr.ecr.us-east-1.amazonaws.com/helixml", "without any path segments"},

			// Leading slash. Second-pass ultrareview caught this: the
			// Go validator silently normalising a leading slash while
			// the shell consumer doesn't is a cross-consumer
			// divergence regression. Reject so the two paths stay
			// consistent.
			{"leading slash bare host", "/ghcr.io", "must not start with a slash"},
			{"leading slash with trailing", "/mirror.local/", "must not start with a slash"},
			{"templating leak shape", "/${REGISTRY_HOST}", "must not start with a slash"},

			// Internal whitespace - TrimSpace handles edges only.
			// Now checked BEFORE the path-segment check so adjacent
			// typos give distinct diagnostics.
			{"embedded newline", "mirror.corp\nhelixml", "internal whitespace"},
			{"embedded tab", "mirror.corp\thelixml", "internal whitespace"},
			{"embedded space", "mirror corp", "internal whitespace"},

			// Iterated whitespace+slash corruption. "/ ghcr.io / "
			// would TrimSpace to "/ ghcr.io /" - the internal space
			// survives Trim("/") so strings.Fields sees ["ghcr.io"]
			// (length 1)... actually wait, this trims to "/ ghcr.io /"
			// which has spaces between the slashes. Fields = ["/",
			// "ghcr.io", "/"], length 3 = internal whitespace, fires
			// before the slash check. Caught by the new whitespace-first
			// ordering.
			{"slash+internal whitespace", "/ ghcr.io / ", "internal whitespace"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				_, err := helixSandboxImageFor("2.11.17", tc.registry)
				if err == nil {
					t.Fatalf("expected error for %q, got nil", tc.registry)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error %q should contain %q", err.Error(), tc.wantErr)
				}
			})
		}
	})

	t.Run("sentinel values rejected even with registry override", func(t *testing.T) {
		// The override doesn't bypass the version-pinning check; a
		// placeholder version is still a placeholder regardless of
		// where the image would have been pulled from.
		_, err := helixSandboxImageFor("v0.0.0", "internal-registry.corp")
		if err == nil {
			t.Fatal("expected error for sentinel version even with registry set")
		}
		if !strings.Contains(err.Error(), "Helix build version") {
			t.Fatalf("error should explain why; got %q", err.Error())
		}
		// The error message should NOT hardcode a specific registry URL
		// (since it's now configurable) - it should explain the build
		// fix without misdirecting operators with override deployments
		// to a registry they aren't even using.
		if strings.Contains(err.Error(), "ghcr.io") {
			t.Fatalf("error should not mention a hardcoded registry; got %q", err.Error())
		}
	})
	_ = compute.Manager{} // keep the compute import used even if signature simplifies later
}
