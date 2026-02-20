package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// SemVer represents a parsed semantic version.
type SemVer struct {
	Major        int    `json:"major"`
	Minor        int    `json:"minor"`
	Patch        int    `json:"patch"`
	PreRelease   string `json:"pre_release,omitempty"`
	IsPreRelease bool   `json:"is_pre_release"`
}

// UpdateInfo describes available update information.
type UpdateInfo struct {
	Available      bool   `json:"available"`
	LatestVersion  string `json:"latest_version"`
	CurrentVersion string `json:"current_version"`
	DMGURL         string `json:"dmg_url,omitempty"`
	VMManifestURL  string `json:"vm_manifest_url,omitempty"`
}

// UpdateProgress reports update download status to the frontend.
type UpdateProgress struct {
	Phase      string  `json:"phase"` // "downloading_app", "installing_app", "downloading_vm", "ready"
	BytesDone  int64   `json:"bytes_done"`
	BytesTotal int64   `json:"bytes_total"`
	Percent    float64 `json:"percent"`
	Speed      string  `json:"speed,omitempty"`
	ETA        string  `json:"eta,omitempty"`
	Error      string  `json:"error,omitempty"`
}

const (
	latestVersionURL = "https://get.helix.ml/latest.txt"
	dmgURLTemplate   = "https://dl.helix.ml/desktop/%s/Helix-for-Mac.dmg"
	vmManifestURLTpl = "https://dl.helix.ml/vm/%s/manifest.json"
)

var semverRegex = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)(?:-(.+))?$`)

// ParseSemVer parses a version string like "1.2.3" or "1.2.3-beta".
// Returns nil if the string is not valid semver.
func ParseSemVer(s string) *SemVer {
	s = strings.TrimSpace(s)

	// Reject empty, "<unknown>", "dev", and SHA1 hashes
	if s == "" || s == "<unknown>" || s == "dev" {
		return nil
	}
	if matched, _ := regexp.MatchString(`^[a-f0-9]{40}$`, s); matched {
		return nil
	}

	m := semverRegex.FindStringSubmatch(s)
	if m == nil {
		return nil
	}

	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	patch, _ := strconv.Atoi(m[3])

	return &SemVer{
		Major:        major,
		Minor:        minor,
		Patch:        patch,
		PreRelease:   m[4],
		IsPreRelease: m[4] != "",
	}
}

// IsNewer returns true if latest is a newer version than current.
// Pre-release latest is never considered an update.
// Pre-release current with same base release latest IS newer.
func IsNewer(current, latest string) bool {
	cur := ParseSemVer(current)
	lat := ParseSemVer(latest)

	if cur == nil || lat == nil {
		return false
	}

	// Never offer a pre-release as an update
	if lat.IsPreRelease {
		return false
	}

	// Compare major.minor.patch
	if cur.Major != lat.Major {
		return cur.Major < lat.Major
	}
	if cur.Minor != lat.Minor {
		return cur.Minor < lat.Minor
	}
	if cur.Patch != lat.Patch {
		return cur.Patch < lat.Patch
	}

	// Same base version: pre-release current → release latest is an upgrade
	if cur.IsPreRelease && !lat.IsPreRelease {
		return true
	}

	return false
}

// Updater handles checking for and applying updates.
type Updater struct {
	mu              sync.Mutex
	info            UpdateInfo
	appCancelFunc   context.CancelFunc // cancel for app update download
	vmCancelFunc    context.CancelFunc // cancel for VM update download
	appCtx          context.Context    // Wails app context for event emission
	vmDownloading   bool               // true while DownloadVMUpdate is running
}

// NewUpdater creates an Updater.
func NewUpdater() *Updater {
	return &Updater{}
}

// SetAppContext sets the Wails app context for event emission.
func (u *Updater) SetAppContext(ctx context.Context) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.appCtx = ctx
}

// isDevMode returns true if this is a dev build that should skip update checks.
func isDevMode() bool {
	if os.Getenv("HELIX_DEV_IMAGE") != "" {
		return true
	}
	return Version == "dev"
}

// CheckForUpdate fetches the latest version from the CDN and compares.
func (u *Updater) CheckForUpdate() (UpdateInfo, error) {
	if isDevMode() {
		return UpdateInfo{CurrentVersion: Version}, nil
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(latestVersionURL)
	if err != nil {
		return UpdateInfo{}, fmt.Errorf("failed to check for updates: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return UpdateInfo{}, fmt.Errorf("update check returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 256))
	if err != nil {
		return UpdateInfo{}, fmt.Errorf("failed to read latest version: %w", err)
	}

	latest := strings.TrimSpace(string(body))

	info := UpdateInfo{
		CurrentVersion: Version,
		LatestVersion:  latest,
		Available:      IsNewer(Version, latest),
	}

	if info.Available {
		info.DMGURL = fmt.Sprintf(dmgURLTemplate, latest)
		info.VMManifestURL = fmt.Sprintf(vmManifestURLTpl, latest)
	}

	u.mu.Lock()
	u.info = info
	u.mu.Unlock()

	return info, nil
}

// GetInfo returns the last known update info.
func (u *Updater) GetInfo() UpdateInfo {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.info
}

// IsVMDownloading returns true if a VM download is currently in progress.
func (u *Updater) IsVMDownloading() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.vmDownloading
}

// Cancel cancels any in-progress download (both app and VM).
func (u *Updater) Cancel() {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.appCancelFunc != nil {
		u.appCancelFunc()
		u.appCancelFunc = nil
	}
	if u.vmCancelFunc != nil {
		u.vmCancelFunc()
		u.vmCancelFunc = nil
	}
}

func (u *Updater) emitAppProgress(p UpdateProgress) {
	u.mu.Lock()
	ctx := u.appCtx
	u.mu.Unlock()
	if ctx != nil {
		wailsRuntime.EventsEmit(ctx, "update:app-progress", p)
	}
}

func (u *Updater) emitVMProgress(p UpdateProgress) {
	u.mu.Lock()
	ctx := u.appCtx
	u.mu.Unlock()
	if ctx != nil {
		wailsRuntime.EventsEmit(ctx, "update:vm-progress", p)
	}
}

// ApplyAppUpdate downloads the new DMG, mounts it, copies the .app over the
// current one, and restarts the app.
func (u *Updater) ApplyAppUpdate(appCtx context.Context) error {
	u.mu.Lock()
	info := u.info
	u.mu.Unlock()

	if !info.Available || info.DMGURL == "" {
		return fmt.Errorf("no update available")
	}

	updatesDir := filepath.Join(getHelixDataDir(), "updates")
	if err := os.MkdirAll(updatesDir, 0755); err != nil {
		return fmt.Errorf("failed to create updates directory: %w", err)
	}

	dmgPath := filepath.Join(updatesDir, "Helix-for-Mac.dmg")

	// Download DMG
	u.emitAppProgress(UpdateProgress{Phase: "downloading_app", Percent: 0})

	ctx, cancel := context.WithCancel(context.Background())
	u.mu.Lock()
	u.appCancelFunc = cancel
	u.mu.Unlock()
	defer cancel()

	if err := u.downloadFile(ctx, info.DMGURL, dmgPath, "downloading_app", u.emitAppProgress); err != nil {
		return fmt.Errorf("failed to download update: %w", err)
	}

	// Mount DMG
	u.emitAppProgress(UpdateProgress{Phase: "installing_app", Percent: 0})

	mountPoint, err := mountDMG(dmgPath)
	if err != nil {
		return fmt.Errorf("failed to mount DMG: %w", err)
	}
	defer unmountDMG(mountPoint)

	// Find .app in mounted volume
	entries, err := os.ReadDir(mountPoint)
	if err != nil {
		return fmt.Errorf("failed to read mounted DMG: %w", err)
	}
	var sourceApp string
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".app" {
			sourceApp = filepath.Join(mountPoint, e.Name())
			break
		}
	}
	if sourceApp == "" {
		return fmt.Errorf("no .app found in DMG")
	}

	// Determine current app bundle path
	currentApp := getAppBundlePath()
	if currentApp == "" {
		return fmt.Errorf("cannot determine current app bundle path (not running from .app?)")
	}

	// Backup current app
	backupPath := currentApp + ".backup"
	os.RemoveAll(backupPath)
	if err := exec.Command("cp", "-a", currentApp, backupPath).Run(); err != nil {
		return fmt.Errorf("failed to backup current app: %w", err)
	}

	// Copy new app over current using ditto (preserves code signatures, xattrs)
	u.emitAppProgress(UpdateProgress{Phase: "installing_app", Percent: 50})
	if err := exec.Command("ditto", sourceApp, currentApp).Run(); err != nil {
		// Restore backup
		log.Printf("Update failed, restoring backup: %v", err)
		exec.Command("rm", "-rf", currentApp).Run()
		exec.Command("mv", backupPath, currentApp).Run()
		return fmt.Errorf("failed to install update: %w", err)
	}

	// Clean up backup and DMG
	os.RemoveAll(backupPath)
	os.Remove(dmgPath)

	u.emitAppProgress(UpdateProgress{Phase: "installing_app", Percent: 100})

	// Restart the app
	log.Printf("Update installed, restarting app...")
	go func() {
		time.Sleep(500 * time.Millisecond)
		exec.Command("open", "-n", currentApp).Start()
		wailsRuntime.Quit(appCtx)
	}()

	return nil
}

// DownloadVMUpdate downloads the new VM disk image to a staging path.
// The old VM keeps running while this happens.
// It first tries to fetch the manifest from the CDN (for app version mismatch),
// then falls back to the bundled manifest (for post-app-update or manual install).
// When force is true, the version check is skipped (used by RedownloadVMImage).
func (u *Updater) DownloadVMUpdate(settings *SettingsManager, downloader *VMDownloader, force bool) error {
	u.mu.Lock()
	if u.vmDownloading {
		u.mu.Unlock()
		return fmt.Errorf("VM download already in progress")
	}
	u.vmDownloading = true
	info := u.info
	u.mu.Unlock()
	defer func() {
		u.mu.Lock()
		u.vmDownloading = false
		u.mu.Unlock()
	}()

	var manifest VMManifest

	// Try CDN manifest first (when an app update reported a newer version)
	if info.VMManifestURL != "" || info.LatestVersion != "" {
		manifestURL := info.VMManifestURL
		if manifestURL == "" {
			manifestURL = fmt.Sprintf(vmManifestURLTpl, info.LatestVersion)
		}

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Get(manifestURL)
		if err == nil {
			if resp.StatusCode == http.StatusOK {
				if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
					log.Printf("Failed to parse CDN VM manifest: %v, falling back to bundled", err)
				}
			} else {
				log.Printf("CDN VM manifest returned HTTP %d, falling back to bundled", resp.StatusCode)
			}
			resp.Body.Close()
		} else {
			log.Printf("Failed to fetch CDN VM manifest: %v, falling back to bundled", err)
		}
	}

	// Fall back to bundled manifest (post-app-update or manual .app replacement)
	if manifest.Version == "" {
		m, err := downloader.LoadManifest()
		if err != nil || m == nil {
			return fmt.Errorf("no VM manifest available (CDN and bundled both failed)")
		}
		manifest = *m
		log.Printf("Using bundled VM manifest v%s", manifest.Version)
	}

	// Check if there's actually a new version (skip when force=true for re-download)
	s := settings.Get()
	if !force && manifest.Version == s.InstalledVMVersion {
		log.Printf("VM already at version %s, no update needed", manifest.Version)
		return nil
	}

	vmDir := filepath.Join(getHelixDataDir(), "vm", "helix-desktop")

	ctx, cancel := context.WithCancel(context.Background())
	u.mu.Lock()
	u.vmCancelFunc = cancel
	u.mu.Unlock()
	defer cancel()

	// Download each file in the manifest to a .staged path
	for _, f := range manifest.Files {
		select {
		case <-ctx.Done():
			return fmt.Errorf("VM update download cancelled")
		default:
		}

		// Determine the final filename (after decompression)
		finalName := f.Name
		if f.Compression != "" && f.DecompressedName != "" {
			finalName = f.DecompressedName
		}

		stagedPath := filepath.Join(vmDir, finalName+".staged")
		downloadURL := fmt.Sprintf("%s/%s/%s", manifest.BaseURL, manifest.Version, f.Name)

		// Download the file
		if f.Compression == "zstd" {
			// Download compressed, then decompress to .staged
			compressedPath := stagedPath + ".zst"
			if err := u.downloadFile(ctx, downloadURL, compressedPath, "downloading_vm", u.emitVMProgress); err != nil {
				return fmt.Errorf("failed to download %s: %w", f.Name, err)
			}
			// Decompress
			emitter := &updateEmitter{emitFn: u.emitVMProgress}
			if err := downloader.decompressZstd(emitter, compressedPath, stagedPath, f); err != nil {
				return fmt.Errorf("failed to decompress %s: %w", f.Name, err)
			}
			os.Remove(compressedPath)
		} else {
			if err := u.downloadFile(ctx, downloadURL, stagedPath, "downloading_vm", u.emitVMProgress); err != nil {
				return fmt.Errorf("failed to download %s: %w", f.Name, err)
			}
		}
	}

	// Save the target version so we know what we staged
	stagedVersionPath := filepath.Join(vmDir, ".staged-version")
	os.WriteFile(stagedVersionPath, []byte(manifest.Version), 0644)

	u.emitVMProgress(UpdateProgress{Phase: "ready", Percent: 100})

	// Emit vm-ready event
	u.mu.Lock()
	ctx2 := u.appCtx
	u.mu.Unlock()
	if ctx2 != nil {
		wailsRuntime.EventsEmit(ctx2, "update:vm-ready")
	}

	return nil
}

// updateEmitter adapts the VMDownloader's emitter interface for update progress.
type updateEmitter struct {
	emitFn func(UpdateProgress)
}

func (e *updateEmitter) EventsEmit(eventName string, data ...interface{}) {
	// We only care about progress events for the UI; the decompressor
	// emits "download:progress" events which we translate.
	if len(data) > 0 {
		if p, ok := data[0].(DownloadProgress); ok {
			e.emitFn(UpdateProgress{
				Phase:      "downloading_vm",
				BytesDone:  p.BytesDone,
				BytesTotal: p.BytesTotal,
				Percent:    p.Percent,
				Speed:      p.Speed,
				ETA:        p.ETA,
			})
		}
	}
}

// ApplyVMUpdate stops the VM, swaps the disk, and starts the VM.
func (u *Updater) ApplyVMUpdate(vm *VMManager, settings *SettingsManager) error {
	vmDir := filepath.Join(getHelixDataDir(), "vm", "helix-desktop")

	// Check staged files exist
	stagedDisk := filepath.Join(vmDir, "disk.qcow2.staged")
	if _, err := os.Stat(stagedDisk); err != nil {
		return fmt.Errorf("no staged VM update found")
	}

	// Read staged version
	stagedVersionPath := filepath.Join(vmDir, ".staged-version")
	versionBytes, err := os.ReadFile(stagedVersionPath)
	if err != nil {
		return fmt.Errorf("failed to read staged version: %w", err)
	}
	stagedVersion := strings.TrimSpace(string(versionBytes))

	// Stop VM if running
	if vm.GetStatus().State == VMStateRunning || vm.GetStatus().State == VMStateStarting {
		log.Println("Stopping VM for update...")
		if err := vm.Stop(); err != nil {
			return fmt.Errorf("failed to stop VM: %w", err)
		}
		for i := 0; i < 60; i++ {
			if vm.GetStatus().State == VMStateStopped {
				break
			}
			time.Sleep(time.Second)
		}
		if vm.GetStatus().State != VMStateStopped {
			return fmt.Errorf("VM did not stop within 60 seconds")
		}
	}

	// Swap disk.qcow2
	currentDisk := filepath.Join(vmDir, "disk.qcow2")
	oldDisk := filepath.Join(vmDir, "disk.qcow2.old")

	// Move current → old
	if _, err := os.Stat(currentDisk); err == nil {
		if err := os.Rename(currentDisk, oldDisk); err != nil {
			return fmt.Errorf("failed to backup current disk: %w", err)
		}
	}

	// Move staged → current
	if err := os.Rename(stagedDisk, currentDisk); err != nil {
		// Restore old disk
		os.Rename(oldDisk, currentDisk)
		return fmt.Errorf("failed to swap disk: %w", err)
	}

	// Clean up any leftover efi_vars files from older versions.
	// EFI vars are now ephemeral (snapshot=on in QEMU), so these are unused.
	for _, stale := range []string{"efi_vars.fd", "efi_vars.fd.staged"} {
		p := filepath.Join(vmDir, stale)
		if _, err := os.Stat(p); err == nil {
			log.Printf("Removing unused %s (EFI vars are now ephemeral)", stale)
			os.Remove(p)
		}
	}

	// Update installed version in settings
	s := settings.Get()
	s.InstalledVMVersion = stagedVersion
	if err := settings.Save(s); err != nil {
		log.Printf("Warning: failed to save installed VM version: %v", err)
	}

	// Clean up
	os.Remove(stagedVersionPath)
	// Delete .old files after successful swap (will delete after VM boots)
	go func() {
		time.Sleep(30 * time.Second)
		os.Remove(oldDisk)
	}()

	log.Printf("VM disk updated to version %s", stagedVersion)
	return nil
}

// IsVMUpdateStaged returns true if a staged VM disk exists.
func IsVMUpdateStaged() bool {
	stagedDisk := filepath.Join(getHelixDataDir(), "vm", "helix-desktop", "disk.qcow2.staged")
	_, err := os.Stat(stagedDisk)
	return err == nil
}

// GetStagedVMVersion returns the version of the staged VM update, or "".
func GetStagedVMVersion() string {
	path := filepath.Join(getHelixDataDir(), "vm", "helix-desktop", ".staged-version")
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// downloadFile downloads a URL to a local path with progress reporting.
func (u *Updater) downloadFile(ctx context.Context, url, destPath, phase string, emitFn func(UpdateProgress)) error {
	tmpPath := destPath + ".tmp"

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := fastHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d downloading %s", resp.StatusCode, url)
	}

	totalSize := resp.ContentLength

	out, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}

	buf := make([]byte, 256*1024)
	var done int64
	lastReport := time.Now()
	lastBytes := int64(0)
	smoothedSpeed := 0.0
	const emaAlpha = 0.15

	for {
		select {
		case <-ctx.Done():
			out.Close()
			os.Remove(tmpPath)
			return fmt.Errorf("download cancelled")
		default:
		}

		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := out.Write(buf[:n]); writeErr != nil {
				out.Close()
				os.Remove(tmpPath)
				return writeErr
			}
			done += int64(n)

			if time.Since(lastReport) > 300*time.Millisecond {
				elapsed := time.Since(lastReport).Seconds()
				instantSpeed := float64(done-lastBytes) / elapsed
				if smoothedSpeed == 0 {
					smoothedSpeed = instantSpeed
				} else {
					smoothedSpeed = emaAlpha*instantSpeed + (1-emaAlpha)*smoothedSpeed
				}

				pct := 0.0
				eta := "--"
				if totalSize > 0 {
					pct = float64(done) / float64(totalSize) * 100
					if smoothedSpeed > 0 {
						eta = formatDuration(float64(totalSize-done) / smoothedSpeed)
					}
				}

				emitFn(UpdateProgress{
					Phase:      phase,
					BytesDone:  done,
					BytesTotal: totalSize,
					Percent:    pct,
					Speed:      formatSpeed(smoothedSpeed),
					ETA:        eta,
				})

				lastReport = time.Now()
				lastBytes = done
			}
		}

		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			out.Close()
			os.Remove(tmpPath)
			return readErr
		}
	}

	out.Close()

	return os.Rename(tmpPath, destPath)
}

// mountDMG mounts a DMG and returns the mount point.
func mountDMG(dmgPath string) (string, error) {
	out, err := exec.Command("hdiutil", "attach", dmgPath, "-nobrowse", "-noautoopen", "-mountrandom", "/tmp").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("hdiutil attach failed: %s: %w", string(out), err)
	}

	// Parse mount point from output — last field of last line
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		fields := strings.Fields(lines[i])
		if len(fields) >= 3 {
			mountPoint := fields[len(fields)-1]
			if strings.HasPrefix(mountPoint, "/") {
				return mountPoint, nil
			}
		}
	}

	return "", fmt.Errorf("could not determine mount point from hdiutil output: %s", string(out))
}

// unmountDMG unmounts a DMG volume.
func unmountDMG(mountPoint string) {
	exec.Command("hdiutil", "detach", mountPoint, "-force").Run()
}
