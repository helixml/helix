package gorm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/helixml/helix/api/pkg/org/domain/eventsource"
	"github.com/helixml/helix/api/pkg/org/domain/processor"
	"github.com/helixml/helix/api/pkg/org/domain/store"
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
// no join table is needed.
//
// The input is a terminal eventsource.SourceRef spread over three
// nullable columns — exactly the shape attachmentRow uses, so the same
// (trigger XOR processor+output) invariant reads the same way in both
// tables.
//
// InputTopicID is the retired pre-cutover column. Nothing at runtime
// reads or writes it; it survives only as the input the repeat-safe
// conversion in application/cutover reads, and is dropped once deployed
// data has converted.
type processorRow struct {
	ID               string  `gorm:"primaryKey;type:text"`
	OrgID            string  `gorm:"primaryKey;type:text;index;uniqueIndex:idx_processor_org_name,priority:1"`
	Name             string  `gorm:"not null;uniqueIndex:idx_processor_org_name,priority:2"`
	InputTopicID     string  `gorm:"not null;default:'';index"`
	InputTriggerID   *string `gorm:"index"`
	InputProcessorID *string `gorm:"index"`
	InputOutputID    *string `gorm:"index"`
	Kind             string  `gorm:"not null"`
	Config           string  `gorm:"not null;default:''"`
	Outputs          string  `gorm:"not null;default:'[]'"`
	CreatedBy        string  `gorm:"index"`
	CreatedAt        time.Time
}

// inputColumns spreads a terminal source reference across the row's three
// nullable input columns. A zero reference clears all three (an unwired
// Processor).
func inputColumns(src eventsource.SourceRef) (triggerID, processorID, outputID *string) {
	switch src.Kind {
	case eventsource.KindTrigger:
		id := src.TriggerID
		return &id, nil, nil
	case eventsource.KindProcessorOutput:
		p, o := src.ProcessorID, src.OutputID
		return nil, &p, &o
	default:
		return nil, nil, nil
	}
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
	triggerID, procID, outputID := inputColumns(p.InputSource)
	return processorRow{
		ID:               string(p.ID),
		OrgID:            p.OrganizationID,
		Name:             p.Name,
		InputTriggerID:   triggerID,
		InputProcessorID: procID,
		InputOutputID:    outputID,
		Kind:             string(p.Kind),
		Config:           cfg,
		Outputs:          string(outs),
		CreatedBy:        p.CreatedBy,
		CreatedAt:        p.CreatedAt,
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
	var input eventsource.SourceRef
	switch {
	case row.InputTriggerID != nil:
		input = eventsource.Trigger(*row.InputTriggerID)
	case row.InputProcessorID != nil && row.InputOutputID != nil:
		input = eventsource.ProcessorOutput(*row.InputProcessorID, *row.InputOutputID)
	}
	return processor.NewProcessor(
		processor.ProcessorID(row.ID),
		row.Name,
		input,
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

// ListByInputSource returns every processor in the org reading the given
// source — the runner's fan-out lookup on every published event.
func (r *processorsRepo) ListByInputSource(ctx context.Context, orgID string, in eventsource.SourceRef) ([]processor.Processor, error) {
	opts := []store.Option{store.WithOrg(orgID)}
	switch in.Kind {
	case eventsource.KindTrigger:
		opts = append(opts, store.WithCondition("input_trigger_id", in.TriggerID))
	case eventsource.KindProcessorOutput:
		opts = append(opts, store.WithCondition("input_processor_id", in.ProcessorID), store.WithCondition("input_output_id", in.OutputID))
	default:
		return nil, nil
	}
	return r.Find(ctx, append(opts, store.WithOrderAsc("id"))...)
}

// Update rewrites the mutable subset (name, input source, kind, config,
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
	triggerID, procID, outputID := inputColumns(p.InputSource)
	updates := map[string]any{
		"name":               p.Name,
		"input_trigger_id":   triggerID,
		"input_processor_id": procID,
		"input_output_id":    outputID,
		"kind":               string(p.Kind),
		"config":             cfg,
		"outputs":            string(outs),
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

// Delete removes the processor row. Attachments to its outputs are
// cascaded by the processors application service, not here.
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
