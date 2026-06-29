package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/sandbox/compute"
	"github.com/helixml/helix/api/pkg/sandbox/compute/yellowdog"
	"github.com/rs/zerolog/log"
)

// ydPoolDiscoverer adapts yellowdog.DiscoverNodePools to the
// compute.PoolDiscoverer interface the supervisor consumes.
type ydPoolDiscoverer struct {
	yd config.Yellowdog
}

func (d *ydPoolDiscoverer) DiscoverPools(ctx context.Context) ([]compute.DiscoveredPool, error) {
	pools, err := yellowdog.DiscoverNodePools(ctx, yellowdog.Config{
		APIKeyID:  d.yd.APIKeyID,
		APISecret: d.yd.APISecret,
		BaseURL:   d.yd.BaseURL,
	})
	if err != nil {
		return nil, err
	}
	out := make([]compute.DiscoveredPool, 0, len(pools))
	for _, p := range pools {
		out = append(out, compute.DiscoveredPool{
			// Key is unique per (workerTag, instanceType) so the reconcile
			// diff never collides two pools that share a tag.
			Key:          p.WorkerTag + "|" + p.InstanceType,
			WorkerTag:    p.WorkerTag,
			InstanceType: p.InstanceType,
			NodeCount:    p.NodeCount,
		})
	}
	return out, nil
}

// ydManagerFactory builds one compute.Manager per discovered pool: a YD
// provider scoped to the pool's worker tag with an isolated deployment tag
// (so each Manager's Floor/D3/D4 sees only its own SandboxInstance rows),
// the accelerator-derived GPU vendor, and the one global ManagerConfig.
type ydManagerFactory struct {
	cfg               config.Compute
	deploymentTagBase string
	serverURL         string
	runnerToken       string
	sandboxImage      string
	maxSandboxes      int
	store             compute.SandboxStore
}

func (f *ydManagerFactory) NewPoolManager(p compute.DiscoveredPool) (compute.PoolManager, error) {
	vendor := compute.AcceleratorForInstanceType(p.InstanceType)
	if vendor == "" {
		return nil, fmt.Errorf("unclassifiable instance type %q (no accelerator mapping)", p.InstanceType)
	}
	provider, err := yellowdog.NewProvider(yellowdog.Config{
		APIKeyID:  f.cfg.Yellowdog.APIKeyID,
		APISecret: f.cfg.Yellowdog.APISecret,
		BaseURL:   f.cfg.Yellowdog.BaseURL,
		Namespace: f.cfg.Yellowdog.Namespace,
		// Per-pool deployment tag isolates row ownership: Manager.ownedRows
		// filters by provider.Name() = "yellowdog-"+DeploymentTag.
		DeploymentTag:          f.deploymentTagBase + "-" + sanitizeTagSegment(p.Key),
		WorkerTag:              p.WorkerTag,
		TaskTimeout:            f.cfg.Yellowdog.TaskTimeout,
		MaxRetries:             f.cfg.Yellowdog.MaxRetries,
		HelixURL:               f.serverURL,
		RunnerToken:            f.runnerToken,
		HelixImage:             f.sandboxImage,
		NeuronCompileCacheURL:  f.cfg.NeuronCompileCacheURL,
		RunnerReadinessTimeout: f.cfg.RunnerReadinessTimeout,
	})
	if err != nil {
		return nil, err
	}
	return compute.NewManager(provider, f.store, managerConfig(f.cfg, compute.Spec{
		MaxSandboxes: f.maxSandboxes,
		GPUVendor:    vendor,
	}))
}

// sanitizeTagSegment makes a pool Key safe to embed in a YD deployment tag:
// keep [a-zA-Z0-9-], replace everything else (the "|" separator and the "."
// in e.g. "inf2.8xlarge") with "-".
func sanitizeTagSegment(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-':
			return r
		default:
			return '-'
		}
	}, s)
}

// buildSupervisor constructs the discovery-mode PoolSupervisor: a YD pool
// discoverer + a per-pool Manager factory sharing the global ManagerConfig.
// Returned to the API server as a compute.Service (same Run contract as a
// single Manager). Assumes cfg.DeploymentTag has been resolved by Bootstrap.
func buildSupervisor(cfg config.Compute, maxSandboxesPerHost int, serverURL, runnerToken string, store compute.SandboxStore) (compute.Service, error) {
	// Fail fast at boot on missing credentials rather than only when the
	// supervisor's first reconcile tries to discover/provision.
	if cfg.Yellowdog.APIKeyID == "" || cfg.Yellowdog.APISecret == "" {
		return nil, errors.New("compute: yellowdog APIKeyID and APISecret are required")
	}
	sandboxImage := cfg.SandboxImage
	if sandboxImage == "" {
		img, err := helixSandboxImage(cfg.SandboxRegistry)
		if err != nil {
			return nil, err
		}
		sandboxImage = img
	}
	sup, err := compute.NewPoolSupervisor(
		&ydPoolDiscoverer{yd: cfg.Yellowdog},
		&ydManagerFactory{
			cfg:               cfg,
			deploymentTagBase: cfg.DeploymentTag,
			serverURL:         serverURL,
			runnerToken:       runnerToken,
			sandboxImage:      sandboxImage,
			maxSandboxes:      maxSandboxesPerHost,
			store:             store,
		},
		cfg.ReconcileInterval,
	)
	if err != nil {
		return nil, err
	}
	log.Info().
		Str("deployment_tag_base", cfg.DeploymentTag).
		Str("sandbox_image", sandboxImage).
		Int("floor", cfg.Floor).
		Int("max", cfg.Max).
		Dur("reconcile_interval", cfg.ReconcileInterval).
		Msg("compute subsystem enabled in DISCOVERY mode; PoolSupervisor (one Manager per discovered pool) will start at boot")
	return sup, nil
}
