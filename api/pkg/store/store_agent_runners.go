package store

import (
	"context"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// CreateAgentRunner creates a new agent runner record
func (s *PostgresStore) CreateAgentRunner(ctx context.Context, runnerID string) (*types.AgentRunner, error) {
	if runnerID == "" {
		return nil, fmt.Errorf("runner ID cannot be empty")
	}

	runner := &types.AgentRunner{
		ID:              runnerID,
		Status:          "starting",
		LastSeen:        time.Now(),
		HealthStatus:    "unknown",
		LastHealthCheck: time.Now(),
	}

	err := s.gdb.WithContext(ctx).Create(runner).Error
	if err != nil {
		return nil, fmt.Errorf("failed to create agent runner: %w", err)
	}

	log.Info().
		Str("runner_id", runnerID).
		Msg("Created new agent runner")

	return runner, nil
}

// GetAgentRunner retrieves an agent runner by ID
func (s *PostgresStore) GetAgentRunner(ctx context.Context, runnerID string) (*types.AgentRunner, error) {
	if runnerID == "" {
		return nil, fmt.Errorf("runner ID cannot be empty")
	}

	var runner types.AgentRunner
	err := s.gdb.WithContext(ctx).Where("id = ?", runnerID).First(&runner).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get agent runner: %w", err)
	}

	return &runner, nil
}

// UpdateAgentRunner updates an existing agent runner
func (s *PostgresStore) UpdateAgentRunner(ctx context.Context, runner *types.AgentRunner) error {
	if runner.ID == "" {
		return fmt.Errorf("runner ID cannot be empty")
	}

	runner.UpdatedAt = time.Now()
	err := s.gdb.WithContext(ctx).Save(runner).Error
	if err != nil {
		return fmt.Errorf("failed to update agent runner: %w", err)
	}

	return nil
}

// UpdateAgentRunnerStatus updates the status and last seen time for an agent runner
func (s *PostgresStore) UpdateAgentRunnerStatus(ctx context.Context, runnerID, status string) error {
	if runnerID == "" {
		return fmt.Errorf("runner ID cannot be empty")
	}

	err := s.gdb.WithContext(ctx).Model(&types.AgentRunner{}).
		Where("id = ?", runnerID).
		Updates(map[string]interface{}{
			"status":    status,
			"last_seen": time.Now(),
		}).Error

	if err != nil {
		return fmt.Errorf("failed to update agent runner status: %w", err)
	}

	return nil
}

// UpdateAgentRunnerHeartbeat updates the last seen time for an agent runner
func (s *PostgresStore) UpdateAgentRunnerHeartbeat(ctx context.Context, runnerID string) error {
	if runnerID == "" {
		return fmt.Errorf("runner ID cannot be empty")
	}

	err := s.gdb.WithContext(ctx).Model(&types.AgentRunner{}).
		Where("id = ?", runnerID).
		Update("last_seen", time.Now()).Error

	if err != nil {
		return fmt.Errorf("failed to update agent runner heartbeat: %w", err)
	}

	return nil
}

// ListAgentRunners lists agent runners with pagination and filtering
func (s *PostgresStore) ListAgentRunners(ctx context.Context, query types.ListAgentRunnersQuery) ([]*types.AgentRunner, int64, error) {
	q := s.gdb.WithContext(ctx).Model(&types.AgentRunner{})

	// Apply filters
	if query.Status != "" {
		q = q.Where("status = ?", query.Status)
	}

	if query.HealthStatus != "" {
		q = q.Where("health_status = ?", query.HealthStatus)
	}

	if query.OnlineOnly {
		q = q.Where("status IN ?", []string{"online", "starting"})
	}

	// Count total records
	var total int64
	err := q.Count(&total).Error
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count agent runners: %w", err)
	}

	// Apply ordering
	orderBy := "created_at DESC"
	switch query.OrderBy {
	case "last_seen":
		orderBy = "last_seen DESC"
	case "id":
		orderBy = "id ASC"
	case "status":
		orderBy = "status ASC"
	}
	q = q.Order(orderBy)

	// Apply pagination
	if query.PageSize == 0 {
		query.PageSize = 20
	}
	if query.PageSize > 0 {
		offset := query.Page * query.PageSize
		q = q.Offset(offset).Limit(query.PageSize)
	}

	var runners []*types.AgentRunner
	err = q.Find(&runners).Error
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list agent runners: %w", err)
	}

	return runners, total, nil
}

// DeleteAgentRunner removes an agent runner record
func (s *PostgresStore) DeleteAgentRunner(ctx context.Context, runnerID string) error {
	if runnerID == "" {
		return fmt.Errorf("runner ID cannot be empty")
	}

	result := s.gdb.WithContext(ctx).Where("id = ?", runnerID).Delete(&types.AgentRunner{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete agent runner: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return ErrNotFound
	}

	log.Info().
		Str("runner_id", runnerID).
		Msg("Deleted agent runner")

	return nil
}

// CleanupStaleAgentRunners removes agent runners that haven't been seen in the specified duration
func (s *PostgresStore) CleanupStaleAgentRunners(ctx context.Context, staleThreshold time.Duration) (int64, error) {
	cutoff := time.Now().Add(-staleThreshold)

	result := s.gdb.WithContext(ctx).
		Where("last_seen < ?", cutoff).
		Where("status != ?", "offline").
		Delete(&types.AgentRunner{})

	if result.Error != nil {
		return 0, fmt.Errorf("failed to cleanup stale agent runners: %w", result.Error)
	}

	if result.RowsAffected > 0 {
		log.Info().
			Int64("count", result.RowsAffected).
			Dur("threshold", staleThreshold).
			Msg("Cleaned up stale agent runners")
	}

	return result.RowsAffected, nil
}

// GetOrCreateAgentRunner gets an existing agent runner or creates a new one if it doesn't exist
func (s *PostgresStore) GetOrCreateAgentRunner(ctx context.Context, runnerID string) (*types.AgentRunner, error) {
	runner, err := s.GetAgentRunner(ctx, runnerID)
	if err == nil {
		// Runner exists, update heartbeat and return
		err = s.UpdateAgentRunnerHeartbeat(ctx, runnerID)
		if err != nil {
			log.Warn().Err(err).Str("runner_id", runnerID).Msg("Failed to update runner heartbeat")
		}
		return runner, nil
	}

	if err != ErrNotFound {
		return nil, err
	}

	// Runner doesn't exist, create it
	return s.CreateAgentRunner(ctx, runnerID)
}
