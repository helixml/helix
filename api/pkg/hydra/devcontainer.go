package hydra

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-units"
	"github.com/rs/zerolog/log"
)

// DevContainerManager manages dev container lifecycle (Zed+agent environments)
// This manages container launching functionality
type DevContainerManager struct {
	// Docker client for the sandbox's dockerd (sandbox Docker or isolated Hydra dockerd)
	docker *client.Client

	// Parent Hydra manager for network and Docker instance access
	manager *Manager

	// Track active dev containers
	containers map[string]*DevContainer
	mu         sync.RWMutex
}

// NewDevContainerManager creates a new dev container manager
func NewDevContainerManager(manager *Manager) *DevContainerManager {
	return &DevContainerManager{
		manager:    manager,
		containers: make(map[string]*DevContainer),
	}
}

// getDockerClient returns a Docker client for the specified socket
// If socketPath is empty, uses the default Docker socket
func (dm *DevContainerManager) getDockerClient(socketPath string) (*client.Client, error) {
	if socketPath == "" {
		socketPath = "/var/run/docker.sock"
	}

	return client.NewClientWithOpts(
		client.WithHost("unix://"+socketPath),
		client.WithAPIVersionNegotiation(),
	)
}

// CreateDevContainer creates and starts a dev container
func (dm *DevContainerManager) CreateDevContainer(ctx context.Context, req *CreateDevContainerRequest) (*DevContainerResponse, error) {
	log.Info().
		Str("session_id", req.SessionID).
		Str("image", req.Image).
		Str("container_name", req.ContainerName).
		Str("container_type", string(req.ContainerType)).
		Str("gpu_vendor", req.GPUVendor).
		Msg("Creating dev container via Hydra")

	// Get Docker client for the specified socket
	dockerClient, err := dm.getDockerClient(req.DockerSocket)
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}
	defer dockerClient.Close()

	// Build container configuration
	containerConfig := &container.Config{
		Image:    req.Image,
		Hostname: req.Hostname,
		Env:      dm.buildEnv(req),
	}

	// Build host configuration
	hostConfig := dm.buildHostConfig(req)

	// Configure GPU passthrough
	dm.configureGPU(hostConfig, req.GPUVendor)

	// Network configuration is nil for host network mode
	// (host network mode shares the sandbox's network namespace, so no separate network config needed)
	var networkConfig *network.NetworkingConfig

	// Ensure mount source directories exist before creating container
	for _, m := range req.Mounts {
		// Skip socket files and runtime directories - they're not directories to create
		if m.Source == "" ||
			strings.HasPrefix(m.Source, "/run/") ||
			strings.HasPrefix(m.Source, "/var/run/") ||
			strings.HasSuffix(m.Source, ".sock") {
			continue
		}
		// Create the directory if it doesn't exist
		if err := os.MkdirAll(m.Source, 0755); err != nil {
			log.Warn().Err(err).Str("path", m.Source).Msg("Failed to create mount source directory")
		} else {
			log.Debug().Str("path", m.Source).Msg("Ensured mount source directory exists")
		}
	}

	// Create container
	resp, err := dockerClient.ContainerCreate(ctx, containerConfig, hostConfig, networkConfig, nil, req.ContainerName)
	if err != nil {
		return nil, fmt.Errorf("failed to create container: %w", err)
	}

	log.Info().
		Str("session_id", req.SessionID).
		Str("container_id", resp.ID).
		Str("container_name", req.ContainerName).
		Msg("Container created, starting...")

	// Start container
	if err := dockerClient.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		// Cleanup on failure
		dockerClient.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})
		return nil, fmt.Errorf("failed to start container: %w", err)
	}

	// Get container IP
	// For host network mode, the container shares the host's network namespace
	// so we use "host" to indicate this
	inspect, err := dockerClient.ContainerInspect(ctx, resp.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container: %w", err)
	}

	// With host network mode, container shares host network - use "host" as indicator
	ipAddress := "host"
	_ = inspect // Container info is available but IP is shared with host

	// Track container
	dc := &DevContainer{
		SessionID:     req.SessionID,
		ContainerID:   resp.ID,
		ContainerName: req.ContainerName,
		Status:        DevContainerStatusRunning,
		IPAddress:     ipAddress,
		ContainerType: req.ContainerType,
		UserID:        req.UserID,
		CreatedAt:     time.Now(),
		DockerSocket:  req.DockerSocket,
	}
	dm.mu.Lock()
	dm.containers[req.SessionID] = dc
	dm.mu.Unlock()

	log.Info().
		Str("session_id", req.SessionID).
		Str("container_id", resp.ID).
		Str("container_name", req.ContainerName).
		Str("ip_address", ipAddress).
		Msg("Dev container started successfully")

	return &DevContainerResponse{
		SessionID:     req.SessionID,
		ContainerID:   resp.ID,
		ContainerName: req.ContainerName,
		Status:        DevContainerStatusRunning,
		IPAddress:     ipAddress,
		ContainerType: req.ContainerType,
	}, nil
}

// buildEnv builds environment variables for the container
func (dm *DevContainerManager) buildEnv(req *CreateDevContainerRequest) []string {
	env := make([]string, len(req.Env))
	copy(env, req.Env)

	// Add display settings if this is not a headless container
	if req.ContainerType != DevContainerTypeHeadless {
		if req.DisplayWidth > 0 {
			env = append(env, fmt.Sprintf("GAMESCOPE_WIDTH=%d", req.DisplayWidth))
		}
		if req.DisplayHeight > 0 {
			env = append(env, fmt.Sprintf("GAMESCOPE_HEIGHT=%d", req.DisplayHeight))
		}
		if req.DisplayFPS > 0 {
			env = append(env, fmt.Sprintf("GAMESCOPE_REFRESH=%d", req.DisplayFPS))
		}
	}

	// Add GPU-specific environment variables
	switch req.GPUVendor {
	case "nvidia":
		// Check if already set
		hasVisibleDevices := false
		hasDriverCaps := false
		for _, e := range env {
			if strings.HasPrefix(e, "NVIDIA_VISIBLE_DEVICES=") {
				hasVisibleDevices = true
			}
			if strings.HasPrefix(e, "NVIDIA_DRIVER_CAPABILITIES=") {
				hasDriverCaps = true
			}
		}
		if !hasVisibleDevices {
			env = append(env, "NVIDIA_VISIBLE_DEVICES=all")
		}
		if !hasDriverCaps {
			env = append(env, "NVIDIA_DRIVER_CAPABILITIES=all")
		}
	}

	return env
}

// buildHostConfig builds the host configuration for the container
func (dm *DevContainerManager) buildHostConfig(req *CreateDevContainerRequest) *container.HostConfig {
	// Use the network from the request if specified, otherwise default to bridge.
	// Previously we used host network mode which caused port conflicts when running
	// multiple desktop containers (they all shared ports 9876/9877).
	// With bridge network, each container gets its own IP and can use the same ports.
	networkMode := container.NetworkMode(req.Network)
	if networkMode == "" {
		networkMode = "bridge"
	}

	hostConfig := &container.HostConfig{
		NetworkMode: networkMode,
		IpcMode:     "host",
		Privileged:  false,
		CapAdd:      []string{"SYS_ADMIN", "SYS_NICE", "SYS_PTRACE", "NET_RAW", "MKNOD", "NET_ADMIN"},
		SecurityOpt: []string{"seccomp=unconfined", "apparmor=unconfined"},
		Resources: container.Resources{
			DeviceCgroupRules: dm.getDeviceCgroupRules(),
			Ulimits: []*units.Ulimit{
				{Name: "nofile", Soft: 65536, Hard: 65536},
			},
		},
	}

	// Add ExtraHosts so the container can resolve "api" hostname.
	// The container is on bridge network but needs to reach the API
	// on the helix_* Docker network. We resolve "api" from the sandbox's
	// perspective and add it as an extra host entry.
	hostConfig.ExtraHosts = dm.buildExtraHosts()

	// Build mounts
	hostConfig.Mounts = dm.buildMounts(req)

	return hostConfig
}

// buildMounts builds the mount configuration
func (dm *DevContainerManager) buildMounts(req *CreateDevContainerRequest) []mount.Mount {
	var mounts []mount.Mount

	for _, m := range req.Mounts {
		mounts = append(mounts, mount.Mount{
			Type:     mount.TypeBind,
			Source:   m.Source,
			Target:   m.Destination,
			ReadOnly: m.ReadOnly,
		})
	}

	return mounts
}

// buildExtraHosts resolves hostnames that the container needs to reach
// and returns them as Docker ExtraHosts entries (format: "hostname:ip").
// This is needed because containers on bridge network can't resolve
// the "api" hostname which lives on the helix_* Docker Compose network.
func (dm *DevContainerManager) buildExtraHosts() []string {
	var extraHosts []string

	// Resolve "api" hostname from the sandbox's perspective
	// The sandbox is connected to the helix network and can resolve "api"
	ips, err := net.LookupHost("api")
	if err == nil && len(ips) > 0 {
		apiIP := ips[0]
		extraHosts = append(extraHosts, "api:"+apiIP)
		log.Debug().Str("api_ip", apiIP).Msg("Added API host entry for dev container")
	} else {
		// Fallback: try common Docker network gateway patterns
		// In Docker Compose, the API is typically on 172.19.0.x
		log.Warn().Err(err).Msg("Could not resolve 'api' hostname, container may not connect to API")
	}

	return extraHosts
}

// configureGPU adds GPU-specific Docker configuration
func (dm *DevContainerManager) configureGPU(hostConfig *container.HostConfig, vendor string) {
	switch vendor {
	case "nvidia":
		// NVIDIA: use nvidia-container-runtime
		hostConfig.Runtime = "nvidia"
		hostConfig.DeviceRequests = []container.DeviceRequest{
			{
				DeviceIDs:    []string{"all"},
				Capabilities: [][]string{{"gpu"}},
			},
		}
		log.Debug().Msg("Configured NVIDIA GPU passthrough")

	case "amd":
		// AMD: mount /dev/kfd and /dev/dri/*
		hostConfig.Devices = append(hostConfig.Devices,
			container.DeviceMapping{
				PathOnHost:        "/dev/kfd",
				PathInContainer:   "/dev/kfd",
				CgroupPermissions: "rwm",
			},
		)
		// DRI devices are handled via GOW_REQUIRED_DEVICES env var in the container
		log.Debug().Msg("Configured AMD GPU passthrough")

	case "intel":
		// Intel: mount /dev/dri/* (handled via GOW_REQUIRED_DEVICES in container)
		log.Debug().Msg("Configured Intel GPU passthrough (via env var)")

	default:
		// Software rendering - no special config needed
		log.Debug().Msg("No GPU passthrough configured (software rendering)")
	}
}

// getDeviceCgroupRules returns cgroup rules for hidraw and input devices
func (dm *DevContainerManager) getDeviceCgroupRules() []string {
	// Read major numbers from /proc/devices
	hidrawMajor := dm.getDeviceMajor("hidraw")
	inputMajor := dm.getDeviceMajor("input")

	var rules []string
	if hidrawMajor != "" {
		rules = append(rules, fmt.Sprintf("c %s:* rwm", hidrawMajor))
	}
	if inputMajor != "" {
		rules = append(rules, fmt.Sprintf("c %s:* rwm", inputMajor))
	}

	// Add default rules if we couldn't read from /proc/devices
	if len(rules) == 0 {
		// Default major numbers for hidraw (244) and input (13)
		rules = []string{"c 13:* rmw", "c 244:* rmw"}
	}

	return rules
}

// getDeviceMajor returns the major number for a device type from /proc/devices
func (dm *DevContainerManager) getDeviceMajor(deviceType string) string {
	file, err := os.Open("/proc/devices")
	if err != nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, deviceType) {
			// Example line: "244 hidraw" or " 13 input"
			line = strings.TrimLeft(line, " ")
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				return parts[0]
			}
		}
	}

	return ""
}

// DeleteDevContainer stops and removes a dev container
func (dm *DevContainerManager) DeleteDevContainer(ctx context.Context, sessionID string) (*DevContainerResponse, error) {
	dm.mu.RLock()
	dc, exists := dm.containers[sessionID]
	dm.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("dev container not found for session: %s", sessionID)
	}

	log.Info().
		Str("session_id", sessionID).
		Str("container_id", dc.ContainerID).
		Msg("Stopping dev container")

	// Get Docker client
	dockerClient, err := dm.getDockerClient(dc.DockerSocket)
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}
	defer dockerClient.Close()

	// Stop container with timeout
	timeout := 10
	if err := dockerClient.ContainerStop(ctx, dc.ContainerID, container.StopOptions{Timeout: &timeout}); err != nil {
		log.Warn().Err(err).Str("container_id", dc.ContainerID).Msg("Failed to stop container gracefully")
	}

	// Remove container
	if err := dockerClient.ContainerRemove(ctx, dc.ContainerID, container.RemoveOptions{Force: true}); err != nil {
		log.Warn().Err(err).Str("container_id", dc.ContainerID).Msg("Failed to remove container")
	}

	// Update status
	dm.mu.Lock()
	dc.Status = DevContainerStatusStopped
	delete(dm.containers, sessionID)
	dm.mu.Unlock()

	log.Info().
		Str("session_id", sessionID).
		Str("container_id", dc.ContainerID).
		Msg("Dev container stopped and removed")

	return &DevContainerResponse{
		SessionID:     sessionID,
		ContainerID:   dc.ContainerID,
		ContainerName: dc.ContainerName,
		Status:        DevContainerStatusStopped,
		ContainerType: dc.ContainerType,
	}, nil
}

// GetDevContainer returns the status of a dev container
func (dm *DevContainerManager) GetDevContainer(ctx context.Context, sessionID string) (*DevContainerResponse, error) {
	dm.mu.RLock()
	dc, exists := dm.containers[sessionID]
	dm.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("dev container not found for session: %s", sessionID)
	}

	// Optionally refresh status from Docker
	dockerClient, err := dm.getDockerClient(dc.DockerSocket)
	if err == nil {
		defer dockerClient.Close()
		inspect, err := dockerClient.ContainerInspect(ctx, dc.ContainerID)
		if err == nil {
			if inspect.State.Running {
				dc.Status = DevContainerStatusRunning
			} else {
				dc.Status = DevContainerStatusStopped
			}
		}
	}

	return &DevContainerResponse{
		SessionID:     dc.SessionID,
		ContainerID:   dc.ContainerID,
		ContainerName: dc.ContainerName,
		Status:        dc.Status,
		IPAddress:     dc.IPAddress,
		ContainerType: dc.ContainerType,
	}, nil
}

// ListDevContainers returns all active dev containers
func (dm *DevContainerManager) ListDevContainers() *ListDevContainersResponse {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	containers := make([]DevContainerResponse, 0, len(dm.containers))
	for _, dc := range dm.containers {
		containers = append(containers, DevContainerResponse{
			SessionID:     dc.SessionID,
			ContainerID:   dc.ContainerID,
			ContainerName: dc.ContainerName,
			Status:        dc.Status,
			IPAddress:     dc.IPAddress,
			ContainerType: dc.ContainerType,
		})
	}

	return &ListDevContainersResponse{
		Containers: containers,
	}
}

// FindDevContainerBySessionID finds a dev container by session ID
// Returns nil if not found (does not return error for not found)
func (dm *DevContainerManager) FindDevContainerBySessionID(sessionID string) *DevContainer {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	return dm.containers[sessionID]
}

// RecoverDevContainersFromDocker recovers dev container state from running Docker containers
// This is called on Hydra startup to recover state after restarts
func (dm *DevContainerManager) RecoverDevContainersFromDocker(ctx context.Context, dockerSocket string) error {
	dockerClient, err := dm.getDockerClient(dockerSocket)
	if err != nil {
		return fmt.Errorf("failed to create Docker client: %w", err)
	}
	defer dockerClient.Close()

	// List all running containers
	containers, err := dockerClient.ContainerList(ctx, container.ListOptions{All: false})
	if err != nil {
		return fmt.Errorf("failed to list containers: %w", err)
	}

	recoveredCount := 0
	for _, c := range containers {
		// Check if this looks like a Helix dev container
		// Container names are like "/sway-external-ses_xxx" or "/ubuntu-external-ses_xxx"
		for _, name := range c.Names {
			name = strings.TrimPrefix(name, "/")
			if strings.Contains(name, "-external-") {
				// Extract session ID from container name
				// Format: {type}-external-{session_id_suffix}
				parts := strings.Split(name, "-external-")
				if len(parts) == 2 {
					sessionIDSuffix := parts[1]
					sessionID := "ses_" + sessionIDSuffix

					// Determine container type from prefix
					containerType := DevContainerTypeSway
					if strings.HasPrefix(name, "ubuntu") {
						containerType = DevContainerTypeUbuntu
					}

					// Get container IP
					ipAddress := ""
					for _, net := range c.NetworkSettings.Networks {
						ipAddress = net.IPAddress
						break
					}

					dc := &DevContainer{
						SessionID:     sessionID,
						ContainerID:   c.ID,
						ContainerName: name,
						Status:        DevContainerStatusRunning,
						IPAddress:     ipAddress,
						ContainerType: containerType,
						CreatedAt:     time.Unix(c.Created, 0),
						DockerSocket:  dockerSocket,
					}

					dm.mu.Lock()
					dm.containers[sessionID] = dc
					dm.mu.Unlock()

					recoveredCount++
					log.Info().
						Str("session_id", sessionID).
						Str("container_id", c.ID[:12]).
						Str("container_name", name).
						Msg("Recovered dev container from Docker")
				}
			}
		}
	}

	if recoveredCount > 0 {
		log.Info().Int("count", recoveredCount).Msg("Recovered dev containers from Docker")
	}

	return nil
}

