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
	ID        string `gorm:"primaryKey;type:text"`
	Kind      string `gorm:"not null"` // "human" or "ai"
	Positions string // JSON array of position ids
	CreatedAt time.Time
	UpdatedAt time.Time
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

func workerToRow(worker domain.Worker) (workerRow, error) {
	positions := worker.Positions()
	encoded, err := json.Marshal(positions)
	if err != nil {
		return workerRow{}, fmt.Errorf("marshal positions: %w", err)
	}
	return workerRow{
		ID:        string(worker.ID()),
		Kind:      string(worker.Kind()),
		Positions: string(encoded),
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
		return domain.NewHumanWorker(domain.WorkerID(row.ID), positions)
	case domain.WorkerKindAI:
		return domain.NewAIWorker(domain.WorkerID(row.ID), positions)
	default:
		return nil, fmt.Errorf("unknown worker kind %q", row.Kind)
	}
}
