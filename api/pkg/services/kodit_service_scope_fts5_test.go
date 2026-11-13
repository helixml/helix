//go:build fts5 && !nokodit

// These tests run the repository checks of KoditService against a real Kodit
// SQLite store. Kodit's SQLite store needs FTS5, which the default unit-test
// build does not enable, so they only build with the fts5 tag:
//
//	go test -tags fts5 ./api/pkg/services -run TestKoditServiceRepositoryScope

package services

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/helixml/kodit"
	"github.com/helixml/kodit/domain/enrichment"
	"github.com/helixml/kodit/domain/search"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type zeroEmbedder struct{}

func (zeroEmbedder) Embed(_ context.Context, items []search.EmbeddingItem) ([][]float64, error) {
	return make([][]float64, len(items)), nil
}

func TestKoditServiceRepositoryScope(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "kodit.db")
	client, err := kodit.New(
		kodit.WithSQLite(dbPath),
		kodit.WithSkipProviderValidation(),
		kodit.WithEmbeddingProvider(zeroEmbedder{}),
		kodit.WithWorkerCount(0),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	// Two repositories with one commit each, and one enrichment per commit.
	// Kodit has no public API for commits or associations, so they are
	// written to its tables directly.
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	require.NoError(t, err)
	now := time.Now()
	require.NoError(t, db.Exec(`INSERT INTO git_repos (id, sanitized_remote_uri, remote_uri, created_at, updated_at) VALUES (1, 'a', 'a', ?, ?), (2, 'b', 'b', ?, ?)`, now, now, now, now).Error)
	require.NoError(t, db.Exec(`INSERT INTO git_commits (commit_sha, repo_id, date, message, author, created_at, updated_at) VALUES ('sha_a', 1, ?, 'm', 'x', ?, ?), ('sha_b', 2, ?, 'm', 'x', ?, ?)`, now, now, now, now, now, now).Error)
	enrichmentA, err := client.Enrichments.Save(ctx, enrichment.NewCookbook("repository A"))
	require.NoError(t, err)
	enrichmentB, err := client.Enrichments.Save(ctx, enrichment.NewCookbook("repository B"))
	require.NoError(t, err)
	require.NoError(t, db.Exec(`INSERT INTO enrichment_associations (enrichment_id, entity_type, entity_id, created_at, updated_at) VALUES (?, 'git_commits', 'sha_a', ?, ?), (?, 'git_commits', 'sha_b', ?, ?)`,
		enrichmentA.ID(), now, now, enrichmentB.ID(), now, now).Error)

	svc := NewKoditService(client)
	idA := strconv.FormatInt(enrichmentA.ID(), 10)
	idB := strconv.FormatInt(enrichmentB.ID(), 10)

	t.Run("enrichment of the repository", func(t *testing.T) {
		got, err := svc.GetRepositoryEnrichment(ctx, 1, idA)
		require.NoError(t, err)
		assert.Equal(t, "repository A", got.Content())
	})
	t.Run("enrichment of another repository", func(t *testing.T) {
		_, err := svc.GetRepositoryEnrichment(ctx, 1, idB)
		assert.ErrorIs(t, err, ErrKoditNotFound)
	})
	t.Run("repository without commits", func(t *testing.T) {
		_, err := svc.GetRepositoryEnrichment(ctx, 3, idA)
		assert.ErrorIs(t, err, ErrKoditNotFound)
	})
	t.Run("commit filter with the repository's commit", func(t *testing.T) {
		list, err := svc.GetRepositoryEnrichments(ctx, 1, "", "sha_a")
		require.NoError(t, err)
		require.Len(t, list, 1)
		assert.Equal(t, enrichmentA.ID(), list[0].ID())
	})
	t.Run("commit filter with another repository's commit", func(t *testing.T) {
		_, err := svc.GetRepositoryEnrichments(ctx, 1, "", "sha_b")
		assert.ErrorIs(t, err, ErrKoditNotFound)
	})
	t.Run("rescan of another repository's commit", func(t *testing.T) {
		assert.ErrorIs(t, svc.RescanCommit(ctx, 1, "sha_b"), ErrKoditNotFound)
	})
	t.Run("rescan of the repository's commit passes the check", func(t *testing.T) {
		// The fixture repository has no indexing pipeline, so the rescan
		// itself fails later; only the commit check is under test.
		err := svc.RescanCommit(ctx, 1, "sha_a")
		assert.False(t, errors.Is(err, ErrKoditNotFound), "unexpected not-found: %v", err)
	})
}
