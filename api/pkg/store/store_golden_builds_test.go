package store

import (
	"context"
	"sync"
	"testing"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
)

func TestGoldenBuildsTestSuite(t *testing.T) {
	suite.Run(t, new(GoldenBuildsTestSuite))
}

type GoldenBuildsTestSuite struct {
	suite.Suite
	ctx       context.Context
	db        *PostgresStore
	projectID string
}

func (suite *GoldenBuildsTestSuite) SetupTest() {
	suite.ctx = context.Background()
	suite.db = GetTestDB()
	project, err := suite.db.CreateProject(suite.ctx, &types.Project{
		ID:     "prj_" + system.GenerateID(),
		Name:   "golden-builds-test",
		UserID: "usr_test",
	})
	suite.Require().NoError(err)
	suite.projectID = project.ID
}

func (suite *GoldenBuildsTestSuite) TearDownTest() {
	suite.NoError(suite.db.DeleteGoldenBuilds(suite.ctx, suite.projectID))
}

func (suite *GoldenBuildsTestSuite) TestUpdateCreatesThenModifies() {
	_, err := suite.db.GetGoldenBuild(suite.ctx, suite.projectID, "sb_1")
	suite.ErrorIs(err, ErrNotFound)

	row, err := suite.db.UpdateGoldenBuild(suite.ctx, suite.projectID, "sb_1", func(*types.SandboxCacheState) bool { return false })
	suite.Require().NoError(err)
	suite.Equal(types.GoldenBuildStatusNone, row.Status)

	row, err = suite.db.UpdateGoldenBuild(suite.ctx, suite.projectID, "sb_1", func(s *types.SandboxCacheState) bool {
		s.Status = types.GoldenBuildStatusBuilding
		s.Attempt = 2
		s.PendingRebuild = true
		s.InterruptReason = "sandbox restarted"
		return true
	})
	suite.Require().NoError(err)
	suite.Equal(types.GoldenBuildStatusBuilding, row.Status)

	got, err := suite.db.GetGoldenBuild(suite.ctx, suite.projectID, "sb_1")
	suite.Require().NoError(err)
	suite.Equal(2, got.Attempt)
	suite.True(got.PendingRebuild)
	suite.Equal("sandbox restarted", got.InterruptReason)
}

func (suite *GoldenBuildsTestSuite) TestListFilters() {
	for sb, status := range map[string]string{"sb_a": types.GoldenBuildStatusRetrying, "sb_b": types.GoldenBuildStatusReady} {
		_, err := suite.db.UpdateGoldenBuild(suite.ctx, suite.projectID, sb, func(s *types.SandboxCacheState) bool {
			s.Status = status
			return true
		})
		suite.Require().NoError(err)
	}

	all, err := suite.db.ListGoldenBuilds(suite.ctx, &ListGoldenBuildsQuery{ProjectID: suite.projectID})
	suite.Require().NoError(err)
	suite.Len(all, 2)

	active, err := suite.db.ListGoldenBuilds(suite.ctx, &ListGoldenBuildsQuery{ProjectID: suite.projectID, ActiveOnly: true})
	suite.Require().NoError(err)
	suite.Require().Len(active, 1)
	suite.Equal("sb_a", active[0].SandboxID)

	onB, err := suite.db.ListGoldenBuilds(suite.ctx, &ListGoldenBuildsQuery{ProjectID: suite.projectID, SandboxID: "sb_b"})
	suite.Require().NoError(err)
	suite.Require().Len(onB, 1)
	suite.Equal(types.GoldenBuildStatusReady, onB[0].Status)

	suite.NoError(suite.db.DeleteGoldenBuilds(suite.ctx, suite.projectID))
	none, err := suite.db.ListGoldenBuilds(suite.ctx, &ListGoldenBuildsQuery{ProjectID: suite.projectID})
	suite.Require().NoError(err)
	suite.Empty(none)
}

// Concurrent read-modify-writes must not lose updates: the row lock
// serialises them.
func (suite *GoldenBuildsTestSuite) TestConcurrentUpdatesAreSerialised() {
	const n = 20
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := suite.db.UpdateGoldenBuild(suite.ctx, suite.projectID, "sb_1", func(s *types.SandboxCacheState) bool {
				s.Attempt++
				return true
			})
			suite.NoError(err)
		}()
	}
	wg.Wait()

	got, err := suite.db.GetGoldenBuild(suite.ctx, suite.projectID, "sb_1")
	suite.Require().NoError(err)
	suite.Equal(n, got.Attempt)
}
