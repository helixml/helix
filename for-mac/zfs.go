package main

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ZFSDatasetStats holds per-dataset usage
type ZFSDatasetStats struct {
	Name       string `json:"name"`
	Used       int64  `json:"used"`       // bytes
	Referenced int64  `json:"referenced"` // bytes
	Type       string `json:"type"`       // "dataset" or "zvol"
}

// ZFSStats holds ZFS pool statistics
type ZFSStats struct {
	PoolName         string            `json:"pool_name"`
	PoolSize         int64             `json:"pool_size"`          // bytes
	PoolUsed         int64             `json:"pool_used"`          // bytes
	PoolAvailable    int64             `json:"pool_available"`     // bytes
	DedupRatio       float64           `json:"dedup_ratio"`        // e.g., 2.5x
	CompressionRatio float64           `json:"compression_ratio"`  // e.g., 1.3x
	DedupSavedBytes  int64             `json:"dedup_saved_bytes"`  // estimated bytes saved by dedup
	Datasets         []ZFSDatasetStats `json:"datasets"`           // per-dataset breakdown
	LastUpdated      string            `json:"last_updated"`
	Error            string            `json:"error,omitempty"`
}

// DiskUsage holds disk usage information
type DiskUsage struct {
	RootDiskTotal int64  `json:"root_disk_total"` // bytes
	RootDiskUsed  int64  `json:"root_disk_used"`  // bytes
	RootDiskFree  int64  `json:"root_disk_free"`  // bytes
	ZFSDiskTotal  int64  `json:"zfs_disk_total"`  // bytes
	ZFSDiskUsed   int64  `json:"zfs_disk_used"`   // bytes
	ZFSDiskFree   int64  `json:"zfs_disk_free"`   // bytes
	HostActual    int64  `json:"host_actual"`     // actual bytes on host (after dedup)
	Error         string `json:"error,omitempty"`
}

// ZFSCollector collects ZFS stats from the VM via SSH
type ZFSCollector struct {
	mu        sync.RWMutex
	stats     ZFSStats
	diskUsage DiskUsage
	sshPort   int
	stopCh    chan struct{}
}

// NewZFSCollector creates a new ZFS stats collector
func NewZFSCollector(sshPort int) *ZFSCollector {
	return &ZFSCollector{
		sshPort: sshPort,
		stopCh:  make(chan struct{}),
	}
}

// Start begins periodic ZFS stats collection
func (z *ZFSCollector) Start() {
	go z.pollLoop()
}

// Stop stops the collector
func (z *ZFSCollector) Stop() {
	close(z.stopCh)
}

// GetStats returns the latest ZFS stats
func (z *ZFSCollector) GetStats() ZFSStats {
	z.mu.RLock()
	defer z.mu.RUnlock()
	return z.stats
}

// GetDiskUsage returns the latest disk usage
func (z *ZFSCollector) GetDiskUsage() DiskUsage {
	z.mu.RLock()
	defer z.mu.RUnlock()
	return z.diskUsage
}

func (z *ZFSCollector) pollLoop() {
	// Initial collection
	z.collect()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-z.stopCh:
			return
		case <-ticker.C:
			z.collect()
		}
	}
}

func (z *ZFSCollector) collect() {
	stats, err := z.fetchZFSStats()
	if err != nil {
		stats.Error = err.Error()
	}
	stats.LastUpdated = time.Now().Format(time.RFC3339)

	disk, err := z.fetchDiskUsage()
	if err != nil {
		disk.Error = err.Error()
	}

	z.mu.Lock()
	z.stats = stats
	z.diskUsage = disk
	z.mu.Unlock()
}

func (z *ZFSCollector) sshCmd(command string) (string, error) {
	cmd := exec.Command("ssh",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "ConnectTimeout=5",
		"-p", fmt.Sprintf("%d", z.sshPort),
		"ubuntu@localhost",
		command,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("ssh command failed: %w: %s", err, string(out))
	}
	return strings.TrimSpace(string(out)), nil
}

func (z *ZFSCollector) fetchZFSStats() (ZFSStats, error) {
	var stats ZFSStats
	stats.PoolName = "helix"

	// Get pool info: name, size, alloc, free
	out, err := z.sshCmd("sudo zpool list -Hp helix 2>/dev/null")
	if err != nil {
		return stats, fmt.Errorf("failed to get zpool info: %w", err)
	}

	// zpool list -Hp output: name size alloc free ...
	fields := strings.Fields(out)
	if len(fields) >= 4 {
		stats.PoolSize, _ = strconv.ParseInt(fields[1], 10, 64)
		stats.PoolUsed, _ = strconv.ParseInt(fields[2], 10, 64)
		stats.PoolAvailable, _ = strconv.ParseInt(fields[3], 10, 64)
	}

	// Get dedup ratio
	out, err = z.sshCmd("sudo zpool get -Hp dedupratio helix 2>/dev/null | awk '{print $3}'")
	if err == nil && out != "" {
		// Format is like "2.50x"
		out = strings.TrimSuffix(out, "x")
		stats.DedupRatio, _ = strconv.ParseFloat(out, 64)
	}

	// Get compression ratio
	out, err = z.sshCmd("sudo zfs get -Hp compressratio helix 2>/dev/null | awk '{print $3}'")
	if err == nil && out != "" {
		out = strings.TrimSuffix(out, "x")
		stats.CompressionRatio, _ = strconv.ParseFloat(out, 64)
	}

	// Calculate dedup savings: logical_used * (1 - 1/dedup_ratio)
	if stats.DedupRatio > 1.0 && stats.PoolUsed > 0 {
		logicalUsed := float64(stats.PoolUsed) * stats.DedupRatio
		stats.DedupSavedBytes = int64(logicalUsed - float64(stats.PoolUsed))
	}

	// Per-dataset breakdown
	out, err = z.sshCmd("sudo zfs list -Hp -o name,used,refer,type -r helix 2>/dev/null")
	if err == nil && out != "" {
		for _, line := range strings.Split(out, "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 4 && fields[0] != "helix" {
				ds := ZFSDatasetStats{
					Name: strings.TrimPrefix(fields[0], "helix/"),
					Type: fields[3],
				}
				ds.Used, _ = strconv.ParseInt(fields[1], 10, 64)
				ds.Referenced, _ = strconv.ParseInt(fields[2], 10, 64)
				stats.Datasets = append(stats.Datasets, ds)
			}
		}
	}

	return stats, nil
}

func (z *ZFSCollector) fetchDiskUsage() (DiskUsage, error) {
	var usage DiskUsage

	// Get root disk usage (df for /)
	out, err := z.sshCmd("df -B1 / 2>/dev/null | tail -1")
	if err == nil {
		fields := strings.Fields(out)
		if len(fields) >= 4 {
			usage.RootDiskTotal, _ = strconv.ParseInt(fields[1], 10, 64)
			usage.RootDiskUsed, _ = strconv.ParseInt(fields[2], 10, 64)
			usage.RootDiskFree, _ = strconv.ParseInt(fields[3], 10, 64)
		}
	}

	// ZFS disk usage comes from zpool stats
	stats := z.GetStats()
	usage.ZFSDiskTotal = stats.PoolSize
	usage.ZFSDiskUsed = stats.PoolUsed
	usage.ZFSDiskFree = stats.PoolAvailable

	// Host actual: the physical size of the qcow2 file on the host
	// This shows actual bytes after dedup savings
	out, err = z.sshCmd("sudo zpool get -Hp allocated helix 2>/dev/null | awk '{print $3}'")
	if err == nil && out != "" {
		usage.HostActual, _ = strconv.ParseInt(out, 10, 64)
	}

	return usage, nil
}
