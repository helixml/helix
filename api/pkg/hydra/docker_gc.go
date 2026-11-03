package hydra

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/rs/zerolog/log"
)

// reconcileOrphanDockerResources removes stopped session containers and
// unused per-session engine volumes whose session is not live, once they are
// older than the grace period. Stop keeps both for a warm restart, and nothing
// else removes them when the session dies without a destroy. Running
// containers and sandbox-API (sbx_) containers are never touched.
func (dm *DevContainerManager) reconcileOrphanDockerResources(ctx context.Context, liveSessions map[string]bool, grace time.Duration, dryRun bool) (containersReaped, volumesReaped []string) {
	dockerClient, err := dm.getDockerClient("")
	if err != nil {
		log.Warn().Err(err).Msg("Docker GC: failed to create Docker client")
		return nil, nil
	}
	defer dockerClient.Close()
	cutoff := time.Now().Add(-grace)
	return reapOrphanContainers(ctx, dockerClient, liveSessions, cutoff, dryRun),
		reapOrphanVolumes(ctx, dockerClient, liveSessions, cutoff, dryRun)
}

func reapOrphanContainers(ctx context.Context, dockerClient *client.Client, liveSessions map[string]bool, cutoff time.Time, dryRun bool) (reaped []string) {
	containers, err := dockerClient.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.Arg("label", containerSessionIDLabel)),
	})
	if err != nil {
		log.Warn().Err(err).Msg("Docker GC: failed to list containers")
		return nil
	}
	for _, c := range containers {
		sessionID := c.Labels[containerSessionIDLabel]
		if !isResourceID(sessionID, "ses_") || liveSessions[sessionID] || c.State == "running" {
			continue
		}
		info, err := dockerClient.ContainerInspect(ctx, c.ID)
		if err != nil {
			log.Warn().Err(err).Str("container_id", c.ID).Msg("Docker GC: failed to inspect container")
			continue
		}
		if info.State == nil || info.State.Running {
			continue
		}
		// A never-started container has no finish time; fall back to creation.
		last, err := time.Parse(time.RFC3339Nano, info.State.FinishedAt)
		if err != nil || last.IsZero() || last.Year() < 2000 {
			last = time.Unix(c.Created, 0)
		}
		if last.After(cutoff) {
			continue
		}
		name := strings.TrimPrefix(info.Name, "/")
		if !dryRun {
			// Not forced: if the session restarted the container since the
			// inspect above, Docker refuses to remove a running container,
			// which closes the window between the check and the removal.
			if err := dockerClient.ContainerRemove(ctx, c.ID, container.RemoveOptions{}); err != nil && !client.IsErrNotFound(err) {
				log.Warn().Err(err).Str("container", name).Msg("Docker GC: failed to remove orphan container")
				continue
			}
			log.Info().Str("container", name).Str("session_id", sessionID).Msg("Reaped orphan session container")
		}
		reaped = append(reaped, name)
	}
	return reaped
}

func reapOrphanVolumes(ctx context.Context, dockerClient *client.Client, liveSessions map[string]bool, cutoff time.Time, dryRun bool) (reaped []string) {
	// dangling=true: only volumes no container references, so a volume still
	// mounted by a live or kept container is never a candidate.
	volumes, err := dockerClient.VolumeList(ctx, volume.ListOptions{
		Filters: filters.NewArgs(
			filters.Arg("name", "docker-data-ses_"),
			filters.Arg("name", "rootless-docker-data-ses_"),
			filters.Arg("dangling", "true"),
		),
	})
	if err != nil {
		log.Warn().Err(err).Msg("Docker GC: failed to list volumes")
		return nil
	}
	for _, v := range volumes.Volumes {
		sessionID, ok := sessionIDFromContainerDataDir(v.Name)
		if !ok || !isResourceID(sessionID, "ses_") || liveSessions[sessionID] {
			continue
		}
		created, err := time.Parse(time.RFC3339, v.CreatedAt)
		if err != nil || created.After(cutoff) {
			continue
		}
		if !dryRun {
			if err := dockerClient.VolumeRemove(ctx, v.Name, false); err != nil && !client.IsErrNotFound(err) {
				log.Warn().Err(err).Str("volume", v.Name).Msg("Docker GC: failed to remove orphan volume")
				continue
			}
			log.Info().Str("volume", v.Name).Msg("Reaped orphan session volume")
		}
		reaped = append(reaped, v.Name)
	}
	return reaped
}

// runningTrackedSessions returns the tracked sessions whose container Docker
// reports running, plus any it cannot inspect: an unreachable daemon must not
// make a live desktop's storage look orphaned. A tracked container that has
// stopped is not live — the database live-set and the grace period decide its
// fate, exactly as for an untracked one. Without this, a desktop stopped
// without a destroy (the disk-pressure emergency brake) stayed tracked and so
// pinned its container, zvol and workspace forever.
//
// It also returns the tracked entries whose container no longer exists, for
// the caller to forget.
func (dm *DevContainerManager) runningTrackedSessions(ctx context.Context) (running []string, gone []*DevContainer) {
	dm.mu.RLock()
	tracked := make([]*DevContainer, 0, len(dm.containers))
	for _, dc := range dm.containers {
		tracked = append(tracked, dc)
	}
	dm.mu.RUnlock()

	for _, dc := range tracked {
		isRunning, err := dm.trackedContainerRunning(ctx, dc)
		switch {
		case client.IsErrNotFound(err):
			gone = append(gone, dc)
		case err != nil:
			log.Warn().Err(err).Str("session_id", dc.SessionID).Msg("Docker GC: cannot inspect tracked container; treating its session as live")
			running = append(running, dc.SessionID)
		case isRunning:
			running = append(running, dc.SessionID)
		}
	}
	return running, gone
}

// forgetContainers drops tracked entries whose container no longer exists, so
// hydra stops reporting them to the control plane. An entry replaced by a
// concurrent start is kept.
func (dm *DevContainerManager) forgetContainers(gone []*DevContainer) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	for _, dc := range gone {
		if dm.containers[dc.SessionID] == dc {
			delete(dm.containers, dc.SessionID)
		}
	}
}

// trackedInspectTimeout bounds one inspect, so a wedged dockerd cannot consume
// the whole reconcile deadline on the first tracked container.
const trackedInspectTimeout = 10 * time.Second

func (dm *DevContainerManager) trackedContainerRunning(ctx context.Context, dc *DevContainer) (bool, error) {
	dockerClient, err := dm.getDockerClient(dc.DockerSocket)
	if err != nil {
		return false, err
	}
	defer dockerClient.Close()
	inspectCtx, cancel := context.WithTimeout(ctx, trackedInspectTimeout)
	defer cancel()
	info, err := dockerClient.ContainerInspect(inspectCtx, dc.ContainerID)
	if err != nil {
		return false, err
	}
	if info.State == nil {
		return false, fmt.Errorf("container %s has no state", dc.ContainerID)
	}
	return info.State.Running, nil
}
