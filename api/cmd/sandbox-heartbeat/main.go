package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/gpudetect"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
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
	GPUVendor             string               `json:"gpu_vendor,omitempty"`    // nvidia, amd, intel, none
	InstanceType          string               `json:"instance_type,omitempty"` // cloud instance type via IMDS; empty on bare metal
	HelixVersion          string               `json:"helix_version,omitempty"` // git commit hash or release version

	// Sandbox-absorbs-runner pivot: rich GPU inventory used by the inference
	// router's profile-compatibility check. Detected via nvidia-smi /
	// rocm-smi by the gpudetect package. Empty on hosts without GPUs.
	GPUs []types.GPUStatus `json:"gpus,omitempty"`

	// Inference subsystem state, read from the status.json file the
	// compose-manager writes after each Apply.
	ProfileStatus   string                                       `json:"profile_status,omitempty"`
	ProfileError    string                                       `json:"profile_error,omitempty"`
	ServiceHealth   map[string]string                            `json:"service_health,omitempty"`
	ProfileProgress map[string]types.ServiceDownloadProgress     `json:"profile_progress,omitempty"`
}

// composeManagerStatusFile is where compose-manager persists its current
// view (profile id, status, service health) for sandbox-heartbeat to pick
// up and forward to the API server. The path is intentionally a constant
// here — the compose-manager defaults to the same /etc/helix/status.json.
const composeManagerStatusFile = "/etc/helix/status.json"

// readComposeManagerStatus parses the compose-manager status.json. Returns
// zero values on any error (file missing is the common case before any
// profile is applied).
func readComposeManagerStatus() (status, errMsg string, health map[string]string, progress map[string]types.ServiceDownloadProgress) {
	data, err := os.ReadFile(composeManagerStatusFile)
	if err != nil {
		return "", "", nil, nil
	}
	var s struct {
		ProfileID     string                                   `json:"ProfileID"`
		ProfileName   string                                   `json:"ProfileName"`
		Status        string                                   `json:"Status"`
		Error         string                                   `json:"Error"`
		ServiceHealth map[string]string                        `json:"ServiceHealth"`
		Progress      map[string]types.ServiceDownloadProgress `json:"Progress"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return "", "", nil, nil
	}
	return s.Status, s.Error, s.ServiceHealth, s.Progress
}

func main() {
	// Match the control-plane console format (RFC3339 timestamp + level +
	// caller). Writes to stderr so the heartbeat output doesn't mix with
	// dataplane stdout streams. TTY detection in SetupLoggingTo strips ANSI
	// colour codes when stderr is captured to a file.
	system.SetupLoggingTo(os.Stderr)

	// Get configuration from environment
	apiURL := os.Getenv("HELIX_API_URL")
	runnerToken := os.Getenv("RUNNER_TOKEN")
	sandboxInstanceID := os.Getenv("SANDBOX_INSTANCE_ID")

	if apiURL == "" || runnerToken == "" {
		log.Info().Msg("HELIX_API_URL or RUNNER_TOKEN not set, running in local mode (no heartbeats)")
		// Block forever in local mode
		select {}
	}

	if sandboxInstanceID == "" {
		sandboxInstanceID = "local"
	}

	// Check if privileged mode is enabled
	privilegedModeEnabled := os.Getenv("HYDRA_PRIVILEGED_MODE_ENABLED") == "true"

	log.Info().
		Str("api_url", apiURL).
		Str("sandbox_instance_id", sandboxInstanceID).
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
	sendHeartbeat(apiURL, runnerToken, sandboxInstanceID, privilegedModeEnabled)

	// Main loop
	for {
		select {
		case <-ticker.C:
			sendHeartbeat(apiURL, runnerToken, sandboxInstanceID, privilegedModeEnabled)
		case sig := <-sigChan:
			log.Info().Str("signal", sig.String()).Msg("Received signal, shutting down")
			return
		}
	}
}

// detectInstanceType queries the AWS IMDS for the EC2 instance type (e.g.
// "inf2.8xlarge"). Uses IMDSv2 (token-authenticated). Returns "" on any
// failure — bare-metal hosts (e.g. prime), non-AWS clouds, and IMDS being
// unreachable all just yield an empty string with no hang. The 1.5s client
// timeout caps the wait so a non-existent metadata endpoint can't block the
// heartbeat loop.
func detectInstanceType(ctx context.Context) string {
	const imds = "http://169.254.169.254"
	client := &http.Client{Timeout: 1500 * time.Millisecond}

	tokReq, err := http.NewRequestWithContext(ctx, http.MethodPut, imds+"/latest/api/token", nil)
	if err != nil {
		return ""
	}
	tokReq.Header.Set("X-aws-ec2-metadata-token-ttl-seconds", "60")
	tokResp, err := client.Do(tokReq)
	if err != nil {
		return ""
	}
	token, _ := io.ReadAll(tokResp.Body)
	tokResp.Body.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imds+"/latest/meta-data/instance-type", nil)
	if err != nil {
		return ""
	}
	if len(token) > 0 {
		req.Header.Set("X-aws-ec2-metadata-token", string(token))
	}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	body, _ := io.ReadAll(resp.Body)
	return strings.TrimSpace(string(body))
}

func sendHeartbeat(apiURL, runnerToken, sandboxInstanceID string, privilegedModeEnabled bool) {
	// Discover all desktop versions dynamically
	// Scans /opt/images/helix-*.version files
	desktopVersions := discoverDesktopVersions()

	// Collect disk usage metrics
	diskUsage := collectDiskUsage()
	containerUsage := collectContainerDiskUsage()

	// Read GPU vendor from environment (set by install.sh on the sandbox)
	gpuVendor := os.Getenv("GPU_VENDOR") // nvidia, amd, intel, none

	// Sandbox-absorbs-runner pivot: probe nvidia-smi / rocm-smi for the
	// rich inventory the inference router's compatibility check needs.
	// 5s timeout — if probes hang we ship the heartbeat without GPU data
	// rather than block the loop.
	probeCtx, probeCancel := context.WithTimeout(context.Background(), 5*time.Second)
	gpus := gpudetect.Detect(probeCtx)
	// Cloud instance type (e.g. inf2.8xlarge). Empty on bare-metal / non-AWS.
	instanceType := detectInstanceType(probeCtx)
	probeCancel()

	// Read compose-manager status from the file it writes after each Apply.
	// Best-effort — if the file is missing or malformed, we ship the
	// heartbeat without inference subsystem state (the API server then
	// treats the sandbox as not-running-anything).
	profileStatus, profileError, serviceHealth, profileProgress := readComposeManagerStatus()

	// Build request
	req := HeartbeatRequest{
		DesktopVersions:       desktopVersions,
		DiskUsage:             diskUsage,
		ContainerUsage:        containerUsage,
		PrivilegedModeEnabled: privilegedModeEnabled,
		GPUVendor:             gpuVendor,
		InstanceType:          instanceType,
		HelixVersion:          data.GetHelixVersion(),
		GPUs:                  gpus,
		ProfileStatus:         profileStatus,
		ProfileError:          profileError,
		ServiceHealth:         serviceHealth,
		ProfileProgress:       profileProgress,
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

	url := fmt.Sprintf("%s/api/v1/sandboxes/%s/heartbeat", apiURL, sandboxInstanceID)
	httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		log.Error().Err(err).Msg("Failed to create heartbeat request")
		return
	}

	httpReq.Header.Set("Authorization", "Bearer "+runnerToken)
	httpReq.Header.Set("Content-Type", "application/json")

	// Create HTTP client with insecure TLS (TODO: make configurable)
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}
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
