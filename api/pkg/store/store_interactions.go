package store

import (
	"context"
	"errors"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm/clause"
)

func (s *PostgresStore) ResetRunningInteractions(ctx context.Context) error {
	err := s.gdb.WithContext(ctx).Model(&types.Interaction{}).
		Where("state = ?", types.InteractionStateWaiting).
		Updates(map[string]any{
			"state": types.InteractionStateError,
			"error": "Interrupted",
		}).
		Error
	if err != nil {
		return err
	}
	return nil
}

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

	// Allows overwriting the interaction with the same primary key (ID and generation ID)
	err := db.Clauses(clause.OnConflict{
		UpdateAll: true,
	}).Create(&interaction).Error
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

	// Allows overwriting the interaction with the same primary key (ID and generation ID)
	err := db.Clauses(clause.OnConflict{
		UpdateAll: true,
	}).Create(&interactions).Error
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

	err := db.Clauses(clause.OnConflict{
		UpdateAll: true,
	}).Save(&interaction).Error
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

func (s *PostgresStore) ListInteractions(ctx context.Context, query *types.ListInteractionsQuery) ([]*types.Interaction, int64, error) {
	db := s.gdb.WithContext(ctx)

	q := db.Model(&types.Interaction{})

	if query.PerPage == 0 {
		query.PerPage = -1
	}

	var offset int

	if query.Page > 0 {
		offset = (query.Page - 1) * query.PerPage
	}

	if query.SessionID != "" {
		q = q.Where("session_id = ?", query.SessionID)
	}

	if query.AppID != "" {
		q = q.Where("app_id = ?", query.AppID)
	}

	if query.InteractionID != "" {
		q = q.Where("id = ?", query.InteractionID)
	}

	if query.UserID != "" {
		q = q.Where("user_id = ?", query.UserID)
	}

	if query.GenerationID > 0 {
		q = q.Where("generation_id = ?", query.GenerationID)
	}

	if query.Order == "" {
		query.Order = "id ASC"
	}

	if query.Feedback != "" {
		q = q.Where("feedback = ?", query.Feedback)
	}

	totalCount := int64(0)

	err := q.Count(&totalCount).Error
	if err != nil {
		return nil, 0, err
	}

	var interactions []*types.Interaction
	// Oldest to newest
	err = q.Order(query.Order).Offset(offset).Limit(query.PerPage).Find(&interactions).Error
	if err != nil {
		return nil, 0, err
	}

	return interactions, totalCount, nil
}
