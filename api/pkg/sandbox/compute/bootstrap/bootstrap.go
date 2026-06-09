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
func Bootstrap(cfg config.Compute, store compute.SandboxStore) (*compute.Manager, error) {
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
	// design is tracked for D2b; for now the namespace-derived
	// default + boot-time log + manual override covers the realistic
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

	provider, err := buildProvider(cfg)
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
func buildProvider(cfg config.Compute) (compute.Provider, error) {
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
		return yellowdog.NewProvider(yellowdog.Config{
			APIKeyID:      cfg.Yellowdog.APIKeyID,
			APISecret:     cfg.Yellowdog.APISecret,
			BaseURL:       cfg.Yellowdog.BaseURL,
			Namespace:     cfg.Yellowdog.Namespace,
			DeploymentTag: cfg.DeploymentTag,
			WorkerTag:     workerTag,
			TaskTimeout:   cfg.Yellowdog.TaskTimeout,
			MaxRetries:    cfg.Yellowdog.MaxRetries,
		})
	default:
		return nil, fmt.Errorf("unknown HELIX_COMPUTE_PROVIDER %q (supported: \"yellowdog\")", cfg.Provider)
	}
}
