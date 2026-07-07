package store

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/types"
)

// CreateGuidelinesHistory saves a version of guidelines to history
func (s *PostgresStore) CreateGuidelinesHistory(ctx context.Context, history *types.GuidelinesHistory) error {
	if history.ID == "" {
		return fmt.Errorf("id not specified")
	}
	if history.OrganizationID == "" && history.ProjectID == "" && history.UserID == "" {
		return fmt.Errorf("either organization_id, project_id, or user_id must be specified")
	}
	return s.gdb.WithContext(ctx).Create(history).Error
}

func (s *PostgresStore) ListGuidelinesHistory(ctx context.Context, organizationID, projectID, userID string) ([]*types.GuidelinesHistory, error) {
	var history []*types.GuidelinesHistory
	query := s.gdb.WithContext(ctx)

	if organizationID != "" {
		query = query.Where("organization_id = ?", organizationID)
	}
	if projectID != "" {
		query = query.Where("project_id = ?", projectID)
	}
	if userID != "" {
		query = query.Where("user_id = ?", userID)
	}

	err := query.Order("version DESC").Find(&history).Error
	if err != nil {
		return nil, err
	}
	return history, nil
}
