package spectask

import (
	"bytes"
	"encoding/binary"
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

func newLatencyCommand() *cobra.Command {
	var numTests int
	var testInterval int
	var keycode int
	var verbose bool
	var warmupSeconds int
	var setupEditor bool
	var skipSetup bool

	cmd := &cobra.Command{
		Use:   "latency <session-id>",
		Short: "Measure key-to-eyeball input latency",
		Long: `Measures the latency from sending a keystroke to seeing the screen update.

This works by:
1. Connecting to the video stream
2. Launching a terminal (for keystroke visibility) - optional with --setup
3. Establishing baseline frame timing (idle screens send frames every ~100ms)
4. Sending a keystroke and timing when an out-of-band frame arrives
5. Calculating the delta between input send and screen update

The test relies on damage-based updates - when you press a key, the screen changes
and a new frame is captured/encoded/sent immediately (not waiting for the next
scheduled frame). This creates a measurable spike in frame arrival timing.

By default, this command launches a terminal window (kitty) in the session to
ensure keystrokes are visible. Use --skip-setup if a text editor is already focused.

Examples:
  helix spectask latency ses_01xxx                     # Run 5 latency tests (auto-setup)
  helix spectask latency ses_01xxx --tests 20          # Run 20 tests
  helix spectask latency ses_01xxx --verbose           # Show each measurement
  helix spectask latency ses_01xxx --skip-setup        # Skip terminal launch (editor already focused)
  helix spectask latency ses_01xxx --keycode 57        # Use space key (evdev 57)
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sessionID := args[0]
			apiURL := getAPIURL()
			token := getToken()

			return runLatencyTest(apiURL, token, sessionID, numTests, testInterval, keycode, verbose, warmupSeconds, setupEditor || !skipSetup)
		},
	}

	cmd.Flags().IntVarP(&numTests, "tests", "t", 5, "Number of latency tests to run")
	cmd.Flags().IntVarP(&testInterval, "interval", "i", 500, "Interval between tests in milliseconds")
	cmd.Flags().IntVarP(&keycode, "keycode", "k", 57, "Evdev keycode to send (default 57 = space)")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show verbose output including each measurement")
	cmd.Flags().IntVar(&warmupSeconds, "warmup", 3, "Warmup period in seconds before testing")
	cmd.Flags().BoolVar(&setupEditor, "setup", false, "Launch terminal for keystroke visibility (default: true)")
	cmd.Flags().BoolVar(&skipSetup, "skip-setup", false, "Skip launching terminal (assume editor is already focused)")

	return cmd
}

// latencyStats tracks frame timing and input latency measurements
type latencyStats struct {
	mu sync.Mutex

	// Frame timing
	frameCount      int
	lastFrameTime   time.Time
	frameIntervals  []time.Duration // Recent frame intervals for baseline
	maxIntervals    int

	// Latency measurement
	inputSentTime      time.Time
	waitingForResponse bool
	measurements       []time.Duration

	// Out-of-band detection
	baselineInterval time.Duration // Expected frame interval when idle
	oobThreshold     float64       // Multiplier for detecting out-of-band frames
}

func newLatencyStats() *latencyStats {
	return &latencyStats{
		frameIntervals: make([]time.Duration, 0, 30),
		maxIntervals:   30, // Keep last 30 intervals for baseline
		measurements:   make([]time.Duration, 0),
		oobThreshold:   0.5, // Frame arriving 50% faster than baseline is out-of-band
	}
}

// recordFrame records a frame arrival and detects out-of-band frames
func (s *latencyStats) recordFrame() (isOOB bool, latency time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	s.frameCount++

	if !s.lastFrameTime.IsZero() {
		interval := now.Sub(s.lastFrameTime)

		// Store interval for baseline calculation
		if len(s.frameIntervals) >= s.maxIntervals {
			s.frameIntervals = s.frameIntervals[1:]
		}
		s.frameIntervals = append(s.frameIntervals, interval)

		// Update baseline (use median of recent intervals)
		if len(s.frameIntervals) >= 5 {
			s.baselineInterval = s.calculateMedianInterval()
		}

		// Check if this frame is out-of-band (arrived significantly faster than baseline)
		if s.waitingForResponse && s.baselineInterval > 0 {
			threshold := time.Duration(float64(s.baselineInterval) * s.oobThreshold)
			if interval < threshold {
				// Out-of-band frame detected - this is likely the response to our input
				isOOB = true
				latency = now.Sub(s.inputSentTime)
				s.measurements = append(s.measurements, latency)
				s.waitingForResponse = false
			}
		}
	}

	s.lastFrameTime = now
	return
}

// calculateMedianInterval returns the median frame interval
func (s *latencyStats) calculateMedianInterval() time.Duration {
	if len(s.frameIntervals) == 0 {
		return 0
	}

	// Copy and sort
	sorted := make([]time.Duration, len(s.frameIntervals))
	copy(sorted, s.frameIntervals)
	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j] < sorted[i] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	mid := len(sorted) / 2
	if len(sorted)%2 == 0 {
		return (sorted[mid-1] + sorted[mid]) / 2
	}
	return sorted[mid]
}

// markInputSent marks when an input was sent
func (s *latencyStats) markInputSent() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inputSentTime = time.Now()
	s.waitingForResponse = true
}

// isWaiting returns true if we're waiting for a response frame
func (s *latencyStats) isWaiting() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.waitingForResponse
}

// getBaseline returns the current baseline frame interval
func (s *latencyStats) getBaseline() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.baselineInterval
}

// getMeasurements returns a copy of all latency measurements
func (s *latencyStats) getMeasurements() []time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]time.Duration, len(s.measurements))
	copy(result, s.measurements)
	return result
}

// cancelWaiting cancels a pending measurement (timeout)
func (s *latencyStats) cancelWaiting() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.waitingForResponse = false
}

func runLatencyTest(apiURL, token, sessionID string, numTests, testInterval, keycode int, verbose bool, warmupSeconds int, setupTerminal bool) error {
	fmt.Printf("🔬 Input Latency Measurement\n")
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("Session:     %s\n", sessionID)
	fmt.Printf("Tests:       %d @ %dms interval\n", numTests, testInterval)
	fmt.Printf("Keycode:     %d (evdev)\n", keycode)
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	// Launch terminal for keystroke visibility
	if setupTerminal {
		fmt.Printf("🖥️  Setting up test environment...\n")
		if err := setupLatencyTestEnvironment(apiURL, token, sessionID); err != nil {
			fmt.Printf("⚠️  Warning: Failed to setup test environment: %v\n", err)
			fmt.Printf("   Make sure a text editor or terminal is focused in the session\n\n")
		} else {
			fmt.Printf("✅ Terminal launched and focused\n\n")
		}
		// Give the terminal time to launch and gain focus
		time.Sleep(2 * time.Second)
	}

	// Connect to WebSocket stream
	fmt.Printf("📡 Connecting to video stream...\n")

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
		"width":                   1920,
		"height":                  1080,
		"fps":                     60,
		"bitrate":                 10000,
		"packet_size":             1024,
		"play_audio_local":        false,
		"video_supported_formats": 1,
	}
	initJSON, _ := json.Marshal(initMessage)
	if err := conn.WriteMessage(websocket.TextMessage, initJSON); err != nil {
		return fmt.Errorf("failed to send init: %w", err)
	}
	fmt.Printf("✅ Stream initialized\n\n")

	// Initialize stats
	stats := newLatencyStats()

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	done := make(chan struct{})
	var connErr error

	// Frame receiver goroutine
	go func() {
		defer close(done)
		for {
			msgType, data, err := conn.ReadMessage()
			if err != nil {
				if !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
					connErr = err
				}
				return
			}

			if msgType == websocket.BinaryMessage && len(data) > 0 {
				wsMsgType := data[0]
				if wsMsgType == WsMsgVideoFrame {
					isOOB, latency := stats.recordFrame()
					if isOOB && verbose {
						fmt.Printf("    ⚡ Out-of-band frame detected! Latency: %v\n", latency.Round(time.Microsecond))
					}
				}
			}
		}
	}()

	// Warmup period to establish baseline
	fmt.Printf("⏳ Warming up for %d seconds (establishing baseline frame rate)...\n", warmupSeconds)
	select {
	case <-sigChan:
		fmt.Printf("\n🛑 Interrupted\n")
		return nil
	case <-done:
		if connErr != nil {
			return fmt.Errorf("connection error during warmup: %w", connErr)
		}
		return fmt.Errorf("connection closed during warmup")
	case <-time.After(time.Duration(warmupSeconds) * time.Second):
	}

	baseline := stats.getBaseline()
	fmt.Printf("✅ Baseline frame interval: %v (%.1f fps)\n\n", baseline.Round(time.Millisecond), 1000.0/float64(baseline.Milliseconds()))

	// Run latency tests
	fmt.Printf("🧪 Running %d latency tests...\n", numTests)
	fmt.Printf("───────────────────────────────────────────────────────────────────────────\n")

	completedTests := 0
	failedTests := 0
	testTimeout := time.Duration(testInterval*2) * time.Millisecond // Timeout = 2x interval

	for i := 0; i < numTests; i++ {
		select {
		case <-sigChan:
			fmt.Printf("\n🛑 Interrupted\n")
			goto results
		case <-done:
			if connErr != nil {
				fmt.Printf("\n❌ Connection error: %v\n", connErr)
			}
			goto results
		default:
		}

		// Send keystroke
		stats.markInputSent()
		sendTime := time.Now()

		if err := sendKeyPress(conn, keycode); err != nil {
			fmt.Printf("  Test %d: ❌ Failed to send key: %v\n", i+1, err)
			failedTests++
			stats.cancelWaiting()
			continue
		}

		if verbose {
			fmt.Printf("  Test %d: Sent keycode %d at %v\n", i+1, keycode, sendTime.Format("15:04:05.000"))
		}

		// Wait for response with timeout
		responseWait := time.NewTimer(testTimeout)
		responded := false

		for !responded {
			select {
			case <-responseWait.C:
				// Timeout
				if verbose {
					fmt.Printf("  Test %d: ⚠️  Timeout (no out-of-band frame within %v)\n", i+1, testTimeout)
				}
				stats.cancelWaiting()
				failedTests++
				responded = true
			case <-done:
				responseWait.Stop()
				goto results
			case <-time.After(5 * time.Millisecond):
				// Check if we got a response
				if !stats.isWaiting() {
					responseWait.Stop()
					completedTests++
					responded = true
				}
			}
		}

		// Send key up to clean up
		sendKeyRelease(conn, keycode)

		// Wait between tests
		if i < numTests-1 {
			time.Sleep(time.Duration(testInterval) * time.Millisecond)
		}
	}

results:
	// Print results
	conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	time.Sleep(100 * time.Millisecond)

	printLatencyResults(stats, completedTests, failedTests, baseline)
	return nil
}

// sendKeyPress sends a key down event
func sendKeyPress(conn *websocket.Conn, keycode int) error {
	// Format: msgType(1) + subType(1) + isDown(1) + modifiers(1) + keycode(2 BE)
	msg := make([]byte, 6)
	msg[0] = WsMsgKeyboardInput // 0x10
	msg[1] = 0                  // subType
	msg[2] = 1                  // isDown = true
	msg[3] = 0                  // modifiers
	binary.BigEndian.PutUint16(msg[4:6], uint16(keycode))

	return conn.WriteMessage(websocket.BinaryMessage, msg)
}

// sendKeyRelease sends a key up event
func sendKeyRelease(conn *websocket.Conn, keycode int) error {
	msg := make([]byte, 6)
	msg[0] = WsMsgKeyboardInput
	msg[1] = 0
	msg[2] = 0 // isDown = false
	msg[3] = 0
	binary.BigEndian.PutUint16(msg[4:6], uint16(keycode))

	return conn.WriteMessage(websocket.BinaryMessage, msg)
}

func printLatencyResults(stats *latencyStats, completed, failed int, baseline time.Duration) {
	measurements := stats.getMeasurements()

	fmt.Printf("\n")
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("📊 LATENCY RESULTS\n")
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	fmt.Printf("  Tests:          %d completed, %d failed\n", completed, failed)
	fmt.Printf("  Baseline FPS:   %.1f (interval: %v)\n", 1000.0/float64(baseline.Milliseconds()), baseline.Round(time.Millisecond))
	fmt.Printf("\n")

	if len(measurements) == 0 {
		fmt.Printf("  ❌ No successful measurements\n")
		fmt.Printf("\n  Possible causes:\n")
		fmt.Printf("     - No application focused in session (keyboard input not visible)\n")
		fmt.Printf("     - Session not running or not connected\n")
		fmt.Printf("     - Damage-based updates not working\n")
		fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		return
	}

	// Calculate statistics
	var sum time.Duration
	min := measurements[0]
	max := measurements[0]

	for _, m := range measurements {
		sum += m
		if m < min {
			min = m
		}
		if m > max {
			max = m
		}
	}

	avg := sum / time.Duration(len(measurements))

	// Calculate standard deviation
	var sumSquares float64
	avgMs := float64(avg.Microseconds())
	for _, m := range measurements {
		diff := float64(m.Microseconds()) - avgMs
		sumSquares += diff * diff
	}
	stdDev := time.Duration(0)
	if len(measurements) > 1 {
		variance := sumSquares / float64(len(measurements)-1)
		stdDevUs := int64(0)
		if variance > 0 {
			// sqrt approximation
			for x := variance; ; {
				nx := (x + variance/x) / 2
				if nx >= x {
					stdDevUs = int64(x)
					break
				}
				x = nx
			}
		}
		stdDev = time.Duration(stdDevUs) * time.Microsecond
	}

	// Calculate median
	sorted := make([]time.Duration, len(measurements))
	copy(sorted, measurements)
	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j] < sorted[i] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	median := sorted[len(sorted)/2]

	fmt.Printf("  📈 Key-to-Eyeball Latency:\n")
	fmt.Printf("     Average:     %v\n", avg.Round(time.Millisecond))
	fmt.Printf("     Median:      %v\n", median.Round(time.Millisecond))
	fmt.Printf("     Min:         %v\n", min.Round(time.Millisecond))
	fmt.Printf("     Max:         %v\n", max.Round(time.Millisecond))
	fmt.Printf("     Std Dev:     %v\n", stdDev.Round(time.Millisecond))
	fmt.Printf("\n")

	// Verdict
	if avg < 50*time.Millisecond {
		fmt.Printf("  ✅ EXCELLENT: <50ms average latency\n")
	} else if avg < 80*time.Millisecond {
		fmt.Printf("  👍 GOOD: 50-80ms average latency\n")
	} else if avg < 120*time.Millisecond {
		fmt.Printf("  ⚠️  ACCEPTABLE: 80-120ms average latency\n")
	} else {
		fmt.Printf("  ❌ POOR: >120ms average latency - noticeable lag\n")
	}

	fmt.Printf("\n")

	// Print individual measurements
	fmt.Printf("  📋 Individual Measurements:\n")
	fmt.Printf("     ")
	for i, m := range measurements {
		fmt.Printf("%v", m.Round(time.Millisecond))
		if i < len(measurements)-1 {
			fmt.Printf(", ")
		}
		if (i+1)%5 == 0 && i < len(measurements)-1 {
			fmt.Printf("\n     ")
		}
	}
	fmt.Printf("\n")

	fmt.Printf("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	// Interpretation
	fmt.Printf("\n💡 Interpretation:\n")
	fmt.Printf("   This measures the full key-to-eyeball latency including:\n")
	fmt.Printf("   • WebSocket send time\n")
	fmt.Printf("   • RevDial proxy\n")
	fmt.Printf("   • Input injection (D-Bus/Wayland)\n")
	fmt.Printf("   • Application rendering\n")
	fmt.Printf("   • Screen capture\n")
	fmt.Printf("   • Video encoding\n")
	fmt.Printf("   • WebSocket receive time\n")
	fmt.Printf("   (Does NOT include browser decoding/display - add ~5ms for that)\n")
}

// setupLatencyTestEnvironment launches a terminal in the session for keystroke visibility
func setupLatencyTestEnvironment(apiURL, token, sessionID string) error {
	execURL := fmt.Sprintf("%s/api/v1/external-agents/%s/exec", apiURL, sessionID)

	// Launch a simple terminal that will show keystrokes
	// We use 'cat' which echoes input directly - very visible for latency testing
	// The terminal (kitty) will be focused automatically on launch
	//
	// IMPORTANT: Disable cursor blink to avoid periodic damage that would
	// interfere with out-of-band frame detection. Blinking cursor creates
	// frames at blink rate, making it hard to detect keystroke-triggered frames.
	payload := map[string]interface{}{
		"command": []string{
			"kitty",
			"-o", "cursor_blink_interval=0", // Disable cursor blinking
			"-o", "cursor_shape=block",      // Solid block cursor (no animation)
			"-e", "sh", "-c",
			// Disable cursor blink via ANSI escape, clear screen, print instructions, run cat
			"printf '\\e[?12l' && clear && echo '=== LATENCY TEST ===' && echo 'Keystrokes will appear below:' && echo '(cursor blink disabled for accurate measurement)' && echo '' && cat",
		},
		"background": true,
		"env": map[string]string{
			"WAYLAND_DISPLAY": "wayland-1",
			"XDG_RUNTIME_DIR": "/run/user/1000",
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

// cleanupLatencyTestEnvironment kills the test terminal
func cleanupLatencyTestEnvironment(apiURL, token, sessionID string) error {
	execURL := fmt.Sprintf("%s/api/v1/external-agents/%s/exec", apiURL, sessionID)

	// Kill the cat process which will close the terminal
	payload := map[string]interface{}{
		"command": []string{"pkill", "-f", "cat"},
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
