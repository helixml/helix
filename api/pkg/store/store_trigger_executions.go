package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

// CreateTriggerExecutionUnlessRunning atomically reserves a trigger for one
// execution. If another execution is still running, it records this attempt as
// skipped and returns started=false. Locking the trigger row prevents a manual
// execution racing a scheduled execution from starting two agent sessions.
func (s *PostgresStore) CreateTriggerExecutionUnlessRunning(ctx context.Context, execution *types.TriggerExecution) (*types.TriggerExecution, bool, error) {
	if execution.TriggerConfigurationID == "" {
		return nil, false, errors.New("trigger configuration ID is required")
	}

	started := false
	err := s.gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var trigger types.TriggerConfiguration
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id").
			Where("id = ?", execution.TriggerConfigurationID).
			First(&trigger).Error; err != nil {
			return fmt.Errorf("lock trigger configuration: %w", err)
		}

		var running types.TriggerExecution
		err := tx.Where("trigger_configuration_id = ? AND status = ?", execution.TriggerConfigurationID, types.TriggerExecutionStatusRunning).
			Order("created DESC").
			First(&running).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("find running trigger execution: %w", err)
		}

		if execution.ID == "" {
			execution.ID = system.GenerateTriggerExecutionID()
		}
		execution.Created = time.Now()
		execution.Updated = execution.Created
		if err == nil {
			execution.Status = types.TriggerExecutionStatusSkipped
			execution.Error = fmt.Sprintf("Previous execution %s is still running", running.ID)
			execution.SessionID = ""
		} else {
			execution.Status = types.TriggerExecutionStatusRunning
			started = true
		}

		if err := tx.Create(execution).Error; err != nil {
			return fmt.Errorf("create trigger execution: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return execution, started, nil
}

// FinishTriggerExecution transitions the running execution linked to a session
// exactly once. Repeated task_completed calls and late failure events cannot
// overwrite an already-terminal result.
func (s *PostgresStore) FinishTriggerExecution(ctx context.Context, sessionID string, status types.TriggerExecutionStatus, message string) (*types.TriggerExecution, error) {
	if sessionID == "" {
		return nil, errors.New("session ID is required")
	}
	if status != types.TriggerExecutionStatusSuccess && status != types.TriggerExecutionStatusError {
		return nil, fmt.Errorf("terminal trigger execution status is required, got %q", status)
	}

	var execution types.TriggerExecution
	err := s.gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("session_id = ? AND status = ?", sessionID, types.TriggerExecutionStatusRunning).
			Order("created DESC").
			First(&execution).Error; err != nil {
			return err
		}

		execution.Status = status
		execution.DurationMs = time.Since(execution.Created).Milliseconds()
		if status == types.TriggerExecutionStatusSuccess {
			execution.Output = message
			execution.Error = ""
		} else {
			execution.Error = message
		}
		execution.Updated = time.Now()
		return tx.Save(&execution).Error
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &execution, nil
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
