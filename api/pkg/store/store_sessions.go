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
)

func (s *PostgresStore) GetSessions(ctx context.Context, query GetSessionsQuery) ([]*types.Session, error) {
	// Start with the basic query builder
	q := s.gdb.WithContext(ctx).Model(&types.Session{})

	// Add owner and owner type conditions
	q = q.Where("owner = ? AND owner_type = ?", query.Owner, query.OwnerType)

	// Add parent session condition if specified
	if query.ParentSession != "" {
		q = q.Where("parent_session = ?", query.ParentSession)
	}

	// Handle organization_id differently depending on whether it's specified
	if query.OrganizationID != "" {
		if query.OrganizationID == "default" {
			// For "default" organization ID, explicitly return only sessions with no organization
			q = q.Where("organization_id IS NULL OR organization_id = ''")
		} else {
			// Filter for sessions with this specific organization
			q = q.Where("organization_id = ?", query.OrganizationID)
		}
	} else {
		// No organization filter provided, return all sessions (personal and org)
		// This is different from the "default" case which returns only sessions with no organization
	}

	// Add ordering
	q = q.Order("created DESC")

	// Add pagination
	if query.Limit > 0 {
		q = q.Limit(query.Limit)
	}

	if query.Offset > 0 {
		q = q.Offset(query.Offset)
	}

	// Execute query and return results
	var sessions []*types.Session
	err := q.Find(&sessions).Error
	if err != nil {
		return nil, err
	}

	return sessions, nil
}

func (s *PostgresStore) GetSessionsCounter(ctx context.Context, query GetSessionsQuery) (*types.Counter, error) {
	// Start with the basic query builder
	q := s.gdb.WithContext(ctx).Model(&types.Session{})

	// Add owner and owner type conditions
	q = q.Where("owner = ? AND owner_type = ?", query.Owner, query.OwnerType)

	// Add parent session condition if specified
	if query.ParentSession != "" {
		q = q.Where("parent_session = ?", query.ParentSession)
	}

	// Handle organization_id differently depending on whether it's specified
	if query.OrganizationID != "" {
		if query.OrganizationID == "default" {
			// For "default" organization ID, explicitly count only sessions with no organization
			q = q.Where("organization_id IS NULL OR organization_id = ''")
		} else {
			// Count sessions with this specific organization
			q = q.Where("organization_id = ?", query.OrganizationID)
		}
	} else {
		// No organization filter provided, count all sessions (personal and org)
		// This is different from the "default" case which counts only sessions with no organization
	}

	var counter int64
	err := q.Count(&counter).Error
	if err != nil {
		return nil, err
	}

	return &types.Counter{
		Count: counter,
	}, nil
}

func (s *PostgresStore) CreateSession(ctx context.Context, session types.Session) (*types.Session, error) {
	if session.ID == "" {
		session.ID = system.GenerateSessionID()
	}

	if session.Created.IsZero() {
		session.Created = time.Now()
	}

	err := s.gdb.WithContext(ctx).Create(&session).Error
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
	log.Debug().
		Str("session_id", session.ID).
		Interface("document_ids", session.Metadata.DocumentIDs).
		Str("parent_app", session.ParentApp).
		Msg("🔍 Before UpdateSession - session metadata")

	// Create a debug SQL logger to see the actual SQL query
	debugDB := s.gdb.WithContext(ctx).Debug()

	err := debugDB.Save(&session).Error
	if err != nil {
		log.Error().Err(err).Str("session_id", session.ID).Msg("❌ Failed to update session")
		return nil, err
	}

	// Verify what was saved by retrieving it again
	updatedSession, getErr := s.GetSession(ctx, session.ID)
	if getErr != nil {
		log.Error().Err(getErr).Str("session_id", session.ID).Msg("❌ Failed to retrieve updated session")
	} else {
		log.Debug().
			Str("session_id", session.ID).
			Interface("document_ids_after", updatedSession.Metadata.DocumentIDs).
			Str("parent_app", updatedSession.ParentApp).
			Msg("✅ After UpdateSession - session metadata verified")
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
