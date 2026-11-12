package store

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
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

// ListRequestAuditEvents returns one keyset page of events, newest-first.
//
// Keyset pagination on (created, id) keeps a page query O(limit) no matter how
// deep the caller pages; OFFSET degrades linearly and can skip or duplicate
// rows while new events are being inserted.
func (s *PostgresStore) ListRequestAuditEvents(ctx context.Context, filters *types.RequestAuditEventFilters) (*types.RequestAuditEventResponse, error) {
	if filters == nil {
		filters = &types.RequestAuditEventFilters{}
	}

	db := s.gdb.WithContext(ctx).Model(&types.RequestAuditEvent{})
	if filters.UserID != "" {
		db = db.Where("user_id = ?", filters.UserID)
	}
	if filters.RemoteIP != "" {
		db = db.Where("remote_ip = ?", filters.RemoteIP)
	}
	if filters.ClientIP != "" {
		db = db.Where("client_ip = ?", filters.ClientIP)
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

	limit := requestAuditDefaultLimit
	if filters.Limit > 0 {
		limit = min(filters.Limit, requestAuditMaxLimit)
	}

	var total *int64
	if filters.IncludeTotal {
		var count int64
		// Clone the statement: Count mutates the SELECT clause, and the page
		// query below must still be a row query.
		if err := db.Session(&gorm.Session{}).Count(&count).Error; err != nil {
			return nil, fmt.Errorf("failed to count request audit events: %w", err)
		}
		total = &count
	}

	if filters.Cursor != "" {
		created, id, err := decodeRequestAuditCursor(filters.Cursor)
		if err != nil {
			return nil, err
		}
		db = db.Where("(created, id) < (?, ?)", created, id)
	}

	// Fetch one extra row to know whether another page exists.
	events := make([]*types.RequestAuditEvent, 0, limit+1)
	if err := db.Order("created DESC, id DESC").Limit(limit + 1).Find(&events).Error; err != nil {
		return nil, fmt.Errorf("failed to list request audit events: %w", err)
	}

	response := &types.RequestAuditEventResponse{Limit: limit, Total: total}
	if len(events) > limit {
		events = events[:limit]
		last := events[len(events)-1]
		response.NextCursor = encodeRequestAuditCursor(last.Created, last.ID)
	}
	response.Events = events
	return response, nil
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

// encodeRequestAuditCursor packs the sort position into an opaque token. It is
// not a secret; it is opaque so callers do not build one by hand.
func encodeRequestAuditCursor(created time.Time, id string) string {
	raw := created.UTC().Format(time.RFC3339Nano) + "\x00" + id
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeRequestAuditCursor(cursor string) (time.Time, string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("invalid cursor: %w", err)
	}
	createdValue, id, found := strings.Cut(string(raw), "\x00")
	if !found || id == "" {
		return time.Time{}, "", fmt.Errorf("invalid cursor: missing position")
	}
	created, err := time.Parse(time.RFC3339Nano, createdValue)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("invalid cursor: %w", err)
	}
	return created.UTC(), id, nil
}

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

func escapeLikeValue(value string) string { return likeEscaper.Replace(value) }
