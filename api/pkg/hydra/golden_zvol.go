package hydra

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// ZFS zvol-based golden cache cloning. When available, this provides O(1) session
// startup by cloning the golden zvol snapshot instead of copying millions of files.
//
// Layout:
//
//	prod/container-docker                          ← parent dataset
//	  ├─ prod/container-docker/golden-prj_xxx      ← zvol per project (golden)
//	  │     └─ @gen42                              ← snapshot after golden build
//	  └─ prod/container-docker/ses-ses_yyy          ← zvol clone per session
//
// Each zvol has XFS formatted on it. Clones share blocks with the snapshot
// at the ZFS level (no DDT involvement, no metadata copy).

const (
	// zvolDefaultSize is the volsize for new golden zvols. Thin-provisioned,
	// so only used space is allocated. 500G is generous ceiling.
	zvolDefaultSize = "500G"

	// zvolMountBase is where cloned zvols are mounted.
	zvolMountBase = "/container-docker/zvol-mounts"
)

var (
	// zfsAvailableOnce caches the result of ZFS availability check.
	zfsAvailableOnce sync.Once
	zfsAvailableFlag bool

	// zfsParentDataset is the ZFS dataset under which golden zvols and session
	// clones are created. This is a dedicated dataset (e.g. prod/helix-zvols)
	// created under the pool that contains CONTAINER_DOCKER_PATH's zvol.
	zfsParentDataset string

	// helixZvolsDatasetName is the name of our nested dataset for zvol organisation.
	helixZvolsDatasetName = "helix-zvols"
)

// Command execution functions — override in tests for mocking.
var (
	// execCmdOutput runs a command and returns its stdout.
	execCmdOutput = func(name string, args ...string) ([]byte, error) {
		return exec.Command(name, args...).Output()
	}

	// execCmdCombinedOutput runs a command and returns combined stdout+stderr.
	execCmdCombinedOutput = func(name string, args ...string) ([]byte, error) {
		return exec.Command(name, args...).CombinedOutput()
	}

	// execCmdRun runs a command and returns only the error.
	execCmdRun = func(name string, args ...string) error {
		return exec.Command(name, args...).Run()
	}

	// readMountsFile reads the mount info file. Override in tests.
	readMountsFile = func() ([]byte, error) {
		return os.ReadFile("/proc/mounts")
	}

	// evalSymlinks resolves symlinks. Override in tests.
	evalSymlinks = filepath.EvalSymlinks

	// osMkdirAll creates directories. Override in tests.
	osMkdirAll = os.MkdirAll

	// goldenBaseDirOverride overrides goldenBaseDir for tests.
	// When non-empty, goldenDir() and GCMigratedGoldenDirs() use this instead.
	goldenBaseDirOverride string
)

// ZFSAvailable returns true if ZFS commands work in this environment.
// Result is cached after first call.
func ZFSAvailable() bool {
	zfsAvailableOnce.Do(func() {
		// Check if zfs binary exists and can list datasets
		out, err := execCmdCombinedOutput("zfs", "list", "-H", "-o", "name")
		if err != nil {
			log.Info().Err(err).Str("output", string(out)).
				Msg("ZFS not available, will use file-copy fallback for golden cache")
			return
		}
		zfsAvailableFlag = true

		// Detect the pool root from CONTAINER_DOCKER_PATH, then ensure
		// a dedicated helix-zvols dataset exists under it for cleanliness.
		poolRoot := detectPoolRoot()
		if poolRoot == "" {
			log.Warn().Msg("ZFS available but could not detect pool for container-docker, disabling zvol cloning")
			zfsAvailableFlag = false
			return
		}

		zfsParentDataset = poolRoot + "/" + helixZvolsDatasetName

		// Create the parent dataset if it doesn't exist.
		// Set dedup=off so all child zvols inherit it.
		if !zfsDatasetExists(zfsParentDataset) {
			if err := runCmd("zfs", "create", "-o", "dedup=off", "-o", "compression=lz4", zfsParentDataset); err != nil {
				log.Warn().Err(err).
					Str("dataset", zfsParentDataset).
					Msg("Failed to create helix-zvols dataset, disabling zvol cloning")
				zfsAvailableFlag = false
				return
			}
			log.Info().
				Str("dataset", zfsParentDataset).
				Msg("Created helix-zvols parent dataset")
		}

		log.Info().
			Str("parent_dataset", zfsParentDataset).
			Msg("ZFS zvol cloning enabled for golden cache")
	})
	return zfsAvailableFlag
}

// resetZFSState resets cached ZFS state. Only for tests.
func resetZFSState() {
	zfsAvailableOnce = sync.Once{}
	zfsAvailableFlag = false
	zfsParentDataset = ""
}

// detectPoolRoot finds the ZFS pool root that contains the CONTAINER_DOCKER_PATH zvol.
// e.g. if /prod/container-docker is backed by prod/container-docker zvol, returns "prod".
func detectPoolRoot() string {
	containerDockerPath := os.Getenv("CONTAINER_DOCKER_PATH")
	if containerDockerPath == "" {
		return ""
	}

	// Find which device is mounted at the container-docker path.
	// Inside the sandbox, CONTAINER_DOCKER_PATH is the *host* path (e.g. /prod/container-docker)
	// but the volume is bind-mounted at /container-docker inside the container.
	// Check both paths.
	mountData, err := readMountsFile()
	if err != nil {
		return ""
	}

	candidatePaths := []string{containerDockerPath, "/container-docker"}

	for _, line := range strings.Split(string(mountData), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		matched := false
		for _, cp := range candidatePaths {
			if fields[1] == cp {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		// Found the mount. The device is fields[0], e.g. /dev/zd16
		// or /dev/zvol/prod/container-docker
		// or just "prod" (ZFS dataset mounted directly, fstype=zfs)
		dev := fields[0]
		fstype := ""
		if len(fields) >= 3 {
			fstype = fields[2]
		}

		// If it's a ZFS dataset mount (fstype=zfs, device is the pool/dataset name)
		// e.g. "prod /container-docker zfs rw,..."
		if fstype == "zfs" && !strings.HasPrefix(dev, "/dev/") {
			// Device field is the dataset name (e.g. "prod" or "prod/data")
			// The pool root is the first component
			parts := strings.Split(dev, "/")
			return parts[0]
		}

		// If it's a /dev/zvol/ path, extract the dataset name
		if strings.HasPrefix(dev, "/dev/zvol/") {
			dataset := strings.TrimPrefix(dev, "/dev/zvol/")
			parts := strings.Split(dataset, "/")
			if len(parts) >= 2 {
				return strings.Join(parts[:len(parts)-1], "/")
			}
			return dataset
		}

		// If it's a /dev/zd* device, resolve via zfs
		out, err := execCmdOutput("zfs", "list", "-H", "-o", "name", "-t", "volume")
		if err != nil {
			return ""
		}
		for _, zvol := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			zvolDev := fmt.Sprintf("/dev/zvol/%s", zvol)
			realDev, err := evalSymlinks(zvolDev)
			if err != nil {
				continue
			}
			realMountDev, err := evalSymlinks(dev)
			if err != nil {
				realMountDev = dev
			}
			if realDev == realMountDev {
				parts := strings.Split(zvol, "/")
				if len(parts) >= 2 {
					return strings.Join(parts[:len(parts)-1], "/")
				}
				return zvol
			}
		}
	}

	return ""
}

// goldenZvolName returns the ZFS zvol name for a project's golden cache.
func goldenZvolName(projectID string) string {
	return fmt.Sprintf("%s/golden-%s", zfsParentDataset, projectID)
}

// sessionZvolName returns the ZFS zvol name for a session clone.
func sessionZvolName(sessionID string) string {
	return fmt.Sprintf("%s/ses-%s", zfsParentDataset, sessionID)
}

// sessionZvolMountPath returns where a session's cloned zvol is mounted.
func sessionZvolMountPath(sessionID string) string {
	return filepath.Join(zvolMountBase, sessionID)
}

// zvolDevPath returns the /dev/zvol/ path for a zvol.
func zvolDevPath(zvolName string) string {
	return fmt.Sprintf("/dev/zvol/%s", zvolName)
}

// zfsDatasetExists checks if a ZFS dataset (volume, filesystem, or snapshot) exists.
func zfsDatasetExists(name string) bool {
	return execCmdRun("zfs", "list", "-H", "-o", "name", name) == nil
}

// zfsSnapshotExists checks if a ZFS snapshot exists.
func zfsSnapshotExists(name string) bool {
	return execCmdRun("zfs", "list", "-H", "-t", "snapshot", "-o", "name", name) == nil
}

// latestGoldenSnapshot returns the latest snapshot name for a project's golden zvol,
// or empty string if none exists.
func latestGoldenSnapshot(projectID string) string {
	zvol := goldenZvolName(projectID)
	out, err := execCmdOutput("zfs", "list", "-H", "-t", "snapshot", "-o", "name",
		"-s", "creation", "-r", zvol)
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 || lines[0] == "" {
		return ""
	}
	return lines[len(lines)-1] // latest by creation time
}

// GoldenZvolExists checks if a golden zvol with at least one snapshot exists
// for the project.
func GoldenZvolExists(projectID string) bool {
	return latestGoldenSnapshot(projectID) != ""
}

// SetupGoldenClone creates a ZFS clone of the golden snapshot for a session.
// Returns the mount path where the clone is accessible, ready to bind-mount
// into the container as /var/lib/docker.
func SetupGoldenClone(projectID, sessionID string) (string, error) {
	snapshot := latestGoldenSnapshot(projectID)
	if snapshot == "" {
		return "", fmt.Errorf("no golden snapshot found for project %s", projectID)
	}

	cloneName := sessionZvolName(sessionID)
	mountPath := sessionZvolMountPath(sessionID)

	// If clone already exists and is mounted, reuse it (session restart)
	if zfsDatasetExists(cloneName) {
		if isMounted(mountPath) {
			log.Info().
				Str("clone", cloneName).
				Str("mount", mountPath).
				Msg("Reusing existing ZFS clone (session restart)")
			return mountPath, nil
		}
		// Clone exists but not mounted — mount it
		if err := mountZvol(cloneName, mountPath); err != nil {
			return "", fmt.Errorf("failed to mount existing clone %s: %w", cloneName, err)
		}
		log.Info().
			Str("clone", cloneName).
			Str("mount", mountPath).
			Msg("Mounted existing ZFS clone")
		return mountPath, nil
	}

	start := time.Now()

	// Create the clone
	if err := runCmd("zfs", "clone", snapshot, cloneName); err != nil {
		return "", fmt.Errorf("zfs clone %s → %s failed: %w", snapshot, cloneName, err)
	}

	// Wait for device node to appear (kernel creates /dev/zvol/... asynchronously)
	devPath := zvolDevPath(cloneName)
	for i := 0; i < 30; i++ {
		if _, err := os.Stat(devPath); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Mount the clone with -o nouuid. XFS refuses to mount two filesystems
	// with the same UUID, and clones inherit the golden's UUID. We can't use
	// xfs_admin -U generate because the clone's XFS log may have unplayed
	// entries from the golden build, which xfs_admin refuses to modify.
	// nouuid skips the UUID check entirely — safe because each clone is on
	// its own separate block device (zvol).
	if err := mountZvolWithOptions(cloneName, mountPath, "nouuid"); err != nil {
		// Cleanup the clone on mount failure
		_ = runCmd("zfs", "destroy", cloneName)
		return "", fmt.Errorf("failed to mount clone %s at %s: %w", cloneName, mountPath, err)
	}

	// Remove stale golden build result marker from the clone (same as SetupGoldenCopy)
	resultFile := filepath.Join(mountPath, ".golden-build-result")
	_ = os.Remove(resultFile)

	elapsed := time.Since(start)
	log.Info().
		Str("snapshot", snapshot).
		Str("clone", cloneName).
		Str("mount", mountPath).
		Dur("clone_duration", elapsed).
		Msg("Created ZFS clone for session (instant golden cache)")

	return mountPath, nil
}

// CleanupSessionZvol unmounts and destroys a session's cloned zvol.
func CleanupSessionZvol(sessionID string) error {
	cloneName := sessionZvolName(sessionID)
	mountPath := sessionZvolMountPath(sessionID)

	if !zfsDatasetExists(cloneName) {
		return nil // nothing to clean up
	}

	// Unmount
	if isMounted(mountPath) {
		if err := runCmd("umount", mountPath); err != nil {
			// Try lazy unmount if normal unmount fails (device busy)
			if err2 := runCmd("umount", "-l", mountPath); err2 != nil {
				return fmt.Errorf("failed to unmount %s: %w (lazy also failed: %v)", mountPath, err, err2)
			}
		}
	}

	// Destroy the clone
	if err := runCmd("zfs", "destroy", cloneName); err != nil {
		return fmt.Errorf("failed to destroy clone %s: %w", cloneName, err)
	}

	// Remove mount point directory
	_ = os.Remove(mountPath)

	log.Info().
		Str("clone", cloneName).
		Str("session_id", sessionID).
		Msg("Cleaned up session ZFS clone")

	return nil
}

// PromoteSessionToGoldenZvol takes a session's Docker data and creates/updates
// the project's golden zvol from it.
//
// This is called after a golden build completes. The session was running on a
// cloned zvol (or a fresh one for the first golden build). We:
// 1. Unmount the session clone
// 2. Promote the clone to replace the golden zvol
// 3. Take a new snapshot for future clones
func PromoteSessionToGoldenZvol(projectID, sessionID string) error {
	cloneName := sessionZvolName(sessionID)
	goldenName := goldenZvolName(projectID)
	mountPath := sessionZvolMountPath(sessionID)

	// Take write lock to prevent concurrent clone operations during promotion
	lock := getGoldenLock(projectID)
	lock.Lock()
	defer lock.Unlock()

	// Read current generation
	nextGeneration := 1
	// We can't easily read golden-version.json without mounting, so check snapshot names
	oldSnapshot := latestGoldenSnapshot(projectID)
	if oldSnapshot != "" {
		// Parse generation from snapshot name if possible
		parts := strings.Split(oldSnapshot, "@gen")
		if len(parts) == 2 {
			var gen int
			fmt.Sscanf(parts[1], "%d", &gen)
			nextGeneration = gen + 1
		}
	}

	// Unmount the session clone if mounted
	if isMounted(mountPath) {
		if err := runCmd("umount", mountPath); err != nil {
			return fmt.Errorf("failed to unmount session clone %s: %w", mountPath, err)
		}
	}

	if zfsDatasetExists(goldenName) {
		// Golden zvol already exists (this is a rebuild).
		// Promote the clone: this makes the clone independent of its parent snapshot,
		// then we can destroy the old golden.
		if err := runCmd("zfs", "promote", cloneName); err != nil {
			return fmt.Errorf("zfs promote %s failed: %w", cloneName, err)
		}

		// Destroy old snapshots on the (now-promoted) clone that reference the old golden
		// The promote flipped the parent-child relationship, so old golden's snapshots
		// are now children of the promoted clone.
		out, _ := execCmdOutput("zfs", "list", "-H", "-t", "snapshot", "-o", "name", "-r", cloneName)
		for _, snap := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if snap != "" {
				_ = runCmd("zfs", "destroy", snap)
			}
		}

		// Now destroy the old golden (it has no dependents after promote)
		if err := runCmd("zfs", "destroy", "-r", goldenName); err != nil {
			log.Warn().Err(err).Str("golden", goldenName).
				Msg("Failed to destroy old golden zvol (will retry on next promotion)")
		}

		// Rename the promoted clone to be the golden
		if err := runCmd("zfs", "rename", cloneName, goldenName); err != nil {
			return fmt.Errorf("zfs rename %s → %s failed: %w", cloneName, goldenName, err)
		}
	} else {
		// First golden build — just rename the session zvol to golden
		if err := runCmd("zfs", "rename", cloneName, goldenName); err != nil {
			return fmt.Errorf("zfs rename %s → %s failed: %w", cloneName, goldenName, err)
		}
	}

	// Mount golden, purge containers, write version, unmount
	goldenMount := filepath.Join(zvolMountBase, "golden-"+projectID)
	if err := mountZvol(goldenName, goldenMount); err != nil {
		return fmt.Errorf("failed to mount golden for purge: %w", err)
	}

	// Purge container-specific state
	purgeContainerDirs(goldenMount)

	// Write golden version info
	info := GoldenVersionInfo{
		Generation: nextGeneration,
		CreatedAt:  time.Now(),
		SessionID:  sessionID,
		ProjectID:  projectID,
	}
	data, _ := json.MarshalIndent(info, "", "  ")
	_ = os.WriteFile(filepath.Join(goldenMount, "golden-version.json"), data, 0644)

	// Flush XFS journal before snapshot — ensures clones mount instantly
	// (no journal replay needed). Without this, every clone pays ~2.3s
	// of journal replay on mount.
	_ = runCmd("sync")
	_ = runCmd("xfs_freeze", "-f", goldenMount)

	// Take snapshot while filesystem is frozen (clean journal)
	snapName := fmt.Sprintf("%s@gen%d", goldenName, nextGeneration)
	if err := runCmd("zfs", "snapshot", snapName); err != nil {
		_ = runCmd("xfs_freeze", "-u", goldenMount)
		_ = runCmd("umount", goldenMount)
		_ = os.Remove(goldenMount)
		return fmt.Errorf("zfs snapshot %s failed: %w", snapName, err)
	}

	// Thaw and unmount
	_ = runCmd("xfs_freeze", "-u", goldenMount)
	_ = runCmd("umount", goldenMount)
	_ = os.Remove(goldenMount)

	log.Info().
		Str("project_id", projectID).
		Str("golden", goldenName).
		Str("snapshot", snapName).
		Int("generation", nextGeneration).
		Msg("Promoted session to golden zvol")

	return nil
}

// CreateGoldenZvol creates a new golden zvol for a project (first golden build).
// Returns the mount path for the new zvol.
func CreateGoldenZvol(projectID string) (string, error) {
	zvolName := goldenZvolName(projectID)

	if zfsDatasetExists(zvolName) {
		return "", fmt.Errorf("golden zvol %s already exists", zvolName)
	}

	// Create thin-provisioned zvol with dedup=off. Block sharing comes from
	// ZFS clones (free, no DDT involvement), so dedup adds only overhead here.
	if err := runCmd("zfs", "create", "-V", zvolDefaultSize, "-s",
		"-o", "dedup=off", "-o", "compression=lz4",
		zvolName); err != nil {
		return "", fmt.Errorf("zfs create %s failed: %w", zvolName, err)
	}

	// Wait for device to appear
	devPath := zvolDevPath(zvolName)
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(devPath); err == nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	// Format as XFS
	if err := runCmd("mkfs.xfs", "-f", "-q", devPath); err != nil {
		_ = runCmd("zfs", "destroy", zvolName)
		return "", fmt.Errorf("mkfs.xfs %s failed: %w", devPath, err)
	}

	// Mount
	mountPath := filepath.Join(zvolMountBase, "golden-"+projectID)
	if err := mountZvol(zvolName, mountPath); err != nil {
		_ = runCmd("zfs", "destroy", zvolName)
		return "", fmt.Errorf("mount golden zvol failed: %w", err)
	}

	log.Info().
		Str("zvol", zvolName).
		Str("mount", mountPath).
		Msg("Created new golden zvol")

	return mountPath, nil
}

// CreateSessionZvol creates a fresh zvol for a session (no golden cache).
// Used when no golden snapshot exists yet (first session / first golden build).
func CreateSessionZvol(sessionID string) (string, error) {
	zvolName := sessionZvolName(sessionID)

	if zfsDatasetExists(zvolName) {
		// Already exists — just mount and return
		mountPath := sessionZvolMountPath(sessionID)
		if isMounted(mountPath) {
			return mountPath, nil
		}
		if err := mountZvol(zvolName, mountPath); err != nil {
			return "", err
		}
		return mountPath, nil
	}

	// Create thin-provisioned zvol with dedup=off (same rationale as golden zvols).
	if err := runCmd("zfs", "create", "-V", zvolDefaultSize, "-s",
		"-o", "dedup=off", "-o", "compression=lz4",
		zvolName); err != nil {
		return "", fmt.Errorf("zfs create %s failed: %w", zvolName, err)
	}

	// Wait for device
	devPath := zvolDevPath(zvolName)
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(devPath); err == nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	// Format as XFS
	if err := runCmd("mkfs.xfs", "-f", "-q", devPath); err != nil {
		_ = runCmd("zfs", "destroy", zvolName)
		return "", fmt.Errorf("mkfs.xfs %s failed: %w", devPath, err)
	}

	// Mount
	mountPath := sessionZvolMountPath(sessionID)
	if err := mountZvol(zvolName, mountPath); err != nil {
		_ = runCmd("zfs", "destroy", zvolName)
		return "", err
	}

	log.Info().
		Str("zvol", zvolName).
		Str("mount", mountPath).
		Msg("Created new session zvol (no golden cache)")

	return mountPath, nil
}

// GCOrphanedZvols destroys session zvols that are no longer active.
func GCOrphanedZvols(activeSessions map[string]bool) (int, error) {
	if !ZFSAvailable() {
		return 0, nil
	}

	prefix := zfsParentDataset + "/ses-"
	out, err := execCmdOutput("zfs", "list", "-H", "-o", "name", "-t", "volume", "-r", zfsParentDataset)
	if err != nil {
		return 0, err
	}

	var cleaned int
	for _, name := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		sessionID := strings.TrimPrefix(name, prefix)
		if activeSessions[sessionID] {
			continue
		}

		// Check .last-active marker on the mounted filesystem
		mountPath := sessionZvolMountPath(sessionID)
		if isMounted(mountPath) {
			marker := filepath.Join(mountPath, ".last-active")
			data, err := os.ReadFile(marker)
			if err == nil {
				t, err := time.Parse(time.RFC3339, string(data))
				if err == nil && time.Since(t) < 7*24*time.Hour {
					continue // still recent, keep it
				}
			}
		}

		if err := CleanupSessionZvol(sessionID); err != nil {
			log.Warn().Err(err).Str("session_id", sessionID).Msg("Failed to GC orphaned zvol")
			continue
		}
		cleaned++
	}

	return cleaned, nil
}

// GCStaleSnapshots destroys old golden snapshots that have no remaining clones.
// After session clones are GC'd, their parent snapshots become orphaned but
// aren't automatically destroyed. This reclaims the delta space held by old
// snapshots, keeping only the latest snapshot for each golden zvol.
func GCStaleSnapshots() int {
	if !ZFSAvailable() {
		return 0
	}

	// List all golden zvols
	out, err := execCmdOutput("zfs", "list", "-H", "-o", "name", "-t", "volume", "-r", zfsParentDataset)
	if err != nil {
		return 0
	}

	var cleaned int
	goldenPrefix := zfsParentDataset + "/golden-"
	for _, zvol := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if !strings.HasPrefix(zvol, goldenPrefix) {
			continue
		}

		// List snapshots for this golden, oldest first
		snapOut, err := execCmdOutput("zfs", "list", "-H", "-t", "snapshot", "-o", "name",
			"-s", "creation", "-r", zvol)
		if err != nil {
			continue
		}
		snaps := strings.Split(strings.TrimSpace(string(snapOut)), "\n")
		if len(snaps) <= 1 {
			continue // only one snapshot (or none), nothing to GC
		}

		// Keep the latest, try to destroy all others
		for _, snap := range snaps[:len(snaps)-1] {
			if snap == "" {
				continue
			}
			// zfs destroy will fail if the snapshot has dependent clones — that's fine
			if err := runCmd("zfs", "destroy", snap); err != nil {
				log.Debug().
					Str("snapshot", snap).
					Msg("Cannot destroy snapshot (likely has dependent clones), will retry next GC")
			} else {
				log.Info().
					Str("snapshot", snap).
					Msg("Destroyed stale golden snapshot")
				cleaned++
			}
		}
	}

	return cleaned
}

// effectiveGoldenBaseDir returns the golden base directory, respecting test overrides.
func effectiveGoldenBaseDir() string {
	if goldenBaseDirOverride != "" {
		return goldenBaseDirOverride
	}
	return goldenBaseDir
}

// effectiveGoldenDir returns the golden Docker data path, respecting test overrides.
func effectiveGoldenDir(projectID string) string {
	return filepath.Join(effectiveGoldenBaseDir(), projectID, "docker")
}

// MigrateGoldenToZvol creates a golden zvol from the old file-based golden dir.
// This is the one-time migration path: blocks while copying (~5 min for 59GB),
// but only the first caller pays this cost. Concurrent callers block on the
// golden lock and then find the zvol already exists.
func MigrateGoldenToZvol(projectID string) error {
	lock := getGoldenLock(projectID)
	lock.Lock()
	defer lock.Unlock()

	// Double-check under lock — another goroutine may have migrated already
	if GoldenZvolExists(projectID) {
		return nil
	}

	goldenName := goldenZvolName(projectID)

	// Create the golden zvol
	if zfsDatasetExists(goldenName) {
		// Zvol exists but no snapshot (partial previous migration?) — destroy and retry
		_ = runCmd("zfs", "destroy", "-r", goldenName)
	}

	if err := runCmd("zfs", "create", "-V", zvolDefaultSize, "-s",
		"-o", "dedup=off", "-o", "compression=lz4",
		goldenName); err != nil {
		return fmt.Errorf("zfs create %s failed: %w", goldenName, err)
	}

	// Wait for device
	devPath := zvolDevPath(goldenName)
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(devPath); err == nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	// Format as XFS
	if err := runCmd("mkfs.xfs", "-f", "-q", devPath); err != nil {
		_ = runCmd("zfs", "destroy", goldenName)
		return fmt.Errorf("mkfs.xfs failed: %w", err)
	}

	// Mount
	mountPath := filepath.Join(zvolMountBase, "golden-"+projectID)
	if err := mountZvol(goldenName, mountPath); err != nil {
		_ = runCmd("zfs", "destroy", goldenName)
		return fmt.Errorf("mount failed: %w", err)
	}

	// Seed from old golden dir
	if err := seedZvolFromGoldenDir(projectID, mountPath); err != nil {
		_ = runCmd("umount", mountPath)
		_ = runCmd("zfs", "destroy", goldenName)
		return fmt.Errorf("seed failed: %w", err)
	}

	// Flush XFS journal before snapshot — ensures clones mount instantly
	_ = runCmd("sync")
	_ = runCmd("xfs_freeze", "-f", mountPath)

	// Take snapshot while filesystem is frozen (clean journal)
	snapName := fmt.Sprintf("%s@gen1", goldenName)
	if err := runCmd("zfs", "snapshot", snapName); err != nil {
		_ = runCmd("xfs_freeze", "-u", mountPath)
		_ = runCmd("umount", mountPath)
		_ = os.Remove(mountPath)
		return fmt.Errorf("zfs snapshot failed: %w", err)
	}

	// Thaw and unmount
	_ = runCmd("xfs_freeze", "-u", mountPath)
	if err := runCmd("umount", mountPath); err != nil {
		return fmt.Errorf("umount after seed failed: %w", err)
	}
	_ = os.Remove(mountPath)

	log.Info().
		Str("project_id", projectID).
		Str("golden", goldenName).
		Str("snapshot", snapName).
		Msg("Migrated file-based golden to ZFS zvol (one-time)")

	return nil
}

// GCMigratedGoldenDirs removes old file-based golden dirs for projects that have
// been migrated to zvol-based golden cache. Once a golden zvol exists with a snapshot,
// the old golden dir at /container-docker/golden/{projectID}/ is dead weight.
func GCMigratedGoldenDirs() {
	baseDir := effectiveGoldenBaseDir()
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return // no golden base dir (e.g. fresh install, no ZFS backing)
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		projectID := e.Name()
		if !GoldenZvolExists(projectID) {
			continue // zvol not ready yet — keep the file-based golden
		}

		oldDir := filepath.Join(baseDir, projectID)
		log.Info().
			Str("project_id", projectID).
			Str("old_golden_dir", oldDir).
			Msg("Removing old file-based golden dir (migrated to ZFS zvol)")
		if err := os.RemoveAll(oldDir); err != nil {
			log.Warn().Err(err).Str("path", oldDir).
				Msg("Failed to remove old golden dir")
		}
	}
}

// purgeContainerDirs removes container-specific state from a mounted Docker data dir.
// Same logic as PurgeContainersFromGolden but operates on an arbitrary mount path.
func purgeContainerDirs(dockerDir string) {
	for _, dir := range []string{"containers", "network", "containerd", "buildx", "volumes"} {
		os.RemoveAll(filepath.Join(dockerDir, dir))
	}
	os.Remove(filepath.Join(dockerDir, ".golden-build-result"))
}

const seedCompleteMarker = ".zvol-seed-complete"

// seedZvolFromGoldenDir copies the contents of the old file-based golden dir
// into a freshly created zvol. This is the one-time migration path: it runs
// once per project when transitioning from file-copy to zvol-clone golden cache.
//
// Crash tolerant: if the API crashes mid-copy, the completion marker won't exist.
// On restart, we wipe the partial contents and re-copy from scratch.
func seedZvolFromGoldenDir(projectID, zvolMountPath string) error {
	markerPath := filepath.Join(zvolMountPath, seedCompleteMarker)

	// Already seeded (previous run completed successfully) — skip everything
	if _, err := os.Stat(markerPath); err == nil {
		log.Info().
			Str("project_id", projectID).
			Msg("Zvol already seeded from golden dir (marker present), skipping")
		return nil
	}

	// Wipe any partial contents from a previous interrupted seed.
	// The zvol is freshly formatted XFS so this is safe — there's nothing
	// valuable here that wasn't copied from the golden dir.
	entries, _ := os.ReadDir(zvolMountPath)
	if len(entries) > 0 {
		log.Warn().
			Str("zvol_mount", zvolMountPath).
			Int("partial_entries", len(entries)).
			Msg("Found partial seed data (previous crash?), wiping before re-seed")
		for _, e := range entries {
			os.RemoveAll(filepath.Join(zvolMountPath, e.Name()))
		}
	}

	src := effectiveGoldenDir(projectID)
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("golden dir %s not found: %w", src, err)
	}

	start := time.Now()
	log.Info().
		Str("src", src).
		Str("dst", zvolMountPath).
		Msg("Seeding zvol from golden dir (one-time migration, may take several minutes)")

	// cp -a copies all contents of src/ into dst/
	// The trailing /. ensures we copy contents, not the directory itself
	if err := runCmd("cp", "-a", "--reflink=auto", src+"/.", zvolMountPath+"/"); err != nil {
		return fmt.Errorf("cp golden dir to zvol failed: %w", err)
	}

	// Write completion marker — only after successful copy
	if err := os.WriteFile(markerPath, []byte(time.Now().Format(time.RFC3339)), 0644); err != nil {
		log.Warn().Err(err).Msg("Failed to write seed completion marker (seed succeeded but restart may re-copy)")
	}

	log.Info().
		Str("project_id", projectID).
		Dur("duration", time.Since(start)).
		Msg("Seeded zvol from golden dir (migration complete for this project)")

	return nil
}

// mountZvol mounts a zvol at the given path.
func mountZvol(zvolName, mountPath string) error {
	return mountZvolWithOptions(zvolName, mountPath, "")
}

// mountZvolWithOptions mounts a zvol with optional mount options (e.g. "nouuid" for XFS).
func mountZvolWithOptions(zvolName, mountPath, options string) error {
	if err := osMkdirAll(mountPath, 0755); err != nil {
		return fmt.Errorf("failed to create mount point %s: %w", mountPath, err)
	}
	devPath := zvolDevPath(zvolName)
	if options != "" {
		return runCmd("mount", "-o", options, devPath, mountPath)
	}
	return runCmd("mount", devPath, mountPath)
}

// isMounted checks if a path is a mount point.
func isMounted(path string) bool {
	return execCmdRun("mountpoint", "-q", path) == nil
}

// runCmd runs a command and returns an error with the command's stderr on failure.
func runCmd(name string, args ...string) error {
	out, err := execCmdCombinedOutput(name, args...)
	if err != nil {
		return fmt.Errorf("%s %s: %w (output: %s)", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}
