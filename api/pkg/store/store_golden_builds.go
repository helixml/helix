package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/helixml/helix/api/pkg/types"
)

// ListGoldenBuildsQuery filters golden_builds rows. Empty fields match all.
type ListGoldenBuildsQuery struct {
	ProjectID string
	SandboxID string
	// ActiveOnly returns only builds that are building or retrying.
	ActiveOnly bool
}

// ListGoldenBuilds returns the persisted golden build state rows matching q.
func (s *PostgresStore) ListGoldenBuilds(ctx context.Context, q *ListGoldenBuildsQuery) ([]*types.SandboxCacheState, error) {
	db := s.gdb.WithContext(ctx)
	if q != nil {
		if q.ProjectID != "" {
			db = db.Where("project_id = ?", q.ProjectID)
		}
		if q.SandboxID != "" {
			db = db.Where("sandbox_id = ?", q.SandboxID)
		}
		if q.ActiveOnly {
			db = db.Where("status IN ?", []string{types.GoldenBuildStatusBuilding, types.GoldenBuildStatusRetrying})
		}
	}
	var builds []*types.SandboxCacheState
	if err := db.Order("project_id, sandbox_id").Find(&builds).Error; err != nil {
		return nil, fmt.Errorf("error listing golden builds: %w", err)
	}
	return builds, nil
}

// UpdateGoldenBuild atomically reads, modifies and writes the golden build
// state for projectID on sandboxID (creating it if missing) under a row lock.
// update returns false to leave the row unchanged. Returns the row as it is
// after the call.
func (s *PostgresStore) UpdateGoldenBuild(ctx context.Context, projectID, sandboxID string, update func(*types.SandboxCacheState) bool) (*types.SandboxCacheState, error) {
	if projectID == "" || sandboxID == "" {
		return nil, fmt.Errorf("project ID and sandbox ID are required")
	}
	var state types.SandboxCacheState
	err := s.gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		seed := types.SandboxCacheState{
			ProjectID: projectID,
			SandboxID: sandboxID,
			Status:    types.GoldenBuildStatusNone,
			Updated:   time.Now(),
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&seed).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("project_id = ? AND sandbox_id = ?", projectID, sandboxID).
			First(&state).Error; err != nil {
			return err
		}
		if !update(&state) {
			return nil
		}
		state.Updated = time.Now()
		return tx.Save(&state).Error
	})
	if err != nil {
		return nil, fmt.Errorf("error updating golden build %s/%s: %w", projectID, sandboxID, err)
	}
	return &state, nil
}

// GetGoldenBuild returns the golden build state for projectID on sandboxID.
func (s *PostgresStore) GetGoldenBuild(ctx context.Context, projectID, sandboxID string) (*types.SandboxCacheState, error) {
	var state types.SandboxCacheState
	err := s.gdb.WithContext(ctx).
		Where("project_id = ? AND sandbox_id = ?", projectID, sandboxID).
		First(&state).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("error getting golden build %s/%s: %w", projectID, sandboxID, err)
	}
	return &state, nil
}

// DeleteGoldenBuilds removes every golden build row for projectID.
func (s *PostgresStore) DeleteGoldenBuilds(ctx context.Context, projectID string) error {
	if projectID == "" {
		return fmt.Errorf("project ID is required")
	}
	if err := s.gdb.WithContext(ctx).Where("project_id = ?", projectID).Delete(&types.SandboxCacheState{}).Error; err != nil {
		return fmt.Errorf("error deleting golden builds for project %s: %w", projectID, err)
	}
	return nil
}

// migrateGoldenBuildsFromProjectMetadata moves golden cache state that used to
// live in projects.metadata.docker_cache_status into golden_builds. Idempotent:
// it only touches projects that still carry the old key.
func (s *PostgresStore) migrateGoldenBuildsFromProjectMetadata(ctx context.Context) error {
	return s.gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`
			INSERT INTO golden_builds (project_id, sandbox_id, status, size_bytes, last_build_at, last_ready_at, build_session_id, error, attempt, pending_rebuild, updated)
			SELECT p.id, sb.key,
			       COALESCE(sb.value->>'status', 'none'),
			       COALESCE((sb.value->>'size_bytes')::bigint, 0),
			       (sb.value->>'last_build_at')::timestamptz,
			       (sb.value->>'last_ready_at')::timestamptz,
			       COALESCE(sb.value->>'build_session_id', ''),
			       COALESCE(sb.value->>'error', ''),
			       CASE WHEN sb.value->>'status' = 'building' THEN 1 ELSE 0 END,
			       false,
			       now()
			FROM projects p, jsonb_each(p.metadata->'docker_cache_status'->'sandboxes') sb
			WHERE jsonb_typeof(p.metadata->'docker_cache_status'->'sandboxes') = 'object'
			ON CONFLICT DO NOTHING`).Error; err != nil {
			return err
		}
		return tx.Exec(`UPDATE projects SET metadata = metadata - 'docker_cache_status' WHERE metadata ? 'docker_cache_status'`).Error
	})
}
