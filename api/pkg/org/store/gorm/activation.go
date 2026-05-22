package gorm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/helixml/helix/api/pkg/org/activation"
	"github.com/helixml/helix/api/pkg/org/stream"
	"github.com/helixml/helix/api/pkg/org/worker"
	"github.com/helixml/helix/api/pkg/org/store"
)

// activationRow is the GORM model for the activations table. Triggers
// are stored as a JSON-encoded blob — the trigger payload is opaque to
// the storage layer (it carries a message envelope with transport-
// specific extras), and JSON keeps the AutoMigrate schema flat. The
// (worker_id, started_at) index powers ListForWorker's newest-first
// ordering without a table scan.
type activationRow struct {
	ID                 string    `gorm:"primaryKey;type:text"`
	WorkerID           string    `gorm:"not null;index:idx_activations_worker_started"`
	StartedAt          time.Time `gorm:"not null;index:idx_activations_worker_started,sort:desc"`
	EndedAt            *time.Time
	OutcomeStatus      string `gorm:"type:text"`
	OutcomeError       string `gorm:"type:text"`
	TranscriptStreamID string `gorm:"not null;type:text"`
	TriggersJSON       string `gorm:"not null;type:text"`
}

func (activationRow) TableName() string { return "activations" }

type activationsRepo struct {
	db *gorm.DB
}

func (r *activationsRepo) Create(ctx context.Context, a *activation.Activation) error {
	row, err := activationToRow(a)
	if err != nil {
		return err
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("create activation %q: %w", a.ID, err)
	}
	return nil
}

func (r *activationsRepo) Complete(ctx context.Context, id activation.ID, outcome activation.Outcome, endedAt time.Time) error {
	res := r.db.WithContext(ctx).Model(&activationRow{}).
		Where("id = ?", string(id)).
		Updates(map[string]any{
			"ended_at":       endedAt,
			"outcome_status": string(outcome.Status),
			"outcome_error":  outcome.Error,
		})
	if res.Error != nil {
		return fmt.Errorf("complete activation %q: %w", id, res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("complete activation %q: %w", id, store.ErrNotFound)
	}
	return nil
}

func (r *activationsRepo) Get(ctx context.Context, id activation.ID) (*activation.Activation, error) {
	var row activationRow
	err := r.db.WithContext(ctx).First(&row, "id = ?", string(id)).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("activation %q: %w", id, store.ErrNotFound)
		}
		return nil, fmt.Errorf("get activation %q: %w", id, err)
	}
	return rowToActivation(row)
}

func (r *activationsRepo) ListForWorker(ctx context.Context, workerID worker.ID, limit int) ([]*activation.Activation, error) {
	query := r.db.WithContext(ctx).
		Where("worker_id = ?", string(workerID)).
		Order("started_at DESC, id DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	var rows []activationRow
	if err := query.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list activations for worker %q: %w", workerID, err)
	}
	out := make([]*activation.Activation, 0, len(rows))
	for _, row := range rows {
		a, err := rowToActivation(row)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

func activationToRow(a *activation.Activation) (activationRow, error) {
	triggersJSON, err := json.Marshal(a.Triggers)
	if err != nil {
		return activationRow{}, fmt.Errorf("encode triggers for activation %q: %w", a.ID, err)
	}
	return activationRow{
		ID:                 string(a.ID),
		WorkerID:           string(a.WorkerID),
		StartedAt:          a.StartedAt,
		EndedAt:            a.EndedAt,
		OutcomeStatus:      string(a.Outcome.Status),
		OutcomeError:       a.Outcome.Error,
		TranscriptStreamID: string(a.TranscriptStreamID),
		TriggersJSON:       string(triggersJSON),
	}, nil
}

func rowToActivation(row activationRow) (*activation.Activation, error) {
	var triggers []activation.Trigger
	if err := json.Unmarshal([]byte(row.TriggersJSON), &triggers); err != nil {
		return nil, fmt.Errorf("decode triggers for activation %q: %w", row.ID, err)
	}
	a := &activation.Activation{
		ID:                 activation.ID(row.ID),
		WorkerID:           worker.ID(row.WorkerID),
		Triggers:           triggers,
		StartedAt:          row.StartedAt,
		EndedAt:            row.EndedAt,
		TranscriptStreamID: stream.ID(row.TranscriptStreamID),
	}
	if row.OutcomeStatus != "" {
		a.Outcome = activation.Outcome{
			Status: activation.Status(row.OutcomeStatus),
			Error:  row.OutcomeError,
		}
	}
	return a, nil
}
