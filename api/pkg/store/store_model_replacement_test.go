package store

import (
	"context"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// Replacing a retired model finds sessions by the model in their coding config
// or overrides, across every owner and organization.
func (suite *PostgresStoreTestSuite) TestListSessions_CodeAgentModels() {
	ctx := context.Background()
	model := "retired-" + system.GenerateUUID()
	now := time.Now()
	create := func(owner, org string, metadata types.SessionMetadata) string {
		s, err := suite.db.CreateSession(ctx, types.Session{
			ID: system.GenerateSessionID(), Owner: owner, OrganizationID: org, Created: now, Updated: now, Metadata: metadata,
		})
		suite.Require().NoError(err)
		suite.T().Cleanup(func() { _, _ = suite.db.DeleteSession(ctx, s.ID) })
		return s.ID
	}
	byConfig := create("user_a", "org_"+system.GenerateUUID(), types.SessionMetadata{
		CodeAgentConfig: &types.CodeAgentExecutionConfig{ProviderRef: "pe_x", Model: model},
	})
	byOverrides := create("user_b", "", types.SessionMetadata{
		CodeAgentOverrides: &types.CodeAgentOverrides{ProviderRef: "pe_x", Model: "pe_x/" + model},
	})
	create("user_c", "", types.SessionMetadata{
		CodeAgentConfig: &types.CodeAgentExecutionConfig{ProviderRef: "pe_x", Model: "other-" + model},
	})

	got, _, err := suite.db.ListSessions(ctx, ListSessionsQuery{
		AnyOwner: true, AnyOrganization: true, CodeAgentModels: []string{model, "pe_x/" + model},
	})
	suite.Require().NoError(err)
	ids := []string{}
	for _, s := range got {
		ids = append(ids, s.ID)
	}
	suite.ElementsMatch([]string{byConfig, byOverrides}, ids)
}

func (suite *PostgresStoreTestSuite) TestListProjects_AllScopes() {
	ctx := context.Background()
	personal, err := suite.db.CreateProject(ctx, &types.Project{ID: "prj_" + system.GenerateID(), Name: "personal-" + system.GenerateID(), UserID: "usr_" + system.GenerateID()})
	suite.Require().NoError(err)
	inOrg, err := suite.db.CreateProject(ctx, &types.Project{ID: "prj_" + system.GenerateID(), Name: "org-" + system.GenerateID(), UserID: "usr_" + system.GenerateID(), OrganizationID: "org_" + system.GenerateID()})
	suite.Require().NoError(err)
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteProject(ctx, personal.ID)
		_ = suite.db.DeleteProject(ctx, inOrg.ID)
	})

	all, err := suite.db.ListProjects(ctx, &ListProjectsQuery{AllScopes: true})
	suite.Require().NoError(err)
	ids := map[string]bool{}
	for _, p := range all {
		ids[p.ID] = true
	}
	suite.True(ids[personal.ID], "personal project listed")
	suite.True(ids[inOrg.ID], "org project listed")

	none, err := suite.db.ListProjects(ctx, &ListProjectsQuery{})
	suite.Require().NoError(err)
	suite.Empty(none, "an empty query still lists nothing")

	_, err = suite.db.ListProjects(ctx, &ListProjectsQuery{AllScopes: true, OrganizationID: inOrg.OrganizationID})
	suite.Error(err)
}
