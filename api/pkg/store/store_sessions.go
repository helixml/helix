package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *PostgresStore) ListSessions(ctx context.Context, query ListSessionsQuery) ([]*types.Session, int64, error) {
	// Start with the basic query builder
	q := s.gdb.WithContext(ctx).Model(&types.Session{})

	// Add owner and owner type conditions
	q = q.Where("owner = ? AND owner_type = ?", query.Owner, query.OwnerType)

	// Add parent session condition if specified
	if query.ParentSession != "" {
		q = q.Where("parent_session = ?", query.ParentSession)
	}

	if query.QuestionSetID != "" {
		q = q.Where("question_set_id", query.QuestionSetID)
	}

	if query.QuestionSetExecutionID != "" {
		q = q.Where("question_set_execution_id = ?", query.QuestionSetExecutionID)
	}

	if query.OrganizationID != "" {
		q = q.Where("organization_id = ?", query.OrganizationID)
	} else {
		q = q.Where("organization_id IS NULL OR organization_id = ''")
	}

	// Add ordering
	q = q.Order("created DESC")

	if query.PerPage == 0 {
		query.PerPage = -1
	}

	var offset int

	if query.Page > 0 {
		offset = (query.Page - 1) * query.PerPage
	}

	if query.Search != "" {
		q = q.Where("name LIKE ?", "%"+query.Search+"%")
	}

	totalCount := int64(0)

	err := q.Count(&totalCount).Error
	if err != nil {
		return nil, 0, err
	}

	// Execute query and return results
	var sessions []*types.Session
	err = q.Offset(offset).Limit(query.PerPage).Find(&sessions).Error
	if err != nil {
		return nil, 0, err
	}

	return sessions, totalCount, nil
}

func (s *PostgresStore) CreateSession(ctx context.Context, session types.Session) (*types.Session, error) {
	if session.ID == "" {
		session.ID = system.GenerateSessionID()
	}

	if session.Created.IsZero() {
		session.Created = time.Now()
	}

	err := s.gdb.WithContext(ctx).Omit(clause.Associations).Create(&session).Error
	if err != nil {
		return nil, err
	}

	return &session, nil
}

func (s *PostgresStore) GetSession(ctx context.Context, sessionID string) (*types.Session, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("sessionID cannot be empty")
	}

	var session types.Session
	err := s.gdb.WithContext(ctx).Where("id = ?", sessionID).First(&session).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return &session, nil
}

func (s *PostgresStore) UpdateSession(ctx context.Context, session types.Session) (*types.Session, error) {
	if session.ID == "" {
		return nil, fmt.Errorf("id not specified")
	}

	// Log session metadata before update
	ragResultsCount := 0
	if session.Metadata.SessionRAGResults != nil {
		ragResultsCount = len(session.Metadata.SessionRAGResults)
	}

	log.Debug().
		Str("session_id", session.ID).
		Interface("document_ids", session.Metadata.DocumentIDs).
		Str("parent_app", session.ParentApp).
		Int("rag_results_count", ragResultsCount).
		Bool("has_rag_results", session.Metadata.SessionRAGResults != nil).
		Msg("🔍 Before UpdateSession - session metadata")

	// Create a debug SQL logger to see the actual SQL query
	debugDB := s.gdb.WithContext(ctx).Debug()

	err := debugDB.Omit(clause.Associations).Save(&session).Error
	if err != nil {
		log.Error().Err(err).Str("session_id", session.ID).Msg("❌ Failed to update session")
		return nil, err
	}

	return s.GetSession(ctx, session.ID)
}

func (s *PostgresStore) UpdateSessionMeta(ctx context.Context, data types.SessionMetaUpdate) (*types.Session, error) {
	if data.ID == "" {
		return nil, fmt.Errorf("id not specified")
	}

	err := s.gdb.WithContext(ctx).Where(&types.Session{
		ID: data.ID,
	}).Updates(&types.Session{
		Name:      data.Name,
		Owner:     data.Owner,
		OwnerType: data.OwnerType,
	}).Error
	if err != nil {
		return nil, err
	}

	return s.GetSession(ctx, data.ID)
}

func (s *PostgresStore) UpdateSessionName(ctx context.Context, sessionID, name string) error {
	if sessionID == "" {
		return fmt.Errorf("id not specified")
	}

	if name == "" {
		return fmt.Errorf("name not specified")
	}

	err := s.gdb.WithContext(ctx).Model(&types.Session{}).Where("id = ?", sessionID).Update("name", name).Error
	if err != nil {
		return err
	}
	return nil
}

func (s *PostgresStore) DeleteSession(ctx context.Context, sessionID string) (*types.Session, error) {
	existing, err := s.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	err = s.gdb.WithContext(ctx).Delete(&types.Session{
		ID: sessionID,
	}).Error
	if err != nil {
		return nil, err
	}

	return existing, nil
}
