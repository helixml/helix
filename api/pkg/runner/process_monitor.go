package runner

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// ProcessTracker tracks processes associated with model instances
type ProcessTracker struct {
	mu            sync.RWMutex
	trackedPIDs   map[int]ProcessInfo // PID -> ProcessInfo
	slotProcesses map[uuid.UUID][]int // SlotID -> []PID
	ctx           context.Context
	cancel        context.CancelFunc
	cleanupMu     sync.Mutex // Prevents concurrent cleanup runs
	isRunning     bool       // Tracks if cleanup is currently running

	// Cleanup statistics
	cleanupStats   CleanupStats
	cleanupStatsMu sync.RWMutex
}

// ProcessInfo contains information about a tracked process
type ProcessInfo struct {
	PID       int
	SlotID    uuid.UUID
	ModelName string
	Command   string
	StartTime time.Time
}

// CleanedProcess represents a process that was cleaned up
type CleanedProcess struct {
	PID       int       `json:"pid"`
	Command   string    `json:"command"`
	CleanedAt time.Time `json:"cleaned_at"`
	Method    string    `json:"method"` // "graceful" or "force"
}

// CleanupStats tracks statistics about orphan process cleanup
type CleanupStats struct {
	TotalCleaned     int              `json:"total_cleaned"`
	LastCleanupTime  *time.Time       `json:"last_cleanup_time,omitempty"`
	RecentCleanups   []CleanedProcess `json:"recent_cleanups"` // Last 50 cleanups
	SynchronousRuns  int              `json:"synchronous_runs"`
	AsynchronousRuns int              `json:"asynchronous_runs"`
	ConcurrentSkips  int              `json:"concurrent_skips"`
}

// NewProcessTracker creates a new process tracker
func NewProcessTracker(ctx context.Context) *ProcessTracker {
	ctx, cancel := context.WithCancel(ctx)
	return &ProcessTracker{
		trackedPIDs:   make(map[int]ProcessInfo),
		slotProcesses: make(map[uuid.UUID][]int),
		ctx:           ctx,
		cancel:        cancel,
		cleanupStats: CleanupStats{
			RecentCleanups: make([]CleanedProcess, 0, 50),
		},
	}
}

// RegisterProcess registers a process as being tracked for a specific slot
func (pt *ProcessTracker) RegisterProcess(pid int, slotID uuid.UUID, modelName, command string) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	info := ProcessInfo{
		PID:       pid,
		SlotID:    slotID,
		ModelName: modelName,
		Command:   command,
		StartTime: time.Now(),
	}

	pt.trackedPIDs[pid] = info
	pt.slotProcesses[slotID] = append(pt.slotProcesses[slotID], pid)

	log.Info().
		Int("pid", pid).
		Str("slot_id", slotID.String()).
		Str("model", modelName).
		Msg("PROCESS_TRACKER: Registered process")
}

// UnregisterSlot removes all processes associated with a slot
func (pt *ProcessTracker) UnregisterSlot(slotID uuid.UUID) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	pids, exists := pt.slotProcesses[slotID]
	if !exists {
		return
	}

	for _, pid := range pids {
		delete(pt.trackedPIDs, pid)
	}
	delete(pt.slotProcesses, slotID)

	log.Info().
		Str("slot_id", slotID.String()).
		Interface("pids", pids).
		Msg("PROCESS_TRACKER: Unregistered slot processes")
}

// GetTrackedProcesses returns all currently tracked processes
func (pt *ProcessTracker) GetTrackedProcesses() map[int]ProcessInfo {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	result := make(map[int]ProcessInfo)
	for pid, info := range pt.trackedPIDs {
		result[pid] = info
	}
	return result
}

// StartOrphanMonitor starts the orphan process monitoring loop
func (pt *ProcessTracker) StartOrphanMonitor() {
	go pt.orphanMonitorLoop()
}

// Stop stops the process tracker
func (pt *ProcessTracker) Stop() {
	pt.cancel()
}

// orphanMonitorLoop runs the orphan process detection and cleanup
func (pt *ProcessTracker) orphanMonitorLoop() {
	ticker := time.NewTicker(30 * time.Second) // Check every 30 seconds
	defer ticker.Stop()

	log.Info().Msg("ORPHAN_MONITOR: Started orphan process monitoring")

	for {
		select {
		case <-pt.ctx.Done():
			log.Info().Msg("ORPHAN_MONITOR: Stopping orphan monitor")
			return
		case <-ticker.C:
			_ = pt.scanForOrphans() // Ignore return value for periodic cleanup
		}
	}
}

// RunSynchronousCleanup runs orphan cleanup synchronously (called after slot deletion)
func (pt *ProcessTracker) RunSynchronousCleanup() {
	log.Info().Msg("PROCESS_CLEANUP: Starting synchronous orphan cleanup after slot deletion")

	totalOrphansFound := 0

	// Run cleanup multiple times until stable
	for attempt := 1; attempt <= 3; attempt++ {
		log.Info().Int("attempt", attempt).Msg("PROCESS_CLEANUP: Cleanup attempt")

		orphansFound := pt.scanForOrphansInternal(true) // true = synchronous
		totalOrphansFound += orphansFound

		if orphansFound == 0 {
			log.Info().Int("attempt", attempt).Msg("PROCESS_CLEANUP: No orphans found, cleanup complete")
			break
		}

		if attempt < 3 {
			// Brief pause between attempts to let processes settle
			time.Sleep(1 * time.Second)
		}
	}

	log.Info().
		Int("total_orphans_cleaned", totalOrphansFound).
		Msg("PROCESS_CLEANUP: Synchronous cleanup completed")
}

// scanForOrphans scans for and cleans up orphaned processes using graph analysis
// Returns the number of orphans found and cleaned (asynchronous call)
func (pt *ProcessTracker) scanForOrphans() int {
	return pt.scanForOrphansInternal(false) // false = asynchronous
}

// scanForOrphansInternal scans for and cleans up orphaned processes using graph analysis
// Returns the number of orphans found and cleaned
func (pt *ProcessTracker) scanForOrphansInternal(synchronous bool) int {
	// Prevent concurrent cleanup runs
	pt.cleanupMu.Lock()
	defer pt.cleanupMu.Unlock()

	if pt.isRunning {
		pt.recordCleanupRun(synchronous, true) // skipped = true
		log.Debug().Msg("ORPHAN_MONITOR: Cleanup already running, skipping")
		return 0
	}

	pt.recordCleanupRun(synchronous, false) // skipped = false

	pt.isRunning = true
	defer func() {
		pt.isRunning = false
	}()

	log.Debug().Msg("ORPHAN_MONITOR: Scanning for orphaned processes")

	// Build the complete process tree
	tree, err := pt.buildProcessTree()
	if err != nil {
		log.Error().Err(err).Msg("ORPHAN_MONITOR: Failed to build process tree")
		return 0
	}

	// Find all model processes (VLLM and Ollama) on the system
	modelProcesses, err := pt.findModelProcesses()
	if err != nil {
		log.Error().Err(err).Msg("ORPHAN_MONITOR: Failed to find model processes")
		return 0
	}

	pt.mu.RLock()
	trackedCount := len(pt.trackedPIDs)
	pt.mu.RUnlock()

	// Use graph analysis to determine which processes are truly orphaned
	var trueOrphans []int
	var connectedProcesses []int

	for _, pid := range modelProcesses {
		if pt.isProcessOrphaned(pid, tree) {
			trueOrphans = append(trueOrphans, pid)
		} else {
			connectedProcesses = append(connectedProcesses, pid)
		}
	}

	log.Debug().
		Interface("connected_model_pids", connectedProcesses).
		Interface("orphaned_model_pids", trueOrphans).
		Int("total_model_processes", len(modelProcesses)).
		Int("tracked_processes", trackedCount).
		Msg("ORPHAN_MONITOR: Process tree analysis complete")

	if len(trueOrphans) > 0 {
		log.Warn().
			Interface("true_orphan_pids", trueOrphans).
			Interface("connected_pids", connectedProcesses).
			Int("total_model_processes", len(modelProcesses)).
			Int("tracked_processes", trackedCount).
			Msg("ORPHAN_MONITOR: Found genuinely orphaned model processes")

		pt.cleanupOrphans(trueOrphans, tree)
		return len(trueOrphans)
	} else {
		log.Debug().
			Int("total_model_processes", len(modelProcesses)).
			Int("connected_processes", len(connectedProcesses)).
			Int("tracked_processes", trackedCount).
			Msg("ORPHAN_MONITOR: No orphaned processes found - all model processes are connected to tracked parents")
		return 0
	}
}

// ProcessTreeNode represents a node in the process tree
type ProcessTreeNode struct {
	PID      int
	PPID     int
	Command  string
	Children []*ProcessTreeNode
	Parent   *ProcessTreeNode
}

// ProcessTree represents the complete process tree
type ProcessTree struct {
	Nodes map[int]*ProcessTreeNode
	Roots []*ProcessTreeNode
}

// buildProcessTree builds the complete process tree for the system
func (pt *ProcessTracker) buildProcessTree() (*ProcessTree, error) {
	// Get all processes with PID, PPID, and command
	cmd := exec.Command("ps", "axo", "pid,ppid,command")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run ps command: %w", err)
	}

	lines := strings.Split(string(output), "\n")
	nodes := make(map[int]*ProcessTreeNode)

	// Parse all processes
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "PID") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}

		ppid, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}

		command := strings.Join(fields[2:], " ")

		nodes[pid] = &ProcessTreeNode{
			PID:      pid,
			PPID:     ppid,
			Command:  command,
			Children: make([]*ProcessTreeNode, 0),
		}
	}

	// Build parent-child relationships
	var roots []*ProcessTreeNode
	for _, node := range nodes {
		if parent, exists := nodes[node.PPID]; exists && node.PPID != node.PID {
			// Has a parent in our tree
			node.Parent = parent
			parent.Children = append(parent.Children, node)
		} else {
			// Root node (no parent or parent is itself)
			roots = append(roots, node)
		}
	}

	return &ProcessTree{
		Nodes: nodes,
		Roots: roots,
	}, nil
}

// findModelProcesses finds all VLLM and Ollama processes and analyzes their connectivity
func (pt *ProcessTracker) findModelProcesses() ([]int, error) {
	tree, err := pt.buildProcessTree()
	if err != nil {
		return nil, fmt.Errorf("failed to build process tree: %w", err)
	}

	// Find all VLLM and Ollama processes
	// Pattern 1: Direct VLLM processes
	vllmRegex := regexp.MustCompile(`(?i)(vllm|python.*vllm|uvicorn.*vllm)`)
	// Pattern 2: VLLM worker processes (multiprocessing children) - matches both CUDA and ROCm venvs
	vllmWorkerRegex := regexp.MustCompile(`(?i)(/workspace/vllm(-cuda|-rocm)?/venv/bin/python)`)
	// Pattern 3: Ollama processes
	ollamaRegex := regexp.MustCompile(`(?i)(/usr/bin/ollama|ollama\s+(serve|runner))`)

	var modelProcessPIDs []int

	for pid, node := range tree.Nodes {
		isModelProcess := false
		processType := ""

		if vllmRegex.MatchString(node.Command) {
			isModelProcess = true
			processType = "vllm-main"
		} else if vllmWorkerRegex.MatchString(node.Command) {
			isModelProcess = true
			processType = "vllm-worker"
		} else if ollamaRegex.MatchString(node.Command) {
			isModelProcess = true
			processType = "ollama"
		}

		if isModelProcess {
			modelProcessPIDs = append(modelProcessPIDs, pid)
			log.Debug().
				Int("pid", pid).
				Str("type", processType).
				Str("command", node.Command).
				Msg("PROCESS_TREE: Found model process")
		}
	}

	log.Debug().
		Interface("model_process_pids", modelProcessPIDs).
		Int("total_processes", len(tree.Nodes)).
		Msg("PROCESS_TREE: Found model processes in system")

	return modelProcessPIDs, nil
}

// isProcessOrphaned checks if a process is truly orphaned (disconnected from tracked processes)
func (pt *ProcessTracker) isProcessOrphaned(pid int, tree *ProcessTree) bool {
	node, exists := tree.Nodes[pid]
	if !exists {
		return false // Process doesn't exist
	}

	// Safety check: Never kill critical system processes or their close descendants
	if pt.isSystemCriticalProcess(node, tree) {
		log.Warn().
			Int("pid", pid).
			Str("command", node.Command).
			Msg("PROCESS_TREE: Refusing to kill system-critical process or its close descendant")
		return false
	}

	// Grace period check: Don't kill recently spawned processes to avoid race conditions
	// during VLLM/model startup where workers are spawning rapidly
	if pt.isProcessTooYoung(pid) {
		log.Debug().
			Int("pid", pid).
			Str("command", node.Command).
			Msg("PROCESS_TREE: Process too young, skipping orphan check")
		return false
	}

	// Check if this process or any of its ancestors are tracked
	current := node
	visitedPIDs := make(map[int]bool) // Prevent infinite loops

	for current != nil && !visitedPIDs[current.PID] {
		visitedPIDs[current.PID] = true

		pt.mu.RLock()
		_, isTracked := pt.trackedPIDs[current.PID]
		pt.mu.RUnlock()

		if isTracked {
			// Found a tracked ancestor - this process is NOT orphaned
			log.Debug().
				Int("orphan_candidate_pid", pid).
				Int("tracked_ancestor_pid", current.PID).
				Msg("PROCESS_TREE: Process has tracked ancestor, not orphaned")
			return false
		}

		// Move up to parent
		current = current.Parent
	}

	// No tracked ancestors found - this is potentially orphaned
	// But let's also check if any children are tracked (shouldn't happen but be safe)
	if pt.hasTrackedDescendants(node) {
		log.Debug().
			Int("orphan_candidate_pid", pid).
			Msg("PROCESS_TREE: Process has tracked descendants, not orphaned")
		return false
	}

	log.Debug().
		Int("orphan_pid", pid).
		Str("command", node.Command).
		Int("ppid", node.PPID).
		Msg("PROCESS_TREE: Confirmed orphaned process")

	return true
}

// isSystemCriticalProcess checks if a process is critical to system operation
func (pt *ProcessTracker) isSystemCriticalProcess(node *ProcessTreeNode, tree *ProcessTree) bool {
	// Define critical system processes and patterns
	criticalProcesses := []string{
		"init", "kernel", "kthreadd", "systemd", "dockerd", "containerd",
		"kubelet", "docker-proxy", "systemd-", "/sbin/", "/usr/sbin/",
	}

	// Check the process itself
	for _, critical := range criticalProcesses {
		if strings.Contains(strings.ToLower(node.Command), critical) {
			return true
		}
	}

	// Check if it's too close to PID 1 (init) - within 3 levels
	current := node
	depth := 0
	maxDepth := 3

	for current != nil && depth < maxDepth {
		if current.PID == 1 || current.PPID == 1 {
			log.Debug().
				Int("pid", node.PID).
				Int("depth_from_init", depth).
				Str("command", node.Command).
				Msg("PROCESS_TREE: Process too close to init, marking as critical")
			return true
		}
		current = current.Parent
		depth++
	}

	return false
}

// hasTrackedDescendants checks if any descendants of a node are tracked
func (pt *ProcessTracker) hasTrackedDescendants(node *ProcessTreeNode) bool {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	// Use BFS to check all descendants
	queue := []*ProcessTreeNode{node}
	visited := make(map[int]bool)

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if visited[current.PID] {
			continue
		}
		visited[current.PID] = true

		// Check if this descendant is tracked
		if _, isTracked := pt.trackedPIDs[current.PID]; isTracked {
			return true
		}

		// Add children to queue
		queue = append(queue, current.Children...)
	}

	return false
}

// isProcessTooYoung checks if a process is too young to be considered for orphan cleanup
// This prevents race conditions during model startup where workers are spawning rapidly
func (pt *ProcessTracker) isProcessTooYoung(pid int) bool {
	// Use /proc to get process start time for more accuracy
	cmd := exec.Command("stat", "-c", "%Y", fmt.Sprintf("/proc/%d", pid))
	output, err := cmd.Output()
	if err != nil {
		// If we can't get the process start time, err on the side of caution
		log.Debug().Err(err).Int("pid", pid).Msg("PROCESS_TREE: Could not get process start time, assuming not too young")
		return false
	}

	startTimeStr := strings.TrimSpace(string(output))
	startTime, err := strconv.ParseInt(startTimeStr, 10, 64)
	if err != nil {
		log.Debug().Err(err).Int("pid", pid).Msg("PROCESS_TREE: Could not parse process start time, assuming not too young")
		return false
	}

	// Grace period: don't kill processes younger than 60 seconds
	const gracePeriodSeconds = 60
	processAge := time.Now().Unix() - startTime

	if processAge < gracePeriodSeconds {
		log.Debug().
			Int("pid", pid).
			Int64("process_age_seconds", processAge).
			Msg("PROCESS_TREE: Process is within grace period, not eligible for orphan cleanup")
		return true
	}

	return false
}

// cleanupOrphans attempts to cleanup orphaned processes
func (pt *ProcessTracker) cleanupOrphans(orphanPIDs []int, tree *ProcessTree) {
	log.Info().
		Interface("orphan_pids", orphanPIDs).
		Msg("ORPHAN_MONITOR: Starting cleanup of orphaned processes")

	for _, pid := range orphanPIDs {
		// Get process command for logging
		command := "unknown"
		if node, exists := tree.Nodes[pid]; exists {
			command = node.Command
		}

		log.Info().
			Int("pid", pid).
			Str("command", command).
			Msg("ORPHAN_MONITOR: Attempting to cleanup orphaned process")

		// First try graceful termination
		if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
			log.Error().
				Err(err).
				Int("pid", pid).
				Str("command", command).
				Msg("ORPHAN_MONITOR: Failed to send SIGTERM to orphaned process")
			continue
		}

		// Wait a bit for graceful shutdown
		time.Sleep(2 * time.Second)

		// Check if process still exists
		if err := syscall.Kill(pid, 0); err != nil {
			// Process is gone - record graceful cleanup
			pt.recordCleanup(pid, command, "graceful")
			log.Info().
				Int("pid", pid).
				Str("command", command).
				Msg("ORPHAN_MONITOR: Orphaned process terminated gracefully")
			continue
		}

		// Force kill if still alive
		log.Warn().
			Int("pid", pid).
			Str("command", command).
			Msg("ORPHAN_MONITOR: Force killing stubborn orphaned process")

		if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
			log.Error().
				Err(err).
				Int("pid", pid).
				Str("command", command).
				Msg("ORPHAN_MONITOR: Failed to force kill orphaned process")
		} else {
			// Record force cleanup
			pt.recordCleanup(pid, command, "force")
			log.Info().
				Int("pid", pid).
				Str("command", command).
				Msg("ORPHAN_MONITOR: Force killed orphaned process")
		}
	}
}

// recordCleanup records a cleaned process in statistics
func (pt *ProcessTracker) recordCleanup(pid int, command string, method string) {
	pt.cleanupStatsMu.Lock()
	defer pt.cleanupStatsMu.Unlock()

	now := time.Now()
	cleaned := CleanedProcess{
		PID:       pid,
		Command:   command,
		CleanedAt: now,
		Method:    method,
	}

	// Add to recent cleanups (keep last 50)
	pt.cleanupStats.RecentCleanups = append(pt.cleanupStats.RecentCleanups, cleaned)
	if len(pt.cleanupStats.RecentCleanups) > 50 {
		pt.cleanupStats.RecentCleanups = pt.cleanupStats.RecentCleanups[1:]
	}

	pt.cleanupStats.TotalCleaned++
	pt.cleanupStats.LastCleanupTime = &now

	log.Info().
		Int("pid", pid).
		Str("method", method).
		Str("command", command).
		Time("cleaned_at", now).
		Int("total_cleaned", pt.cleanupStats.TotalCleaned).
		Msg("CLEANUP_STATS: Recorded process cleanup")
}

// recordCleanupRun records statistics about cleanup runs
func (pt *ProcessTracker) recordCleanupRun(synchronous bool, skipped bool) {
	pt.cleanupStatsMu.Lock()
	defer pt.cleanupStatsMu.Unlock()

	if skipped {
		pt.cleanupStats.ConcurrentSkips++
	} else if synchronous {
		pt.cleanupStats.SynchronousRuns++
	} else {
		pt.cleanupStats.AsynchronousRuns++
	}
}

// GetStats returns statistics about tracked processes
func (pt *ProcessTracker) GetStats() map[string]interface{} {
	pt.mu.RLock()
	slotCounts := make(map[uuid.UUID]int)
	for slotID, pids := range pt.slotProcesses {
		slotCounts[slotID] = len(pids)
	}
	totalTracked := len(pt.trackedPIDs)
	totalSlots := len(pt.slotProcesses)
	pt.mu.RUnlock()

	pt.cleanupStatsMu.RLock()
	cleanupStats := pt.cleanupStats
	pt.cleanupStatsMu.RUnlock()

	return map[string]interface{}{
		"total_tracked_processes": totalTracked,
		"total_slots":             totalSlots,
		"slot_process_counts":     slotCounts,
		"cleanup_stats":           cleanupStats,
	}
}
