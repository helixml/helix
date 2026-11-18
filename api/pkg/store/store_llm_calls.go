package store

import (
	"context"
	"time"

	"gorm.io/gorm"

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

	err := s.gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Omit("OriginalRequest", "Request", "Response").Create(call).Error; err != nil {
			return err
		}
		if len(call.OriginalRequest)+len(call.Request)+len(call.Response) == 0 {
			return nil
		}
		if err := ensurePayloadDay(tx, call.Created); err != nil {
			return err
		}
		return tx.Create(&llmCallPayload{ID: call.ID, Created: call.Created, OriginalRequest: call.OriginalRequest, Request: call.Request, Response: call.Response}).Error
	})
	if err != nil {
		return nil, err
	}
	return call, nil
}

type ListLLMCallsQuery struct {
	MetadataOnly  bool
	ID            string
	AppID         string
	SessionID     string
	InteractionID string
	UserID        string

	Page    int
	PerPage int
	Order   string
}

func (s *PostgresStore) ListLLMCalls(ctx context.Context, q *ListLLMCallsQuery) ([]*types.LLMCall, int64, error) {
	var calls []*types.LLMCall
	var totalCount int64

	offset := (q.Page - 1) * q.PerPage

	query := s.gdb.WithContext(ctx).Model(&types.LLMCall{})

	if q.ID != "" {
		query = query.Where("id = ?", q.ID)
	}
	if q.SessionID != "" {
		query = query.Where("session_id = ?", q.SessionID)
	}

	if q.InteractionID != "" {
		query = query.Where("interaction_id = ?", q.InteractionID)
	}

	if q.AppID != "" {
		query = query.Where("app_id = ?", q.AppID)
	}

	if q.UserID != "" {
		query = query.Where("user_id = ?", q.UserID)
	}

	if q.Order != "" {
		query = query.Order(q.Order)
	}

	err := query.Count(&totalCount).Error
	if err != nil {
		return nil, 0, err
	}

	if q.MetadataOnly {
		query = query.Omit("OriginalRequest", "Request", "Response")
	}
	err = query.
		Order("created DESC").
		Offset(offset).
		Limit(q.PerPage).
		Find(&calls).Error

	if err != nil {
		return nil, 0, err
	}

	if !q.MetadataOnly && len(calls) > 0 {
		ids := make([]string, 0, len(calls))
		byID := make(map[string]*types.LLMCall, len(calls))
		first, last := calls[0].Created, calls[0].Created
		for _, call := range calls {
			ids = append(ids, call.ID)
			byID[call.ID] = call
			if call.Created.Before(first) {
				first = call.Created
			}
			if call.Created.After(last) {
				last = call.Created
			}
		}
		var bodies []llmCallPayload
		if err := s.gdb.WithContext(ctx).Where("created BETWEEN ? AND ? AND id IN ?", first, last, ids).Find(&bodies).Error; err != nil {
			return nil, 0, err
		}
		for _, body := range bodies {
			call := byID[body.ID]
			call.OriginalRequest = body.OriginalRequest
			call.Request = body.Request
			call.Response = body.Response
		}
	}
	return calls, totalCount, nil
}
