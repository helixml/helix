package api

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/kelseyhightower/envconfig"
	"github.com/stretchr/testify/suite"
)

func TestOrganizationsRBACTestSuite(t *testing.T) {
	suite.Run(t, new(OrganizationsRBACTestSuite))
}

type OrganizationsRBACTestSuite struct {
	suite.Suite
	ctx      context.Context
	db       *store.PostgresStore
	keycloak *auth.KeycloakAuthenticator

	userOrgOwner       *types.User // Who created the organization
	userOrgOwnerAPIKey string

	organization *types.Organization

	userMember1       *types.User // Will be used to invite to the organization
	userMember1APIKey string

	userMember2       *types.User // Will be used to invite to the organization
	userMember2APIKey string

	userMember3       *types.User // Will be used to invite to the organization
	userMember3APIKey string
	userMember3Team   *types.Team

	userNonMember       *types.User // Will not be in an organization
	userNonMemberAPIKey string
}

func (suite *OrganizationsRBACTestSuite) SetupTest() {
	suite.ctx = context.Background()
	store, err := getStoreClient()
	suite.Require().NoError(err)
	suite.db = store

	var keycloakCfg config.Keycloak

	err = envconfig.Process("", &keycloakCfg)
	suite.NoError(err)

	keycloakAuthenticator, err := auth.NewKeycloakAuthenticator(&config.Keycloak{
		KeycloakURL:         keycloakCfg.KeycloakURL,
		KeycloakFrontEndURL: keycloakCfg.KeycloakFrontEndURL,
		ServerURL:           keycloakCfg.ServerURL,
		APIClientID:         keycloakCfg.APIClientID,
		FrontEndClientID:    keycloakCfg.FrontEndClientID,
		AdminRealm:          keycloakCfg.AdminRealm,
		Realm:               keycloakCfg.Realm,
		Username:            keycloakCfg.Username,
		Password:            keycloakCfg.Password,
	}, suite.db)
	suite.Require().NoError(err)

	suite.keycloak = keycloakAuthenticator

	// Create test user
	emailID := uuid.New().String()
	userOrgOwnerEmail := fmt.Sprintf("org-owner-%s@test.com", emailID)
	userOrgOwner, userOrgOwnerAPIKey, err := createUser(suite.T(), suite.db, suite.keycloak, userOrgOwnerEmail)
	suite.Require().NoError(err)

	suite.userOrgOwner = userOrgOwner
	suite.userOrgOwnerAPIKey = userOrgOwnerAPIKey

	ownerClient, err := getAPIClient(suite.userOrgOwnerAPIKey)
	suite.Require().NoError(err)

	// Create test organization
	organization, err := ownerClient.CreateOrganization(suite.ctx, &types.Organization{
		Name: "test-rbac-" + time.Now().Format("2006-01-02-15-04-05"),
	})
	suite.Require().NoError(err)
	suite.Require().NotNil(organization)
	suite.organization = organization

	// Create test user
	emailID = uuid.New().String()
	userMember1Email := fmt.Sprintf("user1-%s@test.com", emailID)
	userMember1, userMember1APIKey, err := createUser(suite.T(), suite.db, suite.keycloak, userMember1Email)
	suite.Require().NoError(err)

	suite.userMember1 = userMember1
	suite.userMember1APIKey = userMember1APIKey

	// Add userMember1 to the organization
	_, err = ownerClient.AddOrganizationMember(suite.ctx, suite.organization.ID, &types.AddOrganizationMemberRequest{
		UserReference: suite.userMember1.ID,
		Role:          types.OrganizationRoleMember,
	})
	suite.Require().NoError(err)

	// Create test user
	emailID = uuid.New().String()
	userMember2Email := fmt.Sprintf("user2-%s@test.com", emailID)
	userMember2, userMember2APIKey, err := createUser(suite.T(), suite.db, suite.keycloak, userMember2Email)
	suite.Require().NoError(err)

	suite.userMember2 = userMember2
	suite.userMember2APIKey = userMember2APIKey

	// Add userMember2 to the organization
	_, err = ownerClient.AddOrganizationMember(suite.ctx, suite.organization.ID, &types.AddOrganizationMemberRequest{
		UserReference: suite.userMember2.ID,
		Role:          types.OrganizationRoleMember,
	})
	suite.Require().NoError(err)

	// Create test user 3
	emailID = uuid.New().String()
	userMember3Email := fmt.Sprintf("user3-%s@test.com", emailID)
	userMember3, userMember3APIKey, err := createUser(suite.T(), suite.db, suite.keycloak, userMember3Email)
	suite.Require().NoError(err)

	suite.userMember3 = userMember3
	suite.userMember3APIKey = userMember3APIKey

	// Add userMember3 to the organization
	_, err = ownerClient.AddOrganizationMember(suite.ctx, suite.organization.ID, &types.AddOrganizationMemberRequest{
		UserReference: suite.userMember3.ID,
		Role:          types.OrganizationRoleMember,
	})
	suite.Require().NoError(err)

	// Create a team for user
	team, err := ownerClient.CreateTeam(suite.ctx, suite.organization.ID, &types.CreateTeamRequest{
		Name: "test-team",
	})
	suite.Require().NoError(err)
	suite.Require().NotNil(team)
	suite.userMember3Team = team

	// Add userMember3 to the team
	_, err = ownerClient.AddTeamMember(suite.ctx, suite.organization.ID, suite.userMember3Team.ID, &types.AddTeamMemberRequest{
		UserReference: suite.userMember3.ID,
	})
	suite.Require().NoError(err)

	// Create non member user
	emailID = uuid.New().String()
	userNonMemberEmail := fmt.Sprintf("non-member-%s@test.com", emailID)
	userNonMember, userNonMemberAPIKey, err := createUser(suite.T(), suite.db, suite.keycloak, userNonMemberEmail)
	suite.Require().NoError(err)

	suite.userNonMember = userNonMember
	suite.userNonMemberAPIKey = userNonMemberAPIKey
}

// TestAppAccessControls - tests various RBAC controls for apps
// 1. Creates the app as userMember1
// 2. Checks that only userMember1 and admin can see the app
// 3. Checks that userMember2 cannot see the app
// 4. Checks that userNonMember cannot see the app
// 5. Checks that userMember3 can see the app
// 6. Checks that userMember3 can see the app in the team
// 7. Checks that userNonMember cannot see the app in the team

func (suite *OrganizationsRBACTestSuite) TestAppVisibilityWithoutGrantingAccess() {
	// Create the app as userMember1
	userMember1Client, err := getAPIClient(suite.userMember1APIKey)
	suite.Require().NoError(err)

	app, err := userMember1Client.CreateApp(suite.ctx, &types.App{
		OrganizationID: suite.organization.ID,
		AppSource:      types.AppSourceHelix,
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Name:        "test-app-1",
				Description: "test-app-1-description",
				Assistants: []types.AssistantConfig{
					{
						Name:  "test-assistant-1",
						Model: "meta-llama/Llama-3-8b-chat-hf",
					},
				},
			},
		},
	})
	suite.Require().NoError(err)
	suite.Require().NotNil(app)

}
