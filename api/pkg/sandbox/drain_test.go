package sandbox

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/hydra"
)

type drainRecorder struct {
	reqs []*hydra.ExecRequest
	err  error
	// block simulates a wedged container: the exec hangs until ctx expires.
	block bool
}

func (d *drainRecorder) RunSandboxCommand(ctx context.Context, _ string, req *hydra.ExecRequest) (*hydra.SandboxCommandResponse, error) {
	d.reqs = append(d.reqs, req)
	if d.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if d.err != nil {
		return nil, d.err
	}
	return &hydra.SandboxCommandResponse{Stdout: "3\n"}, nil
}

// TestDrainStopsNestedContainersGracefully: the drain must issue a bounded
// `docker stop` INSIDE the sandbox. Stopping the sandbox itself does not stop
// containers on its inner dockerd — that is what corrupted we-find.ai's Postgres.
func TestDrainStopsNestedContainersGracefully(t *testing.T) {
	rec := &drainRecorder{}
	if err := DrainNestedContainers(context.Background(), rec, "sbx_1", 60*time.Second); err != nil {
		t.Fatalf("drain returned error: %v", err)
	}
	if len(rec.reqs) != 1 {
		t.Fatalf("expected exactly 1 exec, got %d", len(rec.reqs))
	}
	script := rec.reqs[0].Args[1]

	if !strings.Contains(script, "docker ps -q") {
		t.Errorf("drain must enumerate running containers: %s", script)
	}
	// `docker stop` (not kill) is what sends each image's STOPSIGNAL — SIGINT
	// for the official Postgres image, which is fast shutdown with a clean
	// checkpoint. `docker kill` here would reintroduce the original bug.
	if !strings.Contains(script, "docker stop --time 60") {
		t.Errorf("drain must use a bounded `docker stop`, got: %s", script)
	}
	if strings.Contains(script, "docker kill") {
		t.Errorf("drain must never SIGKILL nested containers: %s", script)
	}
	// Removing containers/volumes would destroy data the customer expects to
	// persist; we only stop.
	for _, forbidden := range []string{"docker rm", "compose down", "volume rm", "system prune"} {
		if strings.Contains(script, forbidden) {
			t.Errorf("drain must not destroy state (%q): %s", forbidden, script)
		}
	}
	// The exec must outlive the grace period or `docker stop` gets cut off
	// mid-drain, which is the very thing we are preventing.
	if rec.reqs[0].TimeoutSeconds <= 60 {
		t.Errorf("exec timeout (%ds) must exceed the 60s grace period", rec.reqs[0].TimeoutSeconds)
	}
}

// TestDrainDefaultsGrace: a zero/negative grace must fall back to the default
// rather than producing `docker stop --time 0`, which is an instant SIGKILL.
func TestDrainDefaultsGrace(t *testing.T) {
	for _, grace := range []time.Duration{0, -5 * time.Second} {
		rec := &drainRecorder{}
		if err := DrainNestedContainers(context.Background(), rec, "sbx_1", grace); err != nil {
			t.Fatalf("drain returned error: %v", err)
		}
		script := rec.reqs[0].Args[1]
		if strings.Contains(script, "--time 0") || strings.Contains(script, "--time -") {
			t.Errorf("grace %v produced an immediate kill: %s", grace, script)
		}
		if !strings.Contains(script, "docker stop --time 60") {
			t.Errorf("grace %v did not fall back to the default: %s", grace, script)
		}
	}
}

// TestDrainSurvivesDeadDockerd: when the inner dockerd is gone there is nothing
// to drain through. The error is reported, never fatal — teardown must proceed.
func TestDrainSurvivesDeadDockerd(t *testing.T) {
	rec := &drainRecorder{err: errors.New("connection refused")}
	err := DrainNestedContainers(context.Background(), rec, "sbx_1", time.Second)
	if err == nil {
		t.Fatal("expected the failure to be reported to the caller")
	}
	if !strings.Contains(err.Error(), "sbx_1") {
		t.Errorf("error should name the sandbox, got: %v", err)
	}
}

// TestDrainIsBounded: a wedged container must not block teardown forever.
func TestDrainIsBounded(t *testing.T) {
	rec := &drainRecorder{block: true}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = DrainNestedContainers(context.Background(), rec, "sbx_1", time.Second)
	}()
	select {
	case <-done:
	case <-time.After(60 * time.Second):
		t.Fatal("drain did not return within its bound — a wedged container would block teardown forever")
	}
}
