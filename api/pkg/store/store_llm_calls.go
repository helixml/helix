package store

import (
	"context"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

func (s *PostgresStore) CreateLLMCall(ctx context.Context, call *types.LLMCall) (*types.LLMCall, error) {
	if call.ID == "" {
		call.ID = system.GenerateLLMCallID()
	}

	if call.Created.IsZero() {
		call.Created = time.Now()
	}

	err := s.gdb.WithContext(ctx).Create(call).Error
	if err != nil {
		return nil, err
	}
	return call, nil
}

type ListLLMCallsQuery struct {
	AppID         string
	SessionFilter string
	UserID        string

	Page    int
	PerPage int
}

func (s *PostgresStore) ListLLMCalls(ctx context.Context, q *ListLLMCallsQuery) ([]*types.LLMCall, int64, error) {
	var calls []*types.LLMCall
	var totalCount int64

	offset := (q.Page - 1) * q.PerPage

	query := s.gdb.WithContext(ctx).Model(&types.LLMCall{})

	if q.SessionFilter != "" {
		query = query.Where("session_id LIKE ?", "%"+q.SessionFilter+"%")
	}

	if q.AppID != "" {
		query = query.Where("app_id = ?", q.AppID)
	}

	if q.UserID != "" {
		query = query.Where("user_id = ?", q.UserID)
	}

	err := query.Count(&totalCount).Error
	if err != nil {
		return nil, 0, err
	}

	err = query.
		Order("created DESC").
		Offset(offset).
		Limit(q.PerPage).
		Find(&calls).Error

	if err != nil {
		return nil, 0, err
	}

	return calls, totalCount, nil
}
