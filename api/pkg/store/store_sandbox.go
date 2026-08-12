package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/types"
)

// RegisterSandboxInstance registers a new sandbox instance or updates an existing one
func (s *PostgresStore) RegisterSandboxInstance(ctx context.Context, instance *types.SandboxInstance) error {
	// Use upsert to handle reconnecting sandboxes
	return s.gdb.WithContext(ctx).Save(instance).Error
}

// UpdateSandboxHeartbeat updates a sandbox's heartbeat data
func (s *PostgresStore) UpdateSandboxHeartbeat(ctx context.Context, id string, req *types.SandboxHeartbeatRequest) error {
	updates := map[string]interface{}{
		"last_seen":       time.Now(),
		"status":          "online",
		"gpu_vendor":      req.GPUVendor,
		"render_node":     req.RenderNode,
		"privileged_mode": req.PrivilegedModeEnabled,
	}

	// Store helix version if provided
	if req.HelixVersion != "" {
		updates["helix_version"] = req.HelixVersion
	}

	// Store cloud instance type if detected. Empty on bare-metal / non-AWS
	// hosts, so we only write when present to avoid blanking a known value.
	if req.InstanceType != "" {
		updates["instance_type"] = req.InstanceType
	}

	// Store desktop versions as JSON if provided
	if len(req.DesktopVersions) > 0 {
		updates["desktop_versions"] = req.DesktopVersions
	}

	// Sandbox-absorbs-runner: persist GPU inventory and inference subsystem
	// state from the heartbeat. GPUs is a jsonb column carrying the rich
	// per-GPU info (vendor, arch, VRAM) that the inference router uses
	// for the profile-compatibility check.
	if len(req.GPUs) > 0 {
		gpusJSON, err := json.Marshal(req.GPUs)
		if err == nil {
			updates["gpus"] = gpusJSON
		}
	}
	if req.ProfileStatus != "" {
		updates["profile_status"] = req.ProfileStatus
	}
	if req.ProfileError != "" {
		updates["profile_error"] = req.ProfileError
	}
	if len(req.ServiceHealth) > 0 {
		shJSON, err := json.Marshal(req.ServiceHealth)
		if err == nil {
			updates["service_health"] = shJSON
		}
	}
	// Always overwrite ProfileProgress (including with empty) so the
	// progress bar disappears once a download completes — otherwise stale
	// "downloading 95%" lingers forever in the admin UI.
	if pgJSON, err := json.Marshal(req.ProfileProgress); err == nil {
		updates["profile_progress"] = pgJSON
	}

	return s.gdb.WithContext(ctx).
		Model(&types.SandboxInstance{}).
		Where("id = ?", id).
		Updates(updates).Error
}

// GetSandboxInstance retrieves a sandbox host registration by ID.
func (s *PostgresStore) GetSandboxInstance(ctx context.Context, id string) (*types.SandboxInstance, error) {
	var instance types.SandboxInstance
	err := s.gdb.WithContext(ctx).Where("id = ?", id).First(&instance).Error
	if err != nil {
		return nil, fmt.Errorf("error getting sandbox: %w", err)
	}
	return &instance, nil
}

// ListSandboxInstances returns all registered sandbox host instances.
func (s *PostgresStore) ListSandboxInstances(ctx context.Context) ([]*types.SandboxInstance, error) {
	var instances []*types.SandboxInstance
	err := s.gdb.WithContext(ctx).Order("created DESC").Find(&instances).Error
	if err != nil {
		return nil, fmt.Errorf("error listing sandboxes: %w", err)
	}
	return instances, nil
}

// DeregisterSandboxInstance removes a sandbox host instance row.
func (s *PostgresStore) DeregisterSandboxInstance(ctx context.Context, id string) error {
	return s.gdb.WithContext(ctx).Delete(&types.SandboxInstance{}, "id = ?", id).Error
}

// UpdateSandboxInstanceStatus updates only the status field of a sandbox host.
func (s *PostgresStore) UpdateSandboxInstanceStatus(ctx context.Context, id string, status string) error {
	return s.gdb.WithContext(ctx).
		Model(&types.SandboxInstance{}).
		Where("id = ?", id).
		Update("status", status).Error
}

// UpdateSandboxInstanceComputeState writes only the compute_state column
// for the given row. Used by the compute.Manager reconciler to record
// provider-driven lifecycle transitions (provisioning -> ready, etc.)
// WITHOUT overwriting heartbeat-driven fields (status, last_seen,
// active_sandboxes). Pairs with UpdateSandboxInstanceProviderID for
// the post-Provision update flow.
func (s *PostgresStore) UpdateSandboxInstanceComputeState(ctx context.Context, id, computeState string) error {
	return s.gdb.WithContext(ctx).
		Model(&types.SandboxInstance{}).
		Where("id = ?", id).
		Update("compute_state", computeState).Error
}

// UpdateSandboxInstanceProviderID writes only the provider_id column.
// Called by compute.Manager.provisionOne after the upstream Provider
// accepts a new request and returns its opaque ID. Doing it as a
// targeted column update (rather than a full row save) avoids racing
// the heartbeat path that may have written fresher status/last_seen
// for this row in between.
func (s *PostgresStore) UpdateSandboxInstanceProviderID(ctx context.Context, id, providerID string) error {
	return s.gdb.WithContext(ctx).
		Model(&types.SandboxInstance{}).
		Where("id = ?", id).
		Update("provider_id", providerID).Error
}

// UpdateSandboxInstanceNetwork writes only the columns that describe
// where the sandbox host is reachable: its IP address, hostname, and
// the LastSeen timestamp that marks "we just heard from this host".
//
// Used by the auto-register bridge in ensureSandboxRegistered to
// transition a Manager-provisioned row to a registered state without
// reusing the full-row gorm.Save path. Save would have replaced every
// column with the in-memory value loaded a few statements earlier,
// stamping out any heartbeat columns (gpus, service_health,
// profile_status, profile_progress, helix_version, desktop_versions)
// the heartbeat goroutine may have updated in between.
//
// The bridge calls this alongside UpdateSandboxInstanceComputeState
// and UpdateSandboxInstanceStatus; doing it as three targeted updates
// is verbose but each is race-safe in isolation.
func (s *PostgresStore) UpdateSandboxInstanceNetwork(ctx context.Context, id, ipAddress, hostname string, lastSeen time.Time) error {
	updates := map[string]interface{}{
		"last_seen": lastSeen,
	}
	if ipAddress != "" {
		updates["ip_address"] = ipAddress
	}
	if hostname != "" {
		updates["hostname"] = hostname
	}
	return s.gdb.WithContext(ctx).
		Model(&types.SandboxInstance{}).
		Where("id = ?", id).
		Updates(updates).Error
}

// MarkSandboxInstanceOfflineIfStale flips a sandbox row to status="offline"
// only when its current last_seen is older than `staleBefore`. This is a
// compare-and-swap variant of UpdateSandboxInstanceStatus used by the
// reaper to avoid racing a concurrent heartbeat: SELECT-then-UPDATE without
// this guard can flip a now-healthy Runner back to offline when its
// heartbeat lands between the reaper's query and its update.
//
// Returns the number of rows affected (0 = the row was refreshed by a
// recent heartbeat, no transition; 1 = transition applied; ErrNotFound
// only if the id no longer exists).
func (s *PostgresStore) MarkSandboxInstanceOfflineIfStale(ctx context.Context, id string, staleBefore time.Time) (int64, error) {
	res := s.gdb.WithContext(ctx).
		Model(&types.SandboxInstance{}).
		Where("id = ? AND last_seen < ? AND status = ?", id, staleBefore, "online").
		Update("status", "offline")
	if res.Error != nil {
		return 0, fmt.Errorf("error marking sandbox offline: %w", res.Error)
	}
	return res.RowsAffected, nil
}

// IncrementSandboxContainerCount increments the active container count
func (s *PostgresStore) IncrementSandboxContainerCount(ctx context.Context, id string) error {
	return s.gdb.WithContext(ctx).
		Model(&types.SandboxInstance{}).
		Where("id = ?", id).
		UpdateColumn("active_sandboxes", s.gdb.Raw("active_sandboxes + 1")).Error
}

// DecrementSandboxContainerCount decrements the active container count
func (s *PostgresStore) DecrementSandboxContainerCount(ctx context.Context, id string) error {
	return s.gdb.WithContext(ctx).
		Model(&types.SandboxInstance{}).
		Where("id = ?", id).
		UpdateColumn("active_sandboxes", s.gdb.Raw("GREATEST(active_sandboxes - 1, 0)")).Error
}

// ResetSandboxOnReconnect resets sandbox state when it reconnects.
//
// Flips status back to online and refreshes last_seen so the heartbeat
// reaper doesn't immediately mark the row offline again. We deliberately
// do NOT zero active_sandboxes here: a brief RevDial blip on a healthy
// Runner with N running containers must not silently produce a phantom
// "free capacity" signal for the autoscaler. The corrective path is
// DiscoverContainersFromSandbox, which queries hydra for the real
// container list on every reconnect and calls SetSandboxContainerCount
// to write the authoritative value.
func (s *PostgresStore) ResetSandboxOnReconnect(ctx context.Context, id string) error {
	return s.gdb.WithContext(ctx).
		Model(&types.SandboxInstance{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":    "online",
			"last_seen": time.Now(),
		}).Error
}

// SetSandboxContainerCount sets active_sandboxes to an explicit value.
// Used by DiscoverContainersFromSandbox to write the authoritative
// container count after querying hydra. Distinct from Increment /
// Decrement which assume a known delta from the prior value; this
// resyncs from ground truth, recovering from any drift caused by
// API restarts, missed Increment/Decrement calls (e.g. hydra-side
// internal deletes that bypass StopDesktop), or other inconsistencies.
func (s *PostgresStore) SetSandboxContainerCount(ctx context.Context, id string, count int) error {
	return s.gdb.WithContext(ctx).
		Model(&types.SandboxInstance{}).
		Where("id = ?", id).
		UpdateColumn("active_sandboxes", count).Error
}

// BackfillSandboxMaxSandboxes rewrites max_sandboxes on every existing
// Runner row to the supplied value. Called once at API boot so a change
// to HELIX_SANDBOX_MAX_DEV_CONTAINERS takes effect across the entire
// fleet on next restart, not just for Runners that re-register after
// the change. Returns the number of rows updated for logging.
//
// Idempotent: only writes rows where max_sandboxes already differs, so
// repeat boots with the same config are no-ops.
//
// Zero-guard: refuses to backfill with value <= 0. The env var binding
// defaults to 20, so receiving 0 here means the operator either set
// HELIX_SANDBOX_MAX_DEV_CONTAINERS=0 explicitly (almost certainly a
// mistake - it would render every Runner permanently "full") or the
// config was loaded into an uninitialised struct. Either way, safer
// to skip than to zero the entire fleet's ceiling.
func (s *PostgresStore) BackfillSandboxMaxSandboxes(ctx context.Context, value int) (int64, error) {
	if value <= 0 {
		return 0, fmt.Errorf("BackfillSandboxMaxSandboxes refusing to write non-positive value %d (would render every Runner permanently full)", value)
	}
	res := s.gdb.WithContext(ctx).
		Model(&types.SandboxInstance{}).
		Where("max_sandboxes != ?", value).
		UpdateColumn("max_sandboxes", value)
	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}

// GetSandboxInstancesOlderThanHeartbeat returns sandbox hosts that haven't sent a heartbeat recently.
func (s *PostgresStore) GetSandboxInstancesOlderThanHeartbeat(ctx context.Context, olderThan time.Time) ([]*types.SandboxInstance, error) {
	var instances []*types.SandboxInstance
	err := s.gdb.WithContext(ctx).
		Where("last_seen < ?", olderThan).
		Find(&instances).Error
	if err != nil {
		return nil, fmt.Errorf("error getting stale sandboxes: %w", err)
	}
	return instances, nil
}

// FindAvailableSandboxInstance finds a sandbox host that is online, has recent heartbeat, and has the required desktop version.
// Returns nil if no suitable sandbox host is found.
//
// Uses config.DefaultSandboxDispatchStaleThreshold (90s) as the dispatch
// staleness filter. The Runner-side heartbeat cadence is 30s (see
// cmd/sandbox-heartbeat/main.go:27 `HeartbeatInterval`), so 90s = 3
// heartbeat intervals: a Runner that misses one beat stays selectable;
// missing two-or-more excludes it from new dispatch. The reaper uses the
// looser SandboxStaleThreshold for UI/reporting state.
func (s *PostgresStore) FindAvailableSandboxInstance(ctx context.Context, desktopType string) (*types.SandboxInstance, error) {
	staleThreshold := time.Now().Add(-config.DefaultSandboxDispatchStaleThreshold)
	var instances []*types.SandboxInstance
	err := s.gdb.WithContext(ctx).
		Where("status = ? AND last_seen > ?", "online", staleThreshold).
		Order("active_sandboxes ASC"). // Prefer less loaded sandboxes
		Find(&instances).Error
	if err != nil {
		return nil, fmt.Errorf("error finding available sandboxes: %w", err)
	}

	// Find one with the required desktop version
	for _, instance := range instances {
		// Sandboxes only run on render-capable hosts. A neuron/inf2 host
		// (no /dev/dri render node) would otherwise be picked on load
		// alone, then the desktop container FATALs at startup.
		if !instance.CanHostSandbox() {
			continue
		}
		if len(instance.DesktopVersions) > 0 {
			var versions map[string]string
			if err := json.Unmarshal(instance.DesktopVersions, &versions); err != nil {
				continue // Skip sandboxes with invalid version JSON
			}
			if version, ok := versions[desktopType]; ok && version != "" {
				return instance, nil
			}
		}
	}

	return nil, nil // No suitable sandbox found
}

// Disk usage history methods

// CreateDiskUsageHistory stores a disk usage record for alerting and trends
func (s *PostgresStore) CreateDiskUsageHistory(ctx context.Context, history *types.DiskUsageHistory) error {
	return s.gdb.WithContext(ctx).Create(history).Error
}

// GetDiskUsageHistory retrieves disk usage history for a sandbox since a given time
func (s *PostgresStore) GetDiskUsageHistory(ctx context.Context, sandboxID string, since time.Time) ([]*types.DiskUsageHistory, error) {
	var history []*types.DiskUsageHistory
	err := s.gdb.WithContext(ctx).
		Where("sandbox_id = ? AND recorded > ?", sandboxID, since).
		Order("recorded DESC").
		Find(&history).Error
	if err != nil {
		return nil, fmt.Errorf("error getting disk usage history: %w", err)
	}
	return history, nil
}

// DeleteOldDiskUsageHistory removes disk usage records older than the specified time
func (s *PostgresStore) DeleteOldDiskUsageHistory(ctx context.Context, olderThan time.Time) (int64, error) {
	result := s.gdb.WithContext(ctx).
		Where("recorded < ?", olderThan).
		Delete(&types.DiskUsageHistory{})
	return result.RowsAffected, result.Error
}
