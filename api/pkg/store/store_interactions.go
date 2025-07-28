package store

import (
	"context"
	"errors"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

func (s *PostgresStore) CreateInteraction(ctx context.Context, interaction *types.Interaction) (*types.Interaction, error) {
	if interaction.SessionID == "" {
		return nil, errors.New("session_id is required")
	}

	if interaction.UserID == "" {
		return nil, errors.New("user_id is required")
	}

	if interaction.ID == "" {
		interaction.ID = system.GenerateInteractionID()
	}

	db := s.gdb.WithContext(ctx)

	err := db.Create(&interaction).Error
	if err != nil {
		return nil, err
	}

	return interaction, nil
}

func (s *PostgresStore) CreateInteractions(ctx context.Context, interactions ...*types.Interaction) error {
	if len(interactions) == 0 {
		return nil
	}

	for idx, interaction := range interactions {
		if interaction.SessionID == "" {
			return errors.New("session_id is required")
		}

		if interaction.UserID == "" {
			return errors.New("user_id is required")
		}

		if interaction.ID == "" {
			interactions[idx].ID = system.GenerateInteractionID()
		}

		if idx == 0 {
			interactions[idx].Created = time.Now()
		}
	}

	db := s.gdb.WithContext(ctx)

	err := db.Create(&interactions).Error
	if err != nil {
		return err
	}

	return nil
}

func (s *PostgresStore) GetInteraction(ctx context.Context, interactionID string) (*types.Interaction, error) {
	db := s.gdb.WithContext(ctx)

	if interactionID == "" {
		return nil, errors.New("interaction_id is required")
	}

	var interaction types.Interaction
	err := db.Where("id = ?", interactionID).First(&interaction).Error
	if err != nil {
		return nil, err
	}

	return &interaction, nil
}

func (s *PostgresStore) UpdateInteraction(ctx context.Context, interaction *types.Interaction) (*types.Interaction, error) {
	if interaction.ID == "" {
		return nil, errors.New("id is required")
	}

	db := s.gdb.WithContext(ctx)

	err := db.Save(&interaction).Error
	if err != nil {
		return nil, err
	}

	return interaction, nil
}

func (s *PostgresStore) DeleteInteraction(ctx context.Context, interactionID string) error {
	db := s.gdb.WithContext(ctx)

	err := db.Delete(&types.Interaction{}, "id = ?", interactionID).Error
	if err != nil {
		return err
	}

	return nil
}

func (s *PostgresStore) ListInteractions(ctx context.Context, query *types.ListInteractionsQuery) ([]*types.Interaction, error) {
	db := s.gdb.WithContext(ctx)

	q := db.Model(&types.Interaction{})

	if query.SessionID != "" {
		q = q.Where("session_id = ?", query.SessionID)
	}

	if query.UserID != "" {
		q = q.Where("user_id = ?", query.UserID)
	}

	if query.Limit > 0 {
		q = q.Limit(query.Limit)
	}

	if query.Offset > 0 {
		q = q.Offset(query.Offset)
	}

	if query.GenerationID > 0 {
		q = q.Where("generation_id = ?", query.GenerationID)
	}

	var interactions []*types.Interaction
	// Oldest to newest
	err := q.Order("created_at ASC").Find(&interactions).Error
	if err != nil {
		return nil, err
	}

	return interactions, nil
}
