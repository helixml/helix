package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/klauspost/compress/zstd"
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

	// Compression fields (optional — if set, file is compressed on CDN)
	Compression      string `json:"compression,omitempty"`       // "zstd" or empty
	DecompressedName string `json:"decompressed_name,omitempty"` // final filename after decompression
	DecompressedSize int64  `json:"decompressed_size,omitempty"` // size after decompression
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

const (
	// Number of parallel HTTP connections per file download.
	// 16 connections saturates a gigabit link from most CDNs.
	downloadConcurrency = 16

	// Read buffer size per goroutine (1MB for throughput)
	chunkReadBuffer = 1024 * 1024

	// Fixed chunk size for parallel downloads. Small chunks minimize wasted
	// bandwidth when a connection drops — at most 64 MB needs re-downloading.
	downloadChunkSize = 64 * 1024 * 1024
)

// fastHTTPClient is tuned for download throughput on potentially flaky
// connections. Short timeouts ensure we detect dead connections quickly
// (e.g. cellular network switch) and retry on a fresh connection.
var fastHTTPClient = &http.Client{
	// No Client.Timeout — it covers the entire request including body reads,
	// which would kill slow-but-progressing chunk downloads. We rely on
	// DialTimeout, ResponseHeaderTimeout, and TCP KeepAlive instead.
	Transport: &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,  // fail fast on unreachable hosts
			KeepAlive: 15 * time.Second, // detect dead connections sooner
		}).DialContext,
		MaxIdleConns:          downloadConcurrency + 4,
		MaxIdleConnsPerHost:   downloadConcurrency + 4,
		IdleConnTimeout:       30 * time.Second, // don't hold stale connections
		ResponseHeaderTimeout: 10 * time.Second, // server must respond within 10s
		WriteBufferSize:       256 * 1024,
		ReadBufferSize:        1024 * 1024, // 1MB TCP read buffer
		DisableCompression:  true,        // Don't waste CPU decompressing binary blobs
	},
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

	// Try build output directory (dev mode — wails dev runs from for-mac/)
	devBuildPath := filepath.Join("build", "bin", "Helix.app", "Contents", "Resources", "vm", "vm-manifest.json")
	if data, err := os.ReadFile(devBuildPath); err == nil {
		var m VMManifest
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("failed to parse vm-manifest.json: %w", err)
		}
		d.manifest = &m
		return &m, nil
	}

	// Try local data directory (fallback for development)
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
		// For compressed files, check the decompressed output
		checkName := f.Name
		checkSize := f.Size
		if f.Compression != "" && f.DecompressedName != "" {
			checkName = f.DecompressedName
			checkSize = f.DecompressedSize
		}
		path := filepath.Join(vmDir, checkName)
		info, err := os.Stat(path)
		if err != nil || info.Size() != checkSize {
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

	// Disk space preflight check: for compressed files we need both the
	// compressed download AND the decompressed output temporarily on disk.
	var totalRequired int64
	for _, f := range missing {
		totalRequired += f.Size
		if f.Compression != "" && f.DecompressedSize > 0 {
			totalRequired += f.DecompressedSize
		}
	}
	headroom := int64(10 * 1024 * 1024 * 1024) // 10 GB
	if err := checkDiskSpace(vmDir, uint64(totalRequired+headroom)); err != nil {
		return err
	}

	const maxFileRetries = 3

	for _, f := range missing {
		select {
		case <-d.cancel:
			return fmt.Errorf("download cancelled")
		default:
		}

		var fileErr error
		for fileAttempt := 0; fileAttempt < maxFileRetries; fileAttempt++ {
			if fileAttempt > 0 {
				log.Printf("Retrying file %s (attempt %d/%d): %v", f.Name, fileAttempt+1, maxFileRetries, fileErr)
				d.emitProgress(ctx, DownloadProgress{
					File:   f.Name,
					Status: "retrying",
					Error:  fileErr.Error(),
				})
				select {
				case <-d.cancel:
					return fmt.Errorf("download cancelled")
				case <-time.After(5 * time.Second):
				}
			}
			fileErr = d.downloadFileParallel(ctx, f, vmDir)
			if fileErr == nil {
				break
			}
		}
		if fileErr != nil {
			d.emitProgress(ctx, DownloadProgress{
				File:   f.Name,
				Status: "error",
				Error:  fileErr.Error(),
			})
			return fileErr
		}

		// Decompress if needed
		if f.Compression == "zstd" && f.DecompressedName != "" {
			compressedPath := filepath.Join(vmDir, f.Name)
			decompressedPath := filepath.Join(vmDir, f.DecompressedName)

			d.emitProgress(ctx, DownloadProgress{
				File:       f.DecompressedName,
				BytesDone:  0,
				BytesTotal: f.DecompressedSize,
				Status:     "decompressing",
			})

			if err := d.decompressZstd(ctx, compressedPath, decompressedPath, f); err != nil {
				d.emitProgress(ctx, DownloadProgress{
					File:   f.DecompressedName,
					Status: "error",
					Error:  err.Error(),
				})
				return err
			}

			// Remove the compressed file to free disk space
			os.Remove(compressedPath)
		}
	}

	d.emitProgress(ctx, DownloadProgress{
		Status: "complete",
	})

	return nil
}

// downloadFileParallel downloads a single file using N parallel HTTP Range
// requests. Each goroutine fetches a chunk and writes it at the correct
// offset using WriteAt. Progress is tracked atomically across all goroutines.
func (d *VMDownloader) downloadFileParallel(ctx interface{ EventsEmit(string, ...interface{}) }, f VMManifestFile, vmDir string) error {
	destPath := filepath.Join(vmDir, f.Name)
	tmpPath := destPath + ".tmp"
	url := fmt.Sprintf("%s/%s/%s", d.manifest.BaseURL, d.manifest.Version, f.Name)

	log.Printf("Downloading %s from %s (%d parallel connections)", f.Name, url, downloadConcurrency)

	// Verify server supports Range requests with a HEAD
	headReq, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create HEAD request for %s: %w", f.Name, err)
	}
	headResp, err := fastHTTPClient.Do(headReq)
	if err != nil {
		return fmt.Errorf("HEAD request failed for %s: %w", f.Name, err)
	}
	headResp.Body.Close()

	supportsRange := headResp.Header.Get("Accept-Ranges") == "bytes"

	// Always trust the server's Content-Length over the manifest.
	// The manifest size can be stale if the image was rebuilt without updating the manifest.
	if headResp.ContentLength > 0 {
		if headResp.ContentLength != f.Size {
			log.Printf("WARNING: %s server Content-Length %d != manifest size %d — using server size",
				f.Name, headResp.ContentLength, f.Size)
		}
		f.Size = headResp.ContentLength
	}

	if !supportsRange || f.Size < 10*1024*1024 {
		// Fall back to single-connection for small files or no Range support
		log.Printf("Using single connection for %s (Range support: %v, size: %d)", f.Name, supportsRange, f.Size)
		return d.downloadFileSingle(ctx, f, vmDir)
	}

	// Build chunk list — fixed-size chunks, decoupled from concurrency.
	// Small chunks (64 MB) minimize wasted bandwidth on flaky connections:
	// a dropped connection loses at most one chunk worth of progress.
	type chunkInfo struct {
		Index int   `json:"index"`
		Start int64 `json:"start"`
		End   int64 `json:"end"`
	}
	numChunks := int((f.Size + downloadChunkSize - 1) / downloadChunkSize)
	allChunks := make([]chunkInfo, numChunks)
	for i := 0; i < numChunks; i++ {
		allChunks[i] = chunkInfo{
			Index: i,
			Start: int64(i) * downloadChunkSize,
			End:   int64(i)*downloadChunkSize + downloadChunkSize - 1,
		}
		if allChunks[i].End >= f.Size {
			allChunks[i].End = f.Size - 1
		}
	}

	// Resume support: check for existing .tmp file and .chunks progress.
	// Progress file includes chunk_size so we can detect stale progress
	// from a different chunk size (e.g. after code update).
	type chunkProgress struct {
		ChunkSize int64 `json:"chunk_size"`
		Completed []int `json:"completed"`
	}
	progressPath := tmpPath + ".chunks"
	completedChunks := map[int]bool{}
	var resumedBytes int64

	if info, err := os.Stat(tmpPath); err == nil && info.Size() == f.Size {
		// .tmp file exists with correct pre-allocated size — check for chunk progress
		if data, err := os.ReadFile(progressPath); err == nil {
			var progress chunkProgress
			if json.Unmarshal(data, &progress) == nil && progress.ChunkSize == downloadChunkSize {
				for _, idx := range progress.Completed {
					if idx >= 0 && idx < numChunks {
						completedChunks[idx] = true
						c := allChunks[idx]
						resumedBytes += c.End - c.Start + 1
					}
				}
				if len(completedChunks) > 0 {
					log.Printf("Resuming %s: %d/%d chunks already complete (%.1f GB)",
						f.Name, len(completedChunks), numChunks,
						float64(resumedBytes)/(1024*1024*1024))
				}
			} else {
				// Old format or different chunk size — discard stale progress
				log.Printf("Chunk size changed for %s — discarding old progress", f.Name)
				os.Remove(progressPath)
			}
		}
	} else if err == nil {
		// .tmp exists but wrong size (file size changed) — start fresh
		log.Printf("Stale .tmp for %s (size %d, expected %d) — starting fresh", f.Name, info.Size(), f.Size)
		os.Remove(tmpPath)
		os.Remove(progressPath)
	}

	if len(completedChunks) == numChunks {
		// All chunks done — skip straight to verification
		log.Printf("All chunks complete for %s, verifying...", f.Name)
		goto verify
	}

	{
		// Open or create the output file
		var outFile *os.File
		if len(completedChunks) > 0 {
			// Resume: open existing pre-allocated file
			outFile, err = os.OpenFile(tmpPath, os.O_RDWR, 0644)
		} else {
			// Fresh start: create and pre-allocate
			outFile, err = os.OpenFile(tmpPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
			if err == nil {
				err = outFile.Truncate(f.Size)
			}
		}
		if err != nil {
			if outFile != nil {
				outFile.Close()
			}
			return fmt.Errorf("failed to open tmp file for %s: %w", f.Name, err)
		}

		var totalDone atomic.Int64
		totalDone.Add(resumedBytes)
		var chunkErr atomic.Value

		// Track completed chunks for progress file (thread-safe)
		var completedMu sync.Mutex
		saveProgress := func() {
			completedMu.Lock()
			var indices []int
			for idx := range completedChunks {
				indices = append(indices, idx)
			}
			completedMu.Unlock()
			progress := chunkProgress{ChunkSize: downloadChunkSize, Completed: indices}
			if data, err := json.Marshal(progress); err == nil {
				os.WriteFile(progressPath, data, 0644)
			}
		}

		var wg sync.WaitGroup

		// Progress reporter goroutine
		stopProgress := make(chan struct{})
		go func() {
			startTime := time.Now()
			lastBytes := resumedBytes
			lastReport := time.Now()
			smoothedSpeed := 0.0
			const emaAlpha = 0.2

			ticker := time.NewTicker(300 * time.Millisecond)
			defer ticker.Stop()

			for {
				select {
				case <-d.cancel:
					return
				case <-stopProgress:
					return
				case <-ticker.C:
					done := totalDone.Load()
					now := time.Now()
					elapsed := now.Sub(lastReport).Seconds()
					if elapsed < 0.1 {
						continue
					}

					instantSpeed := float64(done-lastBytes) / elapsed
					if smoothedSpeed == 0 {
						totalElapsed := now.Sub(startTime).Seconds()
						if totalElapsed > 0 {
							smoothedSpeed = float64(done-resumedBytes) / totalElapsed
						}
					} else {
						smoothedSpeed = emaAlpha*instantSpeed + (1-emaAlpha)*smoothedSpeed
					}

					remaining := 0.0
					if smoothedSpeed > 0 {
						remaining = float64(f.Size-done) / smoothedSpeed
					}

					pct := float64(done) / float64(f.Size) * 100
					if pct > 100 {
						pct = 100
					}

					d.emitProgress(ctx, DownloadProgress{
						File:       f.Name,
						BytesDone:  done,
						BytesTotal: f.Size,
						Percent:    pct,
						Speed:      formatSpeed(smoothedSpeed),
						ETA:        formatDuration(remaining),
						Status:     "downloading",
					})

					lastReport = now
					lastBytes = done

					if done >= f.Size {
						return
					}
				}
			}
		}()

		// Feed pending chunks into a channel; worker pool pulls from it.
		chunkCh := make(chan chunkInfo, numChunks)
		for _, c := range allChunks {
			if !completedChunks[c.Index] {
				chunkCh <- c
			}
		}
		close(chunkCh)

		// Launch worker pool (16 workers pulling from shared queue)
		for w := 0; w < downloadConcurrency; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for chunk := range chunkCh {
					// Stop taking new chunks if another worker already failed
					if chunkErr.Load() != nil {
						return
					}
					if err := d.downloadChunk(outFile, url, chunk.Start, chunk.End, &totalDone); err != nil {
						chunkErr.CompareAndSwap(nil, err)
						return
					}
					completedMu.Lock()
					completedChunks[chunk.Index] = true
					completedMu.Unlock()
					saveProgress()
				}
			}()
		}

		wg.Wait()
		close(stopProgress)

		// Emit "finalizing" status while the OS flushes dirty pages to disk.
		// Close on an 8 GB file can take 30-60s as the page cache drains.
		d.emitProgress(ctx, DownloadProgress{
			File:       f.Name,
			BytesDone:  f.Size,
			BytesTotal: f.Size,
			Percent:    100,
			Status:     "finalizing",
		})
		outFile.Close()

		// Check for chunk errors (don't delete .tmp — resume will pick up where we left off)
		if errVal := chunkErr.Load(); errVal != nil {
			return errVal.(error)
		}
	}

verify:
	// Verify SHA256 with progress (hashing 8 GB takes 30-60s)
	hash, err := d.sha256FileWithProgress(ctx, tmpPath, f)
	if err != nil {
		return fmt.Errorf("failed to hash %s: %w", f.Name, err)
	}

	if hash != f.SHA256 {
		// SHA256 mismatch — delete everything so next attempt starts fresh
		os.Remove(tmpPath)
		os.Remove(progressPath)
		return fmt.Errorf("SHA256 mismatch for %s: expected %s, got %s", f.Name, f.SHA256, hash)
	}

	// Success — clean up progress file and atomic rename
	os.Remove(progressPath)
	if err := os.Rename(tmpPath, destPath); err != nil {
		return fmt.Errorf("failed to move %s into place: %w", f.Name, err)
	}

	log.Printf("Downloaded and verified %s (%d bytes, %d connections)", f.Name, f.Size, downloadConcurrency)
	return nil
}

const maxChunkRetries = 20
const maxChunkBackoff = 30 * time.Second

// downloadStallTimeout is the maximum time to wait for any data on a chunk
// download before aborting and retrying. This detects dead TCP connections
// after network switches (e.g. WiFi → WiFi, cellular → WiFi) where the old
// socket hangs in a blocked Read until TCP keepalive times out (~2 min on macOS).
const downloadStallTimeout = 30 * time.Second

// downloadChunk downloads a byte range and writes it to the file at the
// correct offset. Tracks bytes downloaded via the atomic counter.
// Retries on transient errors (network failures, 416/5xx from CDN edges).
func (d *VMDownloader) downloadChunk(outFile *os.File, url string, start, end int64, totalDone *atomic.Int64) error {
	var lastErr error
	for attempt := 0; attempt < maxChunkRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			if backoff > maxChunkBackoff {
				backoff = maxChunkBackoff
			}
			log.Printf("Retrying chunk %d-%d (attempt %d/%d, backoff %v): %v",
				start, end, attempt+1, maxChunkRetries, backoff, lastErr)
			select {
			case <-d.cancel:
				return fmt.Errorf("download cancelled")
			case <-time.After(backoff):
			}
		}

		lastErr = d.downloadChunkOnce(outFile, url, start, end, totalDone)
		if lastErr == nil {
			return nil
		}
	}
	return fmt.Errorf("chunk %d-%d failed after %d attempts: %w", start, end, maxChunkRetries, lastErr)
}

func (d *VMDownloader) downloadChunkOnce(outFile *os.File, url string, start, end int64, totalDone *atomic.Int64) error {
	// Context with stall detection: if no bytes arrive for 30s, cancel the
	// request. This aborts the blocked Read and forces a retry on a fresh
	// TCP connection — critical for surviving WiFi/cellular network switches
	// where the old socket is dead but the OS hasn't noticed yet.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Also cancel if the user cancels the download globally.
	go func() {
		select {
		case <-d.cancel:
			cancel()
		case <-ctx.Done():
		}
	}()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))

	resp, err := fastHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("chunk %d-%d: %w", start, end, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("chunk %d-%d: expected 206, got %d", start, end, resp.StatusCode)
	}

	buf := make([]byte, chunkReadBuffer)
	offset := start

	// Stall timer: fires after 30s of no data, cancelling the context and
	// aborting the blocked Read. Reset on every successful read.
	stallTimer := time.AfterFunc(downloadStallTimeout, func() {
		log.Printf("Chunk %d-%d stalled for %v with no data, aborting", start, end, downloadStallTimeout)
		cancel()
	})
	defer stallTimer.Stop()

	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			stallTimer.Reset(downloadStallTimeout)
			if _, writeErr := outFile.WriteAt(buf[:n], offset); writeErr != nil {
				return fmt.Errorf("chunk write at %d: %w", offset, writeErr)
			}
			offset += int64(n)
			totalDone.Add(int64(n))
		}

		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return fmt.Errorf("chunk %d-%d read: %w", start, end, readErr)
		}
	}

	return nil
}

// downloadFileSingle downloads a file with a single connection (fallback for
// small files or servers that don't support Range requests).
func (d *VMDownloader) downloadFileSingle(ctx interface{ EventsEmit(string, ...interface{}) }, f VMManifestFile, vmDir string) error {
	destPath := filepath.Join(vmDir, f.Name)
	tmpPath := destPath + ".tmp"
	url := fmt.Sprintf("%s/%s/%s", d.manifest.BaseURL, d.manifest.Version, f.Name)

	log.Printf("Downloading %s from %s (single connection)", f.Name, url)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request for %s: %w", f.Name, err)
	}

	resp, err := fastHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download %s: %w", f.Name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d downloading %s", resp.StatusCode, f.Name)
	}

	outFile, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to open tmp file for %s: %w", f.Name, err)
	}
	defer outFile.Close()

	bytesDone := int64(0)
	lastReport := time.Now()
	lastBytes := int64(0)
	smoothedSpeed := 0.0
	const emaAlpha = 0.15
	buf := make([]byte, chunkReadBuffer)

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

			if time.Since(lastReport) > 500*time.Millisecond {
				elapsed := time.Since(lastReport).Seconds()
				instantSpeed := float64(bytesDone-lastBytes) / elapsed

				if smoothedSpeed == 0 {
					smoothedSpeed = instantSpeed
				} else {
					smoothedSpeed = emaAlpha*instantSpeed + (1-emaAlpha)*smoothedSpeed
				}

				remaining := float64(f.Size-bytesDone) / smoothedSpeed

				pct := float64(bytesDone) / float64(f.Size) * 100
				if pct > 100 {
					pct = 100
				}

				d.emitProgress(ctx, DownloadProgress{
					File:       f.Name,
					BytesDone:  bytesDone,
					BytesTotal: f.Size,
					Percent:    pct,
					Speed:      formatSpeed(smoothedSpeed),
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

	if err := os.Rename(tmpPath, destPath); err != nil {
		return fmt.Errorf("failed to move %s into place: %w", f.Name, err)
	}

	log.Printf("Downloaded and verified %s (%d bytes)", f.Name, f.Size)
	return nil
}

// decompressZstd decompresses a .zst file with progress reporting.
func (d *VMDownloader) decompressZstd(ctx interface{ EventsEmit(string, ...interface{}) }, srcPath, dstPath string, f VMManifestFile) error {
	log.Printf("Decompressing %s → %s (zstd)", srcPath, dstPath)

	src, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("failed to open compressed file: %w", err)
	}
	defer src.Close()

	decoder, err := zstd.NewReader(src)
	if err != nil {
		return fmt.Errorf("failed to create zstd decoder: %w", err)
	}
	defer decoder.Close()

	tmpPath := dstPath + ".decomp.tmp"
	dst, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to create decompressed file: %w", err)
	}

	buf := make([]byte, 4*1024*1024) // 4 MB buffer for throughput
	var written int64
	lastReport := time.Now()

	for {
		select {
		case <-d.cancel:
			dst.Close()
			os.Remove(tmpPath)
			return fmt.Errorf("decompression cancelled")
		default:
		}

		n, readErr := decoder.Read(buf)
		if n > 0 {
			if _, writeErr := dst.Write(buf[:n]); writeErr != nil {
				dst.Close()
				os.Remove(tmpPath)
				return fmt.Errorf("failed to write decompressed data: %w", writeErr)
			}
			written += int64(n)

			if time.Since(lastReport) > 500*time.Millisecond {
				pct := float64(written) / float64(f.DecompressedSize) * 100
				if pct > 100 {
					pct = 100
				}
				d.emitProgress(ctx, DownloadProgress{
					File:       f.DecompressedName,
					BytesDone:  written,
					BytesTotal: f.DecompressedSize,
					Percent:    pct,
					Status:     "decompressing",
				})
				lastReport = time.Now()
			}
		}

		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			dst.Close()
			os.Remove(tmpPath)
			return fmt.Errorf("zstd decompression error: %w", readErr)
		}
	}

	dst.Close()

	// Verify decompressed size
	info, err := os.Stat(tmpPath)
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to stat decompressed file: %w", err)
	}
	if f.DecompressedSize > 0 && info.Size() != f.DecompressedSize {
		os.Remove(tmpPath)
		return fmt.Errorf("decompressed size mismatch: expected %d, got %d", f.DecompressedSize, info.Size())
	}

	// Atomic rename
	if err := os.Rename(tmpPath, dstPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to move decompressed file into place: %w", err)
	}

	log.Printf("Decompressed %s: %d bytes", dstPath, written)
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

// sha256FileWithProgress hashes a file while emitting "verifying" progress.
func (d *VMDownloader) sha256FileWithProgress(ctx interface{ EventsEmit(string, ...interface{}) }, path string, f VMManifestFile) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	h := sha256.New()
	buf := make([]byte, 4*1024*1024)
	var hashed int64
	lastReport := time.Now()

	for {
		n, readErr := file.Read(buf)
		if n > 0 {
			h.Write(buf[:n])
			hashed += int64(n)

			if time.Since(lastReport) > 300*time.Millisecond {
				pct := float64(hashed) / float64(f.Size) * 100
				if pct > 100 {
					pct = 100
				}
				d.emitProgress(ctx, DownloadProgress{
					File:       f.Name,
					BytesDone:  hashed,
					BytesTotal: f.Size,
					Percent:    pct,
					Status:     "verifying",
				})
				lastReport = time.Now()
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return "", readErr
		}
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

// checkDiskSpace verifies that the directory's filesystem has enough free space.
// Returns an error if available space is less than requiredBytes.
func checkDiskSpace(path string, requiredBytes uint64) error {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return fmt.Errorf("failed to check disk space: %w", err)
	}
	available := stat.Bavail * uint64(stat.Bsize)
	if available < requiredBytes {
		return fmt.Errorf("insufficient disk space: %s available, need %s. Free up space and try again",
			humanizeBytes(available), humanizeBytes(requiredBytes))
	}
	// Warn (but don't fail) if total disk < 256 GB
	totalBytes := stat.Blocks * uint64(stat.Bsize)
	if totalBytes < 256*1024*1024*1024 {
		log.Printf("WARNING: Total disk is %s (recommended: 256 GB+)", humanizeBytes(totalBytes))
	}
	return nil
}

// humanizeBytes formats a byte count as a human-readable string
func humanizeBytes(b uint64) string {
	if b >= 1024*1024*1024 {
		return fmt.Sprintf("%.1f GB", float64(b)/(1024*1024*1024))
	}
	if b >= 1024*1024 {
		return fmt.Sprintf("%.0f MB", float64(b)/(1024*1024))
	}
	return fmt.Sprintf("%.0f KB", float64(b)/1024)
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
