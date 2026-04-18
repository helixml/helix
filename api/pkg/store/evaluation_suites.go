package store

import (
	"context"
	"errors"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
)

func (s *PostgresStore) CreateEvaluationSuite(ctx context.Context, suite *types.EvaluationSuite) (*types.EvaluationSuite, error) {
	if suite.ID == "" {
		suite.ID = system.GenerateEvaluationSuiteID()
	}
	if suite.UserID == "" {
		return nil, errors.New("user_id is required")
	}
	if suite.AppID == "" {
		return nil, errors.New("app_id is required")
	}

	now := time.Now()
	suite.Created = now
	suite.Updated = now

	db := s.gdb.WithContext(ctx)
	if err := db.Create(suite).Error; err != nil {
		return nil, err
	}
	return suite, nil
}

func (s *PostgresStore) GetEvaluationSuite(ctx context.Context, id string) (*types.EvaluationSuite, error) {
	if id == "" {
		return nil, errors.New("id is required")
	}

	db := s.gdb.WithContext(ctx)
	var suite types.EvaluationSuite
	err := db.Where("id = ?", id).First(&suite).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &suite, nil
}

func (s *PostgresStore) UpdateEvaluationSuite(ctx context.Context, suite *types.EvaluationSuite) (*types.EvaluationSuite, error) {
	if suite.ID == "" {
		return nil, errors.New("id is required")
	}

	existing, err := s.GetEvaluationSuite(ctx, suite.ID)
	if err != nil {
		return nil, err
	}

	suite.Created = existing.Created
	suite.Updated = time.Now()

	db := s.gdb.WithContext(ctx)
	if err := db.Save(suite).Error; err != nil {
		return nil, err
	}
	return suite, nil
}

func (s *PostgresStore) ListEvaluationSuites(ctx context.Context, req *types.ListEvaluationSuitesRequest) ([]*types.EvaluationSuite, error) {
	db := s.gdb.WithContext(ctx)
	query := db.Model(&types.EvaluationSuite{})

	if req.UserID != "" {
		query = query.Where("user_id = ?", req.UserID)
	}
	if req.OrganizationID != "" {
		query = query.Where("organization_id = ?", req.OrganizationID)
	}
	if req.AppID != "" {
		query = query.Where("app_id = ?", req.AppID)
	}

	var suites []*types.EvaluationSuite
	if err := query.Order("created DESC").Find(&suites).Error; err != nil {
		return nil, err
	}
	return suites, nil
}

func (s *PostgresStore) DeleteEvaluationSuite(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("id is required")
	}

	db := s.gdb.WithContext(ctx)
	err := db.Where("id = ?", id).Delete(&types.EvaluationSuite{}).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return err
	}
	return nil
}
