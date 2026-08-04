package store

import (
	"errors"
	"strings"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
)

const providerOrgMigration = "migrations/0006_move_personal_providers_to_orgs.up.sql"

func (suite *PostgresStoreTestSuite) TestMigration0006MovesOnlySafeProviderEndpoints() {
	migration, err := fs.ReadFile(providerOrgMigration)
	suite.Require().NoError(err)

	errRollback := errors.New("rollback migration fixtures")
	err = suite.db.gdb.Transaction(func(tx *gorm.DB) error {
		db := *suite.db
		db.gdb = tx
		owner := "user_" + system.GenerateUUID()
		createEndpoint := func() *types.ProviderEndpoint {
			endpoint, err := db.CreateProviderEndpoint(suite.ctx, &types.ProviderEndpoint{
				ID: "pe_" + system.GenerateUUID(), Name: "test", Owner: owner,
				OwnerType: types.OwnerTypeUser, EndpointType: types.ProviderEndpointTypeUser,
				BaseURL: "https://example.com/v1", APIKey: "key",
			})
			suite.Require().NoError(err)
			return endpoint
		}
		createApp := func(endpointID, appOwner, orgID string) {
			_, err := db.CreateApp(suite.ctx, &types.App{
				Owner: appOwner, OwnerType: types.OwnerTypeUser, OrganizationID: orgID,
				Config: types.AppConfig{Helix: types.AppHelixConfig{Assistants: []types.AssistantConfig{{
					SmallGenerationModelProvider: endpointID,
				}}}},
			})
			suite.Require().NoError(err)
		}

		safe := createEndpoint()
		createApp(safe.ID, owner, "org_safe")

		ambiguous := createEndpoint()
		createApp(ambiguous.ID, owner, "org_one")
		createApp(ambiguous.ID, owner, "org_two")

		personalRef := createEndpoint()
		createApp(personalRef.ID, owner, "org_personal_ref")
		createApp(personalRef.ID, owner, "")

		wrongAppOwner := createEndpoint()
		createApp(wrongAppOwner.ID, "other_user", "org_wrong_owner")

		unsafeSession := createEndpoint()
		createApp(unsafeSession.ID, owner, "org_session")
		_, err := db.CreateSession(suite.ctx, types.Session{
			Owner: owner, OwnerType: types.OwnerTypeUser, Provider: unsafeSession.ID,
		})
		suite.Require().NoError(err)

		crossOrgSession := createEndpoint()
		createApp(crossOrgSession.ID, owner, "org_session")
		_, err = db.CreateSession(suite.ctx, types.Session{
			Owner: owner, OwnerType: types.OwnerTypeUser, Provider: crossOrgSession.ID,
			OrganizationID: "org_other",
		})
		suite.Require().NoError(err)

		suite.Require().NoError(tx.Exec(string(migration)).Error)

		for _, tc := range []struct {
			endpoint  *types.ProviderEndpoint
			wantOwner string
			wantType  types.OwnerType
		}{
			{safe, "org_safe", types.OwnerTypeOrg},
			{ambiguous, owner, types.OwnerTypeUser},
			{personalRef, owner, types.OwnerTypeUser},
			{wrongAppOwner, owner, types.OwnerTypeUser},
			{unsafeSession, owner, types.OwnerTypeUser},
			{crossOrgSession, owner, types.OwnerTypeUser},
		} {
			got, err := db.GetProviderEndpoint(suite.ctx, &GetProviderEndpointsQuery{ID: tc.endpoint.ID})
			suite.Require().NoError(err)
			suite.Equal(tc.endpoint.ID, got.ID)
			suite.Equal(types.ProviderEndpointTypeUser, got.EndpointType)
			suite.Equal(tc.wantOwner, got.Owner)
			suite.Equal(tc.wantType, got.OwnerType)
		}
		return errRollback
	})
	suite.ErrorIs(err, errRollback)
}

func (suite *PostgresStoreTestSuite) TestMigration0006NoOpWithoutTables() {
	migration, err := fs.ReadFile(providerOrgMigration)
	suite.Require().NoError(err)

	schema := "migration_0006_" + strings.ReplaceAll(system.GenerateUUID(), "-", "")
	errRollback := errors.New("rollback test schema")
	err = suite.db.gdb.Transaction(func(tx *gorm.DB) error {
		suite.Require().NoError(tx.Exec("CREATE SCHEMA " + schema).Error)
		suite.Require().NoError(tx.Exec("SET LOCAL search_path TO " + schema).Error)
		suite.Require().NoError(tx.Exec(string(migration)).Error)
		return errRollback
	})
	suite.ErrorIs(err, errRollback)
}
