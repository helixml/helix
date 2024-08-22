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

	call.Created = time.Now()

	err := s.gdb.WithContext(ctx).Create(call).Error
	if err != nil {
		return nil, err
	}
	return call, nil
}

func (s *PostgresStore) ListLLMCalls(ctx context.Context, page, pageSize int) ([]*types.LLMCall, int64, error) {
	var calls []*types.LLMCall
	var totalCount int64

	offset := (page - 1) * pageSize

	err := s.gdb.WithContext(ctx).Model(&types.LLMCall{}).Count(&totalCount).Error
	if err != nil {
		return nil, 0, err
	}

	err = s.gdb.WithContext(ctx).
		Order("created DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&calls).Error

	if err != nil {
		return nil, 0, err
	}

	return calls, totalCount, nil
}
