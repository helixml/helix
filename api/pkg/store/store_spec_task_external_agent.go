package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// SpecTask External Agent methods (per-SpecTask agents spanning multiple sessions)

func (s *PostgresStore) CreateSpecTaskExternalAgent(ctx context.Context, agent *types.SpecTaskExternalAgent) error {
	if agent.ID == "" {
		return fmt.Errorf("external agent ID is required")
	}
	if agent.SpecTaskID == "" {
		return fmt.Errorf("spec_task_id is required")
	}

	result := s.gdb.WithContext(ctx).Create(agent)
	if result.Error != nil {
		return fmt.Errorf("failed to create spec task external agent: %w", result.Error)
	}

	log.Info().
		Str("agent_id", agent.ID).
		Str("spec_task_id", agent.SpecTaskID).
		Str("workspace_dir", agent.WorkspaceDir).
		Msg("Created spec task external agent")

	return nil
}

func (s *PostgresStore) GetSpecTaskExternalAgent(ctx context.Context, specTaskID string) (*types.SpecTaskExternalAgent, error) {
	var agent types.SpecTaskExternalAgent
	result := s.gdb.WithContext(ctx).
		Where("spec_task_id = ?", specTaskID).
		First(&agent)

	if result.Error != nil {
		return nil, fmt.Errorf("external agent not found for spec task %s: %w", specTaskID, result.Error)
	}

	return &agent, nil
}

func (s *PostgresStore) GetSpecTaskExternalAgentByID(ctx context.Context, agentID string) (*types.SpecTaskExternalAgent, error) {
	var agent types.SpecTaskExternalAgent
	result := s.gdb.WithContext(ctx).
		Where("id = ?", agentID).
		First(&agent)

	if result.Error != nil {
		return nil, fmt.Errorf("external agent not found: %s: %w", agentID, result.Error)
	}

	return &agent, nil
}

func (s *PostgresStore) UpdateSpecTaskExternalAgent(ctx context.Context, agent *types.SpecTaskExternalAgent) error {
	result := s.gdb.WithContext(ctx).Save(agent)
	if result.Error != nil {
		return fmt.Errorf("failed to update spec task external agent: %w", result.Error)
	}

	log.Debug().
		Str("agent_id", agent.ID).
		Str("status", agent.Status).
		Int("session_count", len(agent.HelixSessionIDs)).
		Msg("Updated spec task external agent")

	return nil
}

func (s *PostgresStore) DeleteSpecTaskExternalAgent(ctx context.Context, agentID string) error {
	result := s.gdb.WithContext(ctx).
		Where("id = ?", agentID).
		Delete(&types.SpecTaskExternalAgent{})

	if result.Error != nil {
		return fmt.Errorf("failed to delete spec task external agent: %w", result.Error)
	}

	log.Info().
		Str("agent_id", agentID).
		Msg("Deleted spec task external agent")

	return nil
}

func (s *PostgresStore) ListSpecTaskExternalAgents(ctx context.Context, userID string) ([]*types.SpecTaskExternalAgent, error) {
	var agents []*types.SpecTaskExternalAgent

	query := s.gdb.WithContext(ctx)
	if userID != "" {
		query = query.Where("user_id = ?", userID)
	}

	result := query.Order("created DESC").Find(&agents)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to list spec task external agents: %w", result.Error)
	}

	return agents, nil
}

// External Agent Activity methods (idle detection)

func (s *PostgresStore) UpsertExternalAgentActivity(ctx context.Context, activity *types.ExternalAgentActivity) error {
	// Update last_interaction timestamp
	activity.LastInteraction = time.Now()

	// Use GORM's Save which does upsert based on primary key
	result := s.gdb.WithContext(ctx).Save(activity)
	if result.Error != nil {
		return fmt.Errorf("failed to upsert external agent activity: %w", result.Error)
	}

	return nil
}

func (s *PostgresStore) GetExternalAgentActivity(ctx context.Context, agentID string) (*types.ExternalAgentActivity, error) {
	var activity types.ExternalAgentActivity
	result := s.gdb.WithContext(ctx).
		Where("external_agent_id = ?", agentID).
		First(&activity)

	if result.Error != nil {
		return nil, fmt.Errorf("activity not found for agent %s: %w", agentID, result.Error)
	}

	return &activity, nil
}

func (s *PostgresStore) GetIdleExternalAgents(ctx context.Context, cutoff time.Time, agentTypes []string) ([]*types.ExternalAgentActivity, error) {
	var activities []*types.ExternalAgentActivity

	query := s.gdb.WithContext(ctx).
		Where("last_interaction < ?", cutoff)

	if len(agentTypes) > 0 {
		query = query.Where("agent_type IN ?", agentTypes)
	}

	result := query.Find(&activities)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to get idle external agents: %w", result.Error)
	}

	log.Info().
		Time("cutoff", cutoff).
		Int("idle_count", len(activities)).
		Msg("Found idle external agents")

	return activities, nil
}

func (s *PostgresStore) DeleteExternalAgentActivity(ctx context.Context, agentID string) error {
	result := s.gdb.WithContext(ctx).
		Where("external_agent_id = ?", agentID).
		Delete(&types.ExternalAgentActivity{})

	if result.Error != nil {
		return fmt.Errorf("failed to delete external agent activity: %w", result.Error)
	}

	return nil
}

// GetExternalAgentActivityByLobbyID retrieves external agent activity by Wolf lobby ID
// Used for admin cleanup when PIN is needed to stop lobbies
func (s *PostgresStore) GetExternalAgentActivityByLobbyID(ctx context.Context, lobbyID string) (*types.ExternalAgentActivity, error) {
	var activity types.ExternalAgentActivity

	err := s.gdb.WithContext(ctx).
		Where("wolf_lobby_id = ?", lobbyID).
		First(&activity).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("no external agent activity found for lobby %s", lobbyID)
		}
		return nil, fmt.Errorf("failed to get external agent activity by lobby ID: %w", err)
	}

	return &activity, nil
}
