package store

import (
	"context"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

func (suite *PostgresStoreTestSuite) TestListGitRepositoriesByKoditRepoID() {
	koditID := time.Now().UnixNano() % 1_000_000_000
	mk := func(meta map[string]any) *types.GitRepository {
		repo := &types.GitRepository{
			ID:        "repo-kodit-" + system.GenerateUUID(),
			Name:      "kodit-lookup",
			OwnerID:   "user-kodit-lookup",
			RepoType:  types.GitRepositoryTypeCode,
			Status:    types.GitRepositoryStatusActive,
			Metadata:  meta,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		suite.Require().NoError(suite.db.CreateGitRepository(suite.ctx, repo))
		suite.T().Cleanup(func() { _ = suite.db.DeleteGitRepository(context.Background(), repo.ID) })
		return repo
	}
	a := mk(map[string]any{"kodit_repo_id": koditID})
	b := mk(map[string]any{"kodit_repo_id": koditID})
	mk(map[string]any{"kodit_repo_id": koditID + 1})
	mk(nil)

	repos, err := suite.db.ListGitRepositoriesByKoditRepoID(suite.ctx, koditID)
	suite.Require().NoError(err)
	ids := []string{}
	for _, r := range repos {
		ids = append(ids, r.ID)
	}
	suite.ElementsMatch([]string{a.ID, b.ID}, ids)
}
