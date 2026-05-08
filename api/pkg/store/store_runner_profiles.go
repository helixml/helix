package store

import (
	"context"
	"errors"

	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
)

// Runner profile CRUD.

func (s *PostgresStore) CreateRunnerProfile(ctx context.Context, p *types.RunnerProfile) (*types.RunnerProfile, error) {
	if p.ID == "" {
		return nil, errors.New("profile ID is required")
	}
	if p.Name == "" {
		return nil, errors.New("profile name is required")
	}
	if p.ComposeYAML == "" {
		return nil, errors.New("profile compose YAML is required")
	}
	if err := s.gdb.WithContext(ctx).Create(p).Error; err != nil {
		return nil, err
	}
	return p, nil
}

func (s *PostgresStore) GetRunnerProfile(ctx context.Context, id string) (*types.RunnerProfile, error) {
	if id == "" {
		return nil, errors.New("profile ID is required")
	}
	var p types.RunnerProfile
	err := s.gdb.WithContext(ctx).Where("id = ?", id).First(&p).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &p, nil
}

func (s *PostgresStore) GetRunnerProfileByName(ctx context.Context, name string) (*types.RunnerProfile, error) {
	if name == "" {
		return nil, errors.New("profile name is required")
	}
	var p types.RunnerProfile
	err := s.gdb.WithContext(ctx).Where("name = ?", name).First(&p).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &p, nil
}

func (s *PostgresStore) UpdateRunnerProfile(ctx context.Context, p *types.RunnerProfile) (*types.RunnerProfile, error) {
	if p.ID == "" {
		return nil, errors.New("profile ID is required")
	}
	// Save() updates all fields (incl. zero values). We want full overwrite
	// because the caller has just re-derived Models + Count from the new
	// YAML and may legitimately be clearing optional fields.
	if err := s.gdb.WithContext(ctx).Save(p).Error; err != nil {
		return nil, err
	}
	return p, nil
}

func (s *PostgresStore) DeleteRunnerProfile(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("profile ID is required")
	}
	res := s.gdb.WithContext(ctx).Where("id = ?", id).Delete(&types.RunnerProfile{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore) ListRunnerProfiles(ctx context.Context) ([]*types.RunnerProfile, error) {
	var out []*types.RunnerProfile
	err := s.gdb.WithContext(ctx).Order("name ASC").Find(&out).Error
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Runner-to-profile assignment CRUD. RunnerID is the primary key; a runner
// has at most one assignment.

func (s *PostgresStore) GetRunnerAssignment(ctx context.Context, runnerID string) (*types.RunnerAssignment, error) {
	if runnerID == "" {
		return nil, errors.New("runner ID is required")
	}
	var a types.RunnerAssignment
	err := s.gdb.WithContext(ctx).Where("runner_id = ?", runnerID).First(&a).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &a, nil
}

func (s *PostgresStore) SetRunnerAssignment(ctx context.Context, a *types.RunnerAssignment) (*types.RunnerAssignment, error) {
	if a.RunnerID == "" {
		return nil, errors.New("runner ID is required")
	}
	if a.ProfileID == "" {
		return nil, errors.New("profile ID is required")
	}
	// Upsert: a runner has at most one assignment, so reassigning replaces.
	if err := s.gdb.WithContext(ctx).Save(a).Error; err != nil {
		return nil, err
	}
	return a, nil
}

func (s *PostgresStore) DeleteRunnerAssignment(ctx context.Context, runnerID string) error {
	if runnerID == "" {
		return errors.New("runner ID is required")
	}
	// Idempotent: deleting a non-existent assignment is not an error
	// (the runner is already in the desired "no profile" state).
	return s.gdb.WithContext(ctx).Where("runner_id = ?", runnerID).Delete(&types.RunnerAssignment{}).Error
}

func (s *PostgresStore) ListRunnerAssignments(ctx context.Context) ([]*types.RunnerAssignment, error) {
	var out []*types.RunnerAssignment
	err := s.gdb.WithContext(ctx).Find(&out).Error
	if err != nil {
		return nil, err
	}
	return out, nil
}
