package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const (
	// HeartbeatInterval is how often to send heartbeats
	HeartbeatInterval = 30 * time.Second

	// DiskWarningThreshold is the percentage at which we warn
	DiskWarningThreshold = 80.0

	// DiskCriticalThreshold is the percentage at which we alert critical
	DiskCriticalThreshold = 95.0
)

// MonitoredMountPoints are the mount points we check for disk usage
var MonitoredMountPoints = []string{"/var", "/"}

// DiskUsageMetric represents disk usage for a single mount point
type DiskUsageMetric struct {
	MountPoint  string  `json:"mount_point"`
	TotalBytes  uint64  `json:"total_bytes"`
	UsedBytes   uint64  `json:"used_bytes"`
	AvailBytes  uint64  `json:"avail_bytes"`
	UsedPercent float64 `json:"used_percent"`
	AlertLevel  string  `json:"alert_level"`
}

// ContainerDiskUsage represents disk usage for a single container
type ContainerDiskUsage struct {
	ContainerID   string `json:"container_id"`
	ContainerName string `json:"container_name"`
	SizeBytes     uint64 `json:"size_bytes"`
	RwSizeBytes   uint64 `json:"rw_size_bytes"`
}

// HeartbeatRequest is the request body sent to the API
type HeartbeatRequest struct {
	// Desktop image versions (content-addressable Docker image hashes)
	// Key: desktop name (e.g., "sway", "zorin", "ubuntu")
	// Value: image hash (e.g., "a1b2c3d4e5f6...")
	DesktopVersions       map[string]string    `json:"desktop_versions,omitempty"`
	DiskUsage             []DiskUsageMetric    `json:"disk_usage,omitempty"`
	ContainerUsage        []ContainerDiskUsage `json:"container_usage,omitempty"`
	PrivilegedModeEnabled bool                 `json:"privileged_mode_enabled,omitempty"`
	GPUVendor             string               `json:"gpu_vendor,omitempty"`  // nvidia, amd, intel, none
	RenderNode            string               `json:"render_node,omitempty"` // /dev/dri/renderD128 or SOFTWARE
}

func main() {
	// Setup logging
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})

	// Get configuration from environment
	apiURL := os.Getenv("HELIX_API_URL")
	runnerToken := os.Getenv("RUNNER_TOKEN")
	wolfInstanceID := os.Getenv("WOLF_INSTANCE_ID")

	if apiURL == "" || runnerToken == "" {
		log.Info().Msg("HELIX_API_URL or RUNNER_TOKEN not set, running in local mode (no heartbeats)")
		// Block forever in local mode
		select {}
	}

	if wolfInstanceID == "" {
		wolfInstanceID = "local"
	}

	// Check if privileged mode is enabled
	privilegedModeEnabled := os.Getenv("HYDRA_PRIVILEGED_MODE_ENABLED") == "true"

	log.Info().
		Str("api_url", apiURL).
		Str("wolf_instance_id", wolfInstanceID).
		Bool("privileged_mode_enabled", privilegedModeEnabled).
		Dur("interval", HeartbeatInterval).
		Msg("Starting sandbox heartbeat daemon")

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Create ticker for heartbeats
	ticker := time.NewTicker(HeartbeatInterval)
	defer ticker.Stop()

	// Send initial heartbeat immediately
	sendHeartbeat(apiURL, runnerToken, wolfInstanceID, privilegedModeEnabled)

	// Main loop
	for {
		select {
		case <-ticker.C:
			sendHeartbeat(apiURL, runnerToken, wolfInstanceID, privilegedModeEnabled)
		case sig := <-sigChan:
			log.Info().Str("signal", sig.String()).Msg("Received signal, shutting down")
			return
		}
	}
}

func sendHeartbeat(apiURL, runnerToken, wolfInstanceID string, privilegedModeEnabled bool) {
	// Discover all desktop versions dynamically
	// Scans /opt/images/helix-*.version files
	desktopVersions := discoverDesktopVersions()

	// Collect disk usage metrics
	diskUsage := collectDiskUsage()
	containerUsage := collectContainerDiskUsage()

	// Read GPU configuration from environment (set by install.sh on the sandbox)
	gpuVendor := os.Getenv("GPU_VENDOR")        // nvidia, amd, intel, none
	renderNode := os.Getenv("WOLF_RENDER_NODE") // /dev/dri/renderD128 or SOFTWARE
	if renderNode == "" {
		// Default render node if not explicitly set
		renderNode = "/dev/dri/renderD128"
	}

	// Build request
	req := HeartbeatRequest{
		DesktopVersions:       desktopVersions,
		DiskUsage:             diskUsage,
		ContainerUsage:        containerUsage,
		PrivilegedModeEnabled: privilegedModeEnabled,
		GPUVendor:             gpuVendor,
		RenderNode:            renderNode,
	}

	// Log disk status
	for _, disk := range diskUsage {
		logEvent := log.Debug()
		if disk.AlertLevel == "critical" {
			logEvent = log.Error()
		} else if disk.AlertLevel == "warning" {
			logEvent = log.Warn()
		}
		logEvent.
			Str("mount", disk.MountPoint).
			Float64("used_percent", disk.UsedPercent).
			Str("alert_level", disk.AlertLevel).
			Msg("Disk usage")
	}

	// Send heartbeat
	body, err := json.Marshal(req)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal heartbeat request")
		return
	}

	url := fmt.Sprintf("%s/api/v1/wolf-instances/%s/heartbeat", apiURL, wolfInstanceID)
	httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		log.Error().Err(err).Msg("Failed to create heartbeat request")
		return
	}

	httpReq.Header.Set("Authorization", "Bearer "+runnerToken)
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to send heartbeat (API may not be ready)")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Warn().Int("status", resp.StatusCode).Msg("Heartbeat returned non-OK status")
		return
	}

	log.Debug().
		Interface("desktop_versions", desktopVersions).
		Str("gpu_vendor", gpuVendor).
		Str("render_node", renderNode).
		Msg("Heartbeat sent successfully")
}

// discoverDesktopVersions scans for all desktop version files
// and returns a map of desktop name -> image hash
func discoverDesktopVersions() map[string]string {
	versions := make(map[string]string)

	// Scan for all version files matching pattern
	files, err := filepath.Glob("/opt/images/helix-*.version")
	if err != nil {
		log.Warn().Err(err).Msg("Failed to scan for desktop version files")
		return versions
	}

	for _, file := range files {
		// Extract desktop name from filename
		// e.g., "/opt/images/helix-sway.version" -> "sway"
		base := filepath.Base(file)                 // "helix-sway.version"
		name := strings.TrimPrefix(base, "helix-")  // "sway.version"
		name = strings.TrimSuffix(name, ".version") // "sway"

		// Read version (image hash)
		data, err := os.ReadFile(file)
		if err != nil {
			log.Warn().Err(err).Str("file", file).Msg("Failed to read version file")
			continue
		}

		version := string(bytes.TrimSpace(data))
		if version != "" {
			versions[name] = version
			log.Debug().
				Str("desktop", name).
				Str("version", version).
				Msg("Discovered desktop version")
		}
	}

	return versions
}

func collectDiskUsage() []DiskUsageMetric {
	var metrics []DiskUsageMetric

	for _, mountPoint := range MonitoredMountPoints {
		metric, err := getDiskUsage(mountPoint)
		if err != nil {
			log.Warn().Err(err).Str("mount", mountPoint).Msg("Failed to get disk usage")
			continue
		}
		metrics = append(metrics, metric)
	}

	return metrics
}

func getDiskUsage(path string) (DiskUsageMetric, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return DiskUsageMetric{}, fmt.Errorf("statfs failed: %w", err)
	}

	// Calculate disk usage
	totalBytes := stat.Blocks * uint64(stat.Bsize)
	freeBytes := stat.Bfree * uint64(stat.Bsize)
	availBytes := stat.Bavail * uint64(stat.Bsize) // Available to non-root users
	usedBytes := totalBytes - freeBytes

	var usedPercent float64
	if totalBytes > 0 {
		usedPercent = float64(usedBytes) / float64(totalBytes) * 100
	}

	// Determine alert level
	alertLevel := "ok"
	if usedPercent >= DiskCriticalThreshold {
		alertLevel = "critical"
	} else if usedPercent >= DiskWarningThreshold {
		alertLevel = "warning"
	}

	return DiskUsageMetric{
		MountPoint:  path,
		TotalBytes:  totalBytes,
		UsedBytes:   usedBytes,
		AvailBytes:  availBytes,
		UsedPercent: usedPercent,
		AlertLevel:  alertLevel,
	}, nil
}

// DockerContainer represents a container from Docker API
type DockerContainer struct {
	ID         string   `json:"Id"`
	Names      []string `json:"Names"`
	SizeRw     int64    `json:"SizeRw"`
	SizeRootFs int64    `json:"SizeRootFs"`
}

// collectContainerDiskUsage queries Docker for container disk usage
func collectContainerDiskUsage() []ContainerDiskUsage {
	var containers []ContainerDiskUsage

	// Query Docker socket for containers with size info
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return net.Dial("unix", "/var/run/docker.sock")
			},
		},
		Timeout: 30 * time.Second, // Size calculation can be slow
	}

	// The size=true parameter makes Docker calculate container sizes
	resp, err := client.Get("http://localhost/containers/json?all=true&size=true")
	if err != nil {
		log.Debug().Err(err).Msg("Failed to query Docker containers (docker.sock may not be accessible)")
		return containers
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Debug().Int("status", resp.StatusCode).Msg("Docker API returned non-OK status")
		return containers
	}

	var dockerContainers []DockerContainer
	if err := json.NewDecoder(resp.Body).Decode(&dockerContainers); err != nil {
		log.Warn().Err(err).Msg("Failed to decode Docker containers response")
		return containers
	}

	for _, c := range dockerContainers {
		name := c.ID[:12] // Short ID as fallback
		if len(c.Names) > 0 {
			// Docker names start with "/"
			name = c.Names[0]
			if len(name) > 0 && name[0] == '/' {
				name = name[1:]
			}
		}

		// Only include containers with non-zero size
		if c.SizeRw > 0 || c.SizeRootFs > 0 {
			containers = append(containers, ContainerDiskUsage{
				ContainerID:   c.ID[:12],
				ContainerName: name,
				SizeBytes:     uint64(c.SizeRootFs),
				RwSizeBytes:   uint64(c.SizeRw),
			})
		}
	}

	if len(containers) > 0 {
		log.Debug().Int("count", len(containers)).Msg("Collected container disk usage")
	}

	return containers
}
