package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
)

func (s *PostgresStore) CreateSpecTaskPRProposal(ctx context.Context, p *types.SpecTaskPRProposal) error {
	if p.ID == "" || p.SpecTaskID == "" || p.ProjectID == "" || p.RepositoryID == "" || p.HeadBranch == "" {
		return fmt.Errorf("id, spec_task_id, project_id, repository_id and head_branch are required")
	}
	if p.Status == "" {
		p.Status = types.PRProposalStatusPending
	}
	now := time.Now()
	p.CreatedAt = now
	p.UpdatedAt = now
	if err := s.gdb.WithContext(ctx).Create(p).Error; err != nil {
		return fmt.Errorf("create pr proposal: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetSpecTaskPRProposal(ctx context.Context, id string) (*types.SpecTaskPRProposal, error) {
	p := &types.SpecTaskPRProposal{}
	if err := s.gdb.WithContext(ctx).Where("id = ?", id).First(p).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get pr proposal: %w", err)
	}
	return p, nil
}

func (s *PostgresStore) ListSpecTaskPRProposals(ctx context.Context, f *types.SpecTaskPRProposalFilter) ([]*types.SpecTaskPRProposal, error) {
	q := s.gdb.WithContext(ctx).Model(&types.SpecTaskPRProposal{})
	if f != nil {
		if f.SpecTaskID != "" {
			q = q.Where("spec_task_id = ?", f.SpecTaskID)
		}
		if f.ProjectID != "" {
			q = q.Where("project_id = ?", f.ProjectID)
		}
		if f.RepositoryID != "" {
			q = q.Where("repository_id = ?", f.RepositoryID)
		}
		if f.HeadBranch != "" {
			q = q.Where("head_branch = ?", f.HeadBranch)
		}
		if len(f.Statuses) > 0 {
			q = q.Where("status IN ?", f.Statuses)
		}
	}
	var out []*types.SpecTaskPRProposal
	if err := q.Order("created_at ASC").Find(&out).Error; err != nil {
		return nil, fmt.Errorf("list pr proposals: %w", err)
	}
	return out, nil
}

// UpdateSpecTaskPRProposal writes every field of p, but only while the stored
// row is still in one of fromStatuses. It returns false when another writer
// moved the proposal first, so two concurrent decisions cannot both apply.
func (s *PostgresStore) UpdateSpecTaskPRProposal(ctx context.Context, p *types.SpecTaskPRProposal, fromStatuses ...types.SpecTaskPRProposalStatus) (bool, error) {
	if p.ID == "" {
		return false, fmt.Errorf("proposal ID is required")
	}
	p.UpdatedAt = time.Now()
	q := s.gdb.WithContext(ctx).Model(&types.SpecTaskPRProposal{}).Where("id = ?", p.ID)
	if len(fromStatuses) > 0 {
		q = q.Where("status IN ?", fromStatuses)
	}
	res := q.Select("*").Omit("id", "created_at").Updates(p)
	if res.Error != nil {
		return false, fmt.Errorf("update pr proposal: %w", res.Error)
	}
	return res.RowsAffected > 0, nil
}
