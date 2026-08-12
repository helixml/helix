package store

import (
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

func (suite *PostgresStoreTestSuite) TestPostgresStore_OrgAuditLogs() {
	orgID := "org_" + system.GenerateUUID()
	otherOrgID := "org_" + system.GenerateUUID()
	createdAt := time.Now().UTC()
	entries := []*types.OrgAuditLog{
		{
			ID: "org_audit_" + system.GenerateUUID(), OrganizationID: orgID,
			ProjectID: "project-1", ActorID: "bot-1", ActorType: types.OrgAuditActorBot,
			AssetID: "asset-1", EventType: types.OrgAuditEventSSHCommand, Action: "exec",
			Status:   types.OrgAuditStatusAttempted,
			Metadata: types.OrgAuditMetadata{Command: "uname -a"}, CreatedAt: createdAt,
		},
		{
			ID: "org_audit_" + system.GenerateUUID(), OrganizationID: orgID,
			ActorID: "bot-1", ActorType: types.OrgAuditActorBot,
			EventType: types.OrgAuditEventMCPCall, Action: "list_assets",
			Status: types.OrgAuditStatusSucceeded, CreatedAt: createdAt.Add(time.Second),
		},
		{
			ID: "org_audit_" + system.GenerateUUID(), OrganizationID: otherOrgID,
			ActorID: "bot-2", ActorType: types.OrgAuditActorBot,
			AssetID: "asset-1", EventType: types.OrgAuditEventSSHCommand, Action: "exec",
			Status:   types.OrgAuditStatusAttempted,
			Metadata: types.OrgAuditMetadata{Command: "hostname"}, CreatedAt: createdAt.Add(2 * time.Second),
		},
	}
	for _, entry := range entries {
		suite.Require().NoError(suite.db.CreateOrgAuditLog(suite.ctx, entry))
	}
	suite.T().Cleanup(func() {
		ids := make([]string, 0, len(entries))
		for _, entry := range entries {
			ids = append(ids, entry.ID)
		}
		_ = suite.db.gdb.WithContext(suite.ctx).Where("id IN ?", ids).Delete(&types.OrgAuditLog{}).Error
	})

	response, err := suite.db.ListOrgAuditLogs(suite.ctx, &types.OrgAuditLogFilters{
		OrganizationID: orgID,
		AssetID:        "asset-1",
		EventType:      types.OrgAuditEventSSHCommand,
		Search:         "uname",
	})
	suite.Require().NoError(err)
	suite.Equal(int64(1), response.Total)
	suite.Require().Len(response.Logs, 1)
	suite.Equal("project-1", response.Logs[0].ProjectID)
	suite.Equal("uname -a", response.Logs[0].Metadata.Command)
}
