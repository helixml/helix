package store

import (
	"context"
	"errors"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

func (s *PostgresStore) CreateStepInfo(ctx context.Context, stepInfo *types.StepInfo) (*types.StepInfo, error) {
	if stepInfo.ID == "" {
		stepInfo.ID = system.GenerateID()
	}

	if stepInfo.Created.IsZero() {
		stepInfo.Created = time.Now()
	}
	if stepInfo.Updated.IsZero() {
		stepInfo.Updated = time.Now()
	}

	err := s.gdb.WithContext(ctx).Create(stepInfo).Error
	if err != nil {
		return nil, err
	}
	return stepInfo, nil
}

type ListStepInfosQuery struct {
	SessionID     string
	InteractionID string
	AppID         string
}

func (s *PostgresStore) ListStepInfos(ctx context.Context, query *ListStepInfosQuery) ([]*types.StepInfo, error) {
	var stepInfos []*types.StepInfo

	dbQuery := s.gdb.WithContext(ctx).Model(&types.StepInfo{})

	if query.SessionID != "" {
		dbQuery = dbQuery.Where("session_id = ?", query.SessionID)
	}

	if query.AppID != "" {
		dbQuery = dbQuery.Where("app_id = ?", query.AppID)
	}

	if query.InteractionID != "" {
		dbQuery = dbQuery.Where("interaction_id = ?", query.InteractionID)
	}

	// Oldest first (left to right)
	err := dbQuery.Order("created ASC").Find(&stepInfos).Error
	if err != nil {
		return nil, err
	}

	return stepInfos, nil
}

func (s *PostgresStore) DeleteStepInfo(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return errors.New("sessionID is required")
	}
	return s.gdb.WithContext(ctx).Where("session_id = ?", sessionID).Delete(&types.StepInfo{}).Error
}
