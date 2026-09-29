package hydra

import (
	"context"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/rs/zerolog/log"
)

// reconcileOrphanDockerResources removes stopped session containers and
// unused docker-data-<ses> volumes whose session is not live, once they are
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
			if err := dockerClient.ContainerRemove(ctx, c.ID, container.RemoveOptions{Force: true}); err != nil && !client.IsErrNotFound(err) {
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
		Filters: filters.NewArgs(filters.Arg("name", "docker-data-ses_"), filters.Arg("dangling", "true")),
	})
	if err != nil {
		log.Warn().Err(err).Msg("Docker GC: failed to list volumes")
		return nil
	}
	for _, v := range volumes.Volumes {
		sessionID, ok := strings.CutPrefix(v.Name, "docker-data-")
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
