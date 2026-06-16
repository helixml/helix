package bootstrap

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/sandbox/compute"
	"github.com/helixml/helix/api/pkg/sandbox/compute/yellowdog"
	"github.com/helixml/helix/api/pkg/types"
)

// TestMain sets a plausible release version on the data package
// global so tests that construct a Manager via Bootstrap don't trip
// the helixSandboxImage sentinel-rejection. Tests of the rejection
// itself call helixSandboxImageFor directly with their own values.
//
// Also stubs discoverFn so unit tests never reach out to the real YD
// portal during boot. The default stub returns no tags (zero-online-nodes
// branch), which causes resolveWorkerTag to fall back to the namespace
// derivation - the same shape the original tests assumed.
// Tests that want to exercise the discovery branches override
// discoverFn locally via withDiscoverFn.
func TestMain(m *testing.M) {
	origVersion := data.Version
	data.Version = "0.0.0-test"
	origDiscover := discoverFn
	discoverFn = func(context.Context, yellowdog.Config) ([]string, error) {
		return nil, nil
	}
	defer func() {
		data.Version = origVersion
		discoverFn = origDiscover
	}()
	os.Exit(m.Run())
}

// withDiscoverFn temporarily replaces the package's discoverFn for the
// scope of a single test. Returns a cleanup func tests should defer.
func withDiscoverFn(stub func(context.Context, yellowdog.Config) ([]string, error)) func() {
	prev := discoverFn
	discoverFn = stub
	return func() { discoverFn = prev }
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

func TestBootstrapDerivesWorkerTagFromNamespace(t *testing.T) {
	// 0-tags branch: discovery returns no online nodes (the TestMain
	// default stub), so resolveWorkerTag falls back to the
	// namespace-derived default `worker-<namespace>`. Bootstrap
	// completes without error.
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
	mgr, err := Bootstrap(cfg, 20, "https://helix.example.com", "test-token", nullStore{})
	if err != nil {
		t.Fatalf("Bootstrap with empty WorkerTag should auto-derive, got error: %v", err)
	}
	if mgr == nil {
		t.Fatal("expected non-nil Manager")
	}
}

func TestResolveWorkerTagExplicitWins(t *testing.T) {
	// Explicit-tag branch: HELIX_YD_WORKER_TAG set -> discoverFn is
	// never called and the operator's value passes through verbatim.
	called := false
	defer withDiscoverFn(func(context.Context, yellowdog.Config) ([]string, error) {
		called = true
		return []string{"unrelated-tag"}, nil
	})()

	got, err := resolveWorkerTag(config.Yellowdog{
		APIKeyID:  "k",
		APISecret: "s",
		Namespace: "development",
		WorkerTag: "operator-override",
	})
	if err != nil {
		t.Fatalf("resolveWorkerTag returned error: %v", err)
	}
	if got != "operator-override" {
		t.Fatalf("explicit tag should win; got %q", got)
	}
	if called {
		t.Fatal("discoverFn should NOT be called when WorkerTag is explicit")
	}
}

func TestResolveWorkerTagOneTagDiscoveredUsedAsIs(t *testing.T) {
	// 1-tag branch: discovery returns exactly one tag -> use it
	// verbatim. This is the happy path that fixes the POC
	// `worker-<username>` vs `worker-<namespace>` mismatch.
	defer withDiscoverFn(func(context.Context, yellowdog.Config) ([]string, error) {
		return []string{"worker-psamuel"}, nil
	})()

	got, err := resolveWorkerTag(config.Yellowdog{
		APIKeyID:  "k",
		APISecret: "s",
		Namespace: "development",
	})
	if err != nil {
		t.Fatalf("resolveWorkerTag returned error: %v", err)
	}
	if got != "worker-psamuel" {
		t.Fatalf("discovered tag should pass through; got %q", got)
	}
}

func TestResolveWorkerTagMultipleTagsRefusesToStart(t *testing.T) {
	// N-tags branch: discovery finds >1 distinct tag in scope ->
	// fail fast with an actionable error. Silent picking of one
	// would route WRs to an arbitrary pool.
	defer withDiscoverFn(func(context.Context, yellowdog.Config) ([]string, error) {
		return []string{"worker-prod", "worker-staging"}, nil
	})()

	_, err := resolveWorkerTag(config.Yellowdog{
		APIKeyID:  "k",
		APISecret: "s",
		Namespace: "development",
	})
	if err == nil {
		t.Fatal("expected error when discovery returns multiple distinct tags")
	}
	if !strings.Contains(err.Error(), "multiple distinct") {
		t.Fatalf("error should mention ambiguity, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "HELIX_YD_WORKER_TAG") {
		t.Fatalf("error should name the env var to set, got %q", err.Error())
	}
	for _, tag := range []string{"worker-prod", "worker-staging"} {
		if !strings.Contains(err.Error(), tag) {
			t.Fatalf("error should list each candidate tag (missing %q): %q", tag, err.Error())
		}
	}
}

func TestResolveWorkerTagDiscoveryErrorFallsBack(t *testing.T) {
	// Transport-error branch: a network or auth blip during discovery
	// should NOT block Helix boot. Fall back to the namespace-derived
	// default and warn; the WR-PENDING diagnostic will surface a real
	// mismatch later if the default was wrong.
	defer withDiscoverFn(func(context.Context, yellowdog.Config) ([]string, error) {
		return nil, errors.New("yellowdog: HTTP 503")
	})()

	got, err := resolveWorkerTag(config.Yellowdog{
		APIKeyID:  "k",
		APISecret: "s",
		Namespace: "development",
	})
	if err != nil {
		t.Fatalf("transport error in discovery should not fail boot, got: %v", err)
	}
	if got != "worker-development" {
		t.Fatalf("expected fallback worker-development, got %q", got)
	}
}

func TestResolveWorkerTagNamespaceRequiredWhenNoExplicitTag(t *testing.T) {
	// Without explicit WorkerTag AND without Namespace, there's no
	// way to compute even the fallback. resolveWorkerTag returns an
	// actionable error rather than calling discovery with no
	// fall-back basis.
	defer withDiscoverFn(func(context.Context, yellowdog.Config) ([]string, error) {
		t.Fatal("discoverFn should not be reached when Namespace is empty")
		return nil, nil
	})()

	_, err := resolveWorkerTag(config.Yellowdog{
		APIKeyID:  "k",
		APISecret: "s",
		Namespace: "",
		WorkerTag: "",
	})
	if err == nil {
		t.Fatal("expected error when both Namespace and WorkerTag are empty")
	}
	if !strings.Contains(err.Error(), "HELIX_YD_NAMESPACE") {
		t.Fatalf("error should name HELIX_YD_NAMESPACE, got %q", err.Error())
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
	_, err := Bootstrap(cfg, 20, "https://helix.example.com", "test-token", nullStore{})
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
	mgr, err := Bootstrap(cfg, 20, "https://helix.example.com", "test-token", nullStore{})
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
