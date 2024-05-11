package store

import (
	"context"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

func (s *PostgresStore) CreateGptScriptRunnerTask(ctx context.Context, task *types.GptScriptRunnerTask) (*types.GptScriptRunnerTask, error) {
	if task.ID == "" {
		task.ID = system.GenerateGptScriptTaskID()
	}

	if task.AppID == "" {
		return nil, fmt.Errorf("app ID not specified")
	}

	task.Created = time.Now()

	err := s.gdb.WithContext(ctx).Create(task).Error
	if err != nil {
		return nil, err
	}

	return task, nil
}

func (s *PostgresStore) ListGptScriptRunnerTasks(ctx context.Context, q *types.GptScriptRunnerTasksQuery) ([]*types.GptScriptRunnerTask, error) {
	var tasks []*types.GptScriptRunnerTask
	query := s.gdb.WithContext(ctx)

	if q.AppID != "" {
		query = query.Where("app_id = ?", q.AppID)
	}

	if q.State != "" {
		query = query.Where("state = ?", q.State)
	}

	if q.Owner != "" {
		query = query.Where("owner = ?", q.Owner)
	}

	err := query.Find(&tasks).Error
	if err != nil {
		return nil, err
	}

	return tasks, nil
}

func (s *PostgresStore) DeleteGptScriptRunnerTask(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("id not specified")
	}

	err := s.gdb.WithContext(ctx).Delete(&types.GptScriptRunnerTask{
		ID: id,
	}).Error
	if err != nil {
		return err
	}

	return nil
}
