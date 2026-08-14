package store

import (
	"encoding/json"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
)

func (suite *PostgresStoreTestSuite) TestCleanupLegacyPersonalProvidersMigration() {
	t := suite.T()
	owner := "user-" + system.GenerateUUID()
	orgOne := "org-" + system.GenerateUUID()
	orgTwo := "org-" + system.GenerateUUID()
	unusedID := "pe_unused_" + system.GenerateUUID()
	movedID := "pe_moved_" + system.GenerateUUID()
	ambiguousID := "pe_ambiguous_" + system.GenerateUUID()
	personalID := "pe_personal_" + system.GenerateUUID()
	crossOrgID := "pe_cross_org_" + system.GenerateUUID()
	orphanID := "pe_orphan_" + system.GenerateUUID()
	validOrgID := "pe_org_" + system.GenerateUUID()
	validGlobalID := "pe_global_" + system.GenerateUUID()

	for _, endpoint := range []*types.ProviderEndpoint{
		{ID: unusedID, Name: "unused", Owner: owner, OwnerType: types.OwnerTypeUser, EndpointType: types.ProviderEndpointTypeUser},
		{ID: movedID, Name: "moved", Owner: owner, OwnerType: types.OwnerTypeUser, EndpointType: types.ProviderEndpointTypeUser, APIKey: "preserved-key"},
		{ID: ambiguousID, Name: "ambiguous", Owner: owner, OwnerType: types.OwnerTypeUser, EndpointType: types.ProviderEndpointTypeUser},
		{ID: personalID, Name: "personal", Owner: owner, OwnerType: types.OwnerTypeUser, EndpointType: types.ProviderEndpointTypeUser},
		{ID: crossOrgID, Name: "cross-org", Owner: owner, OwnerType: types.OwnerTypeUser, EndpointType: types.ProviderEndpointTypeUser},
		{ID: validOrgID, Name: "org", Owner: orgOne, OwnerType: types.OwnerTypeOrg, EndpointType: types.ProviderEndpointTypeOrg},
		{ID: validGlobalID, Name: "global", Owner: owner, OwnerType: types.OwnerTypeUser, EndpointType: types.ProviderEndpointTypeGlobal},
	} {
		_, err := suite.db.CreateProviderEndpoint(suite.ctx, endpoint)
		require.NoError(t, err)
	}

	appOne := suite.createMigrationTestApp(owner, orgOne, map[string]any{
		"unrelated": "preserved",
		"helix": map[string]any{"assistants": []any{
			map[string]any{"name": "moved", "provider": movedID, "model": "moved-model", "custom": map[string]any{"preserved": true}},
			map[string]any{"name": "ambiguous", "provider": ambiguousID, "model": "ambiguous-model"},
			map[string]any{"name": "cross-org", "provider": crossOrgID, "model": "cross-org-model"},
			map[string]any{"name": "valid-org", "provider": validOrgID, "model": "org-model"},
			map[string]any{"name": "valid-global", "provider": validGlobalID, "model": "global-model"},
			map[string]any{
				"name":                            "orphan",
				"provider":                        orphanID,
				"model":                           "model",
				"reasoning_model_provider":        orphanID,
				"reasoning_model":                 "reasoning",
				"generation_model_provider":       orphanID,
				"generation_model":                "generation",
				"small_reasoning_model_provider":  orphanID,
				"small_reasoning_model":           "small-reasoning",
				"small_generation_model_provider": orphanID,
				"small_generation_model":          "small-generation",
			},
		}},
	})
	appTwo := suite.createMigrationTestApp(owner, orgTwo, map[string]any{
		"helix": map[string]any{"assistants": []any{
			map[string]any{"name": "ambiguous", "provider": ambiguousID, "model": "ambiguous-model"},
		}},
	})
	personalApp := suite.createMigrationTestApp(owner, "", map[string]any{
		"helix": map[string]any{"assistants": []any{
			map[string]any{"name": "personal", "provider": personalID, "model": "personal-model"},
		}},
	})
	malformedApp := suite.createMigrationTestApp(owner, orgOne, map[string]any{
		"helix": map[string]any{"assistants": map[string]any{"not": "an array"}},
	})

	sessionID := "ses_" + system.GenerateUUID()
	_, err := suite.db.CreateSession(suite.ctx, types.Session{
		ID: sessionID, OrganizationID: orgOne, Provider: orphanID, ModelName: "orphan-model",
		Metadata: types.SessionMetadata{CodeAgentOverrides: &types.CodeAgentOverrides{
			ProviderRef: movedID, Model: "moved-model", ServiceTier: "priority",
		}},
	})
	require.NoError(t, err)
	crossOrgSessionID := "ses_" + system.GenerateUUID()
	_, err = suite.db.CreateSession(suite.ctx, types.Session{
		ID: crossOrgSessionID, OrganizationID: orgTwo, Provider: crossOrgID, ModelName: "cross-org-model",
	})
	require.NoError(t, err)

	movedTask := &types.SpecTask{
		ID: "task_" + system.GenerateUUID(), ProjectID: "project_" + system.GenerateUUID(), OrganizationID: orgOne,
		CodeAgentOverrides: &types.CodeAgentOverrides{ProviderRef: movedID, Model: "moved-model", ReasoningEffort: "high"},
	}
	orphanTask := &types.SpecTask{
		ID: "task_" + system.GenerateUUID(), ProjectID: "project_" + system.GenerateUUID(), OrganizationID: orgOne,
		CodeAgentOverrides: &types.CodeAgentOverrides{ProviderRef: orphanID, Model: "orphan-model", ReasoningEffort: "low"},
	}
	require.NoError(t, suite.db.gdb.Create(movedTask).Error)
	require.NoError(t, suite.db.gdb.Create(orphanTask).Error)

	interaction := &types.Interaction{
		ID: "int_" + system.GenerateUUID(), SessionID: sessionID,
		CodeAgentConfigSnapshot: &types.InteractionCodeAgentConfigSnapshot{Provider: orphanID, Model: "historical-model"},
	}
	require.NoError(t, suite.db.gdb.Create(interaction).Error)

	t.Cleanup(func() {
		suite.db.gdb.Delete(&types.Interaction{}, "id = ?", interaction.ID)
		suite.db.gdb.Delete(&types.Session{}, "id IN ?", []string{sessionID, crossOrgSessionID})
		suite.db.gdb.Delete(&types.SpecTask{}, "id IN ?", []string{movedTask.ID, orphanTask.ID})
		suite.db.gdb.Delete(&types.App{}, "id IN ?", []string{appOne.ID, appTwo.ID, personalApp.ID, malformedApp.ID})
		suite.db.gdb.Delete(&types.ProviderEndpoint{}, "id IN ?", []string{unusedID, movedID, ambiguousID, personalID, crossOrgID, validOrgID, validGlobalID})
	})

	migration, err := fs.ReadFile("migrations/0009_cleanup_legacy_personal_providers.up.sql")
	require.NoError(t, err)
	require.NoError(t, suite.db.gdb.Exec(string(migration)).Error)
	require.NoError(t, suite.db.gdb.Exec(string(migration)).Error)

	var count int64
	require.NoError(t, suite.db.gdb.Model(&types.ProviderEndpoint{}).Where("id = ?", unusedID).Count(&count).Error)
	require.Zero(t, count)

	moved, err := suite.db.GetProviderEndpoint(suite.ctx, &GetProviderEndpointsQuery{ID: movedID})
	require.NoError(t, err)
	require.Equal(t, orgOne, moved.Owner)
	require.Equal(t, types.OwnerTypeOrg, moved.OwnerType)
	require.Equal(t, types.ProviderEndpointTypeOrg, moved.EndpointType)
	require.Equal(t, "preserved-key", moved.APIKey)

	for _, id := range []string{ambiguousID, personalID, crossOrgID} {
		endpoint, err := suite.db.GetProviderEndpoint(suite.ctx, &GetProviderEndpointsQuery{ID: id})
		require.NoError(t, err)
		require.Equal(t, owner, endpoint.Owner)
		require.Equal(t, types.OwnerTypeUser, endpoint.OwnerType)
		require.Equal(t, types.ProviderEndpointTypeUser, endpoint.EndpointType)
	}
	for _, id := range []string{validOrgID, validGlobalID} {
		require.NoError(t, suite.db.gdb.Model(&types.ProviderEndpoint{}).Where("id = ?", id).Count(&count).Error)
		require.EqualValues(t, 1, count)
	}

	config := suite.getMigrationTestAppConfig(appOne.ID)
	require.Equal(t, "preserved", config["unrelated"])
	assistants := config["helix"].(map[string]any)["assistants"].([]any)
	require.Equal(t, movedID, assistants[0].(map[string]any)["provider"])
	require.Equal(t, true, assistants[0].(map[string]any)["custom"].(map[string]any)["preserved"])
	require.Equal(t, ambiguousID, assistants[1].(map[string]any)["provider"])
	require.Equal(t, crossOrgID, assistants[2].(map[string]any)["provider"])
	require.Equal(t, validOrgID, assistants[3].(map[string]any)["provider"])
	require.Equal(t, validGlobalID, assistants[4].(map[string]any)["provider"])
	require.Equal(t, orphanID, assistants[5].(map[string]any)["provider"])
	require.Equal(t, "model", assistants[5].(map[string]any)["model"])
	require.Equal(t, orphanID, assistants[5].(map[string]any)["reasoning_model_provider"])
	require.Equal(t, orphanID, assistants[5].(map[string]any)["generation_model_provider"])
	require.Equal(t, orphanID, assistants[5].(map[string]any)["small_reasoning_model_provider"])
	require.Equal(t, orphanID, assistants[5].(map[string]any)["small_generation_model_provider"])

	storedSession, err := suite.db.GetSession(suite.ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, orphanID, storedSession.Provider)
	require.Equal(t, "orphan-model", storedSession.ModelName)
	require.Equal(t, movedID, storedSession.Metadata.CodeAgentOverrides.ProviderRef)
	require.Equal(t, "moved-model", storedSession.Metadata.CodeAgentOverrides.Model)
	require.Equal(t, "priority", storedSession.Metadata.CodeAgentOverrides.ServiceTier)

	var storedMovedTask, storedOrphanTask types.SpecTask
	require.NoError(t, suite.db.gdb.First(&storedMovedTask, "id = ?", movedTask.ID).Error)
	require.Equal(t, movedID, storedMovedTask.CodeAgentOverrides.ProviderRef)
	require.Equal(t, "moved-model", storedMovedTask.CodeAgentOverrides.Model)
	require.NoError(t, suite.db.gdb.First(&storedOrphanTask, "id = ?", orphanTask.ID).Error)
	require.Equal(t, orphanID, storedOrphanTask.CodeAgentOverrides.ProviderRef)
	require.Equal(t, "orphan-model", storedOrphanTask.CodeAgentOverrides.Model)
	require.Equal(t, "low", storedOrphanTask.CodeAgentOverrides.ReasoningEffort)

	var storedInteraction types.Interaction
	require.NoError(t, suite.db.gdb.First(&storedInteraction, "id = ?", interaction.ID).Error)
	require.Equal(t, orphanID, storedInteraction.CodeAgentConfigSnapshot.Provider)
	require.Equal(t, "historical-model", storedInteraction.CodeAgentConfigSnapshot.Model)
}

func (suite *PostgresStoreTestSuite) createMigrationTestApp(owner, organizationID string, config map[string]any) *types.App {
	app, err := suite.db.CreateApp(suite.ctx, &types.App{
		Owner: owner, OwnerType: types.OwnerTypeUser, OrganizationID: organizationID, Config: types.AppConfig{},
	})
	suite.Require().NoError(err)
	encoded, err := json.Marshal(config)
	suite.Require().NoError(err)
	suite.Require().NoError(suite.db.gdb.Exec("UPDATE apps SET config = ?::jsonb WHERE id = ?", string(encoded), app.ID).Error)
	return app
}

func (suite *PostgresStoreTestSuite) getMigrationTestAppConfig(id string) map[string]any {
	var stored string
	suite.Require().NoError(suite.db.gdb.Raw("SELECT config FROM apps WHERE id = ?", id).Scan(&stored).Error)
	var config map[string]any
	suite.Require().NoError(json.Unmarshal([]byte(stored), &config))
	return config
}
