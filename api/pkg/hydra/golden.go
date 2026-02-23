package hydra

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	// goldenBaseDir is the base directory for golden Docker cache snapshots.
	// Each project gets its own golden at {goldenBaseDir}/{projectID}/docker/.
	goldenBaseDir = "/container-docker/golden"

	// sessionsBaseDir is where per-session Docker data lives.
	sessionsBaseDir = "/container-docker/sessions"
)

// goldenDir returns the golden Docker data path for a project.
func goldenDir(projectID string) string {
	return filepath.Join(goldenBaseDir, projectID, "docker")
}

// sessionOverlayDir returns the session overlay directory (upper/work/merged).
func sessionOverlayDir(volumeName string) string {
	return filepath.Join(sessionsBaseDir, volumeName)
}

// GoldenExists checks if a golden Docker cache snapshot exists for the project.
func GoldenExists(projectID string) bool {
	if projectID == "" {
		return false
	}
	info, err := os.Stat(goldenDir(projectID))
	return err == nil && info.IsDir()
}

// SetupGoldenCopy copies the golden Docker cache snapshot into the session's
// Docker data directory. This pre-populates the inner dockerd with cached
// images so builds start warm instead of cold.
//
// We use a copy instead of overlayfs because Docker's overlay2 storage driver
// cannot run on top of an overlayfs mount (nested overlayfs upper restriction).
// For a typical golden (~3-5 GB), the copy takes ~5-15s on SSD, which is
// dramatically faster than the cold build it replaces (~10 min).
//
// The onProgress callback (if non-nil) is called periodically with (copiedBytes, totalBytes).
// This enables the API to show real-time progress like "Unpacking build cache (2.1/7.0 GB)".
//
// Returns the docker directory path to use as the bind mount source.
func SetupGoldenCopy(projectID, volumeName string, onProgress func(copied, total int64)) (string, error) {
	golden := goldenDir(projectID)
	base := sessionOverlayDir(volumeName)
	dockerDir := filepath.Join(base, "docker")

	// Create session directory
	if err := os.MkdirAll(base, 0755); err != nil {
		return "", fmt.Errorf("failed to create session dir %s: %w", base, err)
	}

	// Copy golden to session docker dir.
	// cp -a preserves permissions, ownership, timestamps.
	// --reflink=auto uses copy-on-write on supporting filesystems (XFS, btrfs)
	// making the copy near-instant. Falls back silently to full copy on ext4.
	goldenSize := GetGoldenSize(projectID)
	log.Info().
		Str("golden", golden).
		Int64("size_bytes", goldenSize).
		Msg("Copying golden cache to session (reflink if supported)")

	// Start progress monitor — polls destination size every 2s
	done := make(chan struct{})
	if onProgress != nil {
		onProgress(0, goldenSize)
		go func() {
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-done:
					return
				case <-ticker.C:
					out, err := exec.Command("du", "-sb", dockerDir).Output()
					if err == nil {
						var copied int64
						fmt.Sscanf(string(out), "%d", &copied)
						onProgress(copied, goldenSize)
					}
				}
			}
		}()
	}

	start := time.Now()
	cmd := exec.Command("cp", "-a", "--reflink=auto", golden, dockerDir)
	output, err := cmd.CombinedOutput()
	close(done)

	if err != nil {
		return "", fmt.Errorf("failed to copy golden to session: %w (output: %s)", err, string(output))
	}

	// Final progress: report 100%
	if onProgress != nil {
		onProgress(goldenSize, goldenSize)
	}

	elapsed := time.Since(start)
	log.Info().
		Str("golden", golden).
		Str("docker_dir", dockerDir).
		Str("volume", volumeName).
		Int64("size_bytes", goldenSize).
		Dur("copy_duration", elapsed).
		Msg("Golden Docker cache copied to session")

	return dockerDir, nil
}

// CleanupGoldenSession removes the session's Docker data directory.
func CleanupGoldenSession(volumeName string) error {
	base := sessionOverlayDir(volumeName)

	if err := os.RemoveAll(base); err != nil {
		return fmt.Errorf("failed to remove session dir %s: %w", base, err)
	}

	log.Info().Str("path", base).Msg("Cleaned up golden session dir")
	return nil
}

// PromoteSessionToGolden takes a completed golden build session's Docker data
// and promotes it to be the project's golden snapshot.
//
// The session's Docker data (at /container-docker/sessions/{volumeName}/docker/)
// is moved to /container-docker/golden/{projectID}/docker/.
// Any existing golden for the project is replaced atomically.
func PromoteSessionToGolden(projectID, volumeName string) error {
	sessionDockerDir := filepath.Join(sessionsBaseDir, volumeName, "docker")
	goldenProjectDir := filepath.Join(goldenBaseDir, projectID)
	targetDir := goldenDir(projectID)

	// Verify session docker data exists
	if _, err := os.Stat(sessionDockerDir); err != nil {
		return fmt.Errorf("session docker data not found at %s: %w", sessionDockerDir, err)
	}

	// Create golden project parent dir
	if err := os.MkdirAll(goldenProjectDir, 0755); err != nil {
		return fmt.Errorf("failed to create golden project dir: %w", err)
	}

	// If existing golden, move it aside first (atomic swap)
	oldGolden := targetDir + ".old"
	hasOldGolden := false
	if _, err := os.Stat(targetDir); err == nil {
		if err := os.Rename(targetDir, oldGolden); err != nil {
			return fmt.Errorf("failed to move old golden aside: %w", err)
		}
		hasOldGolden = true
	}

	// Move session data to golden
	if err := os.Rename(sessionDockerDir, targetDir); err != nil {
		// Try to restore old golden
		if hasOldGolden {
			_ = os.Rename(oldGolden, targetDir)
		}
		return fmt.Errorf("failed to promote session to golden: %w", err)
	}

	// Clean up old golden in background (can be large)
	if hasOldGolden {
		go func() {
			if err := os.RemoveAll(oldGolden); err != nil {
				log.Warn().Err(err).Str("path", oldGolden).Msg("Failed to remove old golden")
			}
		}()
	}

	// Clean up remaining session directory (upper/work/merged if they exist)
	sessionBase := sessionOverlayDir(volumeName)
	_ = os.RemoveAll(sessionBase)

	log.Info().
		Str("project_id", projectID).
		Str("source", sessionDockerDir).
		Str("golden", targetDir).
		Msg("Promoted session Docker data to golden cache")

	return nil
}

// CleanupSessionDockerDir removes the per-session Docker data directory.
// Works for both golden-seeded and plain sessions that use CONTAINER_DOCKER_PATH.
func CleanupSessionDockerDir(volumeName string) error {
	base := sessionOverlayDir(volumeName)

	if err := os.RemoveAll(base); err != nil {
		return fmt.Errorf("failed to remove session dir %s: %w", base, err)
	}

	log.Info().Str("path", base).Msg("Cleaned up session Docker data dir")
	return nil
}

// GoldenBuildRunning checks if a golden build is currently running by looking
// for a lock file. This provides simple debouncing — only one golden build
// per project at a time.
func GoldenBuildRunning(projectID string) bool {
	lockFile := filepath.Join(goldenBaseDir, projectID, ".building")
	_, err := os.Stat(lockFile)
	return err == nil
}

// SetGoldenBuildRunning creates or removes the golden build lock file.
func SetGoldenBuildRunning(projectID string, running bool) error {
	lockDir := filepath.Join(goldenBaseDir, projectID)
	lockFile := filepath.Join(lockDir, ".building")

	if running {
		if err := os.MkdirAll(lockDir, 0755); err != nil {
			return err
		}
		return os.WriteFile(lockFile, []byte(""), 0644)
	}
	return os.Remove(lockFile)
}

// DeleteGolden removes a project's golden Docker cache snapshot.
func DeleteGolden(projectID string) error {
	projectDir := filepath.Join(goldenBaseDir, projectID)
	if _, err := os.Stat(projectDir); os.IsNotExist(err) {
		return nil // nothing to delete
	}
	if err := os.RemoveAll(projectDir); err != nil {
		return fmt.Errorf("failed to remove golden cache at %s: %w", projectDir, err)
	}
	log.Info().Str("project_id", projectID).Str("path", projectDir).Msg("Deleted golden Docker cache")
	return nil
}

// PurgeContainersFromGolden removes container-specific state from a golden cache.
// The golden cache is a copy of /var/lib/docker from the golden build session.
// It includes container metadata with bind mounts to workspace paths (e.g.
// /home/retro/work/helix/...) that don't exist in new sessions. When inner
// dockerd starts, it tries to restart those containers and auto-creates missing
// bind mount sources as empty directories, corrupting the workspace.
//
// We keep: overlay2/ (image layers), image/ (image metadata), tmp/ (build cache).
func PurgeContainersFromGolden(projectID string) error {
	golden := goldenDir(projectID)

	// Remove container metadata — they reference session-specific bind mounts
	// that corrupt the workspace when dockerd auto-creates them as directories
	os.RemoveAll(filepath.Join(golden, "containers"))

	// Remove network state — stale sandbox-specific bridge/endpoint references
	os.RemoveAll(filepath.Join(golden, "network"))

	// Remove containerd state — stale shim references from the build session
	os.RemoveAll(filepath.Join(golden, "containerd"))

	// Remove buildx state — not needed, sessions use shared buildkit
	os.RemoveAll(filepath.Join(golden, "buildx"))

	log.Info().
		Str("project_id", projectID).
		Str("golden", golden).
		Msg("Purged container/network/containerd/buildx state from golden cache")

	return nil
}

// GetGoldenSize returns the disk usage of a project's golden cache in bytes.
// Returns 0 if no golden exists.
func GetGoldenSize(projectID string) int64 {
	dir := goldenDir(projectID)
	out, err := exec.Command("du", "-sb", dir).Output()
	if err != nil {
		return 0
	}
	var size int64
	fmt.Sscanf(string(out), "%d", &size)
	return size
}
