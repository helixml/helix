package hydra

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog"
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

	// zvolBlockSize is the volblocksize for new zvols. The inner XFS uses
	// 4K blocks; the ZFS default of 16K means each XFS metadata write
	// triggers a 4K → 16K read-modify-write at the zvol layer (4× write
	// amplification). 8K halves that amplification while keeping lz4
	// compression effective. Immutable after zvol creation, so this only
	// affects newly-created zvols.
	zvolBlockSize = "8K"

	// containerDockerRoot is the in-container mount of CONTAINER_DOCKER_PATH —
	// the parent XFS-on-zvol holding buildkit state, file-copy docker-data, and
	// the zvol-mounts tree.
	containerDockerRoot = "/container-docker"

	// zvolMountBase is where cloned zvols are mounted.
	zvolMountBase = containerDockerRoot + "/zvol-mounts"
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

	// statZvolDevice stats a /dev/zvol path, following the udev symlink.
	// Override in tests.
	statZvolDevice = os.Stat

	// zvolDeviceWaitTimeout bounds how long waitForZvolDevice waits for udev
	// to (re)create /dev/zvol/<name> after a zfs create/clone/rename. Generous
	// because udev lags well behind under heavy host load.
	zvolDeviceWaitTimeout = 2 * time.Minute

	// zvolDevicePollInterval is how often waitForZvolDevice re-checks.
	zvolDevicePollInterval = 100 * time.Millisecond

	// goldenBaseDirOverride overrides goldenBaseDir for tests.
	// When non-empty, goldenDir() and GCMigratedGoldenDirs() use this instead.
	goldenBaseDirOverride string
)

// ZFSAvailable returns true if ZFS commands work in this environment.
// Result is cached after first call.
func ZFSAvailable() bool {
	zfsAvailableOnce.Do(func() {
		// When the operator has declared ZFS is configured (HELIX_EXPECT_ZFS),
		// a silent fall-through to file-copy mode is a misconfiguration that
		// leaks disk and slows session starts — escalate those logs to Error so
		// it's visible/alertable instead of a single Info/Warn line.
		expectZFS, _ := strconv.ParseBool(os.Getenv("HELIX_EXPECT_ZFS"))
		fallbackLog := func() *zerolog.Event {
			if expectZFS {
				return log.Error()
			}
			return log.Info()
		}

		// Check if zfs binary exists and can list datasets
		out, err := execCmdCombinedOutput("zfs", "list", "-H", "-o", "name")
		if err != nil {
			fallbackLog().Err(err).Str("output", string(out)).Bool("expect_zfs", expectZFS).
				Msg("ZFS not available, will use file-copy fallback for golden cache")
			return
		}
		zfsAvailableFlag = true

		// Detect the pool root from CONTAINER_DOCKER_PATH, then ensure
		// a dedicated helix-zvols dataset exists under it for cleanliness.
		poolRoot := detectPoolRoot()
		if poolRoot == "" {
			fallbackLog().Bool("expect_zfs", expectZFS).Str("container_docker_path", os.Getenv("CONTAINER_DOCKER_PATH")).
				Msg("ZFS available but could not detect pool for container-docker, disabling zvol cloning (falling back to file-copy)")
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

// touchExternalMarker creates/refreshes the .last-active marker for a session
// on the external filesystem so GC knows it's in use.
func touchExternalMarker(sessionID string) {
	externalDir := filepath.Join(sessionsBaseDir, "docker-data-"+sessionID)
	_ = os.MkdirAll(externalDir, 0755)
	TouchSessionLastActive(externalDir)
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

// ZFSTree and ZFSTreeNode are defined in types package.
// Aliases for backward compatibility within hydra package.
type ZFSTree = types.ZFSTree
type ZFSTreeNode = types.ZFSTreeNode

// GetZFSTree returns the ZFS snapshot and clone tree for a project's golden cache.
func GetZFSTree(projectID string) (*ZFSTree, error) {
	tree := &ZFSTree{
		Available: ZFSAvailable(),
		PoolRoot:  zfsParentDataset,
	}
	if !tree.Available {
		return tree, nil
	}

	goldenName := goldenZvolName(projectID)
	if !zfsDatasetExists(goldenName) {
		return tree, nil
	}

	// Get golden zvol info
	out, err := execCmdOutput("zfs", "list", "-H", "-o", "name,used,refer", goldenName)
	if err != nil {
		return tree, nil
	}
	fields := strings.Fields(strings.TrimSpace(string(out)))
	if len(fields) < 3 {
		return tree, nil
	}

	goldenNode := &ZFSTreeNode{
		Name:  fields[0],
		Type:  "golden",
		Used:  fields[1],
		Refer: fields[2],
	}
	tree.Golden = goldenNode

	// Get all snapshots for this golden
	snapOut, err := execCmdOutput("zfs", "list", "-H", "-t", "snapshot", "-o", "name,used,refer",
		"-s", "creation", "-r", goldenName)
	if err != nil {
		return tree, nil
	}

	// Build a map of snapshot → clone nodes
	// First get all volumes with their origin to find clones
	volOut, _ := execCmdOutput("zfs", "list", "-H", "-o", "name,used,refer,origin", "-t", "volume", "-r", zfsParentDataset)

	type cloneInfo struct {
		name   string
		used   string
		refer  string
		origin string
	}
	var clones []cloneInfo
	for _, line := range strings.Split(strings.TrimSpace(string(volOut)), "\n") {
		f := strings.Fields(line)
		if len(f) < 4 {
			continue
		}
		if f[3] != "-" && strings.HasPrefix(f[0], zfsParentDataset+"/ses-") {
			clones = append(clones, cloneInfo{name: f[0], used: f[1], refer: f[2], origin: f[3]})
		}
	}

	for _, snapLine := range strings.Split(strings.TrimSpace(string(snapOut)), "\n") {
		sf := strings.Fields(snapLine)
		if len(sf) < 3 {
			continue
		}
		snapNode := &ZFSTreeNode{
			Name:  sf[0],
			Type:  "snapshot",
			Used:  sf[1],
			Refer: sf[2],
		}

		// Find clones of this snapshot
		for _, c := range clones {
			if c.origin == sf[0] {
				sessionID := strings.TrimPrefix(c.name, zfsParentDataset+"/ses-")
				mountPath := sessionZvolMountPath(sessionID)
				cloneNode := &ZFSTreeNode{
					Name:      c.name,
					Type:      "clone",
					Used:      c.used,
					Refer:     c.refer,
					Mounted:   isMounted(mountPath),
					SessionID: sessionID,
				}
				snapNode.Children = append(snapNode.Children, cloneNode)
			}
		}

		goldenNode.Children = append(goldenNode.Children, snapNode)
	}

	// Find orphan session zvols (no origin, or origin from a different golden)
	for _, line := range strings.Split(strings.TrimSpace(string(volOut)), "\n") {
		f := strings.Fields(line)
		if len(f) < 4 || !strings.HasPrefix(f[0], zfsParentDataset+"/ses-") {
			continue
		}
		if f[3] == "-" {
			// No origin — fresh zvol, not a clone
			sessionID := strings.TrimPrefix(f[0], zfsParentDataset+"/ses-")
			mountPath := sessionZvolMountPath(sessionID)
			tree.Orphans = append(tree.Orphans, &ZFSTreeNode{
				Name:      f[0],
				Type:      "clone",
				Used:      f[1],
				Refer:     f[2],
				Mounted:   isMounted(mountPath),
				SessionID: sessionID,
			})
		}
	}

	return tree, nil
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
			touchExternalMarker(sessionID)
			return mountPath, nil
		}
		// Clone exists but not mounted (e.g. after reboot) — mount with nouuid
		// because the clone shares the golden's XFS UUID. discard so freed
		// blocks are TRIMmed back to the pool (XFS-on-zvol doesn't reclaim
		// otherwise — see mountZvolWithOptions).
		if err := mountZvolWithOptions(cloneName, mountPath, "nouuid,discard"); err != nil {
			return "", fmt.Errorf("failed to mount existing clone %s: %w", cloneName, err)
		}
		log.Info().
			Str("clone", cloneName).
			Str("mount", mountPath).
			Msg("Mounted existing ZFS clone")
		touchExternalMarker(sessionID)
		return mountPath, nil
	}

	start := time.Now()

	// Create the clone
	if err := runCmd("zfs", "clone", snapshot, cloneName); err != nil {
		return "", fmt.Errorf("zfs clone %s → %s failed: %w", snapshot, cloneName, err)
	}

	// Mount the clone with -o nouuid. XFS refuses to mount two filesystems
	// with the same UUID, and clones inherit the golden's UUID. We can't use
	// xfs_admin -U generate because the clone's XFS log may have unplayed
	// entries from the golden build, which xfs_admin refuses to modify.
	// nouuid skips the UUID check entirely — safe because each clone is on
	// its own separate block device (zvol).
	if err := mountZvolWithOptions(cloneName, mountPath, "nouuid,discard"); err != nil {
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

	touchExternalMarker(sessionID)
	return mountPath, nil
}

// CleanupSessionZvol unmounts and destroys a session's cloned zvol.
//
// Uses `zfs destroy -r` so any descendant snapshots (e.g. ones an operator
// created out-of-band like `@pre-repo-cleanup-...`) are torn down with the
// zvol. Without `-r`, ZFS refuses to destroy a dataset that has children and
// the orphan accumulates forever, polluting GC logs.
//
// Note: `-r` (lowercase) does NOT cascade into clones of those snapshots —
// if someone has manually cloned a child snapshot into another dataset, the
// destroy still fails with "dataset is busy", which is the right answer (we
// don't silently nuke unrelated work). The caller's GC loop logs and moves on.
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

	// Destroy the clone (recursive: takes any descendant snapshots with it)
	if err := runCmd("zfs", "destroy", "-r", cloneName); err != nil {
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

// goldenPendingPromotionProp marks a zvol whose promotion to golden has
// started but whose @genN snapshot hasn't been taken yet. Value is
// "<generation>,<sessionID>". ZFS user properties survive promote and rename,
// so if promotion dies after the rename (e.g. the golden fails to mount) the
// golden still records what is left to do and
// finishPendingGoldenPromotionLocked completes it.
const goldenPendingPromotionProp = "helix:pending-promotion"

// PromoteSessionToGoldenZvol takes a session's Docker data and creates/updates
// the project's golden zvol from it.
//
// This is called after a golden build completes. The session was running on a
// cloned zvol (or a fresh one for the first golden build). We:
// 1. Finish any previously interrupted promotion
// 2. Unmount the session clone and mark it pending promotion
// 3. Promote the clone to replace the golden zvol
// 4. Purge, version and snapshot it for future clones, then clear the mark
func PromoteSessionToGoldenZvol(projectID, sessionID string) error {
	cloneName := sessionZvolName(sessionID)
	goldenName := goldenZvolName(projectID)
	mountPath := sessionZvolMountPath(sessionID)

	// Take write lock to prevent concurrent clone operations during promotion
	lock := getGoldenLock(projectID)
	lock.Lock()
	defer lock.Unlock()

	// A previous promotion that got as far as the rename left the golden
	// unsnapshotted. Finish it first so generations stay monotonic and the
	// rebuild path below sees a normal golden.
	if _, err := finishPendingGoldenPromotionLocked(projectID); err != nil {
		return fmt.Errorf("failed to finish previous golden promotion: %w", err)
	}

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

	if err := runCmd("zfs", "set",
		fmt.Sprintf("%s=%d,%s", goldenPendingPromotionProp, nextGeneration, sessionID), cloneName); err != nil {
		return fmt.Errorf("failed to mark %s pending promotion: %w", cloneName, err)
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

	return completeGoldenPromotionLocked(projectID, nextGeneration, sessionID)
}

// pendingGoldenPromotion reads the pending-promotion mark from a zvol.
func pendingGoldenPromotion(dataset string) (generation int, sessionID string, ok bool, err error) {
	out, err := execCmdOutput("zfs", "get", "-H", "-s", "local", "-o", "value", goldenPendingPromotionProp, dataset)
	if err != nil {
		return 0, "", false, fmt.Errorf("zfs get %s %s: %w", goldenPendingPromotionProp, dataset, err)
	}
	value := strings.TrimSpace(string(out))
	if value == "" || value == "-" {
		return 0, "", false, nil
	}
	genStr, sessionID, _ := strings.Cut(value, ",")
	generation, err = strconv.Atoi(genStr)
	if err != nil || generation < 1 {
		return 0, "", false, fmt.Errorf("invalid %s=%q on %s", goldenPendingPromotionProp, value, dataset)
	}
	return generation, sessionID, true, nil
}

// finishPendingGoldenPromotionLocked completes an interrupted promotion of the
// project's golden zvol, if there is one. Caller must hold the golden write lock.
func finishPendingGoldenPromotionLocked(projectID string) (finished bool, err error) {
	goldenName := goldenZvolName(projectID)
	if !zfsDatasetExists(goldenName) {
		return false, nil
	}
	generation, sessionID, ok, err := pendingGoldenPromotion(goldenName)
	if err != nil || !ok {
		return false, err
	}
	log.Warn().
		Str("project_id", projectID).
		Str("golden", goldenName).
		Int("generation", generation).
		Msg("Golden zvol has an unfinished promotion, completing it")
	if err := completeGoldenPromotionLocked(projectID, generation, sessionID); err != nil {
		return false, err
	}
	return true, nil
}

// ReconcilePendingGoldenPromotions completes every interrupted golden
// promotion on this host. Returns the number completed.
func ReconcilePendingGoldenPromotions() int {
	if zfsParentDataset == "" {
		return 0
	}
	out, err := execCmdOutput("zfs", "get", "-H", "-r", "-t", "volume", "-s", "local",
		"-o", "name", goldenPendingPromotionProp, zfsParentDataset)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to list pending golden promotions")
		return 0
	}
	goldenPrefix := zfsParentDataset + "/golden-"
	completed := 0
	for _, name := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if !strings.HasPrefix(name, goldenPrefix) {
			continue // session clones that never reached the rename are reaped by GC
		}
		projectID := strings.TrimPrefix(name, goldenPrefix)
		lock := getGoldenLock(projectID)
		lock.Lock()
		finished, err := finishPendingGoldenPromotionLocked(projectID)
		lock.Unlock()
		if err != nil {
			log.Error().Err(err).Str("project_id", projectID).Msg("Failed to complete pending golden promotion")
			continue
		}
		if finished {
			completed++
		}
	}
	return completed
}

// completeGoldenPromotionLocked turns the freshly renamed golden zvol into a
// clonable generation: purge container state, write golden-version.json and
// take @gen<generation>, then clear the pending-promotion mark. Idempotent —
// safe to re-run after a failure at any step. Caller must hold the golden
// write lock.
func completeGoldenPromotionLocked(projectID string, generation int, sessionID string) error {
	goldenName := goldenZvolName(projectID)
	snapName := fmt.Sprintf("%s@gen%d", goldenName, generation)

	if !zfsSnapshotExists(snapName) {
		if err := snapshotGoldenLocked(projectID, generation, sessionID); err != nil {
			return err
		}
	}

	if err := runCmd("zfs", "inherit", goldenPendingPromotionProp, goldenName); err != nil {
		return fmt.Errorf("failed to clear pending promotion mark on %s: %w", goldenName, err)
	}

	// Ensure golden ends up a true root dataset. `zfs promote` above only
	// detaches one level, so without this golden stays a clone of the original
	// session's zvol, which then becomes an unreapable clone-origin and leaks
	// old snapshots forever. Metadata-only; the project lock is already held.
	if n, err := flattenGoldenToRootLocked(goldenName); err != nil {
		log.Warn().Err(err).Str("golden", goldenName).Msg("Failed to flatten golden to root after promote")
	} else if n > 0 {
		log.Info().Str("golden", goldenName).Int("promotes", n).Msg("Flattened golden to root after promote")
	}

	log.Info().
		Str("project_id", projectID).
		Str("golden", goldenName).
		Str("snapshot", snapName).
		Int("generation", generation).
		Msg("Promoted session to golden zvol")

	return nil
}

// snapshotGoldenLocked mounts the golden, purges container state, writes
// golden-version.json and takes @gen<generation> with the filesystem frozen.
func snapshotGoldenLocked(projectID string, generation int, sessionID string) error {
	goldenName := goldenZvolName(projectID)
	goldenMount := filepath.Join(zvolMountBase, "golden-"+projectID)

	// A crash mid-promotion can leave it mounted
	if !isMounted(goldenMount) {
		if err := mountZvol(goldenName, goldenMount); err != nil {
			return fmt.Errorf("failed to mount golden for purge: %w", err)
		}
	}
	defer func() {
		_ = runCmd("umount", goldenMount)
		_ = os.Remove(goldenMount)
	}()

	// Purge container-specific state
	purgeContainerDirs(goldenMount)

	// Write golden version info
	info := GoldenVersionInfo{
		Generation: generation,
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
	defer func() { _ = runCmd("xfs_freeze", "-u", goldenMount) }()

	// Take snapshot while filesystem is frozen (clean journal)
	snapName := fmt.Sprintf("%s@gen%d", goldenName, generation)
	if err := runCmd("zfs", "snapshot", snapName); err != nil {
		return fmt.Errorf("zfs snapshot %s failed: %w", snapName, err)
	}
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
		"-o", "volblocksize="+zvolBlockSize,
		"-o", "dedup=off", "-o", "compression=lz4",
		zvolName); err != nil {
		return "", fmt.Errorf("zfs create %s failed: %w", zvolName, err)
	}

	devPath := zvolDevPath(zvolName)
	if err := waitForZvolDevice(zvolName); err != nil {
		_ = runCmd("zfs", "destroy", zvolName)
		return "", err
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
		if err := mountZvolWithOptions(zvolName, mountPath, "nouuid,discard"); err != nil {
			return "", err
		}
		return mountPath, nil
	}

	// Create thin-provisioned zvol with dedup=off (same rationale as golden zvols).
	if err := runCmd("zfs", "create", "-V", zvolDefaultSize, "-s",
		"-o", "volblocksize="+zvolBlockSize,
		"-o", "dedup=off", "-o", "compression=lz4",
		zvolName); err != nil {
		return "", fmt.Errorf("zfs create %s failed: %w", zvolName, err)
	}

	devPath := zvolDevPath(zvolName)
	if err := waitForZvolDevice(zvolName); err != nil {
		_ = runCmd("zfs", "destroy", zvolName)
		return "", err
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
			// Refresh the external marker so that if Hydra crashes or the
			// session runs for weeks, the marker stays fresh and GC won't
			// destroy the zvol after the next restart.
			externalDir := filepath.Join(sessionsBaseDir, "docker-data-"+sessionID)
			_ = os.MkdirAll(externalDir, 0755)
			TouchSessionLastActive(externalDir)
			continue
		}

		// Check .last-active marker on the external filesystem.
		// The marker lives at /container-docker/sessions/docker-data-{sessionID}/
		// which is on the parent ZFS dataset, readable without mounting the XFS zvol.
		externalDir := filepath.Join(sessionsBaseDir, "docker-data-"+sessionID)
		age := sessionLastActiveAge(externalDir)
		if age > 0 && age < 7*24*time.Hour {
			continue // recently active, keep it
		}
		// age == 0 means no marker — pre-marker session, safe to GC since
		// all sessions now get markers on creation and periodic refresh.
		// age >= 7 days — stale session, GC it.

		if err := CleanupSessionZvol(sessionID); err != nil {
			log.Warn().Err(err).Str("session_id", sessionID).Msg("Failed to GC orphaned zvol")
			continue
		}
		cleaned++
	}

	return cleaned, nil
}

// zvolCreationAge returns how long ago a zvol was created, using the ZFS
// `creation` property (unix seconds, via -p). Returns 0 if the property can't
// be read or parsed — callers treat 0 as "unknown age" and apply their own
// safe default.
func zvolCreationAge(zvolName string) time.Duration {
	out, err := execCmdOutput("zfs", "get", "-Hp", "-o", "value", "creation", zvolName)
	if err != nil {
		return 0
	}
	var created int64
	if _, err := fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &created); err != nil || created <= 0 {
		return 0
	}
	return time.Since(time.Unix(created, 0))
}

// ReconcileOrphanZvols reaps session zvols (prefix "<parent>/ses-") that are NOT
// in the DB-derived liveSet, have aged past the grace period, and can be safely
// destroyed.
//
// Safety contract (do not weaken):
//   - Only ses- zvols are ever enumerated; golden-* zvols are never touched
//     (the prefix guard filters them out, mirroring GCOrphanedZvols).
//   - Destroys go through CleanupSessionZvol, which uses `zfs destroy -r`
//     (lowercase). A clone-root or busy zvol fails with "has dependent clones"
//     / "dataset is busy"; we record it as skipped and move on. We NEVER force
//     (no `-R`, no `-f`) — refusing to destroy a busy/parent dataset is the
//     desired behaviour.
//   - Live sessions get their external marker refreshed so the legacy on-disk
//     GC keeps treating them as fresh.
func ReconcileOrphanZvols(liveSet map[string]bool, grace time.Duration, dryRun bool) (reaped []string, skipped []GCSkip) {
	if !ZFSAvailable() {
		return nil, nil
	}

	prefix := zfsParentDataset + "/ses-"
	out, err := execCmdOutput("zfs", "list", "-H", "-o", "name", "-t", "volume", "-r", zfsParentDataset)
	if err != nil {
		log.Warn().Err(err).Msg("ReconcileOrphanZvols: failed to list zvols")
		return nil, nil
	}

	for _, name := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if !strings.HasPrefix(name, prefix) {
			continue // never touch golden- or anything else
		}
		sessionID := strings.TrimPrefix(name, prefix)

		if liveSet[sessionID] {
			// Live per the DB — keep, and refresh the on-disk marker so the
			// legacy 7-day GC also keeps treating it as fresh.
			externalDir := filepath.Join(sessionsBaseDir, "docker-data-"+sessionID)
			_ = os.MkdirAll(externalDir, 0755)
			TouchSessionLastActive(externalDir)
			skipped = append(skipped, GCSkip{Name: name, Reason: "live"})
			continue
		}

		// Not live. Apply the grace period using zvol creation time. age == 0
		// means we couldn't read the creation property — be conservative and
		// skip (treat as within grace).
		age := zvolCreationAge(name)
		if age < grace {
			skipped = append(skipped, GCSkip{Name: name, Reason: "grace"})
			continue
		}

		if dryRun {
			reaped = append(reaped, name)
			continue
		}

		if err := CleanupSessionZvol(sessionID); err != nil {
			msg := err.Error()
			// "has dependent clones" / "dataset is busy" → a golden clone-root
			// or a manually-cloned descendant depends on this zvol. NEVER force.
			if strings.Contains(msg, "dependent clones") || strings.Contains(msg, "busy") {
				skipped = append(skipped, GCSkip{Name: name, Reason: "dependent: " + msg})
				continue
			}
			log.Warn().Err(err).Str("zvol", name).Msg("ReconcileOrphanZvols: failed to destroy zvol")
			skipped = append(skipped, GCSkip{Name: name, Reason: "error: " + msg})
			continue
		}
		reaped = append(reaped, name)
	}

	return reaped, skipped
}

// ReconcileOrphanFileCopyDirs reaps per-session file-copy docker-data dirs
// (sessionsBaseDir/docker-data-<id>) whose session is NOT in the DB-derived
// liveSet and whose on-disk age has passed the grace period.
//
// These dirs hold the inner docker storage on hosts where ZFS is unavailable
// (the file-copy fallback in resolveDockerDataDir), and stale marker dirs on
// ZFS hosts. The durable reaper previously ignored them entirely — only the
// legacy in-memory GCOrphanedSessionDirs touched them, and that path skips any
// dir without a .last-active marker forever (age == 0). Ended-session docker
// data therefore leaked indefinitely (this is what accumulated ~1T of dead
// docker-data dirs on a file-copy host). This closes the gap using the same
// durable, DB-derived live-set the zvol reaper uses.
func ReconcileOrphanFileCopyDirs(liveSet map[string]bool, grace time.Duration, dryRun bool) (reaped []string, skipped []GCSkip) {
	entries, err := os.ReadDir(sessionsBaseDir)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Warn().Err(err).Msg("ReconcileOrphanFileCopyDirs: failed to read sessions dir")
		}
		return nil, nil
	}

	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "docker-data-") {
			continue
		}
		sessionID := strings.TrimPrefix(entry.Name(), "docker-data-")
		dir := filepath.Join(sessionsBaseDir, entry.Name())

		if liveSet[sessionID] {
			// Live per the DB — keep, and refresh the marker so the legacy
			// 7-day GC also keeps treating it as fresh.
			TouchSessionLastActive(dir)
			skipped = append(skipped, GCSkip{Name: dir, Reason: "live"})
			continue
		}

		// Not live. Apply the grace period using fileCopyDirAge (dir mtime or
		// marker, whichever is newer) — never the marker alone, so a dir
		// lacking a .last-active marker is still reapable.
		if fileCopyDirAge(dir) < grace {
			skipped = append(skipped, GCSkip{Name: dir, Reason: "grace"})
			continue
		}

		if dryRun {
			reaped = append(reaped, dir)
			continue
		}

		if err := os.RemoveAll(dir); err != nil {
			log.Warn().Err(err).Str("dir", dir).Msg("ReconcileOrphanFileCopyDirs: failed to remove orphan docker-data dir")
			skipped = append(skipped, GCSkip{Name: dir, Reason: "error: " + err.Error()})
			continue
		}
		reaped = append(reaped, dir)
	}

	return reaped, skipped
}

// fileCopyDirAge returns how long since a session docker-data dir was last
// active, using the newer of the dir's own mtime and its .last-active marker.
// Unlike sessionLastActiveAge it never returns 0 for a marker-less dir: a
// missing marker falls back to the dir mtime, so pre-marker / crashed dirs are
// still reapable. Returns 0 only when the dir can't be stat'd at all (treated
// as fresh — conservative, won't reap).
func fileCopyDirAge(dir string) time.Duration {
	var newest time.Time
	if fi, err := os.Stat(dir); err == nil {
		newest = fi.ModTime()
	}
	if fi, err := os.Stat(filepath.Join(dir, ".last-active")); err == nil && fi.ModTime().After(newest) {
		newest = fi.ModTime()
	}
	if newest.IsZero() {
		return 0
	}
	return time.Since(newest)
}

// TrimContainerDockerStorage runs fstrim over the /container-docker parent
// mount and each mounted session/golden zvol, returning blocks freed inside
// their XFS filesystems back to the ZFS pool. This is a periodic backstop:
//   - the `discard` mount option only trims a freshly-mounted XFS and cannot
//     retroactively reclaim blocks freed before it was set;
//   - operators may mount the parent /container-docker without `discard` at all
//     (Helix ships no parent mount with discard for Linux prod), so its buildkit
//     churn and any file-copy docker-data would otherwise never be reclaimed.
//
// Errors (EOPNOTSUPP on a mount that doesn't support discard, or a busy fs) are
// logged at debug and skipped — fstrim is advisory, never fatal.
func TrimContainerDockerStorage() {
	targets := []string{containerDockerRoot}
	if data, err := readMountsFile(); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 2 && strings.HasPrefix(fields[1], zvolMountBase+"/") {
				targets = append(targets, fields[1])
			}
		}
	}

	trimmed := 0
	for _, mp := range targets {
		if execCmdRun("mountpoint", "-q", mp) != nil {
			continue
		}
		out, err := execCmdCombinedOutput("fstrim", "-v", mp)
		if err != nil {
			log.Debug().Err(err).Str("mount", mp).Str("output", strings.TrimSpace(string(out))).
				Msg("periodic fstrim skipped (no discard support or fs busy)")
			continue
		}
		trimmed++
		log.Debug().Str("mount", mp).Str("result", strings.TrimSpace(string(out))).Msg("periodic fstrim ok")
	}
	if trimmed > 0 {
		log.Info().Int("mounts_trimmed", trimmed).Int("candidates", len(targets)).
			Msg("periodic fstrim of container-docker storage completed")
	}
}

// warnIfContainerDockerLacksDiscard logs a warning if /container-docker is a
// zvol-backed mount without the `discard` option. Without discard (and absent
// the periodic fstrim backstop above) blocks freed inside its XFS are never
// returned to the ZFS pool and the zvol grows unboundedly — the leak that
// filled a production pool. Diagnostic only; runs once at startup.
func warnIfContainerDockerLacksDiscard() {
	data, err := readMountsFile()
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 || fields[1] != containerDockerRoot {
			continue
		}
		dev := fields[0]
		if !strings.HasPrefix(dev, "/dev/zvol/") && !strings.HasPrefix(dev, "/dev/zd") {
			return // not zvol-backed — discard is not relevant
		}
		if !strings.Contains(","+fields[3]+",", ",discard,") {
			log.Warn().Str("device", dev).Str("options", fields[3]).Str("mount", containerDockerRoot).
				Msg("/container-docker is a zvol mounted WITHOUT 'discard' — freed blocks only return to the ZFS pool via the periodic fstrim backstop; mount it with 'discard' (and x-systemd.after=zfs-mount if in fstab) for immediate reclaim")
		}
		return
	}
}

// GCStaleSnapshots destroys golden snapshots older than 7 days that have no
// remaining clones. Keeps recent snapshots so the user can see cache progression.
// Snapshots with active session clones can't be destroyed (ZFS refuses) — that's
// handled gracefully.
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

		// List snapshots with creation time, oldest first
		snapOut, err := execCmdOutput("zfs", "list", "-H", "-t", "snapshot", "-o", "name,creation",
			"-s", "creation", "-r", zvol)
		if err != nil {
			continue
		}
		lines := strings.Split(strings.TrimSpace(string(snapOut)), "\n")
		if len(lines) <= 1 {
			continue // only one snapshot (or none), nothing to GC
		}

		// Always keep the latest snapshot regardless of age
		for _, line := range lines[:len(lines)-1] {
			if line == "" {
				continue
			}
			// Parse name and creation time
			// Format: "pool/helix-zvols/golden-prj_xxx@gen1\tDow Mon DD HH:MM YYYY"
			parts := strings.SplitN(line, "\t", 2)
			snap := parts[0]
			if len(parts) < 2 {
				continue
			}

			// Parse ZFS creation timestamp
			created, err := time.Parse("Mon Jan  2 15:04 2006", strings.TrimSpace(parts[1]))
			if err != nil {
				// Try alternate format (some ZFS versions use different format)
				created, err = time.Parse("Mon Jan 2 15:04 2006", strings.TrimSpace(parts[1]))
				if err != nil {
					continue // can't parse, skip
				}
			}

			if time.Since(created) < 7*24*time.Hour {
				continue // less than 7 days old, keep it
			}

			// zfs destroy will fail if the snapshot has dependent clones — that's fine
			if err := runCmd("zfs", "destroy", snap); err != nil {
				log.Debug().
					Str("snapshot", snap).
					Msg("Cannot destroy snapshot (likely has dependent clones), will retry next GC")
			} else {
				log.Info().
					Str("snapshot", snap).
					Msg("Destroyed stale golden snapshot (>7 days old)")
				cleaned++
			}
		}
	}

	return cleaned
}

// zfsOrigin returns the origin snapshot of a dataset, or "" when it has none
// (a root dataset; ZFS reports "-"). "" is also returned on error.
func zfsOrigin(dataset string) string {
	out, err := execCmdOutput("zfs", "get", "-H", "-o", "value", "origin", dataset)
	if err != nil {
		return ""
	}
	v := strings.TrimSpace(string(out))
	if v == "-" {
		return ""
	}
	return v
}

// flattenGoldenToRootLocked promotes a golden zvol until it is a true root
// dataset (origin == "-"). golden is built by promoting/cloning a running
// session, and `zfs promote` only detaches ONE level — so without this, golden
// stays a clone of the *first* session's zvol, which then becomes an
// unreapable clone-origin holding old snapshots forever (the GC leak).
//
// `zfs promote` is metadata-only: it re-parents the shared snapshots onto
// golden without copying or deleting any data, and live session clones keep
// working throughout. Once golden is a root, the former origin session zvols
// become ordinary leaf clones the orphan reaper can destroy, and their
// now-absorbed old snapshots are pruned by GCStaleSnapshots. Safe + reversible.
//
// The caller MUST hold the project's golden lock.
func flattenGoldenToRootLocked(goldenName string) (promotes int, err error) {
	if !zfsDatasetExists(goldenName) {
		return 0, nil
	}
	// Chains are short (golden ← session ← golden …); guard against an
	// unexpected cycle so we can never spin forever.
	const maxPromotes = 50
	for promotes < maxPromotes {
		if zfsOrigin(goldenName) == "" {
			return promotes, nil // already a root
		}
		if err := runCmd("zfs", "promote", goldenName); err != nil {
			return promotes, fmt.Errorf("zfs promote %s failed: %w", goldenName, err)
		}
		promotes++
	}
	return promotes, fmt.Errorf("golden %s still has an origin after %d promotes (possible clone cycle)", goldenName, maxPromotes)
}

// FlattenGoldenToRoot is the locked, public per-project entry point.
func FlattenGoldenToRoot(projectID string) (int, error) {
	lock := getGoldenLock(projectID)
	lock.Lock()
	defer lock.Unlock()
	return flattenGoldenToRootLocked(goldenZvolName(projectID))
}

// FlattenInvertedGoldens finds golden zvols that are still clones of another
// dataset (origin != "-") — the legacy topology where golden hangs off an
// ended session's zvol — and promotes each to a true root, detaching the dead
// session origin so the orphan reaper can reclaim it. Metadata-only and safe;
// returns the goldens it flattened (or, in dryRun, would flatten).
func FlattenInvertedGoldens(dryRun bool) (flattened []string) {
	if !ZFSAvailable() {
		return nil
	}
	out, err := execCmdOutput("zfs", "list", "-H", "-o", "name", "-t", "volume", "-r", zfsParentDataset)
	if err != nil {
		log.Warn().Err(err).Msg("FlattenInvertedGoldens: failed to list zvols")
		return nil
	}
	goldenPrefix := zfsParentDataset + "/golden-"
	for _, name := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if !strings.HasPrefix(name, goldenPrefix) {
			continue
		}
		if zfsOrigin(name) == "" {
			continue // already a root — nothing to do
		}
		if dryRun {
			flattened = append(flattened, name)
			continue
		}
		projectID := strings.TrimPrefix(name, goldenPrefix)
		promotes, err := FlattenGoldenToRoot(projectID)
		if err != nil {
			log.Warn().Err(err).Str("golden", name).Msg("FlattenInvertedGoldens: failed to flatten golden to root")
			continue
		}
		log.Info().Str("golden", name).Int("promotes", promotes).
			Msg("Flattened inverted golden to root (detached dead session origin)")
		flattened = append(flattened, name)
	}
	return flattened
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

	// A golden with no snapshot is either an interrupted promotion (finish it —
	// it holds the newest build) or a partial previous migration (destroy and retry).
	if finished, err := finishPendingGoldenPromotionLocked(projectID); err != nil || finished {
		return err
	}
	if zfsDatasetExists(goldenName) {
		_ = runCmd("zfs", "destroy", "-r", goldenName)
	}

	if err := runCmd("zfs", "create", "-V", zvolDefaultSize, "-s",
		"-o", "volblocksize="+zvolBlockSize,
		"-o", "dedup=off", "-o", "compression=lz4",
		goldenName); err != nil {
		return fmt.Errorf("zfs create %s failed: %w", goldenName, err)
	}

	devPath := zvolDevPath(goldenName)
	if err := waitForZvolDevice(goldenName); err != nil {
		_ = runCmd("zfs", "destroy", goldenName)
		return err
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

// waitForZvolDevice waits until /dev/zvol/<zvolName> resolves to a block
// device. udev creates (and, after `zfs rename`, recreates) that symlink
// asynchronously, so mounting or formatting straight after zfs
// create/clone/rename can fail with "special device does not exist".
func waitForZvolDevice(zvolName string) error {
	devPath := zvolDevPath(zvolName)
	if isBlockDevice(devPath) {
		return nil
	}
	start := time.Now()
	// Drain the udev event queue first; the poll below is what we rely on
	// (udevadm may be missing or unable to reach udevd from a container).
	if out, err := execCmdCombinedOutput("udevadm", "settle", "--timeout=30"); err != nil {
		log.Debug().Err(err).Str("output", strings.TrimSpace(string(out))).Msg("udevadm settle failed, polling for zvol device")
	}
	for !isBlockDevice(devPath) {
		if time.Since(start) >= zvolDeviceWaitTimeout {
			return fmt.Errorf("zvol device %s did not appear within %s", devPath, zvolDeviceWaitTimeout)
		}
		time.Sleep(zvolDevicePollInterval)
	}
	log.Info().
		Str("device", devPath).
		Dur("waited", time.Since(start)).
		Msg("Waited for zvol device node")
	return nil
}

func isBlockDevice(path string) bool {
	fi, err := statZvolDevice(path)
	return err == nil && fi.Mode()&os.ModeDevice != 0 && fi.Mode()&os.ModeCharDevice == 0
}

// mountZvol mounts a zvol at the given path. Mounts with "discard" so that
// blocks freed inside the XFS filesystem are TRIMmed back to the ZFS pool —
// without it, XFS-on-zvol never returns deleted space and the zvol grows
// unboundedly (this is what leaked ~858G onto the container-docker zvol).
func mountZvol(zvolName, mountPath string) error {
	return mountZvolWithOptions(zvolName, mountPath, "discard")
}

// mountZvolWithOptions mounts a zvol with optional mount options (e.g.
// "nouuid" for XFS clones). Callers should include "discard" so freed blocks
// are reclaimed (see mountZvol).
func mountZvolWithOptions(zvolName, mountPath, options string) error {
	if err := osMkdirAll(mountPath, 0755); err != nil {
		return fmt.Errorf("failed to create mount point %s: %w", mountPath, err)
	}
	if err := waitForZvolDevice(zvolName); err != nil {
		return err
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
