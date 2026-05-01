package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/helixml/helix-org/domain"
	"github.com/helixml/helix-org/store"
)

type workerRow struct {
	ID              string `gorm:"primaryKey;type:text"`
	Kind            string `gorm:"not null"` // "human" or "ai"
	Positions       string // JSON array of position ids
	IdentityContent string // markdown body — domain-owned persona/profile, projected by the spawner
	HelixSessionID  string // live Helix chat session ID; runtime cache, never exported via MCP
	HelixProjectID  string // per-Worker Helix project (one project per AI Worker)
	HelixAgentAppID string // project's auto-provisioned Agent App (carries MCP wiring)
	HelixRepoID     string // project's primary git repo (helix-specs branch holds job/*.md)
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (workerRow) TableName() string { return "workers" }

type workersRepo struct {
	db *gorm.DB
}

func (r *workersRepo) Create(ctx context.Context, worker domain.Worker) error {
	row, err := workerToRow(worker)
	if err != nil {
		return err
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("create worker: %w", err)
	}
	return nil
}

func (r *workersRepo) Get(ctx context.Context, id domain.WorkerID) (domain.Worker, error) {
	var row workerRow
	err := r.db.WithContext(ctx).First(&row, "id = ?", string(id)).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("worker %q: %w", id, store.ErrNotFound)
		}
		return nil, fmt.Errorf("get worker %q: %w", id, err)
	}
	return rowToWorker(row)
}

func (r *workersRepo) List(ctx context.Context) ([]domain.Worker, error) {
	var rows []workerRow
	if err := r.db.WithContext(ctx).Order("id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list workers: %w", err)
	}
	out := make([]domain.Worker, 0, len(rows))
	for _, row := range rows {
		w, err := rowToWorker(row)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, nil
}

// Update rewrites the mutable fields of an existing worker row.
// Positions and Kind are not user-editable and are kept aligned with
// the existing record on save — only IdentityContent is intended to
// change today, but we write all mutable fields for forward-compat.
func (r *workersRepo) Update(ctx context.Context, worker domain.Worker) error {
	row, err := workerToRow(worker)
	if err != nil {
		return err
	}
	res := r.db.WithContext(ctx).
		Model(&workerRow{}).
		Where("id = ?", row.ID).
		Updates(map[string]any{
			"identity_content": row.IdentityContent,
			"positions":        row.Positions,
			"kind":             row.Kind,
		})
	if res.Error != nil {
		return fmt.Errorf("update worker %q: %w", row.ID, res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("worker %q: %w", worker.ID(), store.ErrNotFound)
	}
	return nil
}

// SetHelixSessionID sets the live helix chat session pointer.
func (r *workersRepo) SetHelixSessionID(ctx context.Context, id domain.WorkerID, sessionID string) error {
	res := r.db.WithContext(ctx).
		Model(&workerRow{}).
		Where("id = ?", string(id)).
		Update("helix_session_id", sessionID)
	if res.Error != nil {
		return fmt.Errorf("set helix session for %q: %w", id, res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("worker %q: %w", id, store.ErrNotFound)
	}
	return nil
}

// ClearHelixSessionID clears the helix chat session pointer.
func (r *workersRepo) ClearHelixSessionID(ctx context.Context, id domain.WorkerID) error {
	return r.SetHelixSessionID(ctx, id, "")
}

// SetHelixProject persists the per-Worker project IDs at hire time.
// All three are written together — they're a unit, populated by the
// spawner's ApplyProject step.
func (r *workersRepo) SetHelixProject(ctx context.Context, id domain.WorkerID, projectID, agentAppID, repoID string) error {
	res := r.db.WithContext(ctx).
		Model(&workerRow{}).
		Where("id = ?", string(id)).
		Updates(map[string]any{
			"helix_project_id":   projectID,
			"helix_agent_app_id": agentAppID,
			"helix_repo_id":      repoID,
		})
	if res.Error != nil {
		return fmt.Errorf("set helix project for %q: %w", id, res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("worker %q: %w", id, store.ErrNotFound)
	}
	return nil
}

// ClearHelixProject clears the per-Worker project pointer when the
// Worker is fired and the project gets deleted Helix-side.
func (r *workersRepo) ClearHelixProject(ctx context.Context, id domain.WorkerID) error {
	return r.SetHelixProject(ctx, id, "", "", "")
}

func workerToRow(worker domain.Worker) (workerRow, error) {
	positions := worker.Positions()
	encoded, err := json.Marshal(positions)
	if err != nil {
		return workerRow{}, fmt.Errorf("marshal positions: %w", err)
	}
	return workerRow{
		ID:              string(worker.ID()),
		Kind:            string(worker.Kind()),
		Positions:       string(encoded),
		IdentityContent: worker.IdentityContent(),
		HelixSessionID:  worker.HelixSessionID(),
		HelixProjectID:  worker.HelixProjectID(),
		HelixAgentAppID: worker.HelixAgentAppID(),
		HelixRepoID:     worker.HelixRepoID(),
	}, nil
}

func rowToWorker(row workerRow) (domain.Worker, error) {
	var positions []domain.PositionID
	if row.Positions != "" {
		if err := json.Unmarshal([]byte(row.Positions), &positions); err != nil {
			return nil, fmt.Errorf("unmarshal positions: %w", err)
		}
	}
	switch domain.WorkerKind(row.Kind) {
	case domain.WorkerKindHuman:
		w, err := domain.NewHumanWorker(domain.WorkerID(row.ID), positions, row.IdentityContent)
		if err != nil {
			return nil, err
		}
		return w.WithHelixSessionID(row.HelixSessionID).
			WithHelixProject(row.HelixProjectID, row.HelixAgentAppID, row.HelixRepoID), nil
	case domain.WorkerKindAI:
		w, err := domain.NewAIWorker(domain.WorkerID(row.ID), positions, row.IdentityContent)
		if err != nil {
			return nil, err
		}
		return w.WithHelixSessionID(row.HelixSessionID).
			WithHelixProject(row.HelixProjectID, row.HelixAgentAppID, row.HelixRepoID), nil
	default:
		return nil, fmt.Errorf("unknown worker kind %q", row.Kind)
	}
}
