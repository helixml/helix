package gorm

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/workersecret"
)

type workerSecretBindingRow struct {
	OrganizationID    string `gorm:"primaryKey;column:org_id"`
	WorkerID          string `gorm:"primaryKey;column:worker_id"`
	Name              string `gorm:"primaryKey"`
	Description       string
	Usage             string
	ContentType       string
	SuggestedFilename string
	SourceKind        string `gorm:"not null"`
	SecretID          string
	AccountID         string
	ExportKey         string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (workerSecretBindingRow) TableName() string { return "org_worker_secret_bindings" }

type workerSecretBindingsRepo struct{ db *gorm.DB }

func (r *workerSecretBindingsRepo) Create(ctx context.Context, b workersecret.Binding) error {
	if err := b.Validate(); err != nil {
		return err
	}
	if err := r.db.WithContext(ctx).Create(bindingRow(b)).Error; err != nil {
		return fmt.Errorf("create worker secret binding: %w", err)
	}
	return nil
}
func (r *workerSecretBindingsRepo) Update(ctx context.Context, b workersecret.Binding) error {
	if err := b.Validate(); err != nil {
		return err
	}
	row := bindingRow(b)
	res := r.db.WithContext(ctx).
		Model(&workerSecretBindingRow{}).
		Where("org_id = ? AND worker_id = ? AND name = ?", b.OrganizationID, b.WorkerID, b.Name).
		Select("description", "usage", "content_type", "suggested_filename", "source_kind", "secret_id", "account_id", "export_key", "updated_at").
		Updates(row)
	if res.Error != nil {
		return fmt.Errorf("update worker secret binding: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("worker secret binding: %w", store.ErrNotFound)
	}
	return nil
}
func (r *workerSecretBindingsRepo) Get(ctx context.Context, orgID string, workerID orgchart.NodeID, name string) (workersecret.Binding, error) {
	var row workerSecretBindingRow
	err := r.db.WithContext(ctx).Where("org_id = ? AND worker_id = ? AND name = ?", orgID, workerID, name).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return workersecret.Binding{}, fmt.Errorf("worker secret binding: %w", store.ErrNotFound)
	}
	if err != nil {
		return workersecret.Binding{}, fmt.Errorf("get worker secret binding: %w", err)
	}
	return bindingDomain(row), nil
}
func (r *workerSecretBindingsRepo) List(ctx context.Context, orgID string, workerID orgchart.NodeID) ([]workersecret.Binding, error) {
	var rows []workerSecretBindingRow
	if err := r.db.WithContext(ctx).Where("org_id = ? AND worker_id = ?", orgID, workerID).Order("name").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list worker secret bindings: %w", err)
	}
	out := make([]workersecret.Binding, len(rows))
	for i := range rows {
		out[i] = bindingDomain(rows[i])
	}
	return out, nil
}
func (r *workerSecretBindingsRepo) Delete(ctx context.Context, orgID string, workerID orgchart.NodeID, name string) error {
	res := r.db.WithContext(ctx).Where("org_id = ? AND worker_id = ? AND name = ?", orgID, workerID, name).Delete(&workerSecretBindingRow{})
	if res.Error != nil {
		return fmt.Errorf("delete worker secret binding: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("worker secret binding: %w", store.ErrNotFound)
	}
	return nil
}
func bindingRow(b workersecret.Binding) workerSecretBindingRow {
	return workerSecretBindingRow{
		OrganizationID: b.OrganizationID, WorkerID: string(b.WorkerID), Name: b.Name,
		Description: b.Description, Usage: b.Usage, ContentType: b.ContentType,
		SuggestedFilename: b.SuggestedFilename, SourceKind: string(b.SourceKind),
		SecretID: b.SecretID, AccountID: b.AccountID, ExportKey: b.ExportKey,
		CreatedAt: b.CreatedAt, UpdatedAt: b.UpdatedAt,
	}
}
func bindingDomain(r workerSecretBindingRow) workersecret.Binding {
	return workersecret.Binding{
		OrganizationID: r.OrganizationID, WorkerID: orgchart.NodeID(r.WorkerID), Name: r.Name,
		Description: r.Description, Usage: r.Usage, ContentType: r.ContentType,
		SuggestedFilename: r.SuggestedFilename, SourceKind: workersecret.SourceKind(r.SourceKind),
		SecretID: r.SecretID, AccountID: r.AccountID, ExportKey: r.ExportKey,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}
