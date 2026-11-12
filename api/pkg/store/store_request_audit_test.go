package store

import (
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

func (suite *PostgresStoreTestSuite) TestPostgresStore_RequestAuditEvents() {
	userID := "usr_" + system.GenerateUUID()
	remoteAddr := "203.0.113.9:1234"
	base := time.Now().UTC().Add(-time.Hour)

	events := []*types.RequestAuditEvent{
		{
			ID: "rau_" + system.GenerateUUID(), Created: base, Method: "POST",
			Path: "/api/v1/sessions", Status: 201, UserID: userID, RemoteAddr: remoteAddr,
			CredentialSource: "session", CredentialFingerprint: "abcdefabcdef",
			Body: `{"email":"operator@example.com"}`, BodySize: 29,
		},
		{
			ID: "rau_" + system.GenerateUUID(), Created: base.Add(time.Minute), Method: "GET",
			Path: "/api/v1/sessions/ses_1", Status: 200, UserID: userID, RemoteAddr: remoteAddr,
		},
		{
			ID: "rau_" + system.GenerateUUID(), Created: base.Add(2 * time.Minute), Method: "DELETE",
			Path: "/api/v1/sessions/ses_1", Status: 403, UserID: userID, RemoteAddr: remoteAddr,
		},
	}
	suite.Require().NoError(suite.db.InsertRequestAuditEvents(suite.ctx, events))
	suite.T().Cleanup(func() {
		ids := make([]string, 0, len(events))
		for _, event := range events {
			ids = append(ids, event.ID)
		}
		_ = suite.db.gdb.WithContext(suite.ctx).Where("id IN ?", ids).Delete(&types.RequestAuditEvent{}).Error
	})

	response, err := suite.db.ListRequestAuditEvents(suite.ctx, &types.RequestAuditEventFilters{UserID: userID})
	suite.Require().NoError(err)
	suite.Equal(int64(3), response.Total)
	suite.Require().Len(response.Events, 3)
	suite.Equal("DELETE", response.Events[0].Method, "newest first")
	suite.Equal(403, response.Events[0].Status)

	status := 200
	response, err = suite.db.ListRequestAuditEvents(suite.ctx, &types.RequestAuditEventFilters{UserID: userID, Status: &status})
	suite.Require().NoError(err)
	suite.Require().Len(response.Events, 1)
	suite.Equal("/api/v1/sessions/ses_1", response.Events[0].Path)

	response, err = suite.db.ListRequestAuditEvents(suite.ctx, &types.RequestAuditEventFilters{UserID: userID, PathPrefix: "/api/v1/sessions/"})
	suite.Require().NoError(err)
	suite.Equal(int64(2), response.Total, "path is a prefix filter")

	response, err = suite.db.ListRequestAuditEvents(suite.ctx, &types.RequestAuditEventFilters{UserID: userID, PathPrefix: "/api/v1/sess_ons"})
	suite.Require().NoError(err)
	suite.Equal(int64(0), response.Total, "underscore in the prefix is escaped, not a wildcard")

	response, err = suite.db.ListRequestAuditEvents(suite.ctx, &types.RequestAuditEventFilters{UserID: userID, Method: "get"})
	suite.Require().NoError(err)
	suite.Require().Len(response.Events, 1)
	suite.Equal("GET", response.Events[0].Method)

	response, err = suite.db.ListRequestAuditEvents(suite.ctx, &types.RequestAuditEventFilters{UserID: userID, RemoteAddr: remoteAddr})
	suite.Require().NoError(err)
	suite.Equal(int64(3), response.Total)

	since := base.Add(90 * time.Second)
	response, err = suite.db.ListRequestAuditEvents(suite.ctx, &types.RequestAuditEventFilters{UserID: userID, Since: &since})
	suite.Require().NoError(err)
	suite.Require().Len(response.Events, 1)
	suite.Equal("DELETE", response.Events[0].Method)

	response, err = suite.db.ListRequestAuditEvents(suite.ctx, &types.RequestAuditEventFilters{UserID: userID, Limit: 1, Offset: 1})
	suite.Require().NoError(err)
	suite.Equal(int64(3), response.Total)
	suite.Require().Len(response.Events, 1)
	suite.Equal("GET", response.Events[0].Method)
	suite.Equal(1, response.Limit)
	suite.Equal(1, response.Offset)
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_PruneRequestAuditEvents() {
	userID := "usr_" + system.GenerateUUID()
	oldID := "rau_" + system.GenerateUUID()
	recentID := "rau_" + system.GenerateUUID()

	events := []*types.RequestAuditEvent{
		{ID: oldID, Created: time.Now().UTC().Add(-48 * time.Hour), Method: "GET", Path: "/api/v1/old", Status: 200, UserID: userID},
		{ID: recentID, Created: time.Now().UTC(), Method: "GET", Path: "/api/v1/recent", Status: 200, UserID: userID},
	}
	suite.Require().NoError(suite.db.InsertRequestAuditEvents(suite.ctx, events))
	suite.T().Cleanup(func() {
		_ = suite.db.gdb.WithContext(suite.ctx).Where("id IN ?", []string{oldID, recentID}).Delete(&types.RequestAuditEvent{}).Error
	})

	deleted, err := suite.db.PruneRequestAuditEvents(suite.ctx, time.Now().UTC().Add(-24*time.Hour), 100)
	suite.Require().NoError(err)
	suite.GreaterOrEqual(deleted, int64(1))

	response, err := suite.db.ListRequestAuditEvents(suite.ctx, &types.RequestAuditEventFilters{UserID: userID})
	suite.Require().NoError(err)
	suite.Require().Len(response.Events, 1)
	suite.Equal(recentID, response.Events[0].ID)
}
