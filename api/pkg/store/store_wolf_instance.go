package store

import (
	"context"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
)

// RegisterWolfInstance registers a new Wolf instance in the control plane
func (s *PostgresStore) RegisterWolfInstance(ctx context.Context, instance *types.WolfInstance) error {
	now := time.Now()
	instance.CreatedAt = now
	instance.UpdatedAt = now
	instance.LastHeartbeat = now
	instance.Status = types.WolfInstanceStatusOnline

	return s.gdb.WithContext(ctx).Create(instance).Error
}

// UpdateWolfHeartbeat updates the last heartbeat timestamp and optional metadata for a Wolf instance
func (s *PostgresStore) UpdateWolfHeartbeat(ctx context.Context, id string, swayVersion string) error {
	now := time.Now()
	updates := map[string]interface{}{
		"last_heartbeat": now,
		"updated_at":     now,
		"status":         types.WolfInstanceStatusOnline,
	}
	// Only update sway_version if provided (allows sandboxes to report their version)
	if swayVersion != "" {
		updates["sway_version"] = swayVersion
	}
	return s.gdb.WithContext(ctx).
		Model(&types.WolfInstance{}).
		Where("id = ?", id).
		Updates(updates).Error
}

// GetWolfInstance retrieves a Wolf instance by ID
func (s *PostgresStore) GetWolfInstance(ctx context.Context, id string) (*types.WolfInstance, error) {
	var instance types.WolfInstance
	err := s.gdb.WithContext(ctx).Where("id = ?", id).First(&instance).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &instance, nil
}

// ListWolfInstances retrieves all Wolf instances
func (s *PostgresStore) ListWolfInstances(ctx context.Context) ([]*types.WolfInstance, error) {
	var instances []*types.WolfInstance
	err := s.gdb.WithContext(ctx).
		Order("created_at DESC").
		Find(&instances).Error
	if err != nil {
		return nil, err
	}
	return instances, nil
}

// DeregisterWolfInstance removes a Wolf instance from the registry
func (s *PostgresStore) DeregisterWolfInstance(ctx context.Context, id string) error {
	return s.gdb.WithContext(ctx).Where("id = ?", id).Delete(&types.WolfInstance{}).Error
}

// UpdateWolfStatus updates the status of a Wolf instance
func (s *PostgresStore) UpdateWolfStatus(ctx context.Context, id string, status string) error {
	return s.gdb.WithContext(ctx).
		Model(&types.WolfInstance{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":     status,
			"updated_at": time.Now(),
		}).Error
}

// IncrementWolfSandboxCount increments the connected sandboxes count for a Wolf instance
func (s *PostgresStore) IncrementWolfSandboxCount(ctx context.Context, id string) error {
	return s.gdb.WithContext(ctx).
		Model(&types.WolfInstance{}).
		Where("id = ?", id).
		UpdateColumn("connected_sandboxes", gorm.Expr("connected_sandboxes + ?", 1)).Error
}

// DecrementWolfSandboxCount decrements the connected sandboxes count for a Wolf instance
func (s *PostgresStore) DecrementWolfSandboxCount(ctx context.Context, id string) error {
	return s.gdb.WithContext(ctx).
		Model(&types.WolfInstance{}).
		Where("id = ?", id).
		UpdateColumn("connected_sandboxes", gorm.Expr("GREATEST(connected_sandboxes - ?, 0)", 1)).Error
}

// GetWolfInstancesOlderThanHeartbeat retrieves Wolf instances with heartbeat older than the given time
func (s *PostgresStore) GetWolfInstancesOlderThanHeartbeat(ctx context.Context, olderThan time.Time) ([]*types.WolfInstance, error) {
	var instances []*types.WolfInstance
	err := s.gdb.WithContext(ctx).
		Where("last_heartbeat < ? AND status != ?", olderThan, types.WolfInstanceStatusOffline).
		Find(&instances).Error
	if err != nil {
		return nil, err
	}
	return instances, nil
}
