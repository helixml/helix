package hydra

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	// DefaultSocketDir is the runtime directory for active docker sockets
	DefaultSocketDir = "/var/run/hydra/active"

	// DefaultDataDir is the persistent directory for docker data
	// This path must be on a real volume, not overlay filesystem (overlay2 can't stack on overlay)
	DefaultDataDir = "/hydra-data"

	// DefaultDockerdTimeout is the timeout for dockerd to become ready
	DefaultDockerdTimeout = 30 * time.Second

	// DefaultStopTimeout is the timeout for graceful dockerd shutdown
	DefaultStopTimeout = 30 * time.Second
)

// Manager manages multiple dockerd instances
type Manager struct {
	socketDir             string
	dataDir               string
	instances             map[string]*DockerInstance
	mutex                 sync.RWMutex
	stopChan              chan struct{}
	wg                    sync.WaitGroup
	bridgeIndex           uint8 // Counter for unique bridge IP ranges (1-254)
	bridgeMutex           sync.Mutex
	usedBridges           map[uint8]string // Maps bridge index to scope key
	dnsServer             *DNSServer       // DNS server for container name resolution (optional)
	privilegedModeEnabled bool             // Whether privileged mode (host Docker) is enabled
}

// NewManager creates a new Hydra manager
func NewManager(socketDir, dataDir string) *Manager {
	if socketDir == "" {
		socketDir = DefaultSocketDir
	}
	if dataDir == "" {
		dataDir = DefaultDataDir
	}

	m := &Manager{
		socketDir:   socketDir,
		dataDir:     dataDir,
		instances:   make(map[string]*DockerInstance),
		stopChan:    make(chan struct{}),
		bridgeIndex: 0, // Will start at 1 when first allocation
		usedBridges: make(map[uint8]string),
	}

	// Recover bridge indices from existing network interfaces
	// This handles Hydra restart scenarios where bridges still exist
	m.recoverBridgeIndices()

	return m
}

// recoverBridgeIndices scans for existing hydra* bridges and marks those indices as used
// This prevents bridge index collisions after Hydra process restarts
func (m *Manager) recoverBridgeIndices() {
	output, err := exec.Command("ip", "-o", "link", "show", "type", "bridge").Output()
	if err != nil {
		log.Warn().Err(err).Msg("Failed to list bridges for recovery (may not be running as root)")
		return
	}

	lines := strings.Split(string(output), "\n")
	recoveredCount := 0
	for _, line := range lines {
		// Format: "3: hydra5: <BROADCAST,MULTICAST,UP> ..."
		if idx := strings.Index(line, "hydra"); idx != -1 {
			// Extract bridge index number
			var bridgeNum int
			n, err := fmt.Sscanf(line[idx:], "hydra%d", &bridgeNum)
			if err != nil || n != 1 {
				continue
			}
			if bridgeNum > 0 && bridgeNum < 255 {
				m.bridgeMutex.Lock()
				m.usedBridges[uint8(bridgeNum)] = "recovered"
				// Update bridgeIndex to be at least as high as recovered bridges
				if uint8(bridgeNum) >= m.bridgeIndex {
					m.bridgeIndex = uint8(bridgeNum)
				}
				m.bridgeMutex.Unlock()
				recoveredCount++
				log.Info().
					Int("bridge_index", bridgeNum).
					Str("bridge_name", fmt.Sprintf("hydra%d", bridgeNum)).
					Msg("Recovered existing bridge index")
			}
		}
	}

	if recoveredCount > 0 {
		log.Info().
			Int("count", recoveredCount).
			Msg("Recovered bridge indices from existing interfaces")
	}
}

// SetPrivilegedMode enables or disables privileged mode (host Docker access)
func (m *Manager) SetPrivilegedMode(enabled bool) {
	m.privilegedModeEnabled = enabled
}

// Start initializes the manager and starts background tasks
func (m *Manager) Start(ctx context.Context) error {
	// Create runtime directories
	if err := os.MkdirAll(m.socketDir, 0755); err != nil {
		return fmt.Errorf("failed to create socket directory: %w", err)
	}
	if err := os.MkdirAll(m.dataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	// Start cleanup goroutine
	m.wg.Add(1)
	go m.cleanupLoop(ctx)

	log.Info().
		Str("socket_dir", m.socketDir).
		Str("data_dir", m.dataDir).
		Msg("Hydra manager started")

	return nil
}

// Stop gracefully stops all dockerd instances
func (m *Manager) Stop(ctx context.Context) error {
	close(m.stopChan)

	m.mutex.Lock()
	instances := make([]*DockerInstance, 0, len(m.instances))
	for _, inst := range m.instances {
		instances = append(instances, inst)
	}
	m.mutex.Unlock()

	for _, inst := range instances {
		if err := m.stopDockerd(ctx, inst); err != nil {
			log.Error().Err(err).
				Str("scope_type", string(inst.ScopeType)).
				Str("scope_id", inst.ScopeID).
				Msg("Failed to stop dockerd during shutdown")
		}
	}

	m.wg.Wait()
	log.Info().Msg("Hydra manager stopped")
	return nil
}

// CreateInstance creates or resumes a dockerd instance for the given scope
func (m *Manager) CreateInstance(ctx context.Context, req *CreateDockerInstanceRequest) (*DockerInstanceResponse, error) {
	key := string(req.ScopeType) + "-" + req.ScopeID

	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Check if instance already running
	if inst, exists := m.instances[key]; exists {
		if inst.Status == StatusRunning {
			log.Info().
				Str("scope_type", string(req.ScopeType)).
				Str("scope_id", req.ScopeID).
				Msg("Reusing existing dockerd instance")
			return m.instanceToResponse(inst), nil
		}
	}

	// Create new instance
	inst, err := m.startDockerd(ctx, req)
	if err != nil {
		return nil, err
	}

	m.instances[key] = inst

	log.Info().
		Str("scope_type", string(req.ScopeType)).
		Str("scope_id", req.ScopeID).
		Str("socket", inst.SocketPath).
		Int("pid", inst.PID).
		Msg("Started new dockerd instance")

	return m.instanceToResponse(inst), nil
}

// DeleteInstance stops a dockerd instance (preserves data)
func (m *Manager) DeleteInstance(ctx context.Context, scopeType ScopeType, scopeID string) (*DeleteDockerInstanceResponse, error) {
	key := string(scopeType) + "-" + scopeID

	m.mutex.Lock()
	inst, exists := m.instances[key]
	if !exists {
		m.mutex.Unlock()
		return &DeleteDockerInstanceResponse{
			ScopeType:     scopeType,
			ScopeID:       scopeID,
			Status:        StatusStopped,
			DataPreserved: true,
		}, nil
	}
	delete(m.instances, key)
	m.mutex.Unlock()

	// Clean up desktop bridge veth if it exists
	if inst.VethBridgeName != "" {
		log.Info().
			Str("veth", inst.VethBridgeName).
			Str("scope_id", scopeID).
			Msg("Cleaning up desktop bridge veth on instance deletion")
		m.cleanupOrphanedVeth(inst.VethBridgeName)
	}

	containersStopped := 0
	if inst.Status == StatusRunning {
		// Count containers before stopping
		containersStopped = m.countContainers(inst)

		// Stop the dockerd
		if err := m.stopDockerd(ctx, inst); err != nil {
			log.Error().Err(err).
				Str("scope_type", string(scopeType)).
				Str("scope_id", scopeID).
				Msg("Error stopping dockerd")
		}
	}

	// Clean up runtime files (socket, pid file)
	m.cleanupRuntimeFiles(inst)

	log.Info().
		Str("scope_type", string(scopeType)).
		Str("scope_id", scopeID).
		Int("containers_stopped", containersStopped).
		Msg("Stopped dockerd instance (data preserved)")

	return &DeleteDockerInstanceResponse{
		ScopeType:         scopeType,
		ScopeID:           scopeID,
		Status:            StatusStopped,
		ContainersStopped: containersStopped,
		DataPreserved:     true,
	}, nil
}

// GetInstance returns the status of a dockerd instance
func (m *Manager) GetInstance(scopeType ScopeType, scopeID string) (*DockerInstanceStatusResponse, error) {
	key := string(scopeType) + "-" + scopeID

	m.mutex.RLock()
	inst, exists := m.instances[key]
	m.mutex.RUnlock()

	// Check if data exists even if not running
	dataRoot := m.getDataRoot(scopeType, scopeID)
	dataExists := false
	var dataSize int64
	if info, err := os.Stat(dataRoot); err == nil && info.IsDir() {
		dataExists = true
		dataSize = m.getDirSize(dataRoot)
	}

	if !exists {
		status := StatusStopped
		if !dataExists {
			// No instance and no data
			return nil, fmt.Errorf("instance not found: %s/%s", scopeType, scopeID)
		}
		// Data exists but not running
		return &DockerInstanceStatusResponse{
			ScopeType:     scopeType,
			ScopeID:       scopeID,
			Status:        status,
			DataRoot:      dataRoot,
			DataSizeBytes: dataSize,
		}, nil
	}

	return &DockerInstanceStatusResponse{
		ScopeType:      inst.ScopeType,
		ScopeID:        inst.ScopeID,
		Status:         inst.Status,
		ContainerCount: m.countContainers(inst),
		UptimeSeconds:  int64(time.Since(inst.StartedAt).Seconds()),
		DockerSocket:   inst.SocketPath,
		DataRoot:       inst.DataRoot,
		DataSizeBytes:  dataSize,
		UserID:         inst.UserID,
		CreatedAt:      inst.StartedAt,
	}, nil
}

// ListInstances returns all known instances
func (m *Manager) ListInstances() *ListDockerInstancesResponse {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	instances := make([]DockerInstanceStatusResponse, 0, len(m.instances))
	for _, inst := range m.instances {
		instances = append(instances, DockerInstanceStatusResponse{
			ScopeType:      inst.ScopeType,
			ScopeID:        inst.ScopeID,
			Status:         inst.Status,
			ContainerCount: m.countContainers(inst),
			UptimeSeconds:  int64(time.Since(inst.StartedAt).Seconds()),
			DockerSocket:   inst.SocketPath,
			DataRoot:       inst.DataRoot,
			UserID:         inst.UserID,
			CreatedAt:      inst.StartedAt,
		})
	}

	return &ListDockerInstancesResponse{Instances: instances}
}

// PurgeInstance stops dockerd and deletes all data
func (m *Manager) PurgeInstance(ctx context.Context, scopeType ScopeType, scopeID string) (*PurgeDockerInstanceResponse, error) {
	// First stop the instance
	_, err := m.DeleteInstance(ctx, scopeType, scopeID)
	if err != nil {
		return nil, err
	}

	// Delete data directory
	dataRoot := m.getDataRoot(scopeType, scopeID)
	dataSize := m.getDirSize(dataRoot)

	if err := os.RemoveAll(dataRoot); err != nil {
		return nil, fmt.Errorf("failed to delete data directory: %w", err)
	}

	log.Info().
		Str("scope_type", string(scopeType)).
		Str("scope_id", scopeID).
		Int64("data_deleted_bytes", dataSize).
		Msg("Purged dockerd instance data")

	return &PurgeDockerInstanceResponse{
		ScopeType:        scopeType,
		ScopeID:          scopeID,
		Status:           "purged",
		DataDeletedBytes: dataSize,
	}, nil
}

// allocateBridgeIndex allocates a unique bridge index (1-254) for a new dockerd
func (m *Manager) allocateBridgeIndex(scopeKey string) (uint8, error) {
	m.bridgeMutex.Lock()
	defer m.bridgeMutex.Unlock()

	// Try to find an unused index
	for i := uint8(1); i <= 254; i++ {
		if _, used := m.usedBridges[i]; !used {
			m.usedBridges[i] = scopeKey
			log.Debug().Uint8("index", i).Str("scope", scopeKey).Msg("Allocated bridge index")
			return i, nil
		}
	}
	return 0, fmt.Errorf("no available bridge indices (max 254 concurrent instances)")
}

// releaseBridgeIndex releases a bridge index when a dockerd stops
func (m *Manager) releaseBridgeIndex(index uint8) {
	m.bridgeMutex.Lock()
	defer m.bridgeMutex.Unlock()
	if scopeKey, ok := m.usedBridges[index]; ok {
		delete(m.usedBridges, index)
		log.Debug().Uint8("index", index).Str("scope", scopeKey).Msg("Released bridge index")
	}
}

// createBridge creates a Linux bridge interface with the specified name and IP
func (m *Manager) createBridge(bridgeName, bridgeIP string) error {
	// Check if bridge already exists
	checkCmd := exec.Command("ip", "link", "show", bridgeName)
	if err := checkCmd.Run(); err == nil {
		// Bridge exists, just ensure it's up and has the right IP
		log.Debug().Str("bridge", bridgeName).Msg("Bridge already exists, ensuring it's configured")
	} else {
		// Create the bridge
		if err := m.runCommand("ip", "link", "add", bridgeName, "type", "bridge"); err != nil {
			return fmt.Errorf("failed to create bridge: %w", err)
		}
	}

	// Set the IP address on the bridge
	// First, flush any existing IPs to avoid conflicts
	m.runCommand("ip", "addr", "flush", "dev", bridgeName)

	if err := m.runCommand("ip", "addr", "add", bridgeIP, "dev", bridgeName); err != nil {
		// IP might already be set, check if it's the right one
		log.Warn().Err(err).Str("bridge", bridgeName).Str("ip", bridgeIP).Msg("Failed to add IP (may already exist)")
	}

	// Bring up the bridge
	if err := m.runCommand("ip", "link", "set", bridgeName, "up"); err != nil {
		return fmt.Errorf("failed to bring up bridge: %w", err)
	}

	// Enable IP forwarding for this bridge (needed for container routing)
	m.runCommand("sysctl", "-w", "net.ipv4.ip_forward=1")

	log.Info().
		Str("bridge", bridgeName).
		Str("ip", bridgeIP).
		Msg("Created and configured bridge interface")

	return nil
}

// deleteBridge removes a Linux bridge interface
func (m *Manager) deleteBridge(bridgeName string) {
	// Bring down the bridge first
	m.runCommand("ip", "link", "set", bridgeName, "down")

	// Delete the bridge
	if err := m.runCommand("ip", "link", "del", bridgeName); err != nil {
		log.Warn().Err(err).Str("bridge", bridgeName).Msg("Failed to delete bridge (may not exist)")
	} else {
		log.Info().Str("bridge", bridgeName).Msg("Deleted bridge interface")
	}
}

// startDockerd starts a new dockerd process
func (m *Manager) startDockerd(ctx context.Context, req *CreateDockerInstanceRequest) (*DockerInstance, error) {
	instanceDir := filepath.Join(m.socketDir, string(req.ScopeType)+"-"+req.ScopeID)
	dataRoot := m.getDataRoot(req.ScopeType, req.ScopeID)
	execRoot := filepath.Join(instanceDir, "exec")
	socketPath := filepath.Join(instanceDir, "docker.sock")
	pidFile := filepath.Join(instanceDir, "docker.pid")
	configFile := filepath.Join(instanceDir, "daemon.json")
	scopeKey := string(req.ScopeType) + "-" + req.ScopeID

	// Create directories
	for _, dir := range []string{instanceDir, dataRoot, execRoot} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	// Allocate unique bridge index for this instance
	bridgeIndex, err := m.allocateBridgeIndex(scopeKey)
	if err != nil {
		return nil, fmt.Errorf("failed to allocate bridge: %w", err)
	}

	// Generate unique bridge name and IP range
	// Uses 10.200.X.0/24 where X is the bridgeIndex (1-254)
	// This avoids conflicts with:
	// - docker0: 172.17.0.0/16
	// - helix_default: 172.19.0.0/16 or 172.20.0.0/16
	//
	// We create a custom bridge (hydra1, hydra2, etc) and configure dockerd to use it.
	// This allows multiple dockerd instances to run in the same network namespace
	// without conflicting on docker0.
	bridgeName := fmt.Sprintf("hydra%d", bridgeIndex)
	bridgeIP := fmt.Sprintf("10.200.%d.1/24", bridgeIndex)
	bridgeSubnet := fmt.Sprintf("10.200.%d.0/24", bridgeIndex)

	log.Info().
		Str("scope", scopeKey).
		Str("bridge", bridgeName).
		Str("bridge_ip", bridgeIP).
		Msg("Creating bridge and starting dockerd")

	// Create the bridge interface if it doesn't exist
	if err := m.createBridge(bridgeName, bridgeIP); err != nil {
		m.releaseBridgeIndex(bridgeIndex)
		return nil, fmt.Errorf("failed to create bridge %s: %w", bridgeName, err)
	}

	// Write daemon.json with NVIDIA runtime support
	// We use "bridge": "none" because we created our own bridge
	// and will configure containers to use the default Docker network
	// which connects to our custom bridge via --bridge flag
	//
	// DNS: Point containers to Hydra DNS proxy running on the bridge gateway
	// Hydra DNS proxy (10.200.X.1:53) forwards to Docker's internal DNS (127.0.0.11)
	// which then forwards to the host's DNS (supporting enterprise internal DNS)
	gatewayIP := fmt.Sprintf("10.200.%d.1", bridgeIndex)
	dnsJSON := fmt.Sprintf(`["%s"]`, gatewayIP)

	daemonConfig := fmt.Sprintf(`{
  "runtimes": {
    "nvidia": {
      "path": "nvidia-container-runtime",
      "runtimeArgs": []
    }
  },
  "storage-driver": "overlay2",
  "log-level": "warn",
  "fixed-cidr": "%s",
  "dns": %s
}`, bridgeSubnet, dnsJSON)

	if err := os.WriteFile(configFile, []byte(daemonConfig), 0644); err != nil {
		m.deleteBridge(bridgeName)
		m.releaseBridgeIndex(bridgeIndex)
		return nil, fmt.Errorf("failed to write daemon.json: %w", err)
	}

	// Clean up stale socket
	os.Remove(socketPath)

	// Start dockerd with our custom bridge
	// --bridge specifies the bridge interface to use instead of docker0
	// Note: We don't use --bip because the bridge already has its IP assigned
	// via createBridge(). Using both --bridge and --bip is mutually exclusive.
	cmd := exec.Command("dockerd",
		"--host=unix://"+socketPath,
		"--data-root="+dataRoot,
		"--exec-root="+execRoot,
		"--pidfile="+pidFile,
		"--config-file="+configFile,
		"--bridge="+bridgeName,
	)

	// Redirect output to log with prefix
	cmd.Stdout = &prefixWriter{prefix: fmt.Sprintf("[DOCKERD %s-%s] ", req.ScopeType, req.ScopeID[:8])}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		m.deleteBridge(bridgeName)
		m.releaseBridgeIndex(bridgeIndex)
		return nil, fmt.Errorf("failed to start dockerd: %w", err)
	}

	inst := &DockerInstance{
		ScopeType:     req.ScopeType,
		ScopeID:       req.ScopeID,
		UserID:        req.UserID,
		Status:        StatusStarting,
		PID:           cmd.Process.Pid,
		SocketPath:    socketPath,
		DataRoot:      dataRoot,
		ExecRoot:      execRoot,
		PIDFile:       pidFile,
		ConfigFile:    configFile,
		MaxContainers: req.MaxContainers,
		StartedAt:     time.Now(),
		BridgeIndex:   bridgeIndex,
		BridgeName:    bridgeName,
	}

	// Wait for socket to be ready
	if err := m.waitForSocket(ctx, socketPath); err != nil {
		// Kill the process if it didn't start properly
		cmd.Process.Kill()
		m.deleteBridge(bridgeName)
		m.releaseBridgeIndex(bridgeIndex)
		return nil, fmt.Errorf("dockerd failed to start: %w", err)
	}

	inst.Status = StatusRunning

	// Start process monitor goroutine
	go m.monitorProcess(inst, cmd)

	// Start DNS server for this instance (if DNS is enabled)
	m.startDNSForInstance(inst)

	return inst, nil
}

// stopDockerd gracefully stops a dockerd process
func (m *Manager) stopDockerd(ctx context.Context, inst *DockerInstance) error {
	// Stop DNS server for this instance first
	m.stopDNSForInstance(inst)

	if inst.PID == 0 {
		return nil
	}

	process, err := os.FindProcess(inst.PID)
	if err != nil {
		return nil // Process already gone
	}

	// Send SIGTERM for graceful shutdown
	if err := process.Signal(syscall.SIGTERM); err != nil {
		log.Warn().Err(err).Int("pid", inst.PID).Msg("Failed to send SIGTERM")
	}

	// Wait for process to exit with timeout
	done := make(chan error, 1)
	go func() {
		_, err := process.Wait()
		done <- err
	}()

	select {
	case <-done:
		log.Debug().Int("pid", inst.PID).Msg("dockerd stopped gracefully")
	case <-time.After(DefaultStopTimeout):
		// Force kill
		log.Warn().Int("pid", inst.PID).Msg("dockerd did not stop gracefully, sending SIGKILL")
		process.Kill()
	case <-ctx.Done():
		process.Kill()
		return ctx.Err()
	}

	inst.Status = StatusStopped

	// Delete the bridge interface
	if inst.BridgeName != "" {
		m.deleteBridge(inst.BridgeName)
	}

	// Release the bridge index so it can be reused
	if inst.BridgeIndex > 0 {
		m.releaseBridgeIndex(inst.BridgeIndex)
	}

	return nil
}

// waitForSocket waits for the docker socket to become available
func (m *Manager) waitForSocket(ctx context.Context, socketPath string) error {
	deadline := time.Now().Add(DefaultDockerdTimeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Check if socket exists and is accessible
		if _, err := os.Stat(socketPath); err == nil {
			// Try to connect
			cmd := exec.Command("docker", "-H", "unix://"+socketPath, "info")
			if err := cmd.Run(); err == nil {
				return nil
			}
		}

		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("timeout waiting for docker socket: %s", socketPath)
}

// monitorProcess monitors a dockerd process and updates status on exit
func (m *Manager) monitorProcess(inst *DockerInstance, cmd *exec.Cmd) {
	err := cmd.Wait()
	exitCode := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		exitCode = exitErr.ExitCode()
	}

	m.mutex.Lock()
	if existing, exists := m.instances[inst.InstanceKey()]; exists && existing.PID == inst.PID {
		existing.Status = StatusStopped
		if exitCode != 0 {
			existing.Status = StatusError
		}
	}
	m.mutex.Unlock()

	// Release the bridge index so it can be reused
	if inst.BridgeIndex > 0 {
		m.releaseBridgeIndex(inst.BridgeIndex)
	}

	log.Info().
		Str("scope_type", string(inst.ScopeType)).
		Str("scope_id", inst.ScopeID).
		Int("pid", inst.PID).
		Int("exit_code", exitCode).
		Str("bridge", inst.BridgeName).
		Msg("dockerd process exited")
}

// cleanupRuntimeFiles removes socket, pid file, and exec root
func (m *Manager) cleanupRuntimeFiles(inst *DockerInstance) {
	os.Remove(inst.SocketPath)
	os.Remove(inst.PIDFile)
	os.Remove(inst.ConfigFile)
	os.RemoveAll(inst.ExecRoot)

	instanceDir := filepath.Dir(inst.SocketPath)
	os.Remove(instanceDir) // Remove dir if empty
}

// cleanupLoop periodically checks for orphaned dockerd processes
func (m *Manager) cleanupLoop(ctx context.Context) {
	defer m.wg.Done()

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopChan:
			return
		case <-ticker.C:
			m.cleanupOrphans()
		}
	}
}

// cleanupOrphans removes instances whose processes have died
func (m *Manager) cleanupOrphans() {
	// Collect bridge indices to release after unlocking main mutex
	var bridgesToRelease []uint8

	m.mutex.Lock()
	for key, inst := range m.instances {
		if inst.PID == 0 {
			continue
		}

		// Check if process is still running
		process, err := os.FindProcess(inst.PID)
		if err != nil {
			if inst.BridgeIndex > 0 {
				bridgesToRelease = append(bridgesToRelease, inst.BridgeIndex)
			}
			delete(m.instances, key)
			continue
		}

		// On Unix, FindProcess always succeeds, so we need to send signal 0
		if err := process.Signal(syscall.Signal(0)); err != nil {
			log.Info().
				Str("scope_type", string(inst.ScopeType)).
				Str("scope_id", inst.ScopeID).
				Int("pid", inst.PID).
				Str("bridge", inst.BridgeName).
				Msg("Cleaning up dead dockerd instance")
			m.cleanupRuntimeFiles(inst)
			if inst.BridgeIndex > 0 {
				bridgesToRelease = append(bridgesToRelease, inst.BridgeIndex)
			}
			delete(m.instances, key)
		}
	}
	m.mutex.Unlock()

	// Release bridge indices outside the main mutex to avoid lock ordering issues
	for _, idx := range bridgesToRelease {
		m.releaseBridgeIndex(idx)
	}
}

// countContainers returns the number of containers in a dockerd instance
func (m *Manager) countContainers(inst *DockerInstance) int {
	if inst.Status != StatusRunning {
		return 0
	}

	cmd := exec.Command("docker", "-H", "unix://"+inst.SocketPath, "ps", "-aq")
	output, err := cmd.Output()
	if err != nil {
		return 0
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return 0
	}
	return len(lines)
}

// getDataRoot returns the persistent data directory for a scope
func (m *Manager) getDataRoot(scopeType ScopeType, scopeID string) string {
	var subdir string
	switch scopeType {
	case ScopeTypeSpecTask:
		subdir = "spectasks"
	case ScopeTypeSession:
		subdir = "sessions"
	case ScopeTypeExploratory:
		subdir = "exploratory"
	default:
		subdir = "other"
	}
	return filepath.Join(m.dataDir, subdir, scopeID, "docker")
}

// getDirSize calculates the total size of a directory
func (m *Manager) getDirSize(path string) int64 {
	var size int64
	filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size
}

// instanceToResponse converts a DockerInstance to an API response
func (m *Manager) instanceToResponse(inst *DockerInstance) *DockerInstanceResponse {
	// Calculate network info from bridge index
	subnet := fmt.Sprintf("10.200.%d.0/24", inst.BridgeIndex)
	gateway := fmt.Sprintf("10.200.%d.1", inst.BridgeIndex)

	return &DockerInstanceResponse{
		ScopeType:    inst.ScopeType,
		ScopeID:      inst.ScopeID,
		DockerSocket: inst.SocketPath,
		DockerHost:   "unix://" + inst.SocketPath,
		DataRoot:     inst.DataRoot,
		Status:       inst.Status,
		BridgeName:   inst.BridgeName,
		Subnet:       subnet,
		Gateway:      gateway,
	}
}

// BridgeDesktop creates a veth pair to connect a desktop container (on Wolf's dockerd)
// to the Hydra network (on Hydra's dockerd) for the specified session.
// This enables the desktop's Firefox/Zed to access dev containers started via docker compose.
//
// Self-healing behavior:
// - If already bridged with same container, returns cached response
// - If container PID changed (restart), cleans up old bridge and creates new one
// - Retries container PID lookup if container not yet running
func (m *Manager) BridgeDesktop(ctx context.Context, req *BridgeDesktopRequest) (*BridgeDesktopResponse, error) {
	// Try all possible scope type prefixes since we don't know the exact type
	// The session ID is used as scope ID for all agent types
	m.mutex.Lock()
	var inst *DockerInstance
	for _, prefix := range []string{"session-", "spectask-", "exploratory-"} {
		testKey := prefix + req.SessionID
		if i, exists := m.instances[testKey]; exists {
			inst = i
			break
		}
	}
	m.mutex.Unlock()

	if inst == nil {
		// No Hydra instance found - check if we're in privileged mode
		if m.privilegedModeEnabled {
			log.Info().
				Str("session_id", req.SessionID).
				Msg("No Hydra instance (privileged mode) - bridging to host Docker network")
			return m.BridgeDesktopPrivileged(ctx, req)
		}
		return nil, fmt.Errorf("no Hydra Docker instance found for session %s", req.SessionID)
	}

	if inst.Status != StatusRunning {
		return nil, fmt.Errorf("Hydra Docker instance for session %s is not running (status: %s)", req.SessionID, inst.Status)
	}

	// Get bridge name and calculate IPs
	bridgeName := inst.BridgeName // e.g., "hydra3"
	bridgeIndex := inst.BridgeIndex
	gateway := fmt.Sprintf("10.200.%d.1", bridgeIndex)
	desktopIP := fmt.Sprintf("10.200.%d.254", bridgeIndex) // .254 for desktop
	subnet := fmt.Sprintf("10.200.%d.0/24", bridgeIndex)

	// Get desktop container's PID from Wolf's dockerd with retry
	// Wolf starts containers asynchronously, so we may need to wait
	var containerPID int
	var err error
	for attempt := 0; attempt < 10; attempt++ {
		containerPID, err = m.getContainerPID(req.DesktopContainerID)
		if err == nil && containerPID > 0 {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 500 * time.Millisecond):
			log.Debug().
				Str("container", req.DesktopContainerID).
				Int("attempt", attempt+1).
				Msg("Waiting for container to be running...")
		}
	}
	if err != nil || containerPID == 0 {
		return nil, fmt.Errorf("failed to get desktop container PID after retries: %w", err)
	}

	// Check if already bridged with same container
	m.mutex.Lock()
	if inst.DesktopBridged && inst.DesktopContainerID == req.DesktopContainerID && inst.DesktopPID == containerPID {
		m.mutex.Unlock()
		log.Debug().
			Str("session_id", req.SessionID).
			Str("container", req.DesktopContainerID).
			Msg("Desktop already bridged with same container, returning cached state")
		return &BridgeDesktopResponse{
			DesktopIP: desktopIP,
			Gateway:   gateway,
			Subnet:    subnet,
			Interface: "eth1",
		}, nil
	}

	// If PID changed (container restart), clean up old bridge first
	if inst.DesktopBridged && inst.DesktopPID != containerPID {
		log.Info().
			Str("session_id", req.SessionID).
			Int("old_pid", inst.DesktopPID).
			Int("new_pid", containerPID).
			Msg("Container PID changed (restart detected), cleaning up old bridge")
		if inst.VethBridgeName != "" {
			m.cleanupOrphanedVeth(inst.VethBridgeName)
		}
		inst.DesktopBridged = false
	}
	m.mutex.Unlock()

	// Generate unique veth names using bridge index (guaranteed unique per running instance)
	// Linux interface names are limited to 15 characters, so we use the bridge index
	// instead of session ID for guaranteed uniqueness within the sandbox
	vethDesktop := fmt.Sprintf("vethd-h%d", inst.BridgeIndex)
	vethBridge := fmt.Sprintf("vethb-h%d", inst.BridgeIndex)

	log.Info().
		Str("session_id", req.SessionID).
		Str("desktop_container_id", req.DesktopContainerID).
		Int("container_pid", containerPID).
		Str("bridge_name", bridgeName).
		Str("desktop_ip", desktopIP).
		Str("gateway", gateway).
		Str("veth_desktop", vethDesktop).
		Str("veth_bridge", vethBridge).
		Msg("Bridging desktop container to Hydra network")

	// Clean up any existing orphaned veths (in case of container restart or crash)
	m.cleanupOrphanedVeth(vethBridge)

	// 1. Create veth pair in sandbox namespace
	if err := m.runCommand("ip", "link", "add", vethDesktop, "type", "veth", "peer", "name", vethBridge); err != nil {
		return nil, fmt.Errorf("failed to create veth pair: %w", err)
	}

	// 2. Attach bridge-side veth to Hydra's bridge
	if err := m.runCommand("ip", "link", "set", vethBridge, "master", bridgeName); err != nil {
		m.runCommand("ip", "link", "del", vethDesktop)
		return nil, fmt.Errorf("failed to attach veth to bridge %s: %w", bridgeName, err)
	}

	// 3. Bring up the bridge-side veth
	if err := m.runCommand("ip", "link", "set", vethBridge, "up"); err != nil {
		m.runCommand("ip", "link", "del", vethDesktop)
		return nil, fmt.Errorf("failed to bring up bridge veth: %w", err)
	}

	// 4. Move desktop-side veth into container's network namespace
	if err := m.runCommand("ip", "link", "set", vethDesktop, "netns", fmt.Sprintf("%d", containerPID)); err != nil {
		m.runCommand("ip", "link", "del", vethBridge)
		return nil, fmt.Errorf("failed to move veth into container netns: %w", err)
	}

	// 5. Configure the interface inside the container using nsenter
	// Rename to eth1 (eth0 is Docker's default)
	if err := m.runNsenter(containerPID, "ip", "link", "set", vethDesktop, "name", "eth1"); err != nil {
		return nil, fmt.Errorf("failed to rename veth in container: %w", err)
	}

	// 6. Assign IP address
	if err := m.runNsenter(containerPID, "ip", "addr", "add", desktopIP+"/24", "dev", "eth1"); err != nil {
		return nil, fmt.Errorf("failed to assign IP to eth1: %w", err)
	}

	// 7. Bring up the interface
	if err := m.runNsenter(containerPID, "ip", "link", "set", "eth1", "up"); err != nil {
		return nil, fmt.Errorf("failed to bring up eth1: %w", err)
	}

	// 8. Add route to Hydra subnet via the new interface
	if err := m.runNsenter(containerPID, "ip", "route", "add", subnet, "dev", "eth1"); err != nil {
		// Route may already exist, just log warning
		log.Warn().Err(err).Str("subnet", subnet).Msg("Failed to add route (may already exist)")
	}

	// 9. Configure DNS by adding Hydra's DNS server to resolv.conf
	if err := m.configureDNS(containerPID, gateway); err != nil {
		log.Warn().Err(err).Msg("Failed to configure DNS (non-fatal)")
	}

	// 10. Configure localhost forwarding so `docker run -p 8080:8080` works
	// When user accesses localhost:8080 in desktop, forward to gateway:8080
	// where Docker actually binds the port
	if err := m.configureLocalhostForwarding(containerPID, gateway); err != nil {
		log.Warn().Err(err).Msg("Failed to configure localhost forwarding (non-fatal)")
	}

	// 11. Update bridge state for self-healing
	m.mutex.Lock()
	inst.DesktopBridged = true
	inst.DesktopContainerID = req.DesktopContainerID
	inst.DesktopPID = containerPID
	inst.VethBridgeName = vethBridge
	m.mutex.Unlock()

	log.Info().
		Str("session_id", req.SessionID).
		Str("desktop_ip", desktopIP).
		Str("gateway", gateway).
		Str("interface", "eth1").
		Msg("Successfully bridged desktop to Hydra network")

	return &BridgeDesktopResponse{
		DesktopIP: desktopIP,
		Gateway:   gateway,
		Subnet:    subnet,
		Interface: "eth1",
	}, nil
}

// BridgeDesktopPrivileged bridges a desktop container to the host Docker network
// This is used in privileged mode where dev containers run on the host's Docker
func (m *Manager) BridgeDesktopPrivileged(ctx context.Context, req *BridgeDesktopRequest) (*BridgeDesktopResponse, error) {
	// Get desktop container's PID from Wolf's dockerd with retry
	var containerPID int
	var err error
	for attempt := 0; attempt < 10; attempt++ {
		containerPID, err = m.getContainerPID(req.DesktopContainerID)
		if err == nil && containerPID > 0 {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 500 * time.Millisecond):
			log.Debug().
				Str("container", req.DesktopContainerID).
				Int("attempt", attempt+1).
				Msg("Waiting for container to be running (privileged mode)...")
		}
	}
	if err != nil || containerPID == 0 {
		return nil, fmt.Errorf("failed to get desktop container PID after retries: %w", err)
	}

	// Get host Docker network info
	// The sandbox container is on the host Docker network, so we can route through it
	hostGateway := m.getHostDockerGateway()
	if hostGateway == "" {
		hostGateway = "172.17.0.1" // Default docker0 gateway
	}

	// Generate unique veth names using container PID (unique at runtime)
	// Linux interface names are limited to 15 characters
	vethDesktop := fmt.Sprintf("vethd-p%d", containerPID)
	vethSandbox := fmt.Sprintf("veths-p%d", containerPID)

	// Use .253/.254 in the host Docker subnet to avoid conflicts
	desktopIP := "172.17.255.254"
	sandboxIP := "172.17.255.253"
	subnet := "172.17.0.0/16"

	log.Info().
		Str("session_id", req.SessionID).
		Str("desktop_container_id", req.DesktopContainerID).
		Int("container_pid", containerPID).
		Str("host_gateway", hostGateway).
		Str("desktop_ip", desktopIP).
		Msg("Bridging desktop to host Docker network (privileged mode)")

	// Clean up any existing orphaned veths
	m.cleanupOrphanedVeth(vethSandbox)

	// 1. Create veth pair in sandbox namespace
	if err := m.runCommand("ip", "link", "add", vethDesktop, "type", "veth", "peer", "name", vethSandbox); err != nil {
		return nil, fmt.Errorf("failed to create veth pair: %w", err)
	}

	// 2. Configure sandbox side (stays in sandbox namespace)
	if err := m.runCommand("ip", "addr", "add", sandboxIP+"/16", "dev", vethSandbox); err != nil {
		m.runCommand("ip", "link", "del", vethDesktop)
		return nil, fmt.Errorf("failed to configure sandbox veth: %w", err)
	}
	if err := m.runCommand("ip", "link", "set", vethSandbox, "up"); err != nil {
		m.runCommand("ip", "link", "del", vethDesktop)
		return nil, fmt.Errorf("failed to bring up sandbox veth: %w", err)
	}

	// 3. Move desktop-side veth into container's network namespace
	if err := m.runCommand("ip", "link", "set", vethDesktop, "netns", fmt.Sprintf("%d", containerPID)); err != nil {
		m.runCommand("ip", "link", "del", vethSandbox)
		return nil, fmt.Errorf("failed to move veth into container netns: %w", err)
	}

	// 4. Configure the interface inside the container
	if err := m.runNsenter(containerPID, "ip", "link", "set", vethDesktop, "name", "eth1"); err != nil {
		return nil, fmt.Errorf("failed to rename veth in container: %w", err)
	}
	// Use /32 prefix to avoid auto-creating a route for the entire /16 subnet
	// We'll add an explicit gateway route below
	if err := m.runNsenter(containerPID, "ip", "addr", "add", desktopIP+"/32", "dev", "eth1"); err != nil {
		return nil, fmt.Errorf("failed to assign IP to eth1: %w", err)
	}
	if err := m.runNsenter(containerPID, "ip", "link", "set", "eth1", "up"); err != nil {
		return nil, fmt.Errorf("failed to bring up eth1: %w", err)
	}

	// 5. Add point-to-point route to sandbox gateway first
	if err := m.runNsenter(containerPID, "ip", "route", "add", sandboxIP+"/32", "dev", "eth1"); err != nil {
		log.Warn().Err(err).Str("gateway", sandboxIP).Msg("Failed to add gateway route (may already exist)")
	}

	// 6. Add route to host Docker network via sandbox gateway
	if err := m.runNsenter(containerPID, "ip", "route", "add", subnet, "via", sandboxIP, "dev", "eth1"); err != nil {
		log.Warn().Err(err).Str("subnet", subnet).Msg("Failed to add route (may already exist)")
	}

	// 7. Enable IP forwarding in sandbox for routing
	m.runCommand("sysctl", "-w", "net.ipv4.ip_forward=1")

	// 8. Add iptables MASQUERADE for traffic going to host Docker network
	// This allows the desktop to reach containers on the host Docker network
	// First delete any existing rule to avoid duplicates (idempotent)
	m.runCommand("iptables", "-t", "nat", "-D", "POSTROUTING", "-s", desktopIP, "-o", "eth0", "-j", "MASQUERADE")
	// Now add the rule
	m.runCommand("iptables", "-t", "nat", "-A", "POSTROUTING", "-s", desktopIP, "-o", "eth0", "-j", "MASQUERADE")

	// 9. Configure DNS by adding sandbox's DNS server to resolv.conf
	// In privileged mode, we use the sandbox's gateway as DNS proxy
	// The sandbox can reach Docker's internal DNS or host DNS
	if err := m.configureDNS(containerPID, sandboxIP); err != nil {
		log.Warn().Err(err).Msg("Failed to configure DNS for privileged mode (non-fatal)")
	}

	// 10. Configure localhost forwarding so `docker run -p 8080:8080` works
	// Forward localhost:PORT to hostGateway:PORT (where Docker binds ports)
	if err := m.configureLocalhostForwarding(containerPID, hostGateway); err != nil {
		log.Warn().Err(err).Msg("Failed to configure localhost forwarding (non-fatal)")
	}

	log.Info().
		Str("session_id", req.SessionID).
		Str("desktop_ip", desktopIP).
		Str("host_gateway", hostGateway).
		Str("interface", "eth1").
		Msg("Successfully bridged desktop to host Docker network (privileged mode)")

	return &BridgeDesktopResponse{
		DesktopIP: desktopIP,
		Gateway:   hostGateway,
		Subnet:    subnet,
		Interface: "eth1",
	}, nil
}

// getHostDockerGateway returns the gateway IP for the host Docker network
func (m *Manager) getHostDockerGateway() string {
	// Check docker0 bridge gateway
	output, err := exec.Command("ip", "route", "show", "default").Output()
	if err != nil {
		return ""
	}

	// Parse "default via 172.17.0.1 dev eth0 ..."
	fields := strings.Fields(string(output))
	for i, field := range fields {
		if field == "via" && i+1 < len(fields) {
			return fields[i+1]
		}
	}
	return ""
}

// getContainerPID gets the PID of a container from Wolf's dockerd
func (m *Manager) getContainerPID(containerID string) (int, error) {
	// Use docker inspect to get the PID
	// Wolf's dockerd is at /var/run/docker.sock
	cmd := exec.Command("docker", "-H", "unix:///var/run/docker.sock",
		"inspect", "--format", "{{.State.Pid}}", containerID)
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("docker inspect failed: %w", err)
	}

	pidStr := strings.TrimSpace(string(output))
	var pid int
	if _, err := fmt.Sscanf(pidStr, "%d", &pid); err != nil {
		return 0, fmt.Errorf("failed to parse PID: %w", err)
	}

	if pid == 0 {
		return 0, fmt.Errorf("container not running (PID is 0)")
	}

	return pid, nil
}

// runCommand executes a command and returns error if it fails
func (m *Manager) runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s failed: %w (output: %s)", name, err, string(output))
	}
	return nil
}

// runNsenter executes a command inside a container's network namespace
func (m *Manager) runNsenter(pid int, args ...string) error {
	nsenterArgs := append([]string{"-t", fmt.Sprintf("%d", pid), "-n", "--"}, args...)
	cmd := exec.Command("nsenter", nsenterArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("nsenter failed: %w (output: %s)", err, string(output))
	}
	return nil
}

// cleanupOrphanedVeth removes orphaned veth interfaces from previous sessions
func (m *Manager) cleanupOrphanedVeth(vethName string) {
	// Check if veth exists
	cmd := exec.Command("ip", "link", "show", vethName)
	if err := cmd.Run(); err != nil {
		return // Doesn't exist, nothing to clean up
	}

	// Delete the orphaned veth
	log.Info().Str("veth", vethName).Msg("Cleaning up orphaned veth interface")
	exec.Command("ip", "link", "del", vethName).Run()
}

// configureDNS adds the Hydra DNS server to the container's resolv.conf
func (m *Manager) configureDNS(containerPID int, dnsServer string) error {
	// Read current resolv.conf via nsenter
	cmd := exec.Command("nsenter", "-t", fmt.Sprintf("%d", containerPID), "-m", "--", "cat", "/etc/resolv.conf")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to read resolv.conf: %w", err)
	}

	currentConf := string(output)

	// Check if our DNS server is already present
	if strings.Contains(currentConf, dnsServer) {
		log.Debug().Str("dns_server", dnsServer).Msg("DNS server already configured")
		return nil
	}

	// Prepend our nameserver to the existing config
	// This ensures container DNS queries try our server first
	newConf := fmt.Sprintf("nameserver %s\n%s", dnsServer, currentConf)

	// Write back via nsenter using echo and redirect
	// We use sh -c to handle the redirect
	cmd = exec.Command("nsenter", "-t", fmt.Sprintf("%d", containerPID), "-m", "--",
		"sh", "-c", fmt.Sprintf("echo '%s' > /etc/resolv.conf", newConf))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to write resolv.conf: %w", err)
	}

	log.Info().Str("dns_server", dnsServer).Msg("Added Hydra DNS server to container resolv.conf")
	return nil
}

// prefixWriter writes to stdout with a prefix
type prefixWriter struct {
	prefix string
}

func (w *prefixWriter) Write(p []byte) (n int, err error) {
	lines := strings.Split(string(p), "\n")
	for _, line := range lines {
		if line != "" {
			fmt.Printf("%s%s\n", w.prefix, line)
		}
	}
	return len(p), nil
}

// configureLocalhostForwarding sets up iptables DNAT rules so that connections
// to localhost:PORT in the desktop container are forwarded to gateway:PORT
// where Docker actually binds ports from `docker run -p PORT:PORT`.
//
// This enables the standard Docker developer experience:
//   docker run -p 8080:8080 nginx
//   curl localhost:8080  # Works!
func (m *Manager) configureLocalhostForwarding(containerPID int, gateway string) error {
	// Use iptables DNAT in the OUTPUT chain to redirect localhost → gateway
	// This catches outgoing connections to 127.0.0.1 and rewrites destination to gateway
	//
	// We only redirect TCP (HTTP/HTTPS/websockets) - UDP localhost is rare for dev servers
	// We exclude some well-known localhost-only ports to avoid breaking things:
	// - 6000-6063: X11 display
	// - 631: CUPS printing
	// - 53: Local DNS (though we configure DNS separately)

	// First, ensure iptables nat module is available and delete any existing rules
	// (idempotent - allows re-bridging without duplicate rules)
	m.runNsenter(containerPID, "iptables", "-t", "nat", "-D", "OUTPUT",
		"-o", "lo", "-d", "127.0.0.1", "-p", "tcp",
		"--dport", "1:5999", "-j", "DNAT", "--to-destination", gateway)
	m.runNsenter(containerPID, "iptables", "-t", "nat", "-D", "OUTPUT",
		"-o", "lo", "-d", "127.0.0.1", "-p", "tcp",
		"--dport", "6064:65535", "-j", "DNAT", "--to-destination", gateway)

	// Add DNAT rules for ports 1-5999 (below X11)
	if err := m.runNsenter(containerPID, "iptables", "-t", "nat", "-A", "OUTPUT",
		"-o", "lo", "-d", "127.0.0.1", "-p", "tcp",
		"--dport", "1:5999", "-j", "DNAT", "--to-destination", gateway); err != nil {
		return fmt.Errorf("failed to add localhost DNAT rule (1:5999): %w", err)
	}

	// Add DNAT rules for ports 6064-65535 (above X11)
	if err := m.runNsenter(containerPID, "iptables", "-t", "nat", "-A", "OUTPUT",
		"-o", "lo", "-d", "127.0.0.1", "-p", "tcp",
		"--dport", "6064:65535", "-j", "DNAT", "--to-destination", gateway); err != nil {
		return fmt.Errorf("failed to add localhost DNAT rule (6064:65535): %w", err)
	}

	log.Info().
		Str("gateway", gateway).
		Int("container_pid", containerPID).
		Msg("Configured localhost forwarding (localhost:PORT → gateway:PORT)")

	return nil
}

