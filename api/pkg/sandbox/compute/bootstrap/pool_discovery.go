package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
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
	return toDiscoveredPools(pools), nil
}

// toDiscoveredPools maps YD node-pool groups to provider-agnostic
// DiscoveredPools. Pure (no I/O) so the keying is unit-testable. Key is the
// raw (workerTag, instanceType) join - the supervisor dedups on it, and
// poolDeploymentTag derives a collision-free deployment tag from it.
func toDiscoveredPools(pools []yellowdog.NodePool) []compute.DiscoveredPool {
	out := make([]compute.DiscoveredPool, 0, len(pools))
	for _, p := range pools {
		out = append(out, compute.DiscoveredPool{
			Key:          p.WorkerTag + "|" + p.InstanceType,
			WorkerTag:    p.WorkerTag,
			InstanceType: p.InstanceType,
			NodeCount:    p.NodeCount,
		})
	}
	return out
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
		DeploymentTag:          poolDeploymentTag(f.deploymentTagBase, p.Key),
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

// poolDeploymentTag derives a per-pool YD deployment tag from the base tag
// and the pool Key. The Key is hashed (fnv-32a, 8 hex chars) so the result
// is collision-free even when two distinct Keys would sanitise to the same
// readable suffix. This matters because the deployment tag IS the
// row-ownership boundary (provider.Name() = "yellowdog-"+DeploymentTag): a
// collision would make two pools' Managers share rows and double-count each
// other's floor. The sanitised Key is kept as a readable prefix for admin
// tooling.
func poolDeploymentTag(base, key string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return fmt.Sprintf("%s-%s-%08x", base, sanitizeTagSegment(key), h.Sum32())
}

// sanitizeTagSegment makes a pool Key readable inside a YD deployment tag:
// keep [a-zA-Z0-9-], replace everything else (the "|" separator and the "."
// in e.g. "inf2.8xlarge") with "-". Lossy by design - poolDeploymentTag adds
// a hash for uniqueness; this is just the human-readable part.
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
// Validates the required YD config eagerly so a misconfigured install fails
// fast at boot rather than silently doing nothing at reconcile time.
// Assumes cfg.DeploymentTag has been resolved by Bootstrap.
func buildSupervisor(cfg config.Compute, maxSandboxesPerHost int, serverURL, runnerToken string, store compute.SandboxStore) (*compute.PoolSupervisor, error) {
	if cfg.Yellowdog.APIKeyID == "" || cfg.Yellowdog.APISecret == "" {
		return nil, errors.New("compute: yellowdog APIKeyID and APISecret are required")
	}
	// Namespace is required by every per-pool yellowdog.NewProvider call, but
	// those happen lazily at reconcile - validate here so an empty namespace
	// fails boot instead of silently skipping every pool.
	if cfg.Yellowdog.Namespace == "" {
		return nil, errors.New("compute: yellowdog Namespace is required")
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
