package gorm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/helixml/helix/api/pkg/org/domain/processor"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

// isUniqueViolation reports whether err is a database unique-constraint
// violation, matched portably by message so we don't import a
// driver-specific error type (Postgres "SQLSTATE 23505" / "duplicate
// key", SQLite "UNIQUE constraint failed").
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "23505") ||
		strings.Contains(s, "duplicate key") ||
		strings.Contains(s, "UNIQUE constraint failed")
}

// processorRow is the GORM row for a Processor. Outputs are stored as a
// JSON column (the slice is small and only ever read/written whole), so
// no join table is needed. Mirrors topicRow's shape and (org_id, name)
// unique index.
type processorRow struct {
	ID           string `gorm:"primaryKey;type:text"`
	OrgID        string `gorm:"primaryKey;type:text;index;uniqueIndex:idx_processor_org_name,priority:1"`
	Name         string `gorm:"not null;uniqueIndex:idx_processor_org_name,priority:2"`
	InputTopicID string `gorm:"not null;index"`
	Kind         string `gorm:"not null"`
	Config       string `gorm:"not null;default:''"`
	Outputs      string `gorm:"not null;default:'[]'"`
	CreatedBy    string `gorm:"index"`
	CreatedAt    time.Time
}

func (processorRow) TableName() string { return "org_processors" }

type processorMapper struct{}

func (processorMapper) ToRow(p processor.Processor) (processorRow, error) {
	outs, err := json.Marshal(p.Outputs)
	if err != nil {
		return processorRow{}, fmt.Errorf("marshal processor outputs: %w", err)
	}
	cfg := ""
	if len(p.Config) > 0 {
		cfg = string(p.Config)
	}
	return processorRow{
		ID:           string(p.ID),
		OrgID:        p.OrganizationID,
		Name:         p.Name,
		InputTopicID: string(p.InputTopicID),
		Kind:         string(p.Kind),
		Config:       cfg,
		Outputs:      string(outs),
		CreatedBy:    p.CreatedBy,
		CreatedAt:    p.CreatedAt,
	}, nil
}

func (processorMapper) ToDomain(row processorRow) (processor.Processor, error) {
	var outs []processor.Output
	if row.Outputs != "" {
		if err := json.Unmarshal([]byte(row.Outputs), &outs); err != nil {
			return processor.Processor{}, fmt.Errorf("unmarshal processor outputs: %w", err)
		}
	}
	var cfg json.RawMessage
	if row.Config != "" {
		cfg = json.RawMessage(row.Config)
	}
	return processor.NewProcessor(
		processor.ProcessorID(row.ID),
		row.Name,
		streaming.TopicID(row.InputTopicID),
		processor.Kind(row.Kind),
		cfg,
		outs,
		row.CreatedBy,
		row.CreatedAt,
		row.OrgID,
	)
}

type processorsRepo struct {
	*Repository[processor.Processor, processorRow]
}

func newProcessorsRepo(db *gorm.DB) *processorsRepo {
	return &processorsRepo{Repository: NewRepository[processor.Processor, processorRow](db, processorMapper{}, "processor")}
}

// Create translates a unique-name violation into a clean store.ErrConflict
// so callers get a friendly 409 instead of the raw driver error.
func (r *processorsRepo) Create(ctx context.Context, p processor.Processor) error {
	if err := r.Repository.Create(ctx, p); err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("a processor named %q in this org %w", p.Name, store.ErrConflict)
		}
		return err
	}
	return nil
}

func (r *processorsRepo) Get(ctx context.Context, orgID string, id processor.ProcessorID) (processor.Processor, error) {
	return r.FindOne(ctx, store.WithOrg(orgID), store.WithID(string(id)))
}

func (r *processorsRepo) List(ctx context.Context, orgID string) ([]processor.Processor, error) {
	return r.Find(ctx, store.WithOrg(orgID), store.WithOrderAsc("id"))
}

// ListByInputTopic returns every processor in the org reading the given
// input topic — the dispatcher's fan-out lookup on every publish.
func (r *processorsRepo) ListByInputTopic(ctx context.Context, orgID string, in streaming.TopicID) ([]processor.Processor, error) {
	return r.Find(ctx,
		store.WithOrg(orgID),
		store.WithCondition("input_topic_id", string(in)),
		store.WithOrderAsc("id"),
	)
}

// Update rewrites the mutable subset (name, input topic, kind, config,
// outputs) of the row identified by (id, orgID). Immutable fields on
// the passed Processor are ignored. Returns store.ErrNotFound when no
// row matches.
func (r *processorsRepo) Update(ctx context.Context, p processor.Processor) error {
	outs, err := json.Marshal(p.Outputs)
	if err != nil {
		return fmt.Errorf("marshal processor outputs: %w", err)
	}
	cfg := ""
	if len(p.Config) > 0 {
		cfg = string(p.Config)
	}
	updates := map[string]any{
		"name":           p.Name,
		"input_topic_id": string(p.InputTopicID),
		"kind":           string(p.Kind),
		"config":         cfg,
		"outputs":        string(outs),
	}
	if err := r.Repository.Update(ctx,
		store.WithOrg(p.OrganizationID),
		store.WithID(string(p.ID)),
		store.WithUpdates(updates),
	); err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("a processor named %q in this org %w", p.Name, store.ErrConflict)
		}
		return err
	}
	return nil
}

// Delete removes the processor row. The auto-created output topics are
// cascaded by the processors application service, not here (mirrors how
// topicsRepo.Delete leaves higher-level cleanup to its caller).
func (r *processorsRepo) Delete(ctx context.Context, orgID string, id processor.ProcessorID) error {
	res := r.db.WithContext(ctx).Where("org_id = ? AND id = ?", orgID, string(id)).Delete(&processorRow{})
	if res.Error != nil {
		return fmt.Errorf("delete processor: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("processor: %w", store.ErrNotFound)
	}
	return nil
}
