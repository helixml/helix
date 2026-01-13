package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/types"
)

// RegisterSandbox registers a new sandbox instance or updates an existing one
func (s *PostgresStore) RegisterSandbox(ctx context.Context, instance *types.SandboxInstance) error {
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

	// Store desktop versions as JSON if provided
	if len(req.DesktopVersions) > 0 {
		updates["desktop_versions"] = req.DesktopVersions
	}

	return s.gdb.WithContext(ctx).
		Model(&types.SandboxInstance{}).
		Where("id = ?", id).
		Updates(updates).Error
}

// GetSandbox retrieves a sandbox by ID
func (s *PostgresStore) GetSandbox(ctx context.Context, id string) (*types.SandboxInstance, error) {
	var instance types.SandboxInstance
	err := s.gdb.WithContext(ctx).Where("id = ?", id).First(&instance).Error
	if err != nil {
		return nil, fmt.Errorf("error getting sandbox: %w", err)
	}
	return &instance, nil
}

// ListSandboxes returns all registered sandbox instances
func (s *PostgresStore) ListSandboxes(ctx context.Context) ([]*types.SandboxInstance, error) {
	var instances []*types.SandboxInstance
	err := s.gdb.WithContext(ctx).Order("created DESC").Find(&instances).Error
	if err != nil {
		return nil, fmt.Errorf("error listing sandboxes: %w", err)
	}
	return instances, nil
}

// DeregisterSandbox removes a sandbox instance
func (s *PostgresStore) DeregisterSandbox(ctx context.Context, id string) error {
	return s.gdb.WithContext(ctx).Delete(&types.SandboxInstance{}, "id = ?", id).Error
}

// UpdateSandboxStatus updates only the status field of a sandbox
func (s *PostgresStore) UpdateSandboxStatus(ctx context.Context, id string, status string) error {
	return s.gdb.WithContext(ctx).
		Model(&types.SandboxInstance{}).
		Where("id = ?", id).
		Update("status", status).Error
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

// ResetSandboxOnReconnect resets sandbox state when it reconnects
func (s *PostgresStore) ResetSandboxOnReconnect(ctx context.Context, id string) error {
	return s.gdb.WithContext(ctx).
		Model(&types.SandboxInstance{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":           "online",
			"last_seen":        time.Now(),
			"active_sandboxes": 0,
		}).Error
}

// GetSandboxesOlderThanHeartbeat returns sandboxes that haven't sent a heartbeat recently
func (s *PostgresStore) GetSandboxesOlderThanHeartbeat(ctx context.Context, olderThan time.Time) ([]*types.SandboxInstance, error) {
	var instances []*types.SandboxInstance
	err := s.gdb.WithContext(ctx).
		Where("last_seen < ?", olderThan).
		Find(&instances).Error
	if err != nil {
		return nil, fmt.Errorf("error getting stale sandboxes: %w", err)
	}
	return instances, nil
}

// FindAvailableSandbox finds a sandbox that is online, has recent heartbeat, and has the required desktop version.
// Returns nil if no suitable sandbox is found.
func (s *PostgresStore) FindAvailableSandbox(ctx context.Context, desktopType string) (*types.SandboxInstance, error) {
	// Get sandboxes that are online and have sent heartbeat in the last 2 minutes
	staleThreshold := time.Now().Add(-2 * time.Minute)
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
