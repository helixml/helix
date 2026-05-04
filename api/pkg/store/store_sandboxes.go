package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
)

// ListSandboxesQuery is the filter set for ListSandboxes.
type ListSandboxesQuery struct {
	OrganizationID string
	ProjectID      string
	Owner          string
	Status         types.SandboxStatus
	HostDeviceID   string
	IncludeDeleted bool
}

// CreateSandbox inserts a new sandbox row.
func (s *PostgresStore) CreateSandbox(ctx context.Context, sandbox *types.Sandbox) (*types.Sandbox, error) {
	if sandbox.OrganizationID == "" {
		return nil, fmt.Errorf("organization_id not specified")
	}
	if sandbox.Owner == "" {
		return nil, fmt.Errorf("owner not specified")
	}
	if sandbox.Runtime == "" {
		return nil, fmt.Errorf("runtime not specified")
	}
	if sandbox.ID == "" {
		sandbox.ID = system.GenerateSandboxID()
	}
	if sandbox.Status == "" {
		sandbox.Status = types.SandboxStatusPending
	}
	if sandbox.VCPUs == 0 {
		sandbox.VCPUs = 1
	}
	if sandbox.MemoryMB == 0 {
		sandbox.MemoryMB = 2048
	}
	if sandbox.TimeoutSeconds == 0 {
		sandbox.TimeoutSeconds = 3600
	}
	now := time.Now()
	sandbox.CreatedAt = now
	sandbox.UpdatedAt = now
	// TimeoutSeconds < 0 means "never expire" — leave ExpiresAt nil so the
	// reaper (which queries WHERE expires_at < now) skips it.
	if sandbox.ExpiresAt == nil && sandbox.TimeoutSeconds > 0 {
		exp := now.Add(time.Duration(sandbox.TimeoutSeconds) * time.Second)
		sandbox.ExpiresAt = &exp
	}

	if err := s.gdb.WithContext(ctx).Create(sandbox).Error; err != nil {
		return nil, err
	}
	return s.GetSandbox(ctx, sandbox.ID)
}

// GetSandbox returns a sandbox by ID, ignoring soft-deleted rows.
func (s *PostgresStore) GetSandbox(ctx context.Context, id string) (*types.Sandbox, error) {
	if id == "" {
		return nil, fmt.Errorf("id not specified")
	}
	var sb types.Sandbox
	err := s.gdb.WithContext(ctx).Where("id = ? AND deleted_at IS NULL", id).First(&sb).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &sb, nil
}

// ListSandboxes returns sandboxes matching a query, newest first.
func (s *PostgresStore) ListSandboxes(ctx context.Context, q *ListSandboxesQuery) ([]*types.Sandbox, error) {
	query := s.gdb.WithContext(ctx).Model(&types.Sandbox{})

	if q != nil {
		if q.OrganizationID != "" {
			query = query.Where("organization_id = ?", q.OrganizationID)
		}
		if q.ProjectID != "" {
			query = query.Where("project_id = ?", q.ProjectID)
		}
		if q.Owner != "" {
			query = query.Where("owner = ?", q.Owner)
		}
		if q.Status != "" {
			query = query.Where("status = ?", q.Status)
		}
		if q.HostDeviceID != "" {
			query = query.Where("host_device_id = ?", q.HostDeviceID)
		}
		if !q.IncludeDeleted {
			query = query.Where("deleted_at IS NULL")
		}
	}

	var sandboxes []*types.Sandbox
	err := query.Order("created_at DESC").Find(&sandboxes).Error
	if err != nil {
		return nil, err
	}
	return sandboxes, nil
}

// UpdateSandbox saves the entire row (use sparingly — prefer the targeted updaters below).
func (s *PostgresStore) UpdateSandbox(ctx context.Context, sandbox *types.Sandbox) (*types.Sandbox, error) {
	if sandbox.ID == "" {
		return nil, fmt.Errorf("id not specified")
	}
	sandbox.UpdatedAt = time.Now()
	if err := s.gdb.WithContext(ctx).Save(sandbox).Error; err != nil {
		return nil, err
	}
	return s.GetSandbox(ctx, sandbox.ID)
}

// SetSandboxStatus does a targeted UPDATE of the status fields, optionally
// recording a status message and started/stopped timestamps.
func (s *PostgresStore) SetSandboxStatus(ctx context.Context, id string, status types.SandboxStatus, message string) error {
	if id == "" {
		return fmt.Errorf("id not specified")
	}
	updates := map[string]interface{}{
		"status":         status,
		"status_message": message,
		"updated_at":     time.Now(),
	}
	switch status {
	case types.SandboxStatusRunning:
		now := time.Now()
		updates["started_at"] = &now
		updates["billing_last_charged_at"] = &now
	case types.SandboxStatusStopped, types.SandboxStatusFailed:
		now := time.Now()
		updates["stopped_at"] = &now
	}
	return s.gdb.WithContext(ctx).Model(&types.Sandbox{}).Where("id = ?", id).Updates(updates).Error
}

// SetSandboxBillingLastChargedAt records the end of the most recent billed
// interval for a running sandbox.
func (s *PostgresStore) SetSandboxBillingLastChargedAt(ctx context.Context, id string, chargedAt time.Time) error {
	if id == "" {
		return fmt.Errorf("id not specified")
	}
	return s.gdb.WithContext(ctx).Model(&types.Sandbox{}).Where("id = ?", id).Updates(map[string]interface{}{
		"billing_last_charged_at": &chargedAt,
		"updated_at":              time.Now(),
	}).Error
}

// SetRunningSandboxesBillingLastChargedAt starts billing windows for every
// currently running sandbox. Used when billing is enabled at runtime so old
// free usage is not charged retroactively.
func (s *PostgresStore) SetRunningSandboxesBillingLastChargedAt(ctx context.Context, chargedAt time.Time) error {
	return s.gdb.WithContext(ctx).Model(&types.Sandbox{}).
		Where("deleted_at IS NULL AND status = ?", types.SandboxStatusRunning).
		Updates(map[string]interface{}{
			"billing_last_charged_at": &chargedAt,
			"updated_at":              time.Now(),
		}).Error
}

// SetSandboxContainer records the host_device_id and container_id once the
// scheduler has placed the sandbox on a hydra host.
func (s *PostgresStore) SetSandboxContainer(ctx context.Context, id string, hostDeviceID, containerID string) error {
	return s.gdb.WithContext(ctx).Model(&types.Sandbox{}).Where("id = ?", id).Updates(map[string]interface{}{
		"host_device_id": hostDeviceID,
		"container_id":   containerID,
		"updated_at":     time.Now(),
	}).Error
}

// DeleteSandbox soft-deletes by setting deleted_at + status=stopped.
func (s *PostgresStore) DeleteSandbox(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("id not specified")
	}
	now := time.Now()
	return s.gdb.WithContext(ctx).Model(&types.Sandbox{}).Where("id = ?", id).Updates(map[string]interface{}{
		"deleted_at": &now,
		"status":     types.SandboxStatusStopped,
		"stopped_at": &now,
		"updated_at": now,
	}).Error
}

// SumSandboxCharges totals the magnitude of every transaction tagged with
// this sandbox id, treating amounts as credits debited from the wallet (we
// abs() because usage transactions are recorded as negative deltas).
func (s *PostgresStore) SumSandboxCharges(ctx context.Context, sandboxID string) (float64, error) {
	if sandboxID == "" {
		return 0, fmt.Errorf("sandbox id not specified")
	}
	var sum float64
	err := s.gdb.WithContext(ctx).
		Model(&types.Transaction{}).
		Where("sandbox_id = ?", sandboxID).
		Select("COALESCE(SUM(ABS(amount)), 0)").
		Row().
		Scan(&sum)
	if err != nil {
		return 0, err
	}
	return sum, nil
}

// ListExpiredSandboxes returns sandboxes whose expires_at has passed and which
// haven't been deleted yet — used by the TTL reaper.
func (s *PostgresStore) ListExpiredSandboxes(ctx context.Context, now time.Time) ([]*types.Sandbox, error) {
	var sandboxes []*types.Sandbox
	err := s.gdb.WithContext(ctx).
		Where("deleted_at IS NULL AND expires_at IS NOT NULL AND expires_at < ?", now).
		Find(&sandboxes).Error
	if err != nil {
		return nil, err
	}
	return sandboxes, nil
}

// ListStoppedNonPersistentSandboxes returns stopped ephemeral sandboxes that
// have been stopped since before the cutoff and have not been deleted yet.
func (s *PostgresStore) ListStoppedNonPersistentSandboxes(ctx context.Context, before time.Time) ([]*types.Sandbox, error) {
	var sandboxes []*types.Sandbox
	err := s.gdb.WithContext(ctx).
		Where("deleted_at IS NULL AND status = ? AND persistent = ? AND stopped_at IS NOT NULL AND stopped_at < ?", types.SandboxStatusStopped, false, before).
		Find(&sandboxes).Error
	if err != nil {
		return nil, err
	}
	return sandboxes, nil
}
