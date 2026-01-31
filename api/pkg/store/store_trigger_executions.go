package store

import (
	"context"
	"errors"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// ResetRunningExecutions on start we have to reset any running executions as we will not pick them up again to finish (we could in the future),
// for now we fail them all but if we put them into "pending" state then we could pick them up again and retry
func (s *PostgresStore) ResetRunningExecutions(ctx context.Context) error {
	err := s.gdb.WithContext(ctx).Model(&types.TriggerExecution{}).
		Where("status = ?", types.TriggerExecutionStatusRunning).
		Updates(map[string]any{
			"status": types.TriggerExecutionStatusError,
			"error":  "Execution was interrupted",
		}).
		Error
	if err != nil {
		return err
	}
	return nil
}

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

	query := s.gdb.WithContext(ctx)

	if q.TriggerID != "" {
		query = query.Where("trigger_configuration_id = ?", q.TriggerID)
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
