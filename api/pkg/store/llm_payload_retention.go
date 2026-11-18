package store

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type llmCallPayload struct {
	ID              string         `gorm:"primaryKey"`
	Created         time.Time      `gorm:"primaryKey;type:timestamptz"`
	OriginalRequest datatypes.JSON `gorm:"type:jsonb"`
	Request         datatypes.JSON `gorm:"type:jsonb"`
	Response        datatypes.JSON `gorm:"type:jsonb"`
}

func (llmCallPayload) TableName() string { return "llm_call_payloads" }

const legacyPayload = "(original_request IS NOT NULL OR request IS NOT NULL OR response IS NOT NULL)"

func ensurePayloadDay(tx *gorm.DB, t time.Time) error {
	day := t.UTC().Truncate(24 * time.Hour)
	name := "llm_call_payloads_" + day.Format("20060102")
	var exists bool
	if err := tx.Raw("SELECT to_regclass(?) IS NOT NULL", name).Scan(&exists).Error; err != nil || exists {
		return err
	}
	if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtext(current_schema()),hashtext('llm_call_payloads'))").Error; err != nil {
		return err
	}
	return tx.Exec(fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s PARTITION OF llm_call_payloads FOR VALUES FROM ('%s') TO ('%s')", name, day.Format(time.RFC3339), day.AddDate(0, 0, 1).Format(time.RFC3339))).Error
}

func (s *PostgresStore) MaintainLLMPayloads(ctx context.Context) {
	for ctx.Err() == nil {
		count, err := s.maintainPayloadBatch(ctx, time.Now())
		delay := time.Hour
		if err != nil {
			log.Error().Err(err).Msg("LLM payload maintenance failed")
			delay = time.Minute
		} else if count > 0 {
			delay = time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
	}
}

func (s *PostgresStore) maintainPayloadBatch(ctx context.Context, now time.Time) (int64, error) {
	var count int64
	err := s.gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SET LOCAL lock_timeout = '2s'").Error; err != nil {
			return err
		}
		settingsStore := *s
		settingsStore.gdb = tx.Clauses(clause.Locking{Strength: "SHARE"})
		settings, err := settingsStore.GetSystemSettings(ctx)
		if err != nil {
			return err
		}
		cutoff := now.UTC().Truncate(24*time.Hour).AddDate(0, 0, 1-settings.LLMPayloadRetentionDays)
		var days []string
		if err := tx.Raw("SELECT c.relname FROM pg_inherits i JOIN pg_class c ON c.oid=i.inhrelid WHERE i.inhparent='llm_call_payloads'::regclass AND c.relname ~ '^llm_call_payloads_[0-9]{8}$' AND c.relname::text < ?", "llm_call_payloads_"+cutoff.Format("20060102")).Scan(&days).Error; err != nil {
			return err
		}
		for _, name := range days {
			if err := tx.Exec("DROP TABLE IF EXISTS " + name).Error; err != nil {
				return err
			}
		}
		var rows []struct {
			ID      string
			Created time.Time
		}
		if err := tx.Raw("SELECT id,created FROM llm_calls WHERE " + legacyPayload + " ORDER BY created,id LIMIT 64 FOR UPDATE SKIP LOCKED").Scan(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		ids := make([]string, 0, len(rows))
		var previous string
		for _, r := range rows {
			ids = append(ids, r.ID)
			if r.Created.Before(cutoff) {
				continue
			}
			day := r.Created.UTC().Format(time.DateOnly)
			if day != previous {
				if err := ensurePayloadDay(tx, r.Created); err != nil {
					return err
				}
				previous = day
			}
		}
		if err := tx.Exec("INSERT INTO llm_call_payloads (id,created,original_request,request,response) SELECT id,created,original_request,request,response FROM llm_calls WHERE id IN ? AND created >= ?", ids, cutoff).Error; err != nil {
			return err
		}
		result := tx.Exec("UPDATE llm_calls SET original_request=NULL,request=NULL,response=NULL WHERE id IN ?", ids)
		count = result.RowsAffected
		return result.Error
	})
	return count, err
}
