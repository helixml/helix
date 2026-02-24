package hydra

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
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

// parallelCopyDir copies src to dst using multiple workers for parallelism.
// It creates dst, then copies each top-level entry inside src concurrently.
// For the "overlay2" directory (which dominates golden cache size with hundreds
// of layer dirs), it splits the children across workers too.
//
// Each copy uses cp -a --reflink=auto for CoW on XFS/btrfs.
// workers controls the max concurrency (typically 8).
func parallelCopyDir(src, dst string, workers int) error {
	// Create destination directory preserving source permissions
	srcInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("failed to stat source %s: %w", src, err)
	}
	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return fmt.Errorf("failed to create destination %s: %w", dst, err)
	}

	// Read top-level entries
	entries, err := os.ReadDir(src)
	if err != nil {
		return fmt.Errorf("failed to read source dir %s: %w", src, err)
	}

	// Build list of copy jobs: (srcPath, dstPath)
	type copyJob struct {
		src string
		dst string
	}
	var jobs []copyJob

	for _, entry := range entries {
		name := entry.Name()
		entrySrc := filepath.Join(src, name)
		entryDst := filepath.Join(dst, name)

		// For overlay2, split its children across workers individually.
		// overlay2 typically has 100-500 layer directories, each 50-200MB.
		if name == "overlay2" && entry.IsDir() {
			if err := os.MkdirAll(entryDst, srcInfo.Mode()); err != nil {
				return fmt.Errorf("failed to create overlay2 dir: %w", err)
			}
			subEntries, err := os.ReadDir(entrySrc)
			if err != nil {
				return fmt.Errorf("failed to read overlay2 dir: %w", err)
			}
			for _, sub := range subEntries {
				jobs = append(jobs, copyJob{
					src: filepath.Join(entrySrc, sub.Name()),
					dst: filepath.Join(entryDst, sub.Name()),
				})
			}
			continue
		}

		jobs = append(jobs, copyJob{src: entrySrc, dst: entryDst})
	}

	// Execute jobs with bounded concurrency
	sem := make(chan struct{}, workers)
	var mu sync.Mutex
	var firstErr error
	var wg sync.WaitGroup

	for _, job := range jobs {
		wg.Add(1)
		sem <- struct{}{} // acquire
		go func(j copyJob) {
			defer wg.Done()
			defer func() { <-sem }() // release

			cmd := exec.Command("cp", "-a", "--reflink=auto", j.src, j.dst)
			if output, err := cmd.CombinedOutput(); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = fmt.Errorf("cp %s → %s failed: %w (output: %s)", j.src, j.dst, err, string(output))
				}
				mu.Unlock()
			}
		}(job)
	}

	wg.Wait()
	return firstErr
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
	err := parallelCopyDir(golden, dockerDir, 8)
	close(done)

	if err != nil {
		return "", fmt.Errorf("failed to copy golden to session: %w", err)
	}

	// Remove any stale golden build result marker from the copy.
	// Without this, monitorGoldenBuild() would find the old result file
	// and promote immediately without waiting for the actual build.
	// cp -a creates dockerDir as a copy of golden, so the file is at dockerDir/.golden-build-result.
	os.Remove(filepath.Join(dockerDir, ".golden-build-result"))

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

	// Remove the golden build result marker. If this file is left in the golden
	// cache, monitorGoldenBuild() on subsequent golden builds will find it
	// immediately after SetupGoldenCopy and promote prematurely — before the
	// startup script has actually run. This was the root cause of golden builds
	// completing in ~1 minute instead of the expected 10+ minutes.
	os.Remove(filepath.Join(golden, ".golden-build-result"))

	log.Info().
		Str("project_id", projectID).
		Str("golden", golden).
		Msg("Purged container/network/containerd/buildx/result state from golden cache")

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

// GCStaleGoldenDirs cleans up stale golden cache state:
// 1. Removes .old directories left behind by PromoteSessionToGolden (failed cleanup)
// 2. Removes stale .building lock files (golden builds that crashed without cleanup)
// 3. Removes golden caches for projects not accessed in maxAge (0 = skip age check)
//
// Returns the number of items cleaned and bytes freed.
func GCStaleGoldenDirs(maxAge time.Duration) (int, int64, error) {
	entries, err := os.ReadDir(goldenBaseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, 0, nil
		}
		return 0, 0, fmt.Errorf("failed to read golden dir: %w", err)
	}

	var cleaned int
	var freedBytes int64
	now := time.Now()

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		projectDir := filepath.Join(goldenBaseDir, name)

		// 1. Clean up .old directories (leftover from failed PromoteSessionToGolden)
		dockerOld := filepath.Join(projectDir, "docker.old")
		if info, err := os.Stat(dockerOld); err == nil {
			var size int64
			if out, err := exec.Command("du", "-sb", dockerOld).Output(); err == nil {
				fmt.Sscanf(string(out), "%d", &size)
			}
			age := now.Sub(info.ModTime())
			log.Info().
				Str("path", dockerOld).
				Int64("size_bytes", size).
				Dur("age", age).
				Msg("Removing stale .old golden directory")
			if err := os.RemoveAll(dockerOld); err != nil {
				log.Warn().Err(err).Str("path", dockerOld).Msg("Failed to remove stale .old golden dir")
			} else {
				cleaned++
				freedBytes += size
			}
		}

		// 2. Clean up stale .building lock files (golden builds that crashed)
		buildingFile := filepath.Join(projectDir, ".building")
		if info, err := os.Stat(buildingFile); err == nil {
			// If the lock file is older than 45 minutes, the build definitely crashed
			if now.Sub(info.ModTime()) > 45*time.Minute {
				log.Info().
					Str("project_id", name).
					Time("created", info.ModTime()).
					Msg("Removing stale golden build lock file (build crashed)")
				os.Remove(buildingFile)
				cleaned++
			}
		}

		// 3. Remove golden caches not accessed recently
		if maxAge > 0 {
			dockerDir := filepath.Join(projectDir, "docker")
			info, err := os.Stat(dockerDir)
			if err != nil {
				continue // No docker dir — might just have lock file or .old
			}
			age := now.Sub(info.ModTime())
			if age > maxAge {
				var size int64
				if out, err := exec.Command("du", "-sb", dockerDir).Output(); err == nil {
					fmt.Sscanf(string(out), "%d", &size)
				}
				log.Info().
					Str("project_id", name).
					Int64("size_bytes", size).
					Dur("age", age).
					Msg("Removing stale golden cache (not used recently)")
				if err := os.RemoveAll(projectDir); err != nil {
					log.Warn().Err(err).Str("path", projectDir).Msg("Failed to remove stale golden cache")
				} else {
					cleaned++
					freedBytes += size
				}
			}
		}
	}

	if cleaned > 0 {
		log.Info().
			Int("removed", cleaned).
			Int64("freed_bytes", freedBytes).
			Msg("GC_GOLDEN_CLEANUP")
	}

	return cleaned, freedBytes, nil
}

// GCOrphanedSessionDirs removes session Docker data directories that don't
// have a corresponding running container. These accumulate when Hydra restarts
// or containers are removed without proper cleanup.
//
// activeSessions is the set of session IDs that currently have running containers.
func GCOrphanedSessionDirs(activeSessions map[string]bool) (int, int64, error) {
	entries, err := os.ReadDir(sessionsBaseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, 0, nil
		}
		return 0, 0, fmt.Errorf("failed to read sessions dir: %w", err)
	}

	var cleaned int
	var freedBytes int64

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Session dirs are named "docker-data-ses_xxxxx"
		name := entry.Name()
		sessionID := strings.TrimPrefix(name, "docker-data-")

		if activeSessions[sessionID] {
			continue // Container still running
		}

		dir := filepath.Join(sessionsBaseDir, name)

		// Get size before removal (for logging)
		var size int64
		if out, err := exec.Command("du", "-sb", dir).Output(); err == nil {
			fmt.Sscanf(string(out), "%d", &size)
		}

		if err := os.RemoveAll(dir); err != nil {
			log.Warn().Err(err).Str("path", dir).Msg("Failed to remove orphaned session dir")
			continue
		}

		cleaned++
		freedBytes += size
		log.Info().
			Str("session_id", sessionID).
			Str("path", dir).
			Int64("size_bytes", size).
			Msg("Removed orphaned session Docker data")
	}

	if cleaned > 0 {
		log.Info().
			Int("removed", cleaned).
			Int64("freed_bytes", freedBytes).
			Msg("GC_SESSION_CLEANUP")
	}

	return cleaned, freedBytes, nil
}
