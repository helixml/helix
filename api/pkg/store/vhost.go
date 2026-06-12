package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
)

// --- vhost_routes ---

// CreateVHostRoute inserts a new route. Hostname is lowercased. Generates
// an ID if one is not provided. CreatedAt is stamped.
func (s *PostgresStore) CreateVHostRoute(ctx context.Context, r *types.VHostRoute) error {
	if r.Hostname == "" {
		return fmt.Errorf("hostname is required")
	}
	if r.TargetKind == "" || r.TargetID == "" {
		return fmt.Errorf("target_kind and target_id are required")
	}
	if r.ID == "" {
		r.ID = system.GenerateVHostRouteID()
	}
	r.Hostname = strings.ToLower(r.Hostname)
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now()
	}
	return s.gdb.WithContext(ctx).Create(r).Error
}

// GetVHostRouteByHostname returns the route for the given hostname or
// ErrNotFound. Hostname is matched case-insensitively.
func (s *PostgresStore) GetVHostRouteByHostname(ctx context.Context, hostname string) (*types.VHostRoute, error) {
	if hostname == "" {
		return nil, fmt.Errorf("hostname is required")
	}
	var r types.VHostRoute
	err := s.gdb.WithContext(ctx).Where("hostname = ?", strings.ToLower(hostname)).First(&r).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &r, nil
}

// GetVHostRouteByID returns a single route by ID.
func (s *PostgresStore) GetVHostRouteByID(ctx context.Context, id string) (*types.VHostRoute, error) {
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}
	var r types.VHostRoute
	err := s.gdb.WithContext(ctx).Where("id = ?", id).First(&r).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &r, nil
}

// ListVHostRoutesByTarget returns all routes pointing at the given target.
func (s *PostgresStore) ListVHostRoutesByTarget(ctx context.Context, kind types.VHostTargetKind, targetID string) ([]*types.VHostRoute, error) {
	if targetID == "" {
		return nil, fmt.Errorf("target_id is required")
	}
	var routes []*types.VHostRoute
	err := s.gdb.WithContext(ctx).
		Where("target_kind = ? AND target_id = ?", kind, targetID).
		Order("created_at ASC").
		Find(&routes).Error
	if err != nil {
		return nil, err
	}
	return routes, nil
}

// DeleteVHostRoute removes one route by ID.
func (s *PostgresStore) DeleteVHostRoute(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("id is required")
	}
	return s.gdb.WithContext(ctx).Delete(&types.VHostRoute{}, "id = ?", id).Error
}

// DeleteVHostRoutesByTarget removes every route pointing at the given target.
// Used by sandbox/session cleanup hooks and the web-service disable path.
func (s *PostgresStore) DeleteVHostRoutesByTarget(ctx context.Context, kind types.VHostTargetKind, targetID string) error {
	if targetID == "" {
		return fmt.Errorf("target_id is required")
	}
	return s.gdb.WithContext(ctx).
		Where("target_kind = ? AND target_id = ?", kind, targetID).
		Delete(&types.VHostRoute{}).Error
}

// RotateVHostRouteHostname replaces the hostname on an existing route and
// stamps rotated_at. Used by the preview-token rotate endpoint.
func (s *PostgresStore) RotateVHostRouteHostname(ctx context.Context, id, newHostname string) error {
	if id == "" || newHostname == "" {
		return fmt.Errorf("id and new hostname are required")
	}
	now := time.Now()
	return s.gdb.WithContext(ctx).
		Model(&types.VHostRoute{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"hostname":   strings.ToLower(newHostname),
			"rotated_at": &now,
		}).Error
}

// MarkVHostRouteVerified flips verified_at to now for a route.
func (s *PostgresStore) MarkVHostRouteVerified(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("id is required")
	}
	now := time.Now()
	return s.gdb.WithContext(ctx).
		Model(&types.VHostRoute{}).
		Where("id = ?", id).
		Update("verified_at", &now).Error
}

// --- project_web_service_states ---

// UpsertProjectWebServiceState inserts or updates the per-project web service
// row. UpdatedAt is stamped.
func (s *PostgresStore) UpsertProjectWebServiceState(ctx context.Context, state *types.ProjectWebServiceState) error {
	if state.ProjectID == "" {
		return fmt.Errorf("project_id is required")
	}
	state.UpdatedAt = time.Now()
	if state.CreatedAt.IsZero() {
		state.CreatedAt = state.UpdatedAt
	}
	return s.gdb.WithContext(ctx).
		Save(state).Error
}

// GetProjectWebServiceState returns the row or ErrNotFound.
func (s *PostgresStore) GetProjectWebServiceState(ctx context.Context, projectID string) (*types.ProjectWebServiceState, error) {
	if projectID == "" {
		return nil, fmt.Errorf("project_id is required")
	}
	var state types.ProjectWebServiceState
	err := s.gdb.WithContext(ctx).Where("project_id = ?", projectID).First(&state).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &state, nil
}

// SetActiveWebServiceSandbox updates the active_sandbox_id column atomically.
// Used by the redeploy cutover step.
func (s *PostgresStore) SetActiveWebServiceSandbox(ctx context.Context, projectID, sandboxID string) error {
	if projectID == "" {
		return fmt.Errorf("project_id is required")
	}
	return s.gdb.WithContext(ctx).
		Model(&types.ProjectWebServiceState{}).
		Where("project_id = ?", projectID).
		Updates(map[string]interface{}{
			"active_sandbox_id": sandboxID,
			"updated_at":        time.Now(),
		}).Error
}

// --- web_service_deploys ---

// CreateWebServiceDeploy inserts a new deploy row.
func (s *PostgresStore) CreateWebServiceDeploy(ctx context.Context, d *types.WebServiceDeploy) error {
	if d.ProjectID == "" {
		return fmt.Errorf("project_id is required")
	}
	if d.ID == "" {
		d.ID = system.GenerateWebServiceDeployID()
	}
	if d.StartedAt.IsZero() {
		d.StartedAt = time.Now()
	}
	if d.Status == "" {
		d.Status = types.WebServiceDeployStatusPending
	}
	return s.gdb.WithContext(ctx).Create(d).Error
}

// UpdateWebServiceDeploy patches a deploy row in place. Used to flip
// status, set finished_at, attach an error.
func (s *PostgresStore) UpdateWebServiceDeploy(ctx context.Context, id string, updates map[string]interface{}) error {
	if id == "" {
		return fmt.Errorf("id is required")
	}
	return s.gdb.WithContext(ctx).
		Model(&types.WebServiceDeploy{}).
		Where("id = ?", id).
		Updates(updates).Error
}

// ListWebServiceDeploys returns the most recent N deploys for a project,
// newest first.
func (s *PostgresStore) ListWebServiceDeploys(ctx context.Context, projectID string, limit int) ([]*types.WebServiceDeploy, error) {
	if projectID == "" {
		return nil, fmt.Errorf("project_id is required")
	}
	if limit <= 0 {
		limit = 20
	}
	var rows []*types.WebServiceDeploy
	err := s.gdb.WithContext(ctx).
		Where("project_id = ?", projectID).
		Order("started_at DESC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}
