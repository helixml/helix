package store

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

type RunnerTokenStoreSuite struct {
	PostgresStoreTestSuite
}

func TestRunnerTokenStoreSuite(t *testing.T) {
	suite.Run(t, new(RunnerTokenStoreSuite))
}

func (s *RunnerTokenStoreSuite) newToken(runnerID string) (*types.RunnerToken, string) {
	secret, err := system.GenerateRunnerToken()
	s.Require().NoError(err)
	created, err := s.db.CreateRunnerToken(s.ctx, &types.RunnerToken{
		ID:          system.GenerateRunnerTokenID(),
		RunnerID:    runnerID,
		TokenHash:   types.HashRunnerToken(secret),
		TokenPrefix: types.RunnerTokenVisiblePrefix(secret),
	})
	s.Require().NoError(err)
	s.T().Cleanup(func() { s.db.gdb.Delete(&types.RunnerToken{}, "id = ?", created.ID) })
	return created, secret
}

// The secret is never stored: the row holds its hash, and a lookup by hash
// finds the token until it is revoked.
func (s *RunnerTokenStoreSuite) TestCreateLookupRevoke() {
	runnerID := "host-" + system.GenerateID()
	created, secret := s.newToken(runnerID)

	var raw types.RunnerToken
	s.Require().NoError(s.db.gdb.Where("id = ?", created.ID).First(&raw).Error)
	s.NotContains(raw.TokenHash, secret)
	s.NotEqual(secret, raw.TokenPrefix)
	s.Equal(types.HashRunnerToken(secret), raw.TokenHash)

	found, err := s.db.GetActiveRunnerTokenByHash(s.ctx, types.HashRunnerToken(secret))
	s.Require().NoError(err)
	s.Equal(created.ID, found.ID)
	s.Equal(runnerID, found.RunnerID)

	_, err = s.db.GetActiveRunnerTokenByHash(s.ctx, types.HashRunnerToken(secret+"x"))
	s.ErrorIs(err, ErrNotFound)

	revoked, err := s.db.RevokeRunnerToken(s.ctx, created.ID, "usr_admin")
	s.Require().NoError(err)
	s.Require().NotNil(revoked.RevokedAt)
	s.Equal("usr_admin", revoked.RevokedBy)

	_, err = s.db.GetActiveRunnerTokenByHash(s.ctx, types.HashRunnerToken(secret))
	s.ErrorIs(err, ErrNotFound, "a revoked token must not authenticate")

	// Revoking again keeps the first revocation time.
	again, err := s.db.RevokeRunnerToken(s.ctx, created.ID, "usr_other")
	s.Require().NoError(err)
	s.True(again.RevokedAt.Equal(*revoked.RevokedAt))
	s.Equal("usr_admin", again.RevokedBy, "a second revoke keeps the first revoker")

	var stored types.RunnerToken
	s.Require().NoError(s.db.gdb.Where("id = ?", created.ID).First(&stored).Error)
	s.Equal("usr_admin", stored.RevokedBy)

	_, err = s.db.RevokeRunnerToken(s.ctx, "rtok_missing", "usr_admin")
	s.ErrorIs(err, ErrNotFound)
}

func (s *RunnerTokenStoreSuite) TestListFiltersByRunner() {
	runnerA := "host-" + system.GenerateID()
	runnerB := "host-" + system.GenerateID()
	a1, _ := s.newToken(runnerA)
	a2, _ := s.newToken(runnerA)
	_, _ = s.newToken(runnerB)
	_, err := s.db.RevokeRunnerToken(s.ctx, a1.ID, "usr_admin")
	s.Require().NoError(err)

	tokens, err := s.db.ListRunnerTokens(s.ctx, runnerA)
	s.Require().NoError(err)
	ids := []string{}
	for _, t := range tokens {
		s.Equal(runnerA, t.RunnerID)
		ids = append(ids, t.ID)
	}
	s.ElementsMatch([]string{a1.ID, a2.ID}, ids, "revoked tokens are listed too")
}
