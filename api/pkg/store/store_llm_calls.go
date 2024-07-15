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
