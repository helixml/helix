package store

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
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
