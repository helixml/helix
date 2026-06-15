package gorm

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

type activationRow struct {
	ID                 string    `gorm:"primaryKey;type:text"`
	OrgID              string    `gorm:"primaryKey;type:text;index"`
	WorkerID           string    `gorm:"not null;index:idx_org_activations_worker_started"`
	StartedAt          time.Time `gorm:"not null;index:idx_org_activations_worker_started,sort:desc"`
	EndedAt            *time.Time
	OutcomeStatus      string `gorm:"type:text"`
	OutcomeError       string `gorm:"type:text"`
	TranscriptStreamID string `gorm:"not null;type:text"`
	TriggersJSON       string `gorm:"not null;type:text"`
}

func (activationRow) TableName() string { return "org_activations" }

type activationMapper struct{}

func (activationMapper) ToRow(a *activation.Activation) (activationRow, error) {
	triggersJSON, err := json.Marshal(a.Triggers)
	if err != nil {
		return activationRow{}, fmt.Errorf("encode triggers for activation %q: %w", a.ID, err)
	}
	return activationRow{
		ID:                 string(a.ID),
		OrgID:              a.OrganizationID,
		WorkerID:           string(a.WorkerID),
		StartedAt:          a.StartedAt,
		EndedAt:            a.EndedAt,
		OutcomeStatus:      string(a.Outcome.Status),
		OutcomeError:       a.Outcome.Error,
		TranscriptStreamID: string(a.TranscriptStreamID),
		TriggersJSON:       string(triggersJSON),
	}, nil
}

func (activationMapper) ToDomain(row activationRow) (*activation.Activation, error) {
	var triggers []activation.Trigger
	if err := json.Unmarshal([]byte(row.TriggersJSON), &triggers); err != nil {
		return nil, fmt.Errorf("decode triggers for activation %q: %w", row.ID, err)
	}
	a := &activation.Activation{
		ID:                 activation.ID(row.ID),
		OrganizationID:     row.OrgID,
		WorkerID:           orgchart.WorkerID(row.WorkerID),
		Triggers:           triggers,
		StartedAt:          row.StartedAt,
		EndedAt:            row.EndedAt,
		TranscriptStreamID: streaming.StreamID(row.TranscriptStreamID),
	}
	if row.OutcomeStatus != "" {
		a.Outcome = activation.Outcome{
			Status: activation.Status(row.OutcomeStatus),
			Error:  row.OutcomeError,
		}
	}
	return a, nil
}

type activationsRepo struct {
	*Repository[*activation.Activation, activationRow]
}

func newActivationsRepo(db *gorm.DB) *activationsRepo {
	return &activationsRepo{Repository: NewRepository[*activation.Activation, activationRow](db, activationMapper{}, "activation")}
}

func (r *activationsRepo) Create(ctx context.Context, a *activation.Activation) error {
	return r.Repository.Create(ctx, a)
}

func (r *activationsRepo) Complete(ctx context.Context, orgID string, id activation.ID, outcome activation.Outcome, endedAt time.Time) error {
	return r.Repository.Update(ctx,
		store.WithOrg(orgID),
		store.WithID(string(id)),
		store.WithUpdates(map[string]any{
			"ended_at":       endedAt,
			"outcome_status": string(outcome.Status),
			"outcome_error":  outcome.Error,
		}),
	)
}

func (r *activationsRepo) Get(ctx context.Context, orgID string, id activation.ID) (*activation.Activation, error) {
	return r.FindOne(ctx, store.WithOrg(orgID), store.WithID(string(id)))
}

func (r *activationsRepo) ListForWorker(ctx context.Context, orgID string, workerID orgchart.WorkerID, limit int) ([]*activation.Activation, error) {
	return r.Repository.Find(ctx,
		store.WithOrg(orgID),
		store.WithCondition("worker_id", string(workerID)),
		store.WithOrderDesc("started_at"),
		store.WithOrderDesc("id"),
		store.WithLimit(limit),
	)
}
