package hydra

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/rs/zerolog/log"
)

// DefaultDrainGraceSeconds bounds how long each persistent dev container gets to
// stop its own nested containers before we give up and let the host go down.
const DefaultDrainGraceSeconds = 60

// DrainPersistentContainers gracefully stops the nested container stacks running
// inside every persistent (web-service) dev container on this host.
//
// This runs when the sandbox host itself is going away — a `docker stop` of
// helix-sandbox-app during a SANDBOX_TAG bump, a host reboot, or a hydra
// shutdown. Without it the shutdown sequence is:
//
//	host container stops → its dockerd dies → every sbx-* container is SIGKILLed
//	→ the dockerd INSIDE each sbx-* dies → the customer's Postgres is SIGKILLed
//	mid-write → corrupt WAL checkpoint → the database never starts again.
//
// That is exactly what happened to we-find.ai on 2026-07-23. Raising the stop
// timeout does not help: SIGTERM goes to each container's PID 1, which knows
// nothing about the containers on its inner dockerd. The stop has to be issued
// from inside, which is what this does.
//
// Ephemeral containers (spec tasks, dev sandboxes) are skipped — they hold no
// durable state, and draining every one would make a routine runner restart
// crawl.
//
// Best-effort and bounded: it must never prevent the host from shutting down.
func (dm *DevContainerManager) DrainPersistentContainers(ctx context.Context, graceSeconds int) int {
	if graceSeconds <= 0 {
		graceSeconds = DefaultDrainGraceSeconds
	}

	dm.mu.RLock()
	var targets []*DevContainer
	for _, dc := range dm.containers {
		if dc.Persistent && dc.Status == DevContainerStatusRunning {
			targets = append(targets, dc)
		}
	}
	dm.mu.RUnlock()

	if len(targets) == 0 {
		log.Info().Msg("Drain: no persistent dev containers to drain")
		return 0
	}

	log.Info().
		Int("containers", len(targets)).
		Int("grace_seconds", graceSeconds).
		Msg("Drain: gracefully stopping nested container stacks before host shutdown")

	// Drain in parallel: N web services must not each wait for the previous
	// one's full grace period.
	var wg sync.WaitGroup
	var mu sync.Mutex
	drained := 0
	for _, dc := range targets {
		wg.Add(1)
		go func(dc *DevContainer) {
			defer wg.Done()
			if err := dm.drainOne(ctx, dc, graceSeconds); err != nil {
				log.Warn().Err(err).
					Str("session_id", dc.SessionID).
					Msg("Drain: failed to stop nested containers; their data may not have flushed")
				return
			}
			mu.Lock()
			drained++
			mu.Unlock()
		}(dc)
	}
	wg.Wait()

	log.Info().Int("drained", drained).Int("total", len(targets)).Msg("Drain: complete")
	return drained
}

// drainOne stops the nested containers inside a single dev container by exec'ing
// `docker stop` in it. `docker stop` sends each image's declared STOPSIGNAL —
// the official Postgres image declares SIGINT, i.e. fast shutdown with a clean
// checkpoint. It deliberately does NOT remove containers, networks or volumes.
func (dm *DevContainerManager) drainOne(ctx context.Context, dc *DevContainer, graceSeconds int) error {
	dockerClient, err := dm.getDockerClient(dc.DockerSocket)
	if err != nil {
		return fmt.Errorf("docker client for %s: %w", dc.SessionID, err)
	}
	defer dockerClient.Close()

	// Allow the exec a margin over the grace period so `docker stop` runs its
	// full timeout rather than being cut off mid-drain.
	execCtx, cancel := context.WithTimeout(ctx, time.Duration(graceSeconds+15)*time.Second)
	defer cancel()

	script := fmt.Sprintf(
		`ids=$(docker ps -q 2>/dev/null); if [ -n "$ids" ]; then docker stop --time %d $ids >/dev/null 2>&1 || true; fi`,
		graceSeconds)

	execID, err := dockerClient.ContainerExecCreate(execCtx, dc.ContainerID, dockertypes.ExecConfig{
		Cmd:          []string{"/bin/sh", "-c", script},
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return fmt.Errorf("exec create: %w", err)
	}
	resp, err := dockerClient.ContainerExecAttach(execCtx, execID.ID, dockertypes.ExecStartCheck{})
	if err != nil {
		return fmt.Errorf("exec attach: %w", err)
	}
	defer resp.Close()
	_, _ = io.ReadAll(resp.Reader)

	log.Info().
		Str("session_id", dc.SessionID).
		Str("container_name", dc.ContainerName).
		Msg("Drain: nested containers stopped gracefully")
	return nil
}

// DrainPersistentDevContainers drains this host's persistent dev containers.
// Exported for the hydra binary's shutdown path (api/cmd/hydra/main.go).
func (s *Server) DrainPersistentDevContainers(ctx context.Context, graceSeconds int) int {
	return s.devContainerManager.DrainPersistentContainers(ctx, graceSeconds)
}

// handleDrain exposes the drain over hydra's unix socket so the container's
// PID 1 SIGTERM trap (sandbox/startup-app.sh) can request it before the host
// goes down. Synchronous: the caller must block until the drain finishes, or
// Docker will SIGKILL everything out from under it.
func (s *Server) handleDrain(w http.ResponseWriter, r *http.Request) {
	grace := DefaultDrainGraceSeconds
	if v := r.URL.Query().Get("grace"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			grace = parsed
		}
	}
	drained := s.devContainerManager.DrainPersistentContainers(r.Context(), grace)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"drained":%d}`, drained)
}
