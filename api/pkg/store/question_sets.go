package store

import (
	"context"
	"errors"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
)

func (s *PostgresStore) CreateQuestionSet(ctx context.Context, questionSet *types.QuestionSet) (*types.QuestionSet, error) {
	if questionSet.ID == "" {
		questionSet.ID = system.GenerateQuestionSetID()
	}

	if questionSet.UserID == "" {
		return nil, errors.New("user_id is required")
	}

	now := time.Now()
	questionSet.Created = now
	questionSet.Updated = now

	db := s.gdb.WithContext(ctx)
	err := db.Create(questionSet).Error
	if err != nil {
		return nil, err
	}

	return questionSet, nil
}

func (s *PostgresStore) GetQuestionSet(ctx context.Context, id string) (*types.QuestionSet, error) {
	if id == "" {
		return nil, errors.New("id is required")
	}

	db := s.gdb.WithContext(ctx)
	var questionSet types.QuestionSet
	err := db.Where("id = ?", id).First(&questionSet).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return &questionSet, nil
}

func (s *PostgresStore) UpdateQuestionSet(ctx context.Context, questionSet *types.QuestionSet) (*types.QuestionSet, error) {
	if questionSet.ID == "" {
		return nil, errors.New("id is required")
	}

	existing, err := s.GetQuestionSet(ctx, questionSet.ID)
	if err != nil {
		return nil, err
	}

	questionSet.Created = existing.Created
	questionSet.Updated = time.Now()

	db := s.gdb.WithContext(ctx)
	err = db.Save(questionSet).Error
	if err != nil {
		return nil, err
	}

	return questionSet, nil
}

func (s *PostgresStore) ListQuestionSets(ctx context.Context, req *types.ListQuestionSetsRequest) ([]*types.QuestionSet, error) {
	db := s.gdb.WithContext(ctx)

	query := db.Model(&types.QuestionSet{})

	if req.UserID != "" {
		query = query.Where("user_id = ?", req.UserID)
	}
	if req.OrganizationID != "" {
		query = query.Where("organization_id = ?", req.OrganizationID)
	}

	var questionSets []*types.QuestionSet

	err := query.Order("created DESC").Find(&questionSets).Error
	if err != nil {
		return nil, err
	}

	return questionSets, nil
}

func (s *PostgresStore) DeleteQuestionSet(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("id is required")
	}

	db := s.gdb.WithContext(ctx)
	err := db.Where("id = ?", id).Delete(&types.QuestionSet{}).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return err
	}

	return nil
}
