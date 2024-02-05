package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
)

func (s *PostgresStore) GetSessions(ctx context.Context, query GetSessionsQuery) ([]*types.Session, error) {

	q := s.gdb.WithContext(ctx).Model(&types.Session{}).Where(&types.Session{
		Owner:     query.Owner,
		OwnerType: query.OwnerType,
	})

	q = q.Order("created DESC")

	if query.Limit > 0 {
		q = q.Limit(query.Limit)
	}

	if query.Offset > 0 {
		q = q.Offset(query.Offset)
	}

	var sessions []*types.Session
	err := q.Find(&sessions).Error
	if err != nil {
		return nil, err
	}

	return sessions, nil
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

	err := s.gdb.WithContext(ctx).Save(&session).Error
	if err != nil {
		return nil, err
	}
	return s.GetSession(ctx, session.ID)
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

func (d *PostgresStore) GetSessionsCounter(
	ctx context.Context,
	query GetSessionsQuery,
) (*types.Counter, error) {
	count, err := d.db.
		From("session").
		Where(d.getSessionsWhere(query)).
		Count()
	if err != nil {
		return nil, err
	}
	return &types.Counter{
		Count: count,
	}, nil
}
