package store

import (
	"context"
	"errors"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
)

func (s *PostgresStore) CreateEvaluationRun(ctx context.Context, run *types.EvaluationRun) (*types.EvaluationRun, error) {
	if run.ID == "" {
		run.ID = system.GenerateEvaluationRunID()
	}
	if run.SuiteID == "" {
		return nil, errors.New("suite_id is required")
	}
	if run.AppID == "" {
		return nil, errors.New("app_id is required")
	}

	now := time.Now()
	run.Created = now
	run.Updated = now

	db := s.gdb.WithContext(ctx)
	if err := db.Create(run).Error; err != nil {
		return nil, err
	}
	return run, nil
}

func (s *PostgresStore) GetEvaluationRun(ctx context.Context, id string) (*types.EvaluationRun, error) {
	if id == "" {
		return nil, errors.New("id is required")
	}

	db := s.gdb.WithContext(ctx)
	var run types.EvaluationRun
	err := db.Where("id = ?", id).First(&run).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &run, nil
}

func (s *PostgresStore) UpdateEvaluationRun(ctx context.Context, run *types.EvaluationRun) (*types.EvaluationRun, error) {
	if run.ID == "" {
		return nil, errors.New("id is required")
	}

	existing, err := s.GetEvaluationRun(ctx, run.ID)
	if err != nil {
		return nil, err
	}

	run.Created = existing.Created
	run.Updated = time.Now()

	db := s.gdb.WithContext(ctx)
	if err := db.Save(run).Error; err != nil {
		return nil, err
	}
	return run, nil
}

func (s *PostgresStore) ListEvaluationRuns(ctx context.Context, req *types.ListEvaluationRunsRequest) ([]*types.EvaluationRun, error) {
	db := s.gdb.WithContext(ctx)
	query := db.Model(&types.EvaluationRun{})

	if req.SuiteID != "" {
		query = query.Where("suite_id = ?", req.SuiteID)
	}
	if req.AppID != "" {
		query = query.Where("app_id = ?", req.AppID)
	}
	if req.UserID != "" {
		query = query.Where("user_id = ?", req.UserID)
	}
	if req.OrganizationID != "" {
		query = query.Where("organization_id = ?", req.OrganizationID)
	}

	if req.Limit > 0 {
		query = query.Limit(req.Limit)
	}
	if req.Offset > 0 {
		query = query.Offset(req.Offset)
	}

	var runs []*types.EvaluationRun
	if err := query.Order("created DESC").Find(&runs).Error; err != nil {
		return nil, err
	}
	return runs, nil
}

func (s *PostgresStore) DeleteEvaluationRun(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("id is required")
	}

	db := s.gdb.WithContext(ctx)
	err := db.Where("id = ?", id).Delete(&types.EvaluationRun{}).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return err
	}
	return nil
}
