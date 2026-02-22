package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ScanoutStats holds DRM scanout usage statistics
type ScanoutStats struct {
	TotalConnectors int    `json:"total_connectors"` // Total available (excluding Virtual-0)
	ActiveDisplays  int    `json:"active_displays"`  // Currently connected/leased
	MaxScanouts     int    `json:"max_scanouts"`     // Protocol limit (15)
	Displays        []DisplayInfo `json:"displays,omitempty"`
	LastUpdated     string `json:"last_updated"`
	Error           string `json:"error,omitempty"`
}

// DisplayInfo holds info about a single display/scanout
type DisplayInfo struct {
	Name      string `json:"name"`       // e.g., "Virtual-1"
	Connected bool   `json:"connected"`  // true if lease is active
	Width     int    `json:"width"`      // resolution width (0 if not connected)
	Height    int    `json:"height"`     // resolution height (0 if not connected)
}

// ScanoutCollector collects DRM scanout stats from the VM via SSH
type ScanoutCollector struct {
	mu         sync.RWMutex
	stats      ScanoutStats
	sshPort    int
	sshKeyPath string
	stopCh     chan struct{}
}

// NewScanoutCollector creates a new scanout stats collector
func NewScanoutCollector(sshPort int) *ScanoutCollector {
	keyPath := filepath.Join(getHelixDataDir(), "ssh", "helix_ed25519")
	if _, err := os.Stat(keyPath); err != nil {
		keyPath = "" // Dev mode: use default SSH agent keys
	}
	return &ScanoutCollector{
		sshPort:    sshPort,
		sshKeyPath: keyPath,
		stopCh:     make(chan struct{}),
		stats: ScanoutStats{
			MaxScanouts: 15,
		},
	}
}

// Start begins periodic scanout stats collection
func (s *ScanoutCollector) Start() {
	go s.pollLoop()
}

// Stop stops the collector
func (s *ScanoutCollector) Stop() {
	close(s.stopCh)
}

// GetStats returns the latest scanout stats
func (s *ScanoutCollector) GetStats() ScanoutStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stats
}

func (s *ScanoutCollector) pollLoop() {
	// Initial collection
	s.collect()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.collect()
		}
	}
}

func (s *ScanoutCollector) collect() {
	stats, err := s.fetchScanoutStats()
	if err != nil {
		stats.Error = err.Error()
	}
	stats.LastUpdated = time.Now().Format(time.RFC3339)
	stats.MaxScanouts = 15

	s.mu.Lock()
	s.stats = stats
	s.mu.Unlock()
}

func (s *ScanoutCollector) sshCmd(command string) (string, error) {
	args := []string{
		"-F", "/dev/null", // Don't read ~/.ssh/config (triggers macOS TCC dialog)
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "ConnectTimeout=5",
	}
	if s.sshKeyPath != "" {
		args = append(args, "-i", s.sshKeyPath, "-o", "IdentitiesOnly=yes")
	}
	args = append(args, "-p", fmt.Sprintf("%d", s.sshPort), "ubuntu@localhost", command)
	cmd := exec.Command("ssh", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("ssh command failed: %w: %s", err, string(out))
	}
	return strings.TrimSpace(string(out)), nil
}

func (s *ScanoutCollector) fetchScanoutStats() (ScanoutStats, error) {
	var stats ScanoutStats

	// Query all Virtual-N connector statuses via sysfs
	// Each connector has a "status" file: "connected" or "disconnected"
	// Also get the current mode (resolution) if connected
	// Script outputs one line per connector: "Virtual-N connected 1920x1080" or "Virtual-N disconnected"
	script := `for d in /sys/class/drm/card0-Virtual-*/; do
  name=$(basename "$d" | sed 's/card0-//')
  status=$(cat "$d/status" 2>/dev/null || echo "unknown")
  mode=""
  if [ "$status" = "connected" ]; then
    mode=$(cat "$d/modes" 2>/dev/null | head -1)
  fi
  echo "$name $status $mode"
done 2>/dev/null`

	out, err := s.sshCmd(script)
	if err != nil {
		return stats, fmt.Errorf("failed to query connectors: %w", err)
	}

	if out == "" {
		return stats, nil
	}

	lines := strings.Split(out, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		name := fields[0]
		connected := fields[1] == "connected"

		// Skip Virtual-0 (VM console)
		if name == "Virtual-0" {
			continue
		}

		display := DisplayInfo{
			Name:      name,
			Connected: connected,
		}

		// Parse resolution if available (e.g., "1920x1080")
		if connected && len(fields) >= 3 {
			parts := strings.Split(fields[2], "x")
			if len(parts) == 2 {
				display.Width, _ = strconv.Atoi(parts[0])
				display.Height, _ = strconv.Atoi(parts[1])
			}
		}

		stats.Displays = append(stats.Displays, display)
		stats.TotalConnectors++
		if connected {
			stats.ActiveDisplays++
		}
	}

	return stats, nil
}
