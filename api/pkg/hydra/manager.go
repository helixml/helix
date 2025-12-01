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
	DefaultDataDir = "/filestore/hydra"

	// DefaultDockerdTimeout is the timeout for dockerd to become ready
	DefaultDockerdTimeout = 30 * time.Second

	// DefaultStopTimeout is the timeout for graceful dockerd shutdown
	DefaultStopTimeout = 30 * time.Second
)

// Manager manages multiple dockerd instances
type Manager struct {
	socketDir    string
	dataDir      string
	instances    map[string]*DockerInstance
	mutex        sync.RWMutex
	stopChan     chan struct{}
	wg           sync.WaitGroup
	bridgeIndex  uint8 // Counter for unique bridge IP ranges (1-254)
	bridgeMutex  sync.Mutex
	usedBridges  map[uint8]string // Maps bridge index to scope key
}

// NewManager creates a new Hydra manager
func NewManager(socketDir, dataDir string) *Manager {
	if socketDir == "" {
		socketDir = DefaultSocketDir
	}
	if dataDir == "" {
		dataDir = DefaultDataDir
	}

	return &Manager{
		socketDir:   socketDir,
		dataDir:     dataDir,
		instances:   make(map[string]*DockerInstance),
		stopChan:    make(chan struct{}),
		bridgeIndex: 0, // Will start at 1 when first allocation
		usedBridges: make(map[uint8]string),
	}
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
	// We use "bridge": "none" to disable Docker's default bridge entirely.
	// Each dockerd creates its own network namespace with its own bridges.
	// This is the cleanest isolation - no shared bridges between instances.
	bridgeName := fmt.Sprintf("hydra%d", bridgeIndex)
	bridgeIP := fmt.Sprintf("10.200.%d.1/24", bridgeIndex)

	log.Info().
		Str("scope", scopeKey).
		Str("bridge", bridgeName).
		Str("bip", bridgeIP).
		Msg("Starting dockerd with unique bridge")

	// Write daemon.json with NVIDIA runtime support and unique IP range
	// Use only "bip" (bridge IP) - Docker will create its own docker0 bridge with this IP.
	// Each dockerd instance gets its own network namespace via unique exec-root,
	// so multiple docker0 bridges don't conflict.
	daemonConfig := fmt.Sprintf(`{
  "runtimes": {
    "nvidia": {
      "path": "nvidia-container-runtime",
      "runtimeArgs": []
    }
  },
  "storage-driver": "overlay2",
  "log-level": "warn",
  "bip": "%s",
  "fixed-cidr": "10.200.%d.0/24"
}`, bridgeIP, bridgeIndex)

	if err := os.WriteFile(configFile, []byte(daemonConfig), 0644); err != nil {
		m.releaseBridgeIndex(bridgeIndex)
		return nil, fmt.Errorf("failed to write daemon.json: %w", err)
	}

	// Clean up stale socket
	os.Remove(socketPath)

	// Start dockerd
	cmd := exec.Command("dockerd",
		"--host=unix://"+socketPath,
		"--data-root="+dataRoot,
		"--exec-root="+execRoot,
		"--pidfile="+pidFile,
		"--config-file="+configFile,
	)

	// Redirect output to log with prefix
	cmd.Stdout = &prefixWriter{prefix: fmt.Sprintf("[DOCKERD %s-%s] ", req.ScopeType, req.ScopeID[:8])}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
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
		m.releaseBridgeIndex(bridgeIndex)
		return nil, fmt.Errorf("dockerd failed to start: %w", err)
	}

	inst.Status = StatusRunning

	// Start process monitor goroutine
	go m.monitorProcess(inst, cmd)

	return inst, nil
}

// stopDockerd gracefully stops a dockerd process
func (m *Manager) stopDockerd(ctx context.Context, inst *DockerInstance) error {
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
	return &DockerInstanceResponse{
		ScopeType:    inst.ScopeType,
		ScopeID:      inst.ScopeID,
		DockerSocket: inst.SocketPath,
		DockerHost:   "unix://" + inst.SocketPath,
		DataRoot:     inst.DataRoot,
		Status:       inst.Status,
	}
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
