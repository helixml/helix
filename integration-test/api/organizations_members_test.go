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

func TestOrganizationsMembersTestSuite(t *testing.T) {
	suite.Run(t, new(OrganizationsMembersTestSuite))
}

type OrganizationsMembersTestSuite struct {
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
}

func (suite *OrganizationsMembersTestSuite) SetupTest() {
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

	// Create test organization
	organization, err := suite.db.CreateOrganization(suite.ctx, &types.Organization{
		Name: "test-org-" + time.Now().Format("2006-01-02-15-04-05"),
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

	// Create test user
	emailID = uuid.New().String()
	userMember2Email := fmt.Sprintf("user2-%s@test.com", emailID)
	userMember2, userMember2APIKey, err := createUser(suite.T(), suite.db, suite.keycloak, userMember2Email)
	suite.Require().NoError(err)

	suite.userMember2 = userMember2
	suite.userMember2APIKey = userMember2APIKey
}

func (suite *OrganizationsMembersTestSuite) TestManageOrganizationMembers() {
	ownerClient, err := getAPIClient(suite.userOrgOwnerAPIKey)
	suite.Require().NoError(err)

	// Add userMember1 to the organization
	_, err = ownerClient.AddOrganizationMember(suite.ctx, suite.organization.ID, &types.AddOrganizationMemberRequest{
		UserReference: suite.userMember1.ID,
		Role:          types.OrganizationRoleMember,
	})
	suite.Require().NoError(err)

	// Check memberships
	memberships, err := ownerClient.ListOrganizationMembers(suite.ctx, suite.organization.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(memberships)
	suite.Require().Equal(2, len(memberships), "should be 2 members (owner and member)")

	// Find owner ID and member ID
	var (
		ownerFound  bool
		memberFound bool
	)

	for _, membership := range memberships {
		if membership.Role == types.OrganizationRoleOwner {
			ownerFound = true
		} else {
			memberFound = true
		}
	}

	suite.Require().True(ownerFound, "owner should be found")
	suite.Require().True(memberFound, "member should be found")

	// Remove userMember1 from the organization
	err = ownerClient.RemoveOrganizationMember(suite.ctx, suite.organization.ID, suite.userMember1.ID)
	suite.Require().NoError(err)

	// Check memberships
	memberships, err = ownerClient.ListOrganizationMembers(suite.ctx, suite.organization.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(memberships)
	suite.Require().Equal(1, len(memberships), "should be 1 member (owner)")
	suite.Require().Equal(memberships[0].Role, types.OrganizationRoleOwner, "owner should be the only member")
}
