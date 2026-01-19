package spectask

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"
)

func newBenchmarkCommand() *cobra.Command {
	var duration int
	var width, height, fps, bitrate int
	var skipVkcube bool
	var videoMode string
	var outputFile string

	cmd := &cobra.Command{
		Use:   "benchmark <session-id>",
		Short: "Benchmark video streaming performance with GPU stress test",
		Long: `Measures video streaming FPS over the WebSocket connection.

For accurate results, you should run vkcube (Vulkan cube demo) inside the session
to stress the GPU and generate continuous frame updates.

Before running benchmark:
  1. Open a terminal in the desktop session
  2. Run: vkcube
  3. Then run this benchmark

The FPS reported is the actual frames received by the client, including any
network, encoding, or capture bottlenecks.

Video modes:
  - shm:      Shared memory path (default, most compatible, 1-2 CPU copies)
  - native:   Native GStreamer DMA-BUF (requires GStreamer 1.24+, fewer copies)
  - zerocopy: pipewirezerocopysrc plugin (true zero-copy, requires plugin)

Examples:
  helix spectask benchmark ses_01xxx                        # Run 10 second benchmark (default)
  helix spectask benchmark ses_01xxx --duration 30          # Run 30 second benchmark
  helix spectask benchmark ses_01xxx --fps 120              # Test 120fps capability
  helix spectask benchmark ses_01xxx --skip-vkcube          # Skip vkcube check (static screen)
  helix spectask benchmark ses_01xxx --video-mode zerocopy  # Benchmark zero-copy mode
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sessionID := args[0]
			apiURL := getAPIURL()
			token := getToken()

			return runBenchmark(apiURL, token, sessionID, duration, width, height, fps, bitrate, skipVkcube, videoMode, outputFile)
		},
	}

	cmd.Flags().IntVarP(&duration, "duration", "d", 10, "Benchmark duration in seconds")
	cmd.Flags().IntVar(&width, "width", 3840, "Video stream width in pixels (default: 4K)")
	cmd.Flags().IntVar(&height, "height", 2160, "Video stream height in pixels (default: 4K)")
	cmd.Flags().IntVar(&fps, "fps", 60, "Target frames per second")
	cmd.Flags().IntVar(&bitrate, "bitrate", 30000, "Video bitrate in kbps (default: 30Mbps for 4K)")
	cmd.Flags().BoolVar(&skipVkcube, "skip-vkcube", false, "Skip vkcube check (for static screen testing)")
	cmd.Flags().StringVar(&videoMode, "video-mode", "", "Video capture mode: shm, native, or zerocopy (default: container env)")
	cmd.Flags().StringVarP(&outputFile, "output", "o", "", "Write raw H.264 video frames to file")

	return cmd
}

func runBenchmark(apiURL, token, sessionID string, duration, width, height, fps, bitrate int, skipVkcube bool, videoMode, outputFile string) error {
	fmt.Printf("🚀 Video Streaming Benchmark\n")
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("Session:    %s\n", sessionID)
	fmt.Printf("Duration:   %d seconds\n", duration)
	fmt.Printf("Resolution: %dx%d @ %dfps target\n", width, height, fps)
	fmt.Printf("Bitrate:    %d kbps\n", bitrate)
	if videoMode != "" {
		fmt.Printf("Video Mode: %s\n", videoMode)
	}
	if outputFile != "" {
		fmt.Printf("Output:     %s\n", outputFile)
	}
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	// Open output file if specified
	var outFile *os.File
	if outputFile != "" {
		var err error
		outFile, err = os.Create(outputFile)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
		defer outFile.Close()
	}

	// Start vkcube for GPU stress test (generates continuous frame updates)
	if !skipVkcube {
		fmt.Printf("🎮 Starting vkcube (GPU stress test)...\n")
		if err := startVkcube(apiURL, token, sessionID); err != nil {
			fmt.Printf("⚠️  Warning: Failed to start vkcube: %v\n", err)
			fmt.Printf("   You may need to start it manually for accurate FPS testing\n")
		} else {
			fmt.Printf("✅ vkcube started\n")
		}
		// Give vkcube a moment to start rendering
		time.Sleep(500 * time.Millisecond)
	} else {
		fmt.Printf("⏭️  Skipping vkcube (--skip-vkcube flag set)\n")
	}

	// Step 2: Connect to WebSocket stream
	fmt.Printf("\n📡 Connecting to video stream...\n")

	wsURL := strings.Replace(apiURL, "http://", "ws://", 1)
	wsURL = strings.Replace(wsURL, "https://", "wss://", 1)
	streamURL := fmt.Sprintf("%s/api/v1/external-agents/%s/ws/stream", wsURL, url.QueryEscape(sessionID))

	header := http.Header{}
	header.Set("Authorization", "Bearer "+token)

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, resp, err := dialer.Dial(streamURL, header)
	if err != nil {
		if resp != nil {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("WebSocket connection failed: %w - %s", err, string(body))
		}
		return fmt.Errorf("WebSocket connection failed: %w", err)
	}
	defer conn.Close()
	fmt.Printf("✅ Connected\n")

	// Send init message
	initMessage := map[string]interface{}{
		"type":                    "init",
		"session_id":              sessionID,
		"width":                   width,
		"height":                  height,
		"fps":                     fps,
		"bitrate":                 bitrate,
		"packet_size":             1024,
		"play_audio_local":        false,
		"video_supported_formats": 1, // H264
	}
	if videoMode != "" {
		initMessage["video_mode"] = videoMode
	}
	initJSON, _ := json.Marshal(initMessage)
	if err := conn.WriteMessage(websocket.TextMessage, initJSON); err != nil {
		return fmt.Errorf("failed to send init: %w", err)
	}
	fmt.Printf("✅ Stream initialized\n\n")

	// Step 3: Collect statistics
	fmt.Printf("📊 Running benchmark for %d seconds...\n", duration)
	fmt.Printf("───────────────────────────────────────────────────────────────────────────\n")

	stats := &benchmarkStats{
		startTime: time.Now(),
		fpsBuckets: make([]int, duration), // One bucket per second
	}

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	done := make(chan struct{})
	var lastError error

	// Message reading goroutine
	go func() {
		defer close(done)
		for {
			msgType, data, err := conn.ReadMessage()
			if err != nil {
				if !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
					lastError = err
				}
				return
			}

			stats.mu.Lock()
			if msgType == websocket.BinaryMessage && len(data) > 0 {
				wsMsgType := data[0]
				if wsMsgType == WsMsgVideoFrame {
					now := time.Now()
					stats.totalFrames++
					stats.totalBytes += int64(len(data))

					// Jitter tracking - measure inter-frame arrival time
					if !stats.lastFrameTime.IsZero() {
						intervalMs := now.Sub(stats.lastFrameTime).Milliseconds()
						stats.totalIntervalMs += intervalMs
						stats.intervalCount++
						if stats.minIntervalMs == 0 || intervalMs < stats.minIntervalMs {
							stats.minIntervalMs = intervalMs
						}
						if intervalMs > stats.maxIntervalMs {
							stats.maxIntervalMs = intervalMs
						}
						// Track significant gaps
						if intervalMs > 50 {
							stats.gapCount50ms++
						}
						if intervalMs > 100 {
							stats.gapCount100ms++
						}
						if intervalMs > 200 {
							stats.gapCount200ms++
						}
					}
					stats.lastFrameTime = now

					// Track frame in current second bucket
					elapsed := time.Since(stats.startTime)
					bucket := int(elapsed.Seconds())
					if bucket < len(stats.fpsBuckets) {
						stats.fpsBuckets[bucket]++
					}

					if len(data) >= 15 {
						flags := data[2]
						isKeyframe := (flags & 0x01) != 0
						if isKeyframe {
							stats.keyframes++
						}
						frameSize := len(data) - 15
						if frameSize < stats.minFrameSize || stats.minFrameSize == 0 {
							stats.minFrameSize = frameSize
						}
						if frameSize > stats.maxFrameSize {
							stats.maxFrameSize = frameSize
						}

						// Write raw H.264 frame data to file (skip 15-byte header)
						if outFile != nil && frameSize > 0 {
							outFile.Write(data[15:])
						}
					}
				} else if wsMsgType == WsMsgStreamInit && len(data) >= 7 {
					stats.codec = data[1]
					stats.width = int(data[2])<<8 | int(data[3])
					stats.height = int(data[4])<<8 | int(data[5])
					stats.streamFPS = int(data[6])
				}
			}
			stats.mu.Unlock()
		}
	}()

	// Progress printer
	progressTicker := time.NewTicker(time.Second)
	defer progressTicker.Stop()

	timeoutChan := time.After(time.Duration(duration) * time.Second)
	progressCount := 0

	for {
		select {
		case <-sigChan:
			fmt.Printf("\n🛑 Interrupted\n")
			goto cleanup

		case <-done:
			if lastError != nil {
				fmt.Printf("\n❌ Connection error: %v\n", lastError)
			}
			goto cleanup

		case <-timeoutChan:
			fmt.Printf("\n✅ Benchmark complete\n")
			goto cleanup

		case <-progressTicker.C:
			progressCount++
			stats.mu.Lock()
			currentFPS := 0
			if progressCount <= len(stats.fpsBuckets) && progressCount > 0 {
				currentFPS = stats.fpsBuckets[progressCount-1]
			}
			fmt.Printf("  [%2d/%2ds] Frames: %4d | Current FPS: %3d | Total: %s\n",
				progressCount, duration, stats.totalFrames, currentFPS, formatBytes(stats.totalBytes))
			stats.mu.Unlock()
		}
	}

cleanup:
	// Stop vkcube
	fmt.Printf("\n🛑 Stopping vkcube...\n")
	if err := stopVkcube(apiURL, token, sessionID); err != nil {
		fmt.Printf("⚠️  Warning: Failed to stop vkcube: %v\n", err)
	}

	// Close WebSocket
	conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	time.Sleep(100 * time.Millisecond)

	// Print final results
	printBenchmarkResults(stats, duration, fps)

	// Report output file
	if outFile != nil {
		fileInfo, err := outFile.Stat()
		if err == nil {
			fmt.Printf("\n📹 Video saved to: %s (%s)\n", outputFile, formatBytes(fileInfo.Size()))
			fmt.Printf("   Play with: ffplay -f h264 %s\n", outputFile)
		}
	}

	return nil
}

type benchmarkStats struct {
	mu           sync.Mutex
	startTime    time.Time
	totalFrames  int
	totalBytes   int64
	keyframes    int
	minFrameSize int
	maxFrameSize int
	codec        byte
	width        int
	height       int
	streamFPS    int
	fpsBuckets   []int // Frames received per second

	// Jitter tracking - measures variance in inter-frame arrival times
	lastFrameTime   time.Time
	minIntervalMs   int64
	maxIntervalMs   int64
	totalIntervalMs int64
	intervalCount   int64

	// Track gaps > 50ms for detailed analysis
	gapCount50ms  int // Gaps > 50ms
	gapCount100ms int // Gaps > 100ms
	gapCount200ms int // Gaps > 200ms
}

func printBenchmarkResults(stats *benchmarkStats, _ /* duration */, targetFPS int) {
	stats.mu.Lock()
	defer stats.mu.Unlock()

	elapsed := time.Since(stats.startTime)
	avgFPS := float64(stats.totalFrames) / elapsed.Seconds()
	avgBitrate := float64(stats.totalBytes*8) / elapsed.Seconds()

	// Calculate FPS statistics from buckets
	minFPS, maxFPS := 0, 0
	validBuckets := 0
	totalFPS := 0
	for _, fps := range stats.fpsBuckets {
		if fps > 0 {
			validBuckets++
			totalFPS += fps
			if minFPS == 0 || fps < minFPS {
				minFPS = fps
			}
			if fps > maxFPS {
				maxFPS = fps
			}
		}
	}

	// Calculate percentage of target FPS achieved
	fpsPercentage := (avgFPS / float64(targetFPS)) * 100

	fmt.Printf("\n")
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("📊 BENCHMARK RESULTS\n")
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	if stats.width > 0 {
		fmt.Printf("  Resolution:       %dx%d\n", stats.width, stats.height)
	}
	if stats.codec > 0 {
		fmt.Printf("  Codec:            %s\n", codecName(stats.codec))
	}
	fmt.Printf("  Duration:         %v\n", elapsed.Round(time.Millisecond))
	fmt.Printf("\n")

	fmt.Printf("  📈 Frame Rate:\n")
	fmt.Printf("     Average:       %.1f fps", avgFPS)
	if fpsPercentage < 90 {
		fmt.Printf(" ⚠️  (%.0f%% of %d target)\n", fpsPercentage, targetFPS)
	} else {
		fmt.Printf(" ✅ (%.0f%% of %d target)\n", fpsPercentage, targetFPS)
	}
	if minFPS > 0 {
		fmt.Printf("     Min/Max:       %d / %d fps\n", minFPS, maxFPS)
	}
	fmt.Printf("     Total Frames:  %d (%d keyframes)\n", stats.totalFrames, stats.keyframes)
	fmt.Printf("\n")

	fmt.Printf("  📦 Bandwidth:\n")
	fmt.Printf("     Average:       %s/s\n", formatBits(int64(avgBitrate)))
	fmt.Printf("     Total Data:    %s\n", formatBytes(stats.totalBytes))
	if stats.totalFrames > 0 {
		avgFrameSize := stats.totalBytes / int64(stats.totalFrames)
		fmt.Printf("     Avg Frame:     %s\n", formatBytes(avgFrameSize))
		fmt.Printf("     Frame Range:   %s - %s\n",
			formatBytes(int64(stats.minFrameSize)),
			formatBytes(int64(stats.maxFrameSize)))
	}

	// Jitter stats
	fmt.Printf("\n")
	fmt.Printf("  📉 Frame Jitter (inter-frame arrival variance):\n")
	if stats.intervalCount > 0 {
		avgIntervalMs := stats.totalIntervalMs / stats.intervalCount
		fmt.Printf("     Interval:      min=%dms avg=%dms max=%dms\n",
			stats.minIntervalMs, avgIntervalMs, stats.maxIntervalMs)
		fmt.Printf("     Gaps >50ms:    %d (%.1f%%)\n",
			stats.gapCount50ms, float64(stats.gapCount50ms)/float64(stats.intervalCount)*100)
		fmt.Printf("     Gaps >100ms:   %d (%.1f%%)\n",
			stats.gapCount100ms, float64(stats.gapCount100ms)/float64(stats.intervalCount)*100)
		fmt.Printf("     Gaps >200ms:   %d (%.1f%%)\n",
			stats.gapCount200ms, float64(stats.gapCount200ms)/float64(stats.intervalCount)*100)

		// Warning if significant jitter
		if stats.maxIntervalMs > 100 {
			fmt.Printf("     ⚠️  High jitter detected! Max gap: %dms\n", stats.maxIntervalMs)
		}
	}

	fmt.Printf("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	// Summary verdict
	if avgFPS >= float64(targetFPS)*0.95 {
		fmt.Printf("✅ EXCELLENT: Achieving %.0f%% of target FPS\n", fpsPercentage)
	} else if avgFPS >= float64(targetFPS)*0.8 {
		fmt.Printf("👍 GOOD: Achieving %.0f%% of target FPS\n", fpsPercentage)
	} else if avgFPS >= float64(targetFPS)*0.5 {
		fmt.Printf("⚠️  DEGRADED: Only %.0f%% of target FPS - check GPU/encoder load\n", fpsPercentage)
	} else {
		fmt.Printf("❌ POOR: Only %.0f%% of target FPS - significant bottleneck\n", fpsPercentage)
	}
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
}

// startVkcube starts vkcube in the session container via the exec endpoint
func startVkcube(apiURL, token, sessionID string) error {
	execURL := fmt.Sprintf("%s/api/v1/external-agents/%s/exec", apiURL, sessionID)

	// vkcube needs Wayland display environment set
	// --wsi wayland forces Wayland backend (default is X11)
	// --present_mode 1 = MAILBOX (triple buffered, no tearing, max FPS)
	// IMMEDIATE (mode 0) is often not supported on virtual displays
	payload := map[string]interface{}{
		"command": []string{
			"vkcube",
			"--wsi", "wayland",
			"--present_mode", "1",
		},
		"background": true,
		"env": map[string]string{
			"WAYLAND_DISPLAY":  "wayland-0",
			"XDG_RUNTIME_DIR":  "/run/user/1000",
		},
	}
	jsonData, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", execURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("exec API returned %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// stopVkcube kills vkcube in the session container
func stopVkcube(apiURL, token, sessionID string) error {
	execURL := fmt.Sprintf("%s/api/v1/external-agents/%s/exec", apiURL, sessionID)

	payload := map[string]interface{}{
		"command": []string{"pkill", "-f", "vkcube"},
	}
	jsonData, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", execURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Ignore exit code - pkill may return non-zero if process not found
	return nil
}
