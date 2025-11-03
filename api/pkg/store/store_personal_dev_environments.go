package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/types"
)

// CreatePersonalDevEnvironment creates a new personal development environment
func (s *PostgresStore) CreatePersonalDevEnvironment(ctx context.Context, pde *types.PersonalDevEnvironment) (*types.PersonalDevEnvironment, error) {
	if pde.ID == "" {
		pde.ID = uuid.New().String()
	}

	now := time.Now()
	pde.Created = now
	pde.Updated = now

	if err := s.gdb.WithContext(ctx).Create(pde).Error; err != nil {
		return nil, err
	}

	return pde, nil
}

// GetPersonalDevEnvironment retrieves a personal development environment by ID
func (s *PostgresStore) GetPersonalDevEnvironment(ctx context.Context, id string) (*types.PersonalDevEnvironment, error) {
	var pde types.PersonalDevEnvironment

	if err := s.gdb.WithContext(ctx).Where("id = ?", id).First(&pde).Error; err != nil {
		return nil, err
	}

	return &pde, nil
}

// GetPersonalDevEnvironmentByWolfAppID retrieves a personal development environment by Wolf app ID
func (s *PostgresStore) GetPersonalDevEnvironmentByWolfAppID(ctx context.Context, wolfAppID string) (*types.PersonalDevEnvironment, error) {
	var pde types.PersonalDevEnvironment

	if err := s.gdb.WithContext(ctx).Where("wolf_app_id = ?", wolfAppID).First(&pde).Error; err != nil {
		return nil, err
	}

	return &pde, nil
}

// UpdatePersonalDevEnvironment updates an existing personal development environment
func (s *PostgresStore) UpdatePersonalDevEnvironment(ctx context.Context, pde *types.PersonalDevEnvironment) (*types.PersonalDevEnvironment, error) {
	pde.Updated = time.Now()

	if err := s.gdb.WithContext(ctx).Save(pde).Error; err != nil {
		return nil, err
	}

	return pde, nil
}

// ListPersonalDevEnvironments lists all personal development environments for a user
// If userID is empty, returns all personal dev environments across all users (for reconciliation)
func (s *PostgresStore) ListPersonalDevEnvironments(ctx context.Context, userID string) ([]*types.PersonalDevEnvironment, error) {
	var pdes []*types.PersonalDevEnvironment

	query := s.gdb.WithContext(ctx)
	if userID != "" {
		query = query.Where("user_id = ?", userID)
	}
	query = query.Order("created DESC")

	if err := query.Find(&pdes).Error; err != nil {
		return nil, err
	}

	return pdes, nil
}

// DeletePersonalDevEnvironment deletes a personal development environment
func (s *PostgresStore) DeletePersonalDevEnvironment(ctx context.Context, id string) error {
	return s.gdb.WithContext(ctx).Delete(&types.PersonalDevEnvironment{}, "id = ?", id).Error
}