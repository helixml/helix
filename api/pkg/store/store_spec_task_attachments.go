package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
)

func (s *PostgresStore) CreateSpecTaskAttachment(ctx context.Context, a *types.SpecTaskAttachment) error {
	if a.ID == "" {
		return fmt.Errorf("attachment ID is required")
	}
	if a.SpecTaskID == "" {
		return fmt.Errorf("spec_task_id is required")
	}
	if a.ProjectID == "" {
		return fmt.Errorf("project_id is required")
	}
	if a.Filename == "" {
		return fmt.Errorf("filename is required")
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now()
	}
	if err := s.gdb.WithContext(ctx).Create(a).Error; err != nil {
		return fmt.Errorf("create spec task attachment: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetSpecTaskAttachment(ctx context.Context, id string) (*types.SpecTaskAttachment, error) {
	if id == "" {
		return nil, fmt.Errorf("attachment ID is required")
	}
	a := &types.SpecTaskAttachment{}
	err := s.gdb.WithContext(ctx).Where("id = ?", id).First(a).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("attachment not found: %s", id)
		}
		return nil, fmt.Errorf("get spec task attachment: %w", err)
	}
	return a, nil
}

func (s *PostgresStore) UpdateSpecTaskAttachment(ctx context.Context, a *types.SpecTaskAttachment) error {
	if a.ID == "" {
		return fmt.Errorf("attachment ID is required")
	}
	if err := s.gdb.WithContext(ctx).Save(a).Error; err != nil {
		return fmt.Errorf("update spec task attachment: %w", err)
	}
	return nil
}

func (s *PostgresStore) DeleteSpecTaskAttachment(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("attachment ID is required")
	}
	if err := s.gdb.WithContext(ctx).Delete(&types.SpecTaskAttachment{}, "id = ?", id).Error; err != nil {
		return fmt.Errorf("delete spec task attachment: %w", err)
	}
	return nil
}

func (s *PostgresStore) ListSpecTaskAttachments(ctx context.Context, specTaskID string) ([]*types.SpecTaskAttachment, error) {
	if specTaskID == "" {
		return nil, fmt.Errorf("spec_task_id is required")
	}
	var out []*types.SpecTaskAttachment
	err := s.gdb.WithContext(ctx).
		Where("spec_task_id = ?", specTaskID).
		Order("created_at ASC").
		Find(&out).Error
	if err != nil {
		return nil, fmt.Errorf("list spec task attachments: %w", err)
	}
	return out, nil
}

func (s *PostgresStore) DeleteSpecTaskAttachmentsByTaskID(ctx context.Context, specTaskID string) error {
	if specTaskID == "" {
		return fmt.Errorf("spec_task_id is required")
	}
	if err := s.gdb.WithContext(ctx).
		Where("spec_task_id = ?", specTaskID).
		Delete(&types.SpecTaskAttachment{}).Error; err != nil {
		return fmt.Errorf("delete spec task attachments by task: %w", err)
	}
	return nil
}
