package store

import (
	"strings"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// HelixOS names hypothesis bots after the full hypothesis statement, which
// overflowed the old varchar(128) sandboxes.name column and stopped the bot's
// desktop from ever starting. Must run against Postgres: SQLite ignores
// varchar lengths.
func (suite *PostgresStoreTestSuite) TestSandboxAcceptsLongName() {
	name := strings.Repeat("n", 200)
	sb, err := suite.db.CreateSandbox(suite.ctx, &types.Sandbox{
		ID:             system.GenerateID(),
		Name:           name,
		OrganizationID: "org_" + system.GenerateID(),
		Owner:          "user_1",
		SessionID:      "ses_" + system.GenerateID(),
		OrgBotID:       "hyp-hyp_04b4aba24867486eb0fc3c0489c1e572-luke",
		Runtime:        types.SandboxRuntimeUbuntuDesktop,
		Status:         types.SandboxStatusPending,
	})
	suite.Require().NoError(err)
	suite.T().Cleanup(func() { _ = suite.db.DeleteSandbox(suite.ctx, sb.ID) })

	sb.Name = strings.Repeat("漢", types.SandboxNameMaxLen)
	updated, err := suite.db.UpdateSandbox(suite.ctx, sb)
	suite.Require().NoError(err)
	suite.Require().Equal(sb.Name, updated.Name)

	got, err := suite.db.GetSandbox(suite.ctx, sb.ID)
	suite.Require().NoError(err)
	suite.Require().Equal(sb.Name, got.Name)
}
