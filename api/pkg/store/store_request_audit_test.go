package store

import (
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

func (suite *PostgresStoreTestSuite) TestPostgresStore_RequestAuditEvents() {
	userID := "usr_" + system.GenerateUUID()
	remoteIP := "203.0.113.9"
	base := time.Now().UTC().Add(-time.Hour)

	events := []*types.RequestAuditEvent{
		{
			ID: "rau_" + system.GenerateUUID(), Created: base, Method: "POST",
			Path: "/api/v1/sessions", Status: 201, UserID: userID, RemoteAddr: remoteIP + ":1234",
			RemoteIP: remoteIP, RemotePort: 1234,
			CredentialSource: "session", CredentialFingerprint: "abcdefabcdef",
			Body: `{"email":"operator@example.com"}`, BodySize: 29,
		},
		{
			ID: "rau_" + system.GenerateUUID(), Created: base.Add(time.Minute), Method: "GET",
			Path: "/api/v1/sessions/ses_1", Status: 200, UserID: userID, RemoteAddr: remoteIP + ":1234",
			RemoteIP: remoteIP, RemotePort: 1234,
		},
		{
			ID: "rau_" + system.GenerateUUID(), Created: base.Add(2 * time.Minute), Method: "DELETE",
			Path: "/api/v1/sessions/ses_1", Status: 403, UserID: userID, RemoteAddr: remoteIP + ":1234",
			RemoteIP: remoteIP, RemotePort: 1234,
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
	suite.Nil(response.Total, "total is opt-in")
	suite.Require().Len(response.Events, 3)
	suite.Equal("DELETE", response.Events[0].Method, "newest first")
	suite.Equal(403, response.Events[0].Status)
	suite.Empty(response.NextCursor, "single page")

	response, err = suite.db.ListRequestAuditEvents(suite.ctx, &types.RequestAuditEventFilters{UserID: userID, IncludeTotal: true})
	suite.Require().NoError(err)
	suite.Require().NotNil(response.Total)
	suite.Equal(int64(3), *response.Total)
	suite.Require().Len(response.Events, 3, "the count must not replace the page query")

	status := 200
	response, err = suite.db.ListRequestAuditEvents(suite.ctx, &types.RequestAuditEventFilters{UserID: userID, Status: &status})
	suite.Require().NoError(err)
	suite.Require().Len(response.Events, 1)
	suite.Equal("/api/v1/sessions/ses_1", response.Events[0].Path)

	response, err = suite.db.ListRequestAuditEvents(suite.ctx, &types.RequestAuditEventFilters{UserID: userID, PathPrefix: "/api/v1/sessions/"})
	suite.Require().NoError(err)
	suite.Require().Len(response.Events, 2, "path is a prefix filter")

	response, err = suite.db.ListRequestAuditEvents(suite.ctx, &types.RequestAuditEventFilters{UserID: userID, PathPrefix: "/api/v1/sess_ons"})
	suite.Require().NoError(err)
	suite.Empty(response.Events, "underscore in the prefix is escaped, not a wildcard")

	response, err = suite.db.ListRequestAuditEvents(suite.ctx, &types.RequestAuditEventFilters{UserID: userID, Method: "get"})
	suite.Require().NoError(err)
	suite.Require().Len(response.Events, 1)
	suite.Equal("GET", response.Events[0].Method)

	// The filter is on the address without the ephemeral port.
	response, err = suite.db.ListRequestAuditEvents(suite.ctx, &types.RequestAuditEventFilters{UserID: userID, RemoteIP: remoteIP})
	suite.Require().NoError(err)
	suite.Require().Len(response.Events, 3)

	response, err = suite.db.ListRequestAuditEvents(suite.ctx, &types.RequestAuditEventFilters{UserID: userID, RemoteIP: "198.51.100.7"})
	suite.Require().NoError(err)
	suite.Empty(response.Events)

	since := base.Add(90 * time.Second)
	response, err = suite.db.ListRequestAuditEvents(suite.ctx, &types.RequestAuditEventFilters{UserID: userID, Since: &since})
	suite.Require().NoError(err)
	suite.Require().Len(response.Events, 1)
	suite.Equal("DELETE", response.Events[0].Method)

	// Keyset pagination: walk the whole set one row at a time.
	var paged []string
	cursor := ""
	for page := 0; page < 5; page++ {
		response, err = suite.db.ListRequestAuditEvents(suite.ctx, &types.RequestAuditEventFilters{
			UserID: userID, Limit: 1, Cursor: cursor,
		})
		suite.Require().NoError(err)
		for _, event := range response.Events {
			paged = append(paged, event.ID)
		}
		if response.NextCursor == "" {
			break
		}
		cursor = response.NextCursor
	}
	suite.Equal([]string{events[2].ID, events[1].ID, events[0].ID}, paged, "newest first, no duplicates or gaps")
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_RequestAuditEventsRejectsBadCursor() {
	_, err := suite.db.ListRequestAuditEvents(suite.ctx, &types.RequestAuditEventFilters{Cursor: "not-a-cursor"})
	suite.Require().Error(err)
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
