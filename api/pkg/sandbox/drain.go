package sandbox

import (
	"context"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/hydra"
	"github.com/rs/zerolog/log"
)

// DefaultDrainGrace bounds how long we wait for a sandbox's nested containers to
// shut down cleanly before the platform proceeds with teardown anyway. Postgres
// fast shutdown is normally sub-second; 60s covers a large busy database while
// still bounding how long a teardown can block.
const DefaultDrainGrace = 60 * time.Second

// nestedDrainClient is the slice of the hydra client the drain needs. Extracted
// so tests can assert on the exec without standing up a RevDial connection.
type nestedDrainClient interface {
	RunSandboxCommand(ctx context.Context, sessionID string, req *hydra.ExecRequest) (*hydra.SandboxCommandResponse, error)
}

// DrainNestedContainers gracefully stops every container running inside a
// sandbox's own dockerd, before the platform kills the sandbox itself.
//
// This exists because stopping a sandbox does NOT stop the containers running
// inside it. A web-service sandbox hosts the customer's docker-compose stack on
// its inner dockerd; SIGTERM to the sandbox goes to the sandbox's PID 1, which
// knows nothing about them. When the sandbox dies, its dockerd dies, and every
// nested container is SIGKILLed mid-write. That is how we-find.ai's Postgres got
// a corrupt WAL checkpoint on 2026-07-23 and never started again:
//
//	PANIC: could not locate a valid checkpoint record
//
// Raising the sandbox's stop timeout does not help — it just delays the SIGKILL.
// The stop has to be issued from INSIDE the sandbox, which is what this does.
//
// `docker stop` (not `compose down`) is deliberate:
//   - it is generic; the startup mechanism is not assumed to be docker-compose,
//     and `docker ps` works regardless of how the app was launched;
//   - it sends each image's declared STOPSIGNAL — the official Postgres image
//     declares SIGINT, which is Postgres *fast shutdown*, exactly the clean
//     checkpoint we need;
//   - it leaves containers, networks and volumes in place, so nothing the
//     customer expects to persist is destroyed and the next start is fast.
//
// Best-effort and bounded: a wedged container must never block teardown. Returns
// an error only so callers can log it; no caller should abort a teardown on it.
func DrainNestedContainers(ctx context.Context, hc nestedDrainClient, sandboxID string, grace time.Duration) error {
	if grace <= 0 {
		grace = DefaultDrainGrace
	}
	graceSecs := int(grace.Seconds())

	// Give the exec a margin over the grace period so `docker stop` gets to run
	// its full timeout and report, rather than being cut off mid-drain.
	execTimeout := graceSecs + 15
	cctx, cancel := context.WithTimeout(ctx, time.Duration(execTimeout+10)*time.Second)
	defer cancel()

	start := time.Now()
	// `docker stop` on an explicit id list; no-op when nothing is running. The
	// containers are left stopped, so the restart=unless-stopped policy the
	// deploy applies will not race us by bringing them back mid-drain.
	script := fmt.Sprintf(
		`ids=$(docker ps -q 2>/dev/null); if [ -n "$ids" ]; then docker stop --time %d $ids >/dev/null 2>&1 || true; echo "$ids" | wc -l; else echo 0; fi`,
		graceSecs)

	resp, err := hc.RunSandboxCommand(cctx, sandboxID, &hydra.ExecRequest{
		SandboxID:      sandboxID,
		Cmd:            "/bin/sh",
		Args:           []string{"-c", script},
		Cwd:            "/",
		TimeoutSeconds: execTimeout,
	})
	if err != nil {
		// Expected when the inner dockerd is already dead or the sandbox is
		// unreachable — there is nothing to drain through, and teardown must
		// continue regardless.
		log.Warn().Err(err).Str("sandbox_id", sandboxID).Dur("elapsed", time.Since(start)).
			Msg("web-service drain: could not stop nested containers gracefully; proceeding with teardown (nested data may not have flushed)")
		return fmt.Errorf("drain nested containers in %s: %w", sandboxID, err)
	}

	log.Info().
		Str("sandbox_id", sandboxID).
		Dur("elapsed", time.Since(start)).
		Int("grace_seconds", graceSecs).
		Str("containers", trimOutput(resp)).
		Msg("web-service drain: nested containers stopped gracefully before teardown")
	return nil
}

func trimOutput(resp *hydra.SandboxCommandResponse) string {
	if resp == nil {
		return ""
	}
	out := resp.Stdout
	if len(out) > 64 {
		out = out[:64]
	}
	for len(out) > 0 && (out[len(out)-1] == '\n' || out[len(out)-1] == ' ') {
		out = out[:len(out)-1]
	}
	return out
}

// DrainSandbox drains a sandbox's nested containers using the controller's hydra
// client. Callers that already hold a client should use DrainNestedContainers.
func (c *Controller) DrainSandbox(ctx context.Context, sandboxID string, grace time.Duration) error {
	sb, err := c.store.GetSandbox(ctx, sandboxID)
	if err != nil {
		return err
	}
	hc, err := c.HydraClient(sb)
	if err != nil {
		return err
	}
	return DrainNestedContainers(ctx, hc, sandboxID, grace)
}
