package store

import (
	"context"
	"errors"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
)

func (s *PostgresStore) CreateQuestionSetExecution(ctx context.Context, execution *types.QuestionSetExecution) (*types.QuestionSetExecution, error) {
	if execution.ID == "" {
		execution.ID = system.GenerateQuestionSetExecutionID()
	}

	execution.Created = time.Now()
	execution.Updated = time.Now()

	err := s.gdb.WithContext(ctx).Create(&execution).Error
	if err != nil {
		return nil, err
	}
	return execution, nil
}

func (s *PostgresStore) GetQuestionSetExecution(ctx context.Context, id string) (*types.QuestionSetExecution, error) {
	if id == "" {
		return nil, errors.New("id is required")
	}

	db := s.gdb.WithContext(ctx)
	var execution types.QuestionSetExecution
	err := db.Where("id = ?", id).First(&execution).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return &execution, nil
}

func (s *PostgresStore) UpdateQuestionSetExecution(ctx context.Context, execution *types.QuestionSetExecution) (*types.QuestionSetExecution, error) {
	if execution.ID == "" {
		return nil, errors.New("execution ID is required")
	}

	execution.Updated = time.Now()
	err := s.gdb.WithContext(ctx).Save(&execution).Error
	if err != nil {
		return nil, err
	}
	return execution, nil
}

func (s *PostgresStore) ListQuestionSetExecutions(ctx context.Context, q *ListQuestionSetExecutionsQuery) ([]*types.QuestionSetExecution, error) {
	var executions []*types.QuestionSetExecution

	query := s.gdb.WithContext(ctx)

	if q.QuestionSetID != "" {
		query = query.Where("question_set_id = ?", q.QuestionSetID)
	}

	if q.AppID != "" {
		query = query.Where("app_id = ?", q.AppID)
	}

	if q.Offset > 0 {
		query = query.Offset(q.Offset)
	}

	if q.Limit > 0 {
		query = query.Limit(q.Limit)
	}

	err := query.Order("created DESC").Find(&executions).Error
	if err != nil {
		return nil, err
	}
	return executions, nil
}
