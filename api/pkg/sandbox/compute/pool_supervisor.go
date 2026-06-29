package compute

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// DiscoveredPool is one provisioned worker pool as observed from live
// nodes, in provider-agnostic terms.
//
// Key is the supervisor's identity for the pool - one Manager per distinct
// Key. It MUST be unique per (WorkerTag, InstanceType): a worker tag that
// spans accelerator types is two pools, and keying on the tag alone would
// collide them in the reconcile diff and silently drop one. WorkerTag is
// what a work requirement targets; InstanceType classifies the accelerator.
// NOTE: because YD work-requirement placement is by worker tag ONLY, a
// single tag that genuinely spans accelerators cannot be split safely (a
// neuron task could land on an nvidia node) - the factory should reject
// such a tag rather than build two Managers for it. See the design doc.
type DiscoveredPool struct {
	Key          string
	WorkerTag    string
	InstanceType string
	NodeCount    int
}

// PoolDiscoverer enumerates the worker pools currently visible from live
// nodes. Implemented in the provider layer (e.g. the YellowDog
// DiscoverNodePools call) and injected so this package stays
// provider-agnostic and import-cycle free.
type PoolDiscoverer interface {
	DiscoverPools(ctx context.Context) ([]DiscoveredPool, error)
}

// poolManager is the slice of *Manager the supervisor drives. An
// interface (not the concrete *Manager) so the supervisor is testable
// without constructing a real Provider + store.
type poolManager interface {
	Run(ctx context.Context) error
}

// ManagerFactory builds a Manager scoped to a single discovered pool.
// The factory owns provider construction (per-pool worker tag, an
// isolated deployment tag so each Manager's Floor/D3/D4 only sees its
// own SandboxInstance rows, and the accelerator-specific sandbox image)
// and applies the ONE global ManagerConfig. The supervisor never sees
// per-pool policy because there is none - Floor/Max/Idle are global.
type ManagerFactory interface {
	NewPoolManager(p DiscoveredPool) (poolManager, error)
}

// PoolSupervisor keeps one running Manager per discovered pool. Each
// reconcile cycle it diffs the live pools (from PoolDiscoverer) against
// the running Managers: start a Manager when a pool appears, stop it
// when the pool's nodes are gone. Each Manager runs the existing
// Floor/D3/D4 loop unchanged, scoped to its pool.
//
// This is the discovery-driven alternative to declaring pools in config:
// whatever the operator brought up with `yd-provision` shows up here and
// gets a Manager; whatever they tear down loses its Manager next cycle.
type PoolSupervisor struct {
	discoverer PoolDiscoverer
	factory    ManagerFactory
	interval   time.Duration

	mu sync.Mutex
	// running maps a pool key to its in-flight Manager. The *poolRun
	// pointer identity lets a Manager goroutine remove ONLY its own
	// entry on exit without racing a newer start for the same key.
	running map[string]*poolRun
}

// poolRun is one started Manager's cancel handle, tracked by pointer
// identity in PoolSupervisor.running.
type poolRun struct {
	cancel context.CancelFunc
}

// NewPoolSupervisor validates inputs and returns a supervisor that has
// not started reconciling. interval is how often the pool set is
// re-discovered.
func NewPoolSupervisor(d PoolDiscoverer, f ManagerFactory, interval time.Duration) (*PoolSupervisor, error) {
	if d == nil {
		return nil, errors.New("compute.NewPoolSupervisor: discoverer is required")
	}
	if f == nil {
		return nil, errors.New("compute.NewPoolSupervisor: factory is required")
	}
	if interval <= 0 {
		return nil, fmt.Errorf("compute.NewPoolSupervisor: interval must be > 0, got %s", interval)
	}
	return &PoolSupervisor{
		discoverer: d,
		factory:    f,
		interval:   interval,
		running:    map[string]*poolRun{},
	}, nil
}

// Run blocks until ctx is cancelled, reconciling the pool set each
// interval (first pass immediately so pre-warming starts at boot). On
// exit every per-pool Manager is signalled to stop (their goroutines
// then drain independently - Run does not wait for them).
func (s *PoolSupervisor) Run(ctx context.Context) error {
	t := time.NewTicker(s.interval)
	defer t.Stop()

	s.reconcile(ctx)
	for {
		select {
		case <-ctx.Done():
			s.stopAll()
			return ctx.Err()
		case <-t.C:
			s.reconcile(ctx)
		}
	}
}

// reconcile diffs discovered pools against running Managers. A discovery
// error is logged and the current Manager set is left intact - a
// transient YD blip must not tear down healthy pre-warming.
func (s *PoolSupervisor) reconcile(ctx context.Context) {
	pools, err := s.discoverer.DiscoverPools(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("compute pool supervisor: discovery failed; keeping current managers")
		return
	}

	desired := make(map[string]DiscoveredPool, len(pools))
	for _, p := range pools {
		if p.Key == "" {
			continue
		}
		desired[p.Key] = p
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Stop Managers whose pool is no longer present (operator tore the
	// pool down). Cancelling the Manager's context stops its loop; the
	// pool's sandbox WRs die with the nodes that are already gone.
	for key, run := range s.running {
		if _, ok := desired[key]; !ok {
			log.Info().Str("pool", key).Msg("compute pool supervisor: pool gone, stopping its manager")
			run.cancel()
			delete(s.running, key)
		}
	}

	// Start a Manager for each newly-seen pool.
	for key, p := range desired {
		if _, ok := s.running[key]; ok {
			continue
		}
		mgr, err := s.factory.NewPoolManager(p)
		if err != nil {
			// Most likely an unclassifiable instance type (no image
			// mapped). Skip this pool; retry next cycle in case the
			// mapping or the pool changes.
			log.Error().Err(err).Str("pool", key).Str("instance_type", p.InstanceType).
				Msg("compute pool supervisor: cannot build manager for pool; skipping")
			continue
		}
		mctx, cancel := context.WithCancel(ctx)
		run := &poolRun{cancel: cancel}
		s.running[key] = run
		log.Info().Str("pool", key).Str("worker_tag", p.WorkerTag).
			Str("instance_type", p.InstanceType).Int("nodes", p.NodeCount).
			Msg("compute pool supervisor: starting manager for pool")
		go func(p DiscoveredPool, run *poolRun, mctx context.Context) {
			err := mgr.Run(mctx)
			if err != nil && !errors.Is(err, context.Canceled) {
				log.Warn().Err(err).Str("pool", p.Key).Msg("compute pool supervisor: manager exited with error")
			}
			// Self-remove so a Manager that exited on its own (crash, or
			// a non-cancel error) is rebuilt next cycle instead of being
			// stuck "running" forever. Pointer check: only delete if this
			// run is still the current one (a pool that was torn down and
			// rebuilt for the same key has a different *poolRun).
			s.mu.Lock()
			if s.running[p.Key] == run {
				delete(s.running, p.Key)
			}
			s.mu.Unlock()
		}(p, run, mctx)
	}
}

func (s *PoolSupervisor) stopAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, run := range s.running {
		run.cancel()
		delete(s.running, key)
	}
}

// AcceleratorForInstanceType classifies an AWS instance type into the
// accelerator family Helix must target, which selects the sandbox image
// and GPU_VENDOR for that pool's Manager. Returns "" for types it can't
// classify (the factory then skips the pool rather than guess).
//
// ponytail: prefix heuristic, not an exhaustive table. inf*/trn* = AWS
// Neuron, g*/p* = NVIDIA GPU - which covers every pool this POC runs.
// If an AMD GPU family (g4ad) or a new accelerator needs a distinct
// path, add an explicit case before the g*/p* fallthrough.
func AcceleratorForInstanceType(instanceType string) string {
	fam, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(instanceType)), ".")
	switch {
	case strings.HasPrefix(fam, "inf"), strings.HasPrefix(fam, "trn"):
		return "neuron"
	case strings.HasPrefix(fam, "g"), strings.HasPrefix(fam, "p"):
		return "nvidia"
	default:
		return ""
	}
}
