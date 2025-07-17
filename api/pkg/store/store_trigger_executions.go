package store

import (
	"context"
	"errors"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

func (s *PostgresStore) CreateTriggerExecution(ctx context.Context, execution *types.TriggerExecution) (*types.TriggerExecution, error) {
	if execution.ID == "" {
		execution.ID = system.GenerateTriggerExecutionID()
	}

	execution.Created = time.Now()
	execution.Updated = time.Now()

	err := s.gdb.WithContext(ctx).Create(&execution).Error
	if err != nil {
		return nil, err
	}
	return execution, nil
}

func (s *PostgresStore) UpdateTriggerExecution(ctx context.Context, execution *types.TriggerExecution) (*types.TriggerExecution, error) {
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

func (s *PostgresStore) ListTriggerExecutions(ctx context.Context, q *ListTriggerExecutionsQuery) ([]*types.TriggerExecution, error) {
	var executions []*types.TriggerExecution
	err := s.gdb.WithContext(ctx).Where("trigger_configuration_id = ?", q.TriggerID).Order("created DESC").Find(&executions).Error
	if err != nil {
		return nil, err
	}
	return executions, nil
}
