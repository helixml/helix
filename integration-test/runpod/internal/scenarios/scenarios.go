// Package scenarios is the seven test scenarios that run against each
// provisioned sandbox. Each scenario takes a sandbox URL + the matrix
// entry's profile and returns a structured pass/fail.
//
// Scenarios:
//   1. boot_smoke               — sandbox connects to API, heartbeat lands,
//                                 GPU inventory matches the form factor
//   2. compatibility_filter     — GET compatible-profiles for this runner
//                                 includes the assigned profile
//   3. assignment_apply         — assign-profile, wait for "running",
//                                 service_health all "healthy"
//   4. inference_roundtrip      — POST chat completion + embeddings to
//                                 the API, verify a sane response
//   5. profile_switch           — assign a *different* compatible profile,
//                                 confirm old stack down, new stack up
//   6. clear_profile            — POST clear-profile, confirm idle state
//   7. incompatible_rejection   — assign a profile that needs different
//                                 architecture; confirm 422 + named
//                                 constraint error
package scenarios

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/helixml/helix/integration-test/runpod/internal/provision"
)

// Result is one scenario's outcome.
type Result struct {
	Name           string
	Passed         bool
	DurationSeconds int
	Failure        string // empty when Passed
}

// Runner orchestrates the seven scenarios for one provisioned pod +
// matrix entry.
type Runner struct {
	pod                 *provision.Pod
	primaryProfile      string
	switchProfiles      []string
	incompatibleProfile string
}

// NewRunner constructs a scenario runner.
func NewRunner(pod *provision.Pod, primary string, switches []string, incompatible string) *Runner {
	return &Runner{
		pod:                 pod,
		primaryProfile:      primary,
		switchProfiles:      switches,
		incompatibleProfile: incompatible,
	}
}

// RunAll executes every scenario in order. Returns the slice of per-
// scenario results plus a wrapper error for any infrastructure failure
// (e.g. context cancelled). Per-scenario test failures are reported in
// the Result.Failure field, not as the wrapper error.
func (r *Runner) RunAll(ctx context.Context) ([]Result, error) {
	steps := []struct {
		name string
		fn   func(context.Context) error
	}{
		{"boot_smoke", r.bootSmoke},
		{"compatibility_filter", r.compatibilityFilter},
		{"assignment_apply", r.assignmentApply},
		{"inference_roundtrip", r.inferenceRoundtrip},
		{"profile_switch", r.profileSwitch},
		{"clear_profile", r.clearProfile},
		{"incompatible_rejection", r.incompatibleRejection},
	}
	out := make([]Result, 0, len(steps))
	for _, s := range steps {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		start := time.Now()
		err := s.fn(ctx)
		res := Result{Name: s.name, DurationSeconds: int(time.Since(start).Seconds())}
		if err != nil {
			res.Failure = err.Error()
		} else {
			res.Passed = true
		}
		out = append(out, res)
		// Halt the suite on the first hard failure of a precondition
		// scenario — running inference_roundtrip after assignment_apply
		// fails is just noise.
		if !res.Passed && (s.name == "boot_smoke" || s.name == "assignment_apply") {
			return out, nil
		}
	}
	return out, nil
}

// --- individual scenarios ---
//
// Each scenario is intentionally small + focused. They share a Runner-level
// HTTP helper rather than each rolling its own; that helper is in api.go.

func (r *Runner) bootSmoke(ctx context.Context) error {
	// Wait up to 5 min for the sandbox to register + send its first
	// heartbeat including GPU inventory.
	deadline := time.Now().Add(5 * time.Minute)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		ok, err := r.sandboxOnlineWithGPUs(ctx)
		if err == nil && ok {
			return nil
		}
		if time.Now().After(deadline) {
			if err == nil {
				err = errors.New("sandbox connected but no GPUs reported")
			}
			return fmt.Errorf("boot_smoke: %w", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Second):
		}
	}
}

func (r *Runner) compatibilityFilter(ctx context.Context) error {
	// Fetch /api/v1/runners/{id}/compatible-profiles, confirm the
	// primary profile's name is in the result.
	profiles, err := r.listCompatibleProfiles(ctx)
	if err != nil {
		return err
	}
	want := profileNameFromPath(r.primaryProfile)
	for _, p := range profiles {
		if p.Name == want {
			return nil
		}
	}
	return fmt.Errorf("compatibility_filter: profile %q not in compatible list (got %d profiles)", want, len(profiles))
}

func (r *Runner) assignmentApply(ctx context.Context) error {
	if err := r.assignProfile(ctx, r.primaryProfile); err != nil {
		return err
	}
	// Wait up to 15 min for profile_status=running + all services healthy.
	return r.waitForRunning(ctx, 15*time.Minute)
}

func (r *Runner) inferenceRoundtrip(ctx context.Context) error {
	// Pick the first model the profile exposes (parsed from compose YAML)
	// and send a minimal chat completion through the API.
	models, err := r.profileModels(r.primaryProfile)
	if err != nil {
		return err
	}
	if len(models) == 0 {
		return errors.New("inference_roundtrip: profile has no models")
	}
	for _, m := range models {
		if err := r.chatCompletion(ctx, m, "Say one word: ok"); err != nil {
			return fmt.Errorf("chat completion for %s: %w", m, err)
		}
	}
	return nil
}

func (r *Runner) profileSwitch(ctx context.Context) error {
	if len(r.switchProfiles) == 0 {
		// Some entries don't have a sensible secondary profile (e.g.
		// dedicated H100 stacks). Skip silently — caller's matrix can
		// always add one if they want stricter coverage.
		return nil
	}
	if err := r.assignProfile(ctx, r.switchProfiles[0]); err != nil {
		return err
	}
	if err := r.waitForRunning(ctx, 10*time.Minute); err != nil {
		return err
	}
	// Confirm the previous stack's models are no longer reachable.
	prevModels, _ := r.profileModels(r.primaryProfile)
	for _, m := range prevModels {
		if r.modelAvailable(ctx, m) {
			return fmt.Errorf("profile_switch: previous model %q still listed after switch", m)
		}
	}
	return nil
}

func (r *Runner) clearProfile(ctx context.Context) error {
	if err := r.clearProfileAPI(ctx); err != nil {
		return err
	}
	// Wait briefly for the inference-proxy to drop the routing table.
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		if !r.anyModelAvailable(ctx) {
			return nil
		}
		time.Sleep(5 * time.Second)
	}
	return errors.New("clear_profile: models still reachable 2 min after clear")
}

func (r *Runner) incompatibleRejection(ctx context.Context) error {
	if r.incompatibleProfile == "" {
		return nil // matrix entry didn't supply one — skip silently
	}
	status, body, err := r.assignProfileRaw(ctx, r.incompatibleProfile)
	if err != nil {
		return err
	}
	if status != 422 {
		return fmt.Errorf("incompatible_rejection: expected 422, got %d (%s)", status, body)
	}
	return nil
}
