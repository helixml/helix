package api

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/client"
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
		Name: "test-rbac-" + time.Now().Format("2006-01-02-15-04-05-06"),
	})
	suite.Require().NoError(err)
	suite.Require().NotNil(organization)
	suite.organization = organization

	suite.T().Cleanup(func() {
		err := ownerClient.DeleteOrganization(suite.ctx, suite.organization.ID)
		suite.Require().NoError(err)
	})

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
// 7. Checks that userNonMember cannot see the app in the organization

func (suite *OrganizationsRBACTestSuite) TestAppVisibilityWithoutGrantingAccess() {
	// Create the app as userMember1
	userMember1Client, err := getAPIClient(suite.userMember1APIKey)
	suite.Require().NoError(err)

	app, err := userMember1Client.CreateApp(suite.ctx, &types.App{
		OrganizationID: suite.organization.ID,
		AppSource:      types.AppSourceHelix,
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Name:        "TestAppVisibilityWithoutGrantingAccess",
				Description: "TestAppVisibilityWithoutGrantingAccess-description",
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

	// Org owner should see the app
	orgOwnerClient, err := getAPIClient(suite.userOrgOwnerAPIKey)
	suite.Require().NoError(err)

	suite.True(assertAppVisibility(suite, orgOwnerClient, suite.organization.ID, app.ID), "org owner should see the app")

	// userMember1 should see the app (he created the app)
	suite.True(assertAppVisibility(suite, userMember1Client, suite.organization.ID, app.ID), "userMember1 should see the app (creator)")

	// userMember2 should not see the app (access not granted)
	userMember2Client, err := getAPIClient(suite.userMember2APIKey)
	suite.Require().NoError(err)
	suite.False(assertAppVisibility(suite, userMember2Client, suite.organization.ID, app.ID), "userMember2 should not see the app (access not granted)")

	// userMember3 should not see the app (access not granted)
	userMember3Client, err := getAPIClient(suite.userMember3APIKey)
	suite.Require().NoError(err)
	suite.False(assertAppVisibility(suite, userMember3Client, suite.organization.ID, app.ID), "userMember3 should not see the app (access not granted)")

	// userNonMember should not see the app (not in the organization, no way to grant access)
	userNonMemberClient, err := getAPIClient(suite.userNonMemberAPIKey)
	suite.Require().NoError(err)
	_, err = userNonMemberClient.ListApps(context.Background(), &client.AppFilter{
		OrganizationID: suite.organization.ID,
	})
	suite.Require().Error(err)

	// Shouldn't see without the organization ID too
	suite.False(assertAppVisibility(suite, userNonMemberClient, "", app.ID), "userNonMemberClient should not see the app (access not granted)")
}

func (suite *OrganizationsRBACTestSuite) TestAppVisibility_GrantedAccessToSingleUser() {
	userMember1Client, err := getAPIClient(suite.userMember1APIKey)
	suite.Require().NoError(err)

	app, err := userMember1Client.CreateApp(suite.ctx, &types.App{
		OrganizationID: suite.organization.ID,
		AppSource:      types.AppSourceHelix,
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Name:        "test-app-single-user-access",
				Description: "test-app-single-user-access-description",
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

	// Grant access to userMember2
	_, err = userMember1Client.CreateAppAccessGrant(suite.ctx, app.ID, &types.CreateAccessGrantRequest{
		UserReference: suite.userMember2.ID,
		Roles:         []string{"read"},
	})
	suite.Require().NoError(err)

	/*
		VALIDATE APP ACCESS
	*/

	// Org owner should see the app
	orgOwnerClient, err := getAPIClient(suite.userOrgOwnerAPIKey)
	suite.Require().NoError(err)

	suite.True(assertAppVisibility(suite, orgOwnerClient, suite.organization.ID, app.ID), "org owner should see the app")

	// userMember1 should see the app (he created the app)
	suite.True(assertAppVisibility(suite, userMember1Client, suite.organization.ID, app.ID), "userMember1 should see the app (creator)")

	// userMember2 should see the app (access granted)
	userMember2Client, err := getAPIClient(suite.userMember2APIKey)
	suite.Require().NoError(err)
	suite.True(assertAppVisibility(suite, userMember2Client, suite.organization.ID, app.ID), "userMember2 should see the app (access granted)")

	// userMember3 should not see the app (access not granted)
	userMember3Client, err := getAPIClient(suite.userMember3APIKey)
	suite.Require().NoError(err)
	suite.False(assertAppVisibility(suite, userMember3Client, suite.organization.ID, app.ID), "userMember3 should not see the app (access not granted)")

	// userNonMember should not see the app (not in the organization, no way to grant access)
	userNonMemberClient, err := getAPIClient(suite.userNonMemberAPIKey)
	suite.Require().NoError(err)
	_, err = userNonMemberClient.ListApps(context.Background(), &client.AppFilter{
		OrganizationID: suite.organization.ID,
	})
	suite.Require().Error(err)
}

func assertAppVisibility(suite *OrganizationsRBACTestSuite, userClient *client.HelixClient, orgID, appID string) bool {
	suite.T().Helper()

	var found bool

	apps, err := userClient.ListApps(context.Background(), &client.AppFilter{
		OrganizationID: orgID,
	})
	suite.Require().NoError(err)

	for _, app := range apps {
		if app.ID == appID {
			found = true
			break
		}
	}

	return found
}
