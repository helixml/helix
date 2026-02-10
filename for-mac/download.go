package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// VMManifest describes the VM images required for download
type VMManifest struct {
	Version string           `json:"version"`
	BaseURL string           `json:"base_url"`
	Files   []VMManifestFile `json:"files"`
}

// VMManifestFile describes a single file in the manifest
type VMManifestFile struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// DownloadProgress reports download status to the frontend
type DownloadProgress struct {
	File       string  `json:"file"`
	BytesDone  int64   `json:"bytes_done"`
	BytesTotal int64   `json:"bytes_total"`
	Percent    float64 `json:"percent"`
	Speed      string  `json:"speed"`
	ETA        string  `json:"eta"`
	Status     string  `json:"status"` // "downloading", "verifying", "complete", "error"
	Error      string  `json:"error,omitempty"`
}

// VMDownloader handles downloading VM images from a CDN
type VMDownloader struct {
	mu       sync.Mutex
	manifest *VMManifest
	progress DownloadProgress
	cancel   chan struct{}
	running  bool
}

// NewVMDownloader creates a new downloader
func NewVMDownloader() *VMDownloader {
	return &VMDownloader{}
}

// LoadManifest loads the vm-manifest.json from the app bundle
func (d *VMDownloader) LoadManifest() (*VMManifest, error) {
	// Try app bundle first
	bundlePath := getAppBundlePath()
	if bundlePath != "" {
		manifestPath := filepath.Join(bundlePath, "Contents", "Resources", "vm", "vm-manifest.json")
		if data, err := os.ReadFile(manifestPath); err == nil {
			var m VMManifest
			if err := json.Unmarshal(data, &m); err != nil {
				return nil, fmt.Errorf("failed to parse vm-manifest.json: %w", err)
			}
			d.manifest = &m
			return &m, nil
		}
	}

	// Try local dev path (for development without app bundle)
	devPath := filepath.Join(getHelixDataDir(), "vm-manifest.json")
	if data, err := os.ReadFile(devPath); err == nil {
		var m VMManifest
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("failed to parse vm-manifest.json: %w", err)
		}
		d.manifest = &m
		return &m, nil
	}

	return nil, fmt.Errorf("vm-manifest.json not found in app bundle or data directory")
}

// CheckFilesExist checks which manifest files already exist at the target location
func (d *VMDownloader) CheckFilesExist() (allExist bool, missing []VMManifestFile, err error) {
	if d.manifest == nil {
		if _, err := d.LoadManifest(); err != nil {
			return false, nil, err
		}
	}

	vmDir := filepath.Join(getHelixDataDir(), "vm", "helix-desktop")
	for _, f := range d.manifest.Files {
		path := filepath.Join(vmDir, f.Name)
		info, err := os.Stat(path)
		if err != nil || info.Size() != f.Size {
			missing = append(missing, f)
		}
	}

	return len(missing) == 0, missing, nil
}

// DownloadAll downloads all missing VM images with progress reporting
func (d *VMDownloader) DownloadAll(ctx interface{ EventsEmit(string, ...interface{}) }) error {
	d.mu.Lock()
	if d.running {
		d.mu.Unlock()
		return fmt.Errorf("download already in progress")
	}
	d.running = true
	d.cancel = make(chan struct{})
	d.mu.Unlock()

	defer func() {
		d.mu.Lock()
		d.running = false
		d.mu.Unlock()
	}()

	if d.manifest == nil {
		if _, err := d.LoadManifest(); err != nil {
			return err
		}
	}

	_, missing, err := d.CheckFilesExist()
	if err != nil {
		return err
	}
	if len(missing) == 0 {
		return nil
	}

	vmDir := filepath.Join(getHelixDataDir(), "vm", "helix-desktop")
	if err := os.MkdirAll(vmDir, 0755); err != nil {
		return fmt.Errorf("failed to create VM directory: %w", err)
	}

	for _, f := range missing {
		select {
		case <-d.cancel:
			return fmt.Errorf("download cancelled")
		default:
		}

		if err := d.downloadFile(ctx, f, vmDir); err != nil {
			d.emitProgress(ctx, DownloadProgress{
				File:   f.Name,
				Status: "error",
				Error:  err.Error(),
			})
			return err
		}
	}

	d.emitProgress(ctx, DownloadProgress{
		Status: "complete",
	})

	return nil
}

// downloadFile downloads a single file with resume support and SHA256 verification
func (d *VMDownloader) downloadFile(ctx interface{ EventsEmit(string, ...interface{}) }, f VMManifestFile, vmDir string) error {
	destPath := filepath.Join(vmDir, f.Name)
	tmpPath := destPath + ".tmp"

	url := fmt.Sprintf("%s/%s/%s", d.manifest.BaseURL, d.manifest.Version, f.Name)

	// Check for partial download to resume
	var startByte int64
	if info, err := os.Stat(tmpPath); err == nil {
		startByte = info.Size()
	}

	log.Printf("Downloading %s from %s (resume at byte %d)", f.Name, url, startByte)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request for %s: %w", f.Name, err)
	}

	if startByte > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", startByte))
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download %s: %w", f.Name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusRequestedRangeNotSatisfiable {
		// File already complete on server side, start fresh
		startByte = 0
		os.Remove(tmpPath)
		req.Header.Del("Range")
		resp.Body.Close()
		resp, err = http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("failed to download %s: %w", f.Name, err)
		}
		defer resp.Body.Close()
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("HTTP %d downloading %s", resp.StatusCode, f.Name)
	}

	// If server doesn't support Range and we had a partial, start fresh
	if startByte > 0 && resp.StatusCode == http.StatusOK {
		startByte = 0
		os.Remove(tmpPath)
	}

	flags := os.O_CREATE | os.O_WRONLY
	if startByte > 0 {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
	}

	outFile, err := os.OpenFile(tmpPath, flags, 0644)
	if err != nil {
		return fmt.Errorf("failed to open tmp file for %s: %w", f.Name, err)
	}
	defer outFile.Close()

	// Track progress
	bytesDone := startByte
	lastReport := time.Now()
	lastBytes := startByte
	buf := make([]byte, 256*1024) // 256KB buffer

	for {
		select {
		case <-d.cancel:
			return fmt.Errorf("download cancelled")
		default:
		}

		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := outFile.Write(buf[:n]); writeErr != nil {
				return fmt.Errorf("failed to write %s: %w", f.Name, writeErr)
			}
			bytesDone += int64(n)

			// Report progress every 250ms
			if time.Since(lastReport) > 250*time.Millisecond {
				elapsed := time.Since(lastReport).Seconds()
				speed := float64(bytesDone-lastBytes) / elapsed
				remaining := float64(f.Size-bytesDone) / speed

				pct := float64(bytesDone) / float64(f.Size) * 100
				if pct > 100 {
					pct = 100
				}

				d.emitProgress(ctx, DownloadProgress{
					File:       f.Name,
					BytesDone:  bytesDone,
					BytesTotal: f.Size,
					Percent:    pct,
					Speed:      formatSpeed(speed),
					ETA:        formatDuration(remaining),
					Status:     "downloading",
				})

				lastReport = time.Now()
				lastBytes = bytesDone
			}
		}

		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return fmt.Errorf("failed reading %s: %w", f.Name, readErr)
		}
	}

	outFile.Close()

	// Verify SHA256
	d.emitProgress(ctx, DownloadProgress{
		File:       f.Name,
		BytesDone:  f.Size,
		BytesTotal: f.Size,
		Percent:    100,
		Status:     "verifying",
	})

	hash, err := sha256File(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to hash %s: %w", f.Name, err)
	}

	if hash != f.SHA256 {
		os.Remove(tmpPath)
		return fmt.Errorf("SHA256 mismatch for %s: expected %s, got %s", f.Name, f.SHA256, hash)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, destPath); err != nil {
		return fmt.Errorf("failed to move %s into place: %w", f.Name, err)
	}

	log.Printf("Downloaded and verified %s (%d bytes)", f.Name, f.Size)
	return nil
}

// Cancel stops an in-progress download
func (d *VMDownloader) Cancel() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.running && d.cancel != nil {
		close(d.cancel)
	}
}

// GetProgress returns the current download progress
func (d *VMDownloader) GetProgress() DownloadProgress {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.progress
}

// IsRunning returns whether a download is in progress
func (d *VMDownloader) IsRunning() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.running
}

func (d *VMDownloader) emitProgress(ctx interface{ EventsEmit(string, ...interface{}) }, p DownloadProgress) {
	d.mu.Lock()
	d.progress = p
	d.mu.Unlock()

	ctx.EventsEmit("download:progress", p)
}

// sha256File computes the SHA256 hash of a file
func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// formatSpeed formats bytes/sec as human-readable
func formatSpeed(bytesPerSec float64) string {
	if bytesPerSec >= 1024*1024*1024 {
		return fmt.Sprintf("%.1f GB/s", bytesPerSec/(1024*1024*1024))
	}
	if bytesPerSec >= 1024*1024 {
		return fmt.Sprintf("%.1f MB/s", bytesPerSec/(1024*1024))
	}
	if bytesPerSec >= 1024 {
		return fmt.Sprintf("%.0f KB/s", bytesPerSec/1024)
	}
	return fmt.Sprintf("%.0f B/s", bytesPerSec)
}

// formatDuration formats seconds as human-readable ETA
func formatDuration(seconds float64) string {
	if seconds < 0 || seconds > 86400 {
		return "--"
	}
	d := time.Duration(seconds * float64(time.Second))
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
}

// getAppBundlePath returns the path to the running .app bundle, if any.
func getAppBundlePath() string {
	execPath, err := os.Executable()
	if err != nil {
		return ""
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return ""
	}
	macosDir := filepath.Dir(execPath)
	if filepath.Base(macosDir) != "MacOS" {
		return ""
	}
	contentsDir := filepath.Dir(macosDir)
	if filepath.Base(contentsDir) != "Contents" {
		return ""
	}
	appDir := filepath.Dir(contentsDir)
	if filepath.Ext(appDir) != ".app" {
		return ""
	}
	return appDir
}
