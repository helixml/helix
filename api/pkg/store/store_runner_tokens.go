package store

import (
	"context"
	"errors"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
)

// Per-runner token CRUD. Rows hold a hash of the secret, never the secret.

func (s *PostgresStore) CreateRunnerToken(ctx context.Context, t *types.RunnerToken) (*types.RunnerToken, error) {
	if t.ID == "" {
		return nil, errors.New("runner token ID is required")
	}
	if t.RunnerID == "" {
		return nil, errors.New("runner ID is required")
	}
	if t.TokenHash == "" {
		return nil, errors.New("runner token hash is required")
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now()
	}
	if err := s.gdb.WithContext(ctx).Create(t).Error; err != nil {
		return nil, err
	}
	return t, nil
}

// GetActiveRunnerTokenByHash returns the unrevoked token with this hash, or
// ErrNotFound.
func (s *PostgresStore) GetActiveRunnerTokenByHash(ctx context.Context, tokenHash string) (*types.RunnerToken, error) {
	if tokenHash == "" {
		return nil, errors.New("runner token hash is required")
	}
	var t types.RunnerToken
	err := s.gdb.WithContext(ctx).
		Where("token_hash = ? AND revoked_at IS NULL", tokenHash).
		First(&t).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &t, nil
}

// ListRunnerTokens lists tokens newest first, revoked ones included. An empty
// runnerID lists every runner's tokens.
func (s *PostgresStore) ListRunnerTokens(ctx context.Context, runnerID string) ([]*types.RunnerToken, error) {
	q := s.gdb.WithContext(ctx).Order("created_at DESC")
	if runnerID != "" {
		q = q.Where("runner_id = ?", runnerID)
	}
	var tokens []*types.RunnerToken
	if err := q.Find(&tokens).Error; err != nil {
		return nil, err
	}
	return tokens, nil
}

// RevokeRunnerToken marks a token revoked. Revoking an already revoked token
// keeps its original revocation time. Returns ErrNotFound for an unknown id.
func (s *PostgresStore) RevokeRunnerToken(ctx context.Context, id string) (*types.RunnerToken, error) {
	if id == "" {
		return nil, errors.New("runner token ID is required")
	}
	var t types.RunnerToken
	err := s.gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", id).First(&t).Error; err != nil {
			return err
		}
		if t.RevokedAt != nil {
			return nil
		}
		// Postgres keeps microseconds; truncate so the returned value matches
		// what a later read sees.
		now := time.Now().Truncate(time.Microsecond)
		if err := tx.Model(&t).Update("revoked_at", now).Error; err != nil {
			return err
		}
		t.RevokedAt = &now
		return nil
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &t, nil
}
