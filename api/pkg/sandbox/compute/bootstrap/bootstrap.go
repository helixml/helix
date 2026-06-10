// Package bootstrap wires operator-supplied env-var config into a
// concrete compute.Manager + Provider. Separated from package compute
// because compute itself cannot import a concrete Provider (cyclic),
// and from package server because the API server should not need to
// know the catalogue of supported providers.
package bootstrap

import (
	"errors"
	"fmt"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/sandbox/compute"
	"github.com/helixml/helix/api/pkg/sandbox/compute/yellowdog"
	"github.com/rs/zerolog/log"
)

// Bootstrap constructs a Manager from operator-supplied config, OR
// returns (nil, nil) when the compute subsystem is disabled. The
// caller MUST handle a nil return as "disabled" and not start a
// goroutine - all logging and contract is documented here so the
// boot site stays small.
//
// Disabled when cfg.Provider is empty (the default). In that mode
// no Provider is constructed, no goroutine runs, and Helix continues
// to depend entirely on self-registered hosts (the legacy path).
//
// Enabled requires cfg.Provider to be a recognised value AND the
// provider-specific config block to be valid. Construction errors
// are fatal: returning an error from here causes the API server to
// fail fast at boot rather than start with a half-configured
// Manager.
//
// serverURL and runnerToken come from the wider ServerConfig
// (cfg.WebServer.URL and cfg.WebServer.RunnerToken). Helix already
// requires these for its own operation; we reuse them rather than
// introduce parallel env vars. The Provider injects them into the
// upstream task environment so helix-sandbox knows how to phone
// home and how to authenticate.
func Bootstrap(cfg config.Compute, serverURL, runnerToken string, store compute.SandboxStore) (*compute.Manager, error) {
	if cfg.Provider == "" {
		log.Info().Msg("HELIX_COMPUTE_PROVIDER unset; compute subsystem disabled (no Provider, no Manager, no reconcile)")
		return nil, nil
	}
	if store == nil {
		return nil, errors.New("compute: store is required")
	}

	// Auto-derive DeploymentTag from the provider-specific namespace
	// when unset. This is enough to distinguish WRs created by Helix
	// from WRs created by other tools in the same YD account (the
	// primary purpose of the tag). For the niche case of multiple
	// Helix installs sharing the same YD namespace, the operator MUST
	// set HELIX_COMPUTE_DEPLOYMENT_TAG explicitly per install - the
	// derivation cannot detect that scenario.
	//
	// A more robust "persistent install ID hashed into the tag"
	// design is a follow-up; for now the namespace-derived default
	// + boot-time log + manual override covers the realistic
	// deployment shapes.
	if cfg.DeploymentTag == "" {
		derived := deriveDeploymentTag(cfg)
		if derived == "" {
			return nil, errors.New("compute: HELIX_COMPUTE_DEPLOYMENT_TAG is required when Provider is set and no provider namespace is configured to derive it from")
		}
		cfg.DeploymentTag = derived
		log.Info().
			Str("deployment_tag", derived).
			Str("provider", cfg.Provider).
			Msg("compute: HELIX_COMPUTE_DEPLOYMENT_TAG auto-derived from provider namespace; set explicitly if multiple Helix installs share this YD namespace")
	}

	provider, err := buildProvider(cfg, serverURL, runnerToken)
	if err != nil {
		return nil, fmt.Errorf("build %q provider: %w", cfg.Provider, err)
	}

	mgr, err := compute.NewManager(provider, store, compute.ManagerConfig{
		Floor:                   cfg.Floor,
		ReconcileInterval:       cfg.ReconcileInterval,
		HealthCheckTimeout:      cfg.HealthCheckTimeout,
		MaxConcurrentProvisions: cfg.MaxConcurrentProvisions,
		MaxProvisioningAge:      cfg.MaxProvisioningAge,
	})
	if err != nil {
		return nil, fmt.Errorf("construct compute manager: %w", err)
	}

	log.Info().
		Str("provider", provider.Name()).
		Int("floor", cfg.Floor).
		Dur("reconcile_interval", cfg.ReconcileInterval).
		Dur("max_provisioning_age", cfg.MaxProvisioningAge).
		Int("max_concurrent_provisions", cfg.MaxConcurrentProvisions).
		Msg("compute subsystem enabled; Manager will start at boot")
	return mgr, nil
}

// deriveDeploymentTag computes the default DeploymentTag from the
// provider-specific config when no operator override is set. Returns
// empty string if no derivation is possible.
//
// One arm per supported provider, mirroring buildProvider. Each
// provider exposes a namespace concept; we prefix with "helix-" so
// the tag is recognisable in admin UIs and operator tooling that
// inspects the upstream system directly.
func deriveDeploymentTag(cfg config.Compute) string {
	switch cfg.Provider {
	case "yellowdog":
		if cfg.Yellowdog.Namespace != "" {
			return "helix-" + cfg.Yellowdog.Namespace
		}
	}
	return ""
}

// buildProvider dispatches on cfg.Provider to construct the
// appropriate concrete Provider. Add new providers here as
// implementations land.
func buildProvider(cfg config.Compute, serverURL, runnerToken string) (compute.Provider, error) {
	switch cfg.Provider {
	case "yellowdog":
		// Auto-derive WorkerTag from Namespace when unset, matching
		// the yd-provision POC convention (`worker_tag = "worker-<tag>"`).
		// Operator who set up their pool per the POC docs gets a
		// working default; anyone with a custom pool naming scheme
		// overrides via HELIX_YD_WORKER_TAG.
		workerTag := cfg.Yellowdog.WorkerTag
		if workerTag == "" && cfg.Yellowdog.Namespace != "" {
			workerTag = "worker-" + cfg.Yellowdog.Namespace
			log.Info().
				Str("worker_tag", workerTag).
				Msg("compute: HELIX_YD_WORKER_TAG auto-derived from namespace; override if your YD worker pool advertises a different tag")
		}
		sandboxImage, err := helixSandboxImage()
		if err != nil {
			return nil, err
		}
		return yellowdog.NewProvider(yellowdog.Config{
			APIKeyID:      cfg.Yellowdog.APIKeyID,
			APISecret:     cfg.Yellowdog.APISecret,
			BaseURL:       cfg.Yellowdog.BaseURL,
			Namespace:     cfg.Yellowdog.Namespace,
			DeploymentTag: cfg.DeploymentTag,
			WorkerTag:     workerTag,
			TaskTimeout:   cfg.Yellowdog.TaskTimeout,
			MaxRetries:    cfg.Yellowdog.MaxRetries,
			HelixURL:      serverURL,
			RunnerToken:   runnerToken,
			HelixImage:    sandboxImage,
		})
	default:
		return nil, fmt.Errorf("unknown HELIX_COMPUTE_PROVIDER %q (supported: \"yellowdog\")", cfg.Provider)
	}
}

// helixSandboxImage returns the helix-sandbox image tag the YD task
// should docker-run. Auto-derived from the Helix build version so
// the sandbox image always matches the control plane that's
// dispatching it - no version skew, no operator config knob.
//
// data.GetHelixVersion() returns whatever was baked into the
// controlplane image at build time via ldflags - release tags
// ("2.11.14"), release candidates ("2.11.14-rc1"), and per-PR
// short-SHA tags are all valid. Sandbox images are published at
// the same tag for every Helix build, so any non-sentinel value
// has a matching pullable image.
//
// We refuse to proceed when data.GetHelixVersion() returns one of
// its sentinel values ("<unknown>", empty string) or the dummy
// dev-default ("v0.0.0+dev", "v0.0.0", "0.0.0"). Those mean the
// build didn't pin a version and we have no way to know which
// sandbox image is correct. Fail loudly at boot so the operator
// fixes the build rather than discovering the mismatch later when
// YD tasks ERROR on docker pull.
var sentinelVersions = map[string]bool{
	"":          true,
	"<unknown>": true,
	"v0.0.0":    true,
	"0.0.0":     true,
	"v0.0.0+dev": true,
}

func helixSandboxImage() (string, error) {
	return helixSandboxImageFor(data.GetHelixVersion())
}

// helixSandboxImageFor is the testable inner. Tests inject specific
// version strings; the production wrapper above reads from data.
func helixSandboxImageFor(version string) (string, error) {
	if sentinelVersions[version] {
		return "", fmt.Errorf(
			"compute: cannot derive helix-sandbox image tag - Helix build version %q is a placeholder, not a real version. "+
				"YD compute requires a versioned Helix build so the sandbox image (ghcr.io/helixml/helix-sandbox:<version>) "+
				"matches the control plane. Build with -ldflags=\"-X github.com/helixml/helix/api/pkg/data.Version=X.Y.Z\" "+
				"or run a tagged release.",
			version,
		)
	}
	return "ghcr.io/helixml/helix-sandbox:" + version, nil
}
