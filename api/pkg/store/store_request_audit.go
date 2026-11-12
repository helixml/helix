package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

const (
	requestAuditDefaultLimit = 50
	requestAuditMaxLimit     = 500
)

// InsertRequestAuditEvents persists a batch of audit events in one statement.
func (s *PostgresStore) InsertRequestAuditEvents(ctx context.Context, events []*types.RequestAuditEvent) error {
	if len(events) == 0 {
		return nil
	}
	for _, event := range events {
		if event.ID == "" {
			event.ID = "rau_" + system.GenerateUUID()
		}
	}
	if err := s.gdb.WithContext(ctx).Create(events).Error; err != nil {
		return fmt.Errorf("failed to insert request audit events: %w", err)
	}
	return nil
}

// ListRequestAuditEvents returns events newest-first.
func (s *PostgresStore) ListRequestAuditEvents(ctx context.Context, filters *types.RequestAuditEventFilters) (*types.RequestAuditEventResponse, error) {
	var events []*types.RequestAuditEvent
	var total int64

	db := s.gdb.WithContext(ctx).Model(&types.RequestAuditEvent{})
	if filters != nil {
		if filters.UserID != "" {
			db = db.Where("user_id = ?", filters.UserID)
		}
		if filters.RemoteAddr != "" {
			db = db.Where("remote_addr = ?", filters.RemoteAddr)
		}
		if filters.PathPrefix != "" {
			db = db.Where(`path LIKE ? ESCAPE '\'`, escapeLikeValue(filters.PathPrefix)+"%")
		}
		if filters.Method != "" {
			db = db.Where("method = ?", strings.ToUpper(filters.Method))
		}
		if filters.Status != nil {
			db = db.Where("status = ?", *filters.Status)
		}
		if filters.Since != nil {
			db = db.Where("created >= ?", *filters.Since)
		}
		if filters.Until != nil {
			db = db.Where("created <= ?", *filters.Until)
		}
	}

	if err := db.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("failed to count request audit events: %w", err)
	}

	limit := requestAuditDefaultLimit
	offset := 0
	if filters != nil {
		if filters.Limit > 0 {
			limit = min(filters.Limit, requestAuditMaxLimit)
		}
		if filters.Offset > 0 {
			offset = filters.Offset
		}
	}

	if err := db.Order("created DESC").Limit(limit).Offset(offset).Find(&events).Error; err != nil {
		return nil, fmt.Errorf("failed to list request audit events: %w", err)
	}

	return &types.RequestAuditEventResponse{
		Events: events,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}, nil
}

// PruneRequestAuditEvents deletes events created before the cutoff in chunks so
// a large backlog does not hold one long transaction.
func (s *PostgresStore) PruneRequestAuditEvents(ctx context.Context, before time.Time, batchSize int) (int64, error) {
	if batchSize <= 0 {
		batchSize = 5000
	}

	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		result := s.gdb.WithContext(ctx).Exec(`
			DELETE FROM request_audit_events
			WHERE id IN (
				SELECT id FROM request_audit_events
				WHERE created < ?
				ORDER BY created
				LIMIT ?
			)`, before, batchSize)
		if result.Error != nil {
			return total, fmt.Errorf("failed to prune request audit events: %w", result.Error)
		}
		total += result.RowsAffected
		if result.RowsAffected < int64(batchSize) {
			return total, nil
		}
	}
}

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

func escapeLikeValue(value string) string { return likeEscaper.Replace(value) }
