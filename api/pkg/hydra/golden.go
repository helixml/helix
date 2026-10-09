package hydra

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const (
	// goldenBaseDir is the base directory for golden Docker cache snapshots.
	// Each project gets its own golden at {goldenBaseDir}/{projectID}/docker/.
	goldenBaseDir = "/container-docker/golden"

	// Golden copy markers distinguish a completed copy, an interrupted copy,
	// and legacy markerless Podman session data.
	goldenCopyCompleteMarker         = ".golden-copy-complete"
	goldenCopyInProgressMarkerSuffix = "-golden-copy-in-progress"
)

type goldenCacheKind string

const (
	dockerGoldenCache goldenCacheKind = "docker"
	podmanGoldenCache goldenCacheKind = "podman"
)

// sessionsBaseDir is where per-session Docker data lives.
// Var (not const) so tests can override it.
var sessionsBaseDir = "/container-docker/sessions"

// goldenDir returns the golden Docker data path for a project.
func goldenDir(projectID string) string {
	return goldenDirForKind(projectID, dockerGoldenCache)
}

func goldenDirForKind(projectID string, kind goldenCacheKind) string {
	return filepath.Join(effectiveGoldenBaseDir(), projectID, string(kind))
}

// sessionOverlayDir returns the session overlay directory (upper/work/merged).
func sessionOverlayDir(volumeName string) string {
	return filepath.Join(sessionsBaseDir, volumeName)
}

func goldenCopyInProgressMarkerPath(volumeName string, kind goldenCacheKind) string {
	return filepath.Join(sessionOverlayDir(volumeName), "."+string(kind)+goldenCopyInProgressMarkerSuffix)
}

// goldenLocks provides per-project locking for golden directory access.
// SetupGoldenCopy takes a read lock (concurrent copies OK).
// PromoteSessionToGolden takes a write lock (exclusive — blocks copies during rename).
var (
	goldenLocksMu sync.Mutex
	goldenLocks   = make(map[string]*sync.RWMutex)
)

func getGoldenLock(projectID string) *sync.RWMutex {
	return getGoldenLockForKind(projectID, dockerGoldenCache)
}

func getGoldenLockForKind(projectID string, kind goldenCacheKind) *sync.RWMutex {
	goldenLocksMu.Lock()
	defer goldenLocksMu.Unlock()
	key := projectID + ":" + string(kind)
	if l, ok := goldenLocks[key]; ok {
		return l
	}
	l := &sync.RWMutex{}
	goldenLocks[key] = l
	return l
}

// GoldenVersionInfo records metadata about a golden cache snapshot.
// Written to golden-version.json inside the golden docker directory on
// each promotion. Copied into sessions so containers can identify which
// golden cache they're running from.
type GoldenVersionInfo struct {
	Generation int       `json:"generation"`
	CreatedAt  time.Time `json:"created_at"`
	SessionID  string    `json:"session_id"`
	ProjectID  string    `json:"project_id"`
}

// ReadGoldenVersion reads the golden-version.json from a project's golden cache.
// Returns nil if the file doesn't exist or can't be parsed.
func ReadGoldenVersion(projectID string) *GoldenVersionInfo {
	return readGoldenVersionForKind(projectID, dockerGoldenCache)
}

func readGoldenVersionForKind(projectID string, kind goldenCacheKind) *GoldenVersionInfo {
	path := filepath.Join(goldenDirForKind(projectID, kind), "golden-version.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var info GoldenVersionInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil
	}
	return &info
}

func writeGoldenVersion(projectID, sessionID string, generation int) error {
	return writeGoldenVersionForKind(projectID, sessionID, generation, dockerGoldenCache)
}

func writeGoldenVersionForKind(projectID, sessionID string, generation int, kind goldenCacheKind) error {
	info := GoldenVersionInfo{
		Generation: generation,
		CreatedAt:  time.Now(),
		SessionID:  sessionID,
		ProjectID:  projectID,
	}
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(goldenDirForKind(projectID, kind), "golden-version.json"), data, 0644)
}

// GoldenExists checks if a golden Docker cache snapshot exists for the project.
func GoldenExists(projectID string) bool {
	return goldenExistsForKind(projectID, dockerGoldenCache)
}

func goldenExistsForKind(projectID string, kind goldenCacheKind) bool {
	if projectID == "" {
		return false
	}
	info, err := os.Stat(goldenDirForKind(projectID, kind))
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
			// Get the actual overlay2 directory's mode (not the root's)
			overlay2Info, err := os.Stat(entrySrc)
			if err != nil {
				return fmt.Errorf("failed to stat overlay2 dir: %w", err)
			}
			if err := os.MkdirAll(entryDst, overlay2Info.Mode().Perm()); err != nil {
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
		// Short-circuit: stop launching new jobs once one has failed
		mu.Lock()
		failed := firstErr != nil
		mu.Unlock()
		if failed {
			break
		}

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
	return setupGoldenCopyForKind(projectID, volumeName, dockerGoldenCache, onProgress)
}

func setupGoldenCopyForKind(projectID, volumeName string, kind goldenCacheKind, onProgress func(copied, total int64)) (string, error) {
	golden := goldenDirForKind(projectID, kind)
	base := sessionOverlayDir(volumeName)
	dataDir := filepath.Join(base, string(kind))

	// Take a read lock so PromoteSessionToGolden can't rename the source mid-copy.
	lock := getGoldenLockForKind(projectID, kind)
	lock.RLock()
	defer lock.RUnlock()

	// Create session directory
	if err := os.MkdirAll(base, 0755); err != nil {
		return "", fmt.Errorf("failed to create session dir %s: %w", base, err)
	}
	inProgressMarker := goldenCopyInProgressMarkerPath(volumeName, kind)
	if err := os.WriteFile(inProgressMarker, []byte(time.Now().UTC().Format(time.RFC3339)), 0644); err != nil {
		return "", fmt.Errorf("failed to mark golden copy in progress at %s: %w", inProgressMarker, err)
	}
	markerPath := filepath.Join(dataDir, goldenCopyCompleteMarker)
	if err := os.Remove(markerPath); err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("failed to remove stale golden copy completion marker at %s: %w", markerPath, err)
	}

	// Log which golden version we're copying from
	if ver := readGoldenVersionForKind(projectID, kind); ver != nil {
		log.Info().
			Str("project_id", projectID).
			Int("golden_generation", ver.Generation).
			Str("golden_session_id", ver.SessionID).
			Time("golden_created_at", ver.CreatedAt).
			Msg("Copying from golden cache")
	}

	// Copy golden to session docker dir.
	// cp -a preserves permissions, ownership, timestamps.
	// --reflink=auto uses copy-on-write on supporting filesystems (XFS, btrfs)
	// making the copy near-instant. Falls back silently to full copy on ext4.
	goldenSize := getGoldenSizeForKind(projectID, kind)
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
					out, err := exec.Command("du", "-sb", dataDir).Output()
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
	// Use NumCPU workers — each worker spawns a cp process, and XFS
	// parallelizes inode allocation across allocation groups.
	cpWorkers := runtime.NumCPU()
	if cpWorkers > 16 {
		cpWorkers = 16 // cap to avoid spawning too many cp processes
	}
	if cpWorkers < 2 {
		cpWorkers = 2
	}
	err := parallelCopyDir(golden, dataDir, cpWorkers)
	close(done)

	if err != nil {
		return "", fmt.Errorf("failed to copy golden to session: %w", err)
	}

	// Remove any stale golden build result marker from the copy.
	// Without this, monitorGoldenBuild() would find the old result file
	// and promote immediately without waiting for the actual build.
	// cp -a creates dockerDir as a copy of golden, so the file is at dockerDir/.golden-build-result.
	resultFile := filepath.Join(dataDir, ".golden-build-result")
	if err := os.Remove(resultFile); err != nil && !os.IsNotExist(err) {
		log.Warn().Err(err).Str("path", resultFile).
			Msg("Failed to remove stale golden build result marker — risk of premature promotion")
	}

	// Final progress: report 100%
	if onProgress != nil {
		onProgress(goldenSize, goldenSize)
	}

	// Write completion marker so we can detect interrupted copies on restart.
	if err := os.WriteFile(markerPath, []byte(time.Now().UTC().Format(time.RFC3339)), 0644); err != nil {
		return "", fmt.Errorf("failed to write golden copy completion marker at %s: %w", markerPath, err)
	}
	if err := os.Remove(inProgressMarker); err != nil && !os.IsNotExist(err) {
		log.Warn().Err(err).Str("path", inProgressMarker).
			Msg("Failed to remove golden copy in-progress marker")
	}

	elapsed := time.Since(start)
	log.Info().
		Str("golden", golden).
		Str("data_dir", dataDir).
		Str("volume", volumeName).
		Int64("size_bytes", goldenSize).
		Dur("copy_duration", elapsed).
		Str("cache_kind", string(kind)).
		Msg("Golden container cache copied to session")

	return dataDir, nil
}

// IsGoldenCopyComplete checks whether a session's Docker data directory
// was fully copied from the golden cache. Returns false if the directory
// exists but the copy was interrupted (e.g. by an API crash).
func IsGoldenCopyComplete(volumeName string) bool {
	return isGoldenCopyCompleteForKind(volumeName, dockerGoldenCache)
}

func isGoldenCopyCompleteForKind(volumeName string, kind goldenCacheKind) bool {
	dataDir := filepath.Join(sessionOverlayDir(volumeName), string(kind))
	markerPath := filepath.Join(dataDir, goldenCopyCompleteMarker)
	_, err := os.Stat(markerPath)
	return err == nil
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
// The sessionID is recorded in golden-version.json for runtime identification.
func PromoteSessionToGolden(projectID, volumeName, sessionID string) error {
	return promoteSessionToGoldenForKind(projectID, volumeName, sessionID, dockerGoldenCache)
}

func promoteSessionToGoldenForKind(projectID, volumeName, sessionID string, kind goldenCacheKind) error {
	sessionDataDir := filepath.Join(sessionsBaseDir, volumeName, string(kind))
	goldenProjectDir := filepath.Join(effectiveGoldenBaseDir(), projectID)
	targetDir := goldenDirForKind(projectID, kind)

	// Take a write lock so concurrent SetupGoldenCopy calls finish before we rename.
	lock := getGoldenLockForKind(projectID, kind)
	lock.Lock()
	defer lock.Unlock()

	// Verify session docker data exists
	if _, err := os.Stat(sessionDataDir); err != nil {
		return fmt.Errorf("session container data not found at %s: %w", sessionDataDir, err)
	}

	// Read current generation before we rename the old golden away.
	nextGeneration := 1
	if ver := readGoldenVersionForKind(projectID, kind); ver != nil {
		nextGeneration = ver.Generation + 1
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
	if err := os.Rename(sessionDataDir, targetDir); err != nil {
		// Try to restore old golden
		if hasOldGolden {
			_ = os.Rename(oldGolden, targetDir)
		}
		return fmt.Errorf("failed to promote session to golden: %w", err)
	}

	// Write golden version info so sessions can identify which golden they got.
	if err := writeGoldenVersionForKind(projectID, sessionID, nextGeneration, kind); err != nil {
		log.Warn().Err(err).Str("project_id", projectID).
			Msg("Failed to write golden-version.json (non-fatal)")
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
		Str("source", sessionDataDir).
		Str("golden", targetDir).
		Int("generation", nextGeneration).
		Str("session_id", sessionID).
		Str("cache_kind", string(kind)).
		Msg("Promoted session container data to golden cache")

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

// DeleteGolden removes a project's golden container cache snapshots.
// Handles both file-based golden dirs and ZFS zvol-based goldens.
func DeleteGolden(projectID string) error {
	if ZFSAvailable() {
		for _, kind := range []goldenCacheKind{dockerGoldenCache, podmanGoldenCache} {
			goldenName := goldenZvolNameForKind(projectID, kind)
			if !zfsDatasetExists(goldenName) {
				continue
			}
			runningClones, _, err := goldenCloneSessions(projectID, kind)
			if err != nil {
				return err
			}
			if len(runningClones) > 0 {
				return fmt.Errorf("cannot delete golden cache: %d running session(s) depend on it (stop them first): %s",
					len(runningClones), strings.Join(runningClones, ", "))
			}
		}
	}

	for _, kind := range []goldenCacheKind{dockerGoldenCache, podmanGoldenCache} {
		if err := deleteGoldenForKind(projectID, kind); err != nil {
			return err
		}
	}
	_ = os.Remove(filepath.Join(effectiveGoldenBaseDir(), projectID))
	return nil
}

func deleteGoldenForKind(projectID string, kind goldenCacheKind) error {
	// Delete ZFS golden zvol if it exists
	if ZFSAvailable() {
		goldenName := goldenZvolNameForKind(projectID, kind)
		if zfsDatasetExists(goldenName) {
			_, stoppedClones, err := goldenCloneSessions(projectID, kind)
			if err != nil {
				return err
			}
			for _, sessionID := range stoppedClones {
				log.Info().Str("session_id", sessionID).
					Msg("Destroying stopped session clone before golden cache deletion")
				_ = cleanupSessionZvolForKind(sessionID, kind)
			}

			// Now destroy the golden zvol and its snapshots
			if err := runCmd("zfs", "destroy", "-r", goldenName); err != nil {
				return fmt.Errorf("failed to destroy golden zvol %s: %w", goldenName, err)
			}
			log.Info().Str("project_id", projectID).Str("zvol", goldenName).Msg("Deleted golden ZFS zvol")
		}
	}

	// Delete file-based golden dir if it exists
	dir := goldenDirForKind(projectID, kind)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil // nothing to delete
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("failed to remove golden cache at %s: %w", dir, err)
	}
	log.Info().Str("project_id", projectID).Str("cache_kind", string(kind)).Str("path", dir).Msg("Deleted golden container cache")
	return nil
}

func goldenCloneSessions(projectID string, kind goldenCacheKind) (running, stopped []string, err error) {
	goldenName := goldenZvolNameForKind(projectID, kind)
	out, err := execCmdOutput("zfs", "list", "-H", "-o", "name,origin", "-t", "volume", "-r", zfsParentDataset)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list clones for golden zvol %s: %w", goldenName, err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || !strings.HasPrefix(fields[1], goldenName+"@") {
			continue
		}
		sessionID, cloneKind, ok := parseSessionZvolName(fields[0])
		if !ok || cloneKind != kind {
			continue
		}
		if isMounted(sessionZvolMountPathForKind(sessionID, kind)) {
			running = append(running, sessionID)
		} else {
			stopped = append(stopped, sessionID)
		}
	}
	return running, stopped, nil
}

// PurgeContainersFromGolden removes container-specific state from a golden cache.
// The golden cache is a copy of /var/lib/docker from the golden build session.
// It includes container metadata with bind mounts to workspace paths (e.g.
// /home/retro/work/helix/...) that don't exist in new sessions. When inner
// dockerd starts, it tries to restart those containers and auto-creates missing
// bind mount sources as empty directories, corrupting the workspace.
func PurgeContainersFromGolden(projectID string) error {
	return purgeContainersFromGoldenForKind(projectID, dockerGoldenCache)
}

func purgeContainersFromGoldenForKind(projectID string, kind goldenCacheKind) error {
	golden := goldenDirForKind(projectID, kind)

	// If the golden build result marker is left in the golden cache,
	// monitorGoldenBuild() on subsequent golden builds will find it
	// immediately after SetupGoldenCopy and promote prematurely — before the
	// startup script has actually run.
	resultFile := filepath.Join(golden, ".golden-build-result")
	if err := os.Remove(resultFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove golden build result marker at %s (risk of premature promotion): %w", resultFile, err)
	}

	if kind == dockerGoldenCache {
		purgeContainerDirs(golden)
	}

	log.Info().
		Str("project_id", projectID).
		Str("golden", golden).
		Str("cache_kind", string(kind)).
		Msg("Purged runtime state from golden cache")

	return nil
}

// purgeContainerDirs removes container-specific state from a Docker data dir
// that is about to become a golden snapshot.
//
// Everything else is kept: image/ and overlay2/ hold both image layers and
// BuildKit's snapshots (intermediate layers and RUN --mount=type=cache data),
// and buildkit/ holds the databases that reference those snapshots. BuildKit's
// references are not visible in the image layerdb, so overlay2 must never be
// pruned by "not referenced by an image" — that deletes the build cache the
// golden exists to carry. The build cache is bounded inside the golden build
// instead, by dockerd's own GC (see helix-workspace-setup.sh).
func purgeContainerDirs(dockerDir string) {
	// Container RW layers first: their records are found via layerdb/mounts,
	// which only containers create.
	removeContainerLayers(dockerDir)

	// containers: metadata with session-specific bind mounts.
	// network: stale bridge/endpoint references.
	// containerd: runtime state (shims, tasks) of the build session.
	// volumes: named/anonymous volumes from build steps; sessions create their own.
	// buildx: not dockerd state; removed in case an old desktop image wrote it.
	for _, dir := range []string{"containers", "network", "containerd", "buildx", "volumes"} {
		os.RemoveAll(filepath.Join(dockerDir, dir))
	}
	os.Remove(filepath.Join(dockerDir, ".golden-build-result"))
}

// removeContainerLayers deletes the overlay2 RW and init layers of every
// container recorded in image/overlay2/layerdb/mounts, plus those records.
// purgeContainerDirs removes containers/, so without this the containers'
// writable layers would be orphaned in the golden forever. Only containers
// create layerdb/mounts entries (BuildKit snapshots never do), so this cannot
// touch image layers or build cache.
func removeContainerLayers(dockerDir string) {
	overlay2Dir := filepath.Join(dockerDir, "overlay2")
	mountsDir := filepath.Join(dockerDir, "image", "overlay2", "layerdb", "mounts")

	entries, err := os.ReadDir(mountsDir)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Warn().Err(err).Str("path", mountsDir).Msg("Cannot read layerdb mounts, container layers not removed")
		}
		return
	}

	var removed int
	var freedBytes int64
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		recordDir := filepath.Join(mountsDir, entry.Name())
		for _, idFile := range []string{"mount-id", "init-id"} {
			data, err := os.ReadFile(filepath.Join(recordDir, idFile))
			if err != nil {
				continue
			}
			layerID := strings.TrimSpace(string(data))
			// Must name a direct child of overlay2/ (and not the l/ links dir)
			if layerID == "" || layerID == "l" || layerID == "." || layerID == ".." || strings.Contains(layerID, "/") {
				continue
			}
			layerDir := filepath.Join(overlay2Dir, layerID)
			if _, err := os.Stat(layerDir); err != nil {
				continue
			}
			freedBytes += dirSizeBytes(layerDir)
			if err := os.RemoveAll(layerDir); err != nil {
				log.Warn().Err(err).Str("path", layerDir).Msg("Failed to remove container layer")
				continue
			}
			removed++
		}
		if err := os.RemoveAll(recordDir); err != nil {
			log.Warn().Err(err).Str("path", recordDir).Msg("Failed to remove container layer record")
		}
	}

	removeStaleOverlay2Links(overlay2Dir)

	log.Info().
		Int("container_records", len(entries)).
		Int("removed_layers", removed).
		Int64("freed_bytes", freedBytes).
		Msg("Removed container RW layers from golden cache")
}

// removeStaleOverlay2Links removes overlay2/l/ short-name symlinks whose
// layer directory no longer exists.
func removeStaleOverlay2Links(overlay2Dir string) {
	linksDir := filepath.Join(overlay2Dir, "l")
	linkEntries, err := os.ReadDir(linksDir)
	if err != nil {
		return
	}
	for _, entry := range linkEntries {
		linkPath := filepath.Join(linksDir, entry.Name())
		target, err := os.Readlink(linkPath)
		if err != nil {
			continue
		}
		// Links are relative (e.g., ../abc123/diff)
		if _, err := os.Stat(filepath.Join(linksDir, target)); os.IsNotExist(err) {
			os.Remove(linkPath)
		}
	}
}

// dirSizeBytes returns the apparent size of a directory tree, or 0 on error.
func dirSizeBytes(dir string) int64 {
	var size int64
	if out, err := exec.Command("du", "-sb", dir).Output(); err == nil {
		fmt.Sscanf(string(out), "%d", &size)
	}
	return size
}

// goldenBuildKitStatsFile is written to the Docker data root by
// helix-workspace-setup.sh at the end of a successful golden build. It records
// the BuildKit cache as the startup script left it (build_end) and as it goes
// into the snapshot after the golden's GC pass (pre_snapshot). It stays in the
// golden so sessions can compare their own cache against it.
const goldenBuildKitStatsFile = ".golden-buildkit-stats.json"

// BuildKitCacheStats summarises a dockerd's BuildKit cache
// (see desktop/shared/helix-buildkit-cache-stats.py). Records and TotalBytes
// come from BuildKit; the cache mount fields are measured on disk, because
// BuildKit never refreshes a cache mount's size after its first measurement.
type BuildKitCacheStats struct {
	Records         int   `json:"records"`
	TotalBytes      int64 `json:"total_bytes"`
	CacheMountCount int   `json:"cache_mount_count"`
	CacheMountBytes int64 `json:"cache_mount_bytes"`
	// Newest file mtime across all cache mounts (RFC 3339), empty if none.
	CacheMountLastWritten string `json:"cache_mount_last_written"`
	// Cache mounts whose snapshot directory could not be measured.
	CacheMountUnmeasured int `json:"cache_mount_unmeasured"`
}

// GoldenBuildKitStats is the content of goldenBuildKitStatsFile.
type GoldenBuildKitStats struct {
	BuildEnd    *BuildKitCacheStats `json:"build_end"`
	PreSnapshot *BuildKitCacheStats `json:"pre_snapshot"`
}

// readGoldenBuildKitStats reads goldenBuildKitStatsFile from the first of
// dockerDirs that has it. Returns nil if none does (e.g. older desktop image).
func readGoldenBuildKitStats(dockerDirs ...string) *GoldenBuildKitStats {
	for _, dir := range dockerDirs {
		data, err := os.ReadFile(filepath.Join(dir, goldenBuildKitStatsFile))
		if err != nil {
			continue
		}
		var stats GoldenBuildKitStats
		if err := json.Unmarshal(data, &stats); err != nil {
			log.Warn().Err(err).Str("dir", dir).Msg("Invalid golden BuildKit stats file")
			return nil
		}
		return &stats
	}
	return nil
}

// addGoldenBuildKitStats adds the golden build's BuildKit cache numbers to a
// log event, so cache loss across promotion shows up in GOLDEN_BUILD_SUMMARY.
func addGoldenBuildKitStats(ev *zerolog.Event, stats *GoldenBuildKitStats) *zerolog.Event {
	if stats == nil {
		return ev.Bool("buildkit_stats_missing", true)
	}
	if s := stats.BuildEnd; s != nil {
		ev = ev.Int64("buildkit_build_end_bytes", s.TotalBytes).
			Int64("buildkit_build_end_cache_mount_bytes", s.CacheMountBytes).
			Str("buildkit_build_end_cache_mount_last_written", s.CacheMountLastWritten)
	}
	if s := stats.PreSnapshot; s != nil {
		ev = ev.Int64("buildkit_golden_bytes", s.TotalBytes).
			Int("buildkit_golden_records", s.Records).
			Int("buildkit_golden_cache_mounts", s.CacheMountCount).
			Int64("buildkit_golden_cache_mount_bytes", s.CacheMountBytes).
			Str("buildkit_golden_cache_mount_last_written", s.CacheMountLastWritten).
			Int("buildkit_golden_cache_mounts_unmeasured", s.CacheMountUnmeasured)
	}
	return ev
}

// GetGoldenSize returns the size of a project's golden cache in bytes.
// Uses "refer" (not "used") for ZFS zvols — "used" includes snapshot deltas
// which inflates the reported size. "refer" is the actual filesystem size.
// Returns 0 if no golden exists.
func GetGoldenSize(projectID string) int64 {
	return getGoldenSizeForKind(projectID, dockerGoldenCache)
}

func getGoldenSizeForKind(projectID string, kind goldenCacheKind) int64 {
	// Try ZFS zvol size first (golden may be a zvol after migration)
	if ZFSAvailable() && goldenZvolExistsForKind(projectID, kind) {
		zvol := goldenZvolNameForKind(projectID, kind)
		out, err := execCmdOutput("zfs", "list", "-H", "-o", "refer", "-p", zvol)
		if err == nil {
			var size int64
			fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &size)
			if size > 0 {
				return size
			}
		}
	}

	// Fall back to file-based golden dir
	dir := goldenDirForKind(projectID, kind)
	out, err := execCmdOutput("du", "-sb", dir)
	if err != nil {
		return 0
	}
	var size int64
	fmt.Sscanf(string(out), "%d", &size)
	return size
}

// GCStaleGoldenDirs cleans up stale golden cache state:
// 1. Removes .old directories left behind by PromoteSessionToGolden (failed cleanup)
// 2. Removes golden caches for projects not accessed in maxAge (0 = skip age check)
//
// Returns the number of items cleaned and bytes freed.
func GCStaleGoldenDirs(maxAge time.Duration) (int, int64, error) {
	baseDir := effectiveGoldenBaseDir()
	entries, err := os.ReadDir(baseDir)
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
		projectDir := filepath.Join(baseDir, name)

		for _, kind := range []goldenCacheKind{dockerGoldenCache, podmanGoldenCache} {
			// 1. Clean up .old directories (leftover from failed promotion)
			oldDir := filepath.Join(projectDir, string(kind)+".old")
			if info, err := os.Stat(oldDir); err == nil {
				var size int64
				if out, err := exec.Command("du", "-sb", oldDir).Output(); err == nil {
					fmt.Sscanf(string(out), "%d", &size)
				}
				age := now.Sub(info.ModTime())
				log.Info().
					Str("path", oldDir).
					Int64("size_bytes", size).
					Dur("age", age).
					Msg("Removing stale .old golden directory")
				if err := os.RemoveAll(oldDir); err != nil {
					log.Warn().Err(err).Str("path", oldDir).Msg("Failed to remove stale .old golden dir")
				} else {
					cleaned++
					freedBytes += size
				}
			}

			// 2. Remove golden caches not accessed recently
			if maxAge > 0 {
				dataDir := filepath.Join(projectDir, string(kind))
				info, err := os.Stat(dataDir)
				if err != nil {
					continue
				}
				age := now.Sub(info.ModTime())
				if age > maxAge {
					var size int64
					if out, err := exec.Command("du", "-sb", dataDir).Output(); err == nil {
						fmt.Sscanf(string(out), "%d", &size)
					}
					log.Info().
						Str("project_id", name).
						Int64("size_bytes", size).
						Dur("age", age).
						Msg("Removing stale golden cache (not used recently)")
					if err := os.RemoveAll(dataDir); err != nil {
						log.Warn().Err(err).Str("path", dataDir).Msg("Failed to remove stale golden cache")
					} else {
						cleaned++
						freedBytes += size
					}
				}
			}
		}
		_ = os.Remove(projectDir)
	}

	if cleaned > 0 {
		log.Info().
			Int("removed", cleaned).
			Int64("freed_bytes", freedBytes).
			Msg("GC_GOLDEN_CLEANUP")
	}

	return cleaned, freedBytes, nil
}

const sessionLastActiveFile = ".last-active"

// TouchSessionLastActive writes a timestamp marker into the session dir so that
// GC can determine when the session was last active. Directory mtime is
// unreliable because it only updates when direct entries change, not when
// files deep inside subdirectories are modified by Docker.
//
// Called on container stop and periodically by GC for running containers.
func TouchSessionLastActive(sessionDir string) {
	marker := filepath.Join(sessionDir, sessionLastActiveFile)
	_ = os.WriteFile(marker, []byte(time.Now().Format(time.RFC3339)), 0644)
}

// sessionLastActiveAge returns how long ago a session was last active, based on
// the .last-active marker file. Returns 0 if the marker doesn't exist (session
// predates this change, or was never cleanly stopped).
func sessionLastActiveAge(sessionDir string) time.Duration {
	marker := filepath.Join(sessionDir, sessionLastActiveFile)
	data, err := os.ReadFile(marker)
	if err != nil {
		return 0
	}
	t, err := time.Parse(time.RFC3339, string(data))
	if err != nil {
		return 0
	}
	return time.Since(t)
}

// GCOrphanedSessionDirs removes session Docker data directories that haven't
// been active in over a week. Session dirs are kept around so that session
// restarts can reuse the existing Docker data instead of re-copying the
// golden cache (~28s for 30 GB).
//
// It also touches the .last-active marker for all currently running sessions
// so that if Hydra crashes, the marker is at most 10 minutes stale.
//
// activeSessions is the set of session IDs that currently have running containers.
func GCOrphanedSessionDirs(activeSessions map[string]bool) (int, int64, error) {
	const maxAge = 7 * 24 * time.Hour

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
		dir := filepath.Join(sessionsBaseDir, name)

		if activeSessions[sessionID] {
			// Container still running — refresh the marker so that if
			// Hydra crashes, GC won't consider this dir stale.
			TouchSessionLastActive(dir)
			continue
		}

		// Only clean up dirs that haven't been active in over a week. Use the
		// newer of the dir mtime and the .last-active marker (fileCopyDirAge):
		// a marker-less dir (pre-marker, or a crashed/unclean stop) falls back
		// to the dir mtime rather than being skipped forever — the bug that
		// leaked old file-copy docker-data dirs indefinitely.
		age := fileCopyDirAge(dir)
		if age < maxAge {
			continue
		}

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
			Dur("age", age).
			Msg("Removed stale session Docker data")
	}

	if cleaned > 0 {
		log.Info().
			Int("removed", cleaned).
			Int64("freed_bytes", freedBytes).
			Msg("GC_SESSION_CLEANUP")
	}

	return cleaned, freedBytes, nil
}
