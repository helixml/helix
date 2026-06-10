package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// CreateSpecTaskProposal inserts a new proposal record. Caller is responsible for
// generating the ID and setting Kind / SpecTaskID / ProjectID.
func (s *PostgresStore) CreateSpecTaskProposal(ctx context.Context, p *types.SpecTaskProposal) error {
	if p.ID == "" {
		return fmt.Errorf("proposal ID is required")
	}
	if p.SpecTaskID == "" {
		return fmt.Errorf("spec_task_id is required")
	}
	if p.ProjectID == "" {
		return fmt.Errorf("project_id is required")
	}
	if p.Kind == "" {
		return fmt.Errorf("kind is required")
	}
	if p.Status == "" {
		p.Status = types.ProposalStatusPending
	}
	now := time.Now()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	p.UpdatedAt = now

	if err := s.gdb.WithContext(ctx).Create(p).Error; err != nil {
		return fmt.Errorf("failed to create spec task proposal: %w", err)
	}
	log.Info().
		Str("proposal_id", p.ID).
		Str("spec_task_id", p.SpecTaskID).
		Str("kind", string(p.Kind)).
		Msg("Created spec task proposal")
	return nil
}

// GetSpecTaskProposal fetches a proposal by ID.
func (s *PostgresStore) GetSpecTaskProposal(ctx context.Context, id string) (*types.SpecTaskProposal, error) {
	if id == "" {
		return nil, fmt.Errorf("proposal ID is required")
	}
	p := &types.SpecTaskProposal{}
	err := s.gdb.WithContext(ctx).Where("id = ?", id).First(p).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("spec task proposal not found: %s", id)
		}
		return nil, fmt.Errorf("failed to get spec task proposal: %w", err)
	}
	return p, nil
}

// ListSpecTaskProposals queries proposals by filters.
func (s *PostgresStore) ListSpecTaskProposals(ctx context.Context, filters *types.SpecTaskProposalFilters) ([]*types.SpecTaskProposal, error) {
	if filters == nil {
		filters = &types.SpecTaskProposalFilters{}
	}
	q := s.gdb.WithContext(ctx).Model(&types.SpecTaskProposal{})
	if filters.SpecTaskID != "" {
		q = q.Where("spec_task_id = ?", filters.SpecTaskID)
	}
	if filters.ProjectID != "" {
		q = q.Where("project_id = ?", filters.ProjectID)
	}
	if filters.Kind != "" {
		q = q.Where("kind = ?", filters.Kind)
	}
	if filters.Status != "" {
		q = q.Where("status = ?", filters.Status)
	}
	q = q.Order("created_at DESC")
	if filters.Limit > 0 {
		q = q.Limit(filters.Limit)
	}
	var out []*types.SpecTaskProposal
	if err := q.Find(&out).Error; err != nil {
		return nil, fmt.Errorf("failed to list spec task proposals: %w", err)
	}
	return out, nil
}

// UpdateSpecTaskProposal updates a proposal in place. Caller is responsible for
// not mutating immutable fields (ID, SpecTaskID, ProjectID, Kind, CreatedAt).
func (s *PostgresStore) UpdateSpecTaskProposal(ctx context.Context, p *types.SpecTaskProposal) error {
	if p.ID == "" {
		return fmt.Errorf("proposal ID is required")
	}
	p.UpdatedAt = time.Now()
	if err := s.gdb.WithContext(ctx).Save(p).Error; err != nil {
		return fmt.Errorf("failed to update spec task proposal: %w", err)
	}
	return nil
}
