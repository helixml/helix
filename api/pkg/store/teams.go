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

type ListTeamsQuery struct {
	OrganizationID string
}

type GetTeamQuery struct {
	ID             string
	OrganizationID string
	Name           string
}

// CreateTeam creates a new team within an organization
func (s *PostgresStore) CreateTeam(ctx context.Context, team *types.Team) (*types.Team, error) {
	if team.ID == "" {
		team.ID = system.GenerateTeamID()
	}

	if team.OrganizationID == "" {
		return nil, fmt.Errorf("organization_id not specified")
	}

	team.CreatedAt = time.Now()
	team.UpdatedAt = time.Now()

	err := s.gdb.WithContext(ctx).Create(team).Error
	if err != nil {
		return nil, err
	}
	return s.GetTeam(ctx, &GetTeamQuery{ID: team.ID})
}

// GetTeam retrieves a team by ID and optionally organization ID
func (s *PostgresStore) GetTeam(ctx context.Context, q *GetTeamQuery) (*types.Team, error) {
	if q.ID == "" && (q.OrganizationID == "" || q.Name == "") {
		return nil, fmt.Errorf("id or organization_id and name not specified")
	}

	query := s.gdb.WithContext(ctx)

	if q.ID != "" {
		query = query.Where("id = ?", q.ID)
	}

	if q.OrganizationID != "" {
		query = query.Where("organization_id = ?", q.OrganizationID)
	}

	if q.Name != "" {
		query = query.Where("name = ?", q.Name)
	}

	var team types.Team
	err := query.First(&team).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &team, nil
}

// ListTeams lists teams based on query parameters
func (s *PostgresStore) ListTeams(ctx context.Context, q *ListTeamsQuery) ([]*types.Team, error) {
	query := s.gdb.WithContext(ctx)

	if q != nil && q.OrganizationID != "" {
		query = query.Where("organization_id = ?", q.OrganizationID)
	}

	var teams []*types.Team
	err := query.Find(&teams).Error
	if err != nil {
		return nil, err
	}

	return teams, nil
}

// UpdateTeam updates an existing team
func (s *PostgresStore) UpdateTeam(ctx context.Context, team *types.Team) (*types.Team, error) {
	if team.ID == "" {
		return nil, fmt.Errorf("id not specified")
	}

	if team.OrganizationID == "" {
		return nil, fmt.Errorf("organization_id not specified")
	}

	team.UpdatedAt = time.Now()

	err := s.gdb.WithContext(ctx).Save(team).Error
	if err != nil {
		return nil, err
	}
	return s.GetTeam(ctx, &GetTeamQuery{ID: team.ID})
}

// DeleteTeam deletes a team by ID
func (s *PostgresStore) DeleteTeam(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("id not specified")
	}

	err := s.gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Delete all memberships first
		if err := tx.Where("team_id = ?", id).Delete(&types.Membership{}).Error; err != nil {
			return err
		}

		// Delete the team
		return tx.Delete(&types.Team{ID: id}).Error
	})

	return err
}
