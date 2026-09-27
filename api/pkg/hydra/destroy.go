package hydra

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/rs/zerolog/log"
)

// sessionRuntimeBaseDir holds per-session host state bind-mounted into desktop
// containers (PipeWire runtime dir, crash dumps). Var so tests can override it.
var sessionRuntimeBaseDir = "/data/sessions"

// DestroyDevContainer tears a session down for good. DeleteDevContainer is a
// stop: it keeps the session's inner Docker data and workspace so a restart is
// warm, and it no-ops when hydra has lost track of the container. Destroy also
// removes the container when hydra no longer tracks it, and deletes every
// on-host resource the session owns. specTaskID, when set, additionally removes
// that task's shared workspace; pass it only when the task itself is gone.
func (dm *DevContainerManager) DestroyDevContainer(ctx context.Context, sessionID, specTaskID string) (*DevContainerResponse, error) {
	if !isResourceID(sessionID, "ses_") {
		return nil, fmt.Errorf("invalid session id %q", sessionID)
	}
	if specTaskID != "" && !isResourceID(specTaskID, "spt_") {
		return nil, fmt.Errorf("invalid spec task id %q", specTaskID)
	}

	resp, err := dm.DeleteDevContainer(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	dockerClient, err := dm.getDockerClient("")
	if err != nil {
		return nil, fmt.Errorf("create Docker client: %w", err)
	}
	defer dockerClient.Close()

	var errs []error
	if err := removeSessionContainers(ctx, dockerClient, sessionID); err != nil {
		errs = append(errs, err)
	}
	volume := "docker-data-" + sessionID
	if err := dockerClient.VolumeRemove(ctx, volume, true); err != nil && !client.IsErrNotFound(err) {
		errs = append(errs, fmt.Errorf("remove volume %s: %w", volume, err))
	}
	if ZFSAvailable() {
		if err := CleanupSessionZvol(sessionID); err != nil {
			errs = append(errs, err)
		}
	}
	if err := destroyInstanceDisk(ctx, sessionID); err != nil {
		errs = append(errs, err)
	}
	dirs := []string{
		filepath.Join(sessionsBaseDir, volume),
		filepath.Join(workspacesBaseDir, "sessions", sessionID),
		filepath.Join(sessionRuntimeBaseDir, sessionID),
	}
	if specTaskID != "" {
		dirs = append(dirs, filepath.Join(workspacesBaseDir, "spec-tasks", specTaskID))
	}
	for _, dir := range dirs {
		if err := os.RemoveAll(dir); err != nil {
			errs = append(errs, fmt.Errorf("remove %s: %w", dir, err))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}

	log.Info().
		Str("session_id", sessionID).
		Str("spec_task_id", specTaskID).
		Msg("Destroyed dev container and its on-host resources")
	return resp, nil
}

// removeSessionContainers force-removes every container, running or exited,
// that belongs to the session. Hydra's in-memory map misses exited containers
// after a hydra restart, so look them up in Docker by label and legacy name.
func removeSessionContainers(ctx context.Context, dockerClient *client.Client, sessionID string) error {
	byLabel, err := dockerClient.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.Arg("label", containerSessionIDLabel+"="+sessionID)),
	})
	if err != nil {
		return fmt.Errorf("list containers for %s: %w", sessionID, err)
	}
	byName, err := dockerClient.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.Arg("name", "-external-"+strings.TrimPrefix(sessionID, "ses_"))),
	})
	if err != nil {
		return fmt.Errorf("list containers for %s: %w", sessionID, err)
	}
	legacySuffix := "-external-" + strings.TrimPrefix(sessionID, "ses_")
	ids := map[string]bool{}
	for _, c := range byLabel {
		ids[c.ID] = true
	}
	for _, c := range byName {
		for _, name := range c.Names {
			// Docker's name filter is a substring match; require the exact suffix.
			if strings.HasSuffix(name, legacySuffix) {
				ids[c.ID] = true
			}
		}
	}
	var errs []error
	for id := range ids {
		if err := dockerClient.ContainerRemove(ctx, id, container.RemoveOptions{Force: true}); err != nil && !client.IsErrNotFound(err) {
			errs = append(errs, fmt.Errorf("remove container %s: %w", id, err))
		}
	}
	return errors.Join(errs...)
}

// isResourceID reports whether id is a single well-formed path element with
// the given prefix, so it can be joined onto a base directory safely.
func isResourceID(id, prefix string) bool {
	return len(id) > len(prefix) &&
		strings.HasPrefix(id, prefix) &&
		!strings.ContainsAny(id, "/\\") &&
		id != "." && id != ".."
}
