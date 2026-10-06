package gorm

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
)

// This file is the remnant of the pre-cutover Topic model. Nothing at
// runtime reads or writes these tables: their only user is
// application/cutover, which converts each row into a Trigger, a
// Processor input, or a Worker attachment exactly once and then deletes
// it. The delete is the point — a retained row would recreate its
// Trigger on every subsequent boot, undoing whatever the user deleted
// in between. AutoMigrate no longer manages the row types, so the
// tables are otherwise left exactly as the last pre-cutover release
// wrote them, and they are dropped once deployed data has converted.
//
// Every statement here is deliberately cross-tenant: the conversion
// runs once at boot for the whole deployment, before any org is scoped.

type retiredTopicRow struct {
	ID              string
	OrgID           string
	Name            string
	Description     string
	CreatedBy       string
	CreatedAt       time.Time
	TransportKind   string
	TransportConfig string
}

func (retiredTopicRow) TableName() string { return "org_topics" }

type retiredSubscriptionRow struct {
	OrgID     string
	NodeID    string `gorm:"column:bot_id"`
	TopicID   string
	CreatedAt time.Time
}

func (retiredSubscriptionRow) TableName() string { return "org_subscriptions" }

type retiredReader struct{ db *gorm.DB }

func newRetiredReader(db *gorm.DB) *retiredReader { return &retiredReader{db: db} }

// ListAll returns every pre-cutover Topic across every org. An absent
// table means the deployment never ran a pre-cutover release (or the
// table has already been dropped) — nothing to convert.
func (r *retiredReader) ListAll(ctx context.Context) ([]streaming.Topic, error) {
	if !r.db.Migrator().HasTable("org_topics") {
		return nil, nil
	}
	var rows []retiredTopicRow
	if err := r.db.WithContext(ctx).Order("org_id, created_at, id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list retired topics: %w", err)
	}
	out := make([]streaming.Topic, 0, len(rows))
	for _, row := range rows {
		tp := transport.Transport{Kind: transport.Kind(row.TransportKind)}
		if row.TransportConfig != "" {
			tp.Config = json.RawMessage(row.TransportConfig)
		}
		topic, err := streaming.NewTopic(row.ID, row.Name, row.Description, orgchart.NodeID(row.CreatedBy), row.CreatedAt, tp, row.OrgID)
		if err != nil {
			return nil, fmt.Errorf("retired topic %q in org %q: %w", row.ID, row.OrgID, err)
		}
		out = append(out, topic)
	}
	return out, nil
}

// Delete removes one converted Topic row. The conversion consumes every
// row it reads, so a Trigger the user later deletes is not recreated by
// the next boot.
func (r *retiredReader) Delete(ctx context.Context, orgID, topicID string) error {
	if !r.db.Migrator().HasTable("org_topics") {
		return nil
	}
	if err := r.db.WithContext(ctx).
		Where("org_id = ? AND id = ?", orgID, topicID).
		Delete(&retiredTopicRow{}).Error; err != nil {
		return fmt.Errorf("delete retired topic %q: %w", topicID, err)
	}
	return nil
}

type retiredSubscriptionReader struct{ db *gorm.DB }

func newRetiredSubscriptionReader(db *gorm.DB) *retiredSubscriptionReader {
	return &retiredSubscriptionReader{db: db}
}

// ListAll returns every pre-cutover subscription across every org.
func (r *retiredSubscriptionReader) ListAll(ctx context.Context) ([]streaming.Subscription, error) {
	if !r.db.Migrator().HasTable("org_subscriptions") {
		return nil, nil
	}
	var rows []retiredSubscriptionRow
	if err := r.db.WithContext(ctx).Order("org_id, bot_id, topic_id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list retired subscriptions: %w", err)
	}
	out := make([]streaming.Subscription, 0, len(rows))
	for _, row := range rows {
		sub, err := streaming.NewSubscription(row.NodeID, row.TopicID, row.CreatedAt, row.OrgID)
		if err != nil {
			return nil, fmt.Errorf("retired subscription %q→%q: %w", row.NodeID, row.TopicID, err)
		}
		out = append(out, sub)
	}
	return out, nil
}

// Delete removes one converted subscription row, so a Worker the user
// later detaches is not re-attached by the next boot.
func (r *retiredSubscriptionReader) Delete(ctx context.Context, orgID, workerID, topicID string) error {
	if !r.db.Migrator().HasTable("org_subscriptions") {
		return nil
	}
	if err := r.db.WithContext(ctx).
		Where("org_id = ? AND bot_id = ? AND topic_id = ?", orgID, workerID, topicID).
		Delete(&retiredSubscriptionRow{}).Error; err != nil {
		return fmt.Errorf("delete retired subscription %q→%q: %w", workerID, topicID, err)
	}
	return nil
}

type retiredProcessorInputReader struct{ db *gorm.DB }

func newRetiredProcessorInputReader(db *gorm.DB) *retiredProcessorInputReader {
	return &retiredProcessorInputReader{db: db}
}

// ListAll returns the pre-cutover `input_topic_id` of every Processor
// that still has one, keyed by (org, processor). A Processor whose input
// has already been converted has a NULL/empty legacy column and is
// omitted, which is what makes the conversion repeat-safe.
func (r *retiredProcessorInputReader) ListAll(ctx context.Context) (map[store.OrgScopedID]streaming.StreamID, error) {
	if !r.db.Migrator().HasColumn(&processorRow{}, "input_topic_id") {
		return nil, nil
	}
	var rows []struct {
		OrgID        string
		ID           string
		InputTopicID string
	}
	if err := r.db.WithContext(ctx).Model(&processorRow{}).
		Select("org_id, id, input_topic_id").
		Where("input_topic_id IS NOT NULL AND input_topic_id <> ''").
		Order("org_id, id").
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("list retired processor inputs: %w", err)
	}
	out := make(map[store.OrgScopedID]streaming.StreamID, len(rows))
	for _, row := range rows {
		out[store.OrgScopedID{OrgID: row.OrgID, ID: row.ID}] = row.InputTopicID
	}
	return out, nil
}

// ClearProcessorInput blanks the retired column once a Processor's input
// has been converted, so a repeat run skips it.
func (r *retiredProcessorInputReader) Clear(ctx context.Context, orgID, processorID string) error {
	if !r.db.Migrator().HasColumn(&processorRow{}, "input_topic_id") {
		return nil
	}
	return r.db.WithContext(ctx).Model(&processorRow{}).
		Where("org_id = ? AND id = ?", orgID, processorID).
		Update("input_topic_id", "").Error
}
