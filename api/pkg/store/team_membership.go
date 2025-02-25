package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
)

type ListTeamMembershipsQuery struct {
	TeamID string
	UserID string
}

type GetTeamMembershipQuery struct {
	TeamID string
	UserID string
}

// CreateTeamMembership creates a new team membership
func (s *PostgresStore) CreateTeamMembership(ctx context.Context, membership *types.TeamMembership) (*types.TeamMembership, error) {
	if membership.UserID == "" {
		return nil, fmt.Errorf("user_id not specified")
	}

	if membership.TeamID == "" {
		return nil, fmt.Errorf("team_id not specified")
	}

	membership.CreatedAt = time.Now()
	membership.UpdatedAt = time.Now()

	err := s.gdb.WithContext(ctx).Create(membership).Error
	if err != nil {
		return nil, fmt.Errorf("failed to create team membership: %w", err)
	}
	return s.GetTeamMembership(ctx, &GetTeamMembershipQuery{
		TeamID: membership.TeamID,
		UserID: membership.UserID,
	})
}

// GetTeamMembership retrieves a team membership by team ID and user ID
func (s *PostgresStore) GetTeamMembership(ctx context.Context, q *GetTeamMembershipQuery) (*types.TeamMembership, error) {
	if q.TeamID == "" || q.UserID == "" {
		return nil, fmt.Errorf("team_id and user_id must be specified")
	}

	var membership types.TeamMembership
	err := s.gdb.WithContext(ctx).
		Where("team_id = ? AND user_id = ?", q.TeamID, q.UserID).
		Preload("User").
		Preload("Team").
		First(&membership).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &membership, nil
}

// ListTeamMemberships lists team memberships based on query parameters
func (s *PostgresStore) ListTeamMemberships(ctx context.Context, q *ListTeamMembershipsQuery) ([]*types.TeamMembership, error) {
	query := s.gdb.WithContext(ctx)

	if q != nil {
		if q.TeamID != "" {
			query = query.Where("team_id = ?", q.TeamID)
		}
		if q.UserID != "" {
			query = query.Where("user_id = ?", q.UserID)
		}
	}

	var memberships []*types.TeamMembership
	err := query.Preload("User").Preload("Team").Find(&memberships).Error
	if err != nil {
		return nil, err
	}

	return memberships, nil
}

// DeleteTeamMembership deletes a team membership
func (s *PostgresStore) DeleteTeamMembership(ctx context.Context, teamID, userID string) error {
	if teamID == "" || userID == "" {
		return fmt.Errorf("team_id and user_id must be specified")
	}

	return s.gdb.WithContext(ctx).
		Where("team_id = ? AND user_id = ?", teamID, userID).
		Delete(&types.TeamMembership{}).Error
}
