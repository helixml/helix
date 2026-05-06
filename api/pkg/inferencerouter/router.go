// Package inferencerouter is the API-server-side router that replaces the old
// scheduler's request-routing role.
//
// The router answers a single question: "given an OpenAI-compatible request
// for model M, which connected runner should it go to?" It does not bin-pack
// GPUs, evict slots, or estimate memory — operators express layout in
// compose profiles; the router obeys.
//
// Lives in its own package (not a file in api/pkg/runner/) because the
// existing runner package contains code destined for deletion (Ollama
// memory estimation et al, see AC8) that breaks compilation in
// CGO_ENABLED=0 environments. Decoupling lets the router and its tests
// build and run independently of the runner-package deletion timeline,
// and routing is logically distinct from runner-binary code anyway.
package inferencerouter

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/helixml/helix/api/pkg/types"
)

// RunnerState is what the router knows about one connected runner
// (a Helix sandbox post-absorption). Populated from sandbox HTTP heartbeats.
type RunnerState struct {
	ID            string
	URL           string               // e.g. "http://10.0.0.5:8081" — where to forward inference
	Status        string               // "running" | "starting" | "pulling" | "assigning" | "failed" | ""
	ActiveProfile *types.RunnerProfile // nil if no profile assigned
	GPUs          []types.GPUStatus    // per-GPU inventory (vendor, arch, total VRAM, ...)
	LastSeen      time.Time
}

// RouteableModels returns the set of model names this runner can serve right
// now. Empty if no profile is assigned or the runner isn't ready.
func (r *RunnerState) RouteableModels() []string {
	if r == nil || r.ActiveProfile == nil || r.Status != "running" {
		return nil
	}
	out := make([]string, 0, len(r.ActiveProfile.Models))
	for _, m := range r.ActiveProfile.Models {
		out = append(out, m.Name)
	}
	return out
}

// ErrNoRunner is the package-internal sentinel returned when no Helix-hosted
// model serves the request. Symbol name is preserved for backwards
// compatibility; the user-facing string deliberately avoids "runner"
// terminology so it doesn't leak the Helix-sandbox internals to end users
// who see this error surfaced via OpenAI-compatible 503 responses.
var ErrNoRunner = errors.New("requested model is not available")

// NoRunnerError is the rich version of ErrNoRunner — includes the requested
// model and a list of currently-available models so callers can return a
// useful error to the client.
type NoRunnerError struct {
	RequestedModel  string
	AvailableModels []string
}

func (e *NoRunnerError) Error() string {
	if len(e.AvailableModels) == 0 {
		return fmt.Sprintf("model %q is not available (no models are currently configured)", e.RequestedModel)
	}
	return fmt.Sprintf("model %q is not available (currently configured: %s)",
		e.RequestedModel, strings.Join(e.AvailableModels, ", "))
}

func (e *NoRunnerError) Is(target error) bool { return target == ErrNoRunner }

// Router routes inference requests to runners by model name. Stateful: it
// maintains an in-memory map of connected runners updated from NATS
// heartbeats, plus a round-robin counter per model.
//
// Replaces api/pkg/scheduler/scheduler.go's request-routing role. Bin-pack /
// eviction / memory-estimation logic are gone — operators express layout in
// compose profiles, and the router just picks among runners that already
// have the requested model loaded.
type Router struct {
	mu      sync.RWMutex
	runners map[string]*RunnerState

	// rrCounters[model] = monotonic counter used for round-robin selection.
	// One counter per model so two clients hitting different models don't
	// trample each other's rotation.
	rrCounters sync.Map // map[string]*uint64
}

// NewRouter returns an empty router. Wire it up to NATS heartbeat
// subscribers so SetRunnerState is called whenever a runner reports in.
func NewRouter() *Router {
	return &Router{runners: map[string]*RunnerState{}}
}

// SetRunnerState upserts a runner's state. Called from the NATS heartbeat
// handler whenever a runner reports its status.
func (rt *Router) SetRunnerState(s *RunnerState) {
	if s == nil || s.ID == "" {
		return
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.runners[s.ID] = s
}

// RemoveRunner drops a runner from the routing table — call when a runner
// disconnects (NATS heartbeat timeout) or is explicitly de-registered.
func (rt *Router) RemoveRunner(id string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	delete(rt.runners, id)
}

// GetRunner returns a snapshot of one runner's state, or nil if absent.
func (rt *Router) GetRunner(id string) *RunnerState {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	if s, ok := rt.runners[id]; ok {
		cp := *s
		return &cp
	}
	return nil
}

// ListRunners returns a snapshot of all known runners, sorted by ID for
// deterministic ordering.
func (rt *Router) ListRunners() []*RunnerState {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	out := make([]*RunnerState, 0, len(rt.runners))
	for _, s := range rt.runners {
		cp := *s
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// AvailableModels returns the union of models exposed by all currently
// running runners' active profiles. Used by GET /v1/models and to populate
// the available-models list in NoRunnerError.
func (rt *Router) AvailableModels() []string {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	seen := map[string]struct{}{}
	for _, s := range rt.runners {
		for _, m := range s.RouteableModels() {
			seen[m] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for m := range seen {
		out = append(out, m)
	}
	sort.Strings(out)
	return out
}

// PickRunner returns a connected runner whose active profile includes the
// given model and is in `running` state. Round-robin across qualifying
// runners. Returns *NoRunnerError if no runner qualifies.
func (rt *Router) PickRunner(model string) (*RunnerState, error) {
	rt.mu.RLock()
	candidates := make([]*RunnerState, 0)
	for _, s := range rt.runners {
		for _, m := range s.RouteableModels() {
			if m == model {
				candidates = append(candidates, s)
				break
			}
		}
	}
	rt.mu.RUnlock()

	if len(candidates) == 0 {
		return nil, &NoRunnerError{
			RequestedModel:  model,
			AvailableModels: rt.AvailableModels(),
		}
	}
	// Sort for stable rotation across calls.
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].ID < candidates[j].ID })

	// Per-model round-robin counter. sync.Map.LoadOrStore is the right
	// primitive here — first request for a new model creates the counter.
	ctrAny, _ := rt.rrCounters.LoadOrStore(model, new(uint64))
	ctr := ctrAny.(*uint64)
	idx := atomic.AddUint64(ctr, 1) - 1
	picked := candidates[int(idx%uint64(len(candidates)))]
	cp := *picked
	return &cp, nil
}
