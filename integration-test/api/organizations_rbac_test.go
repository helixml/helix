package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
)

func TestOrganizationsRBACTestSuite(t *testing.T) {
	suite.Run(t, new(OrganizationsRBACTestSuite))
}

type OrganizationsRBACTestSuite struct {
	suite.Suite
	ctx           context.Context
	db            *store.PostgresStore
	authenticator auth.Authenticator

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

	cfg := &config.ServerConfig{}
	authenticator, err := auth.NewHelixAuthenticator(cfg, suite.db, "test-secret", nil)
	suite.Require().NoError(err)

	suite.authenticator = authenticator

	// Create test user
	emailID := uuid.New().String()
	userOrgOwnerEmail := fmt.Sprintf("org-owner-%s@test.com", emailID)
	userOrgOwner, userOrgOwnerAPIKey, err := createUser(suite.T(), suite.db, suite.authenticator, userOrgOwnerEmail)
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
	userMember1, userMember1APIKey, err := createUser(suite.T(), suite.db, suite.authenticator, userMember1Email)
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
	userMember2, userMember2APIKey, err := createUser(suite.T(), suite.db, suite.authenticator, userMember2Email)
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
	userMember3, userMember3APIKey, err := createUser(suite.T(), suite.db, suite.authenticator, userMember3Email)
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
	userNonMember, userNonMemberAPIKey, err := createUser(suite.T(), suite.db, suite.authenticator, userNonMemberEmail)
	suite.Require().NoError(err)

	suite.userNonMember = userNonMember
	suite.userNonMemberAPIKey = userNonMemberAPIKey
}

func (suite *OrganizationsRBACTestSuite) TestOrganizationMemberAndTeamRoutesAcceptOrgSlug() {
	ownerClient, err := getAPIClient(suite.userOrgOwnerAPIKey)
	suite.Require().NoError(err)

	resp, err := ownerClient.AddOrganizationMember(suite.ctx, suite.organization.Name, &types.AddOrganizationMemberRequest{
		UserReference: suite.userNonMember.ID,
		Role:          types.OrganizationRoleMember,
	})
	suite.Require().NoError(err)
	suite.Require().NotNil(resp.Membership, "existing user should produce a membership, not an invitation")
	suite.Require().Equal(suite.organization.ID, resp.Membership.OrganizationID)

	team, err := ownerClient.CreateTeam(suite.ctx, suite.organization.Name, &types.CreateTeamRequest{
		Name: "slug-route-team-" + uuid.New().String(),
	})
	suite.Require().NoError(err)
	suite.Require().Equal(suite.organization.ID, team.OrganizationID)

	teamMembership, err := ownerClient.AddTeamMember(suite.ctx, suite.organization.Name, team.ID, &types.AddTeamMemberRequest{
		UserReference: suite.userNonMember.ID,
	})
	suite.Require().NoError(err)
	suite.Require().Equal(suite.organization.ID, teamMembership.OrganizationID)

	nonMemberClient, err := getAPIClient(suite.userNonMemberAPIKey)
	suite.Require().NoError(err)
	teams, err := nonMemberClient.ListTeams(suite.ctx, suite.organization.Name)
	suite.Require().NoError(err)
	suite.Require().True(containsTeam(teams, team.ID), "new org member should be able to list teams via org slug")
}

func (suite *OrganizationsRBACTestSuite) TestProjectVisibilityAndRepositoryAccess() {
	ownerClient, err := getAPIClient(suite.userOrgOwnerAPIKey)
	suite.Require().NoError(err)

	defaultApp, err := ownerClient.CreateApp(suite.ctx, &types.App{
		OrganizationID: suite.organization.ID,
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Name:        "project-rbac-agent-" + uuid.New().String(),
				Description: "project rbac test agent",
				Assistants: []types.AssistantConfig{{
					Name:  "assistant",
					Model: "openai/gpt-oss-20b",
				}},
			},
		},
	})
	suite.Require().NoError(err)

	projectRepo := suite.createTestRepository("project-repo", suite.userOrgOwner.ID)
	directReadRepo := suite.createTestRepository("direct-read-repo", suite.userOrgOwner.ID)
	directWriteRepo := suite.createTestRepository("direct-write-repo", suite.userOrgOwner.ID)

	var project types.Project
	status, body := apiJSON(suite.T(), suite.userOrgOwnerAPIKey, http.MethodPost, "/projects", &types.ProjectCreateRequest{
		OrganizationID:    suite.organization.Name,
		Name:              "project-rbac-" + uuid.New().String(),
		Description:       "private project",
		DefaultRepoID:     projectRepo.ID,
		DefaultHelixAppID: defaultApp.ID,
	}, &project)
	suite.Require().Equal(http.StatusOK, status, body)
	suite.Require().Equal(suite.organization.ID, project.OrganizationID)

	memberReadKey := suite.userMember2APIKey
	memberWriteKey := suite.userMember1APIKey
	memberNoGrantKey := suite.userMember3APIKey

	suite.assertProjectVisible(memberReadKey, project.ID, false)
	suite.assertProjectVisible(memberNoGrantKey, project.ID, false)

	status, _ = apiJSON(suite.T(), memberNoGrantKey, http.MethodGet, "/spec-tasks?project_id="+url.QueryEscape(project.ID), nil, nil)
	suite.Require().Equal(http.StatusForbidden, status)

	status, _ = apiJSON(suite.T(), memberNoGrantKey, http.MethodGet, "/projects/"+project.ID+"/tasks-progress", nil, nil)
	suite.Require().Equal(http.StatusForbidden, status)

	status, _ = apiJSON(suite.T(), memberNoGrantKey, http.MethodGet, "/projects/"+project.ID+"/tasks-usage", nil, nil)
	suite.Require().Equal(http.StatusForbidden, status)

	status, _ = apiJSON(suite.T(), memberNoGrantKey, http.MethodGet, "/projects/"+project.ID+"/labels", nil, nil)
	suite.Require().Equal(http.StatusForbidden, status)

	status, _ = apiJSON(suite.T(), suite.userNonMemberAPIKey, http.MethodGet, "/projects?organization_id="+url.QueryEscape(suite.organization.ID), nil, &[]*types.Project{})
	suite.Require().Equal(http.StatusForbidden, status)

	var grantResp types.CreateAccessGrantResponse
	status, body = apiJSON(suite.T(), suite.userOrgOwnerAPIKey, http.MethodPost, fmt.Sprintf("/projects/%s/access-grants", project.ID), &types.CreateAccessGrantRequest{
		UserReference: suite.userMember2.ID,
		Roles:         []string{"read"},
	}, &grantResp)
	suite.Require().Equal(http.StatusOK, status, body)
	suite.False(grantResp.AddedToOrganization)

	status, body = apiJSON(suite.T(), suite.userOrgOwnerAPIKey, http.MethodPost, fmt.Sprintf("/projects/%s/access-grants", project.ID), &types.CreateAccessGrantRequest{
		UserReference: suite.userMember1.ID,
		Roles:         []string{"write"},
	}, &grantResp)
	suite.Require().Equal(http.StatusOK, status, body)

	suite.assertProjectVisible(memberReadKey, project.ID, true)
	suite.assertProjectVisible(memberWriteKey, project.ID, true)
	suite.assertProjectVisible(memberNoGrantKey, project.ID, false)

	newDescription := "read user should not update"
	status, _ = apiJSON(suite.T(), memberReadKey, http.MethodPut, "/projects/"+project.ID, &types.ProjectUpdateRequest{
		Description: &newDescription,
	}, &types.Project{})
	suite.Require().Equal(http.StatusForbidden, status)

	newDescription = "write user can update"
	status, body = apiJSON(suite.T(), memberWriteKey, http.MethodPut, "/projects/"+project.ID, &types.ProjectUpdateRequest{
		Description: &newDescription,
	}, &types.Project{})
	suite.Require().Equal(http.StatusOK, status, body)

	var projectRepos []*types.GitRepository
	status, body = apiJSON(suite.T(), memberReadKey, http.MethodGet, "/projects/"+project.ID+"/repositories", nil, &projectRepos)
	suite.Require().Equal(http.StatusOK, status, body)
	suite.Require().True(containsRepository(projectRepos, projectRepo.ID), "project read grant should expose attached repositories")

	var repo types.GitRepository
	status, body = apiJSON(suite.T(), memberReadKey, http.MethodGet, "/git/repositories/"+projectRepo.ID, nil, &repo)
	suite.Require().Equal(http.StatusOK, status, body)
	suite.Require().Equal(projectRepo.ID, repo.ID)

	status, _ = apiJSON(suite.T(), memberReadKey, http.MethodPut, "/git/repositories/"+projectRepo.ID, &types.GitRepositoryUpdateRequest{
		Description: "project read user should not update inherited repo",
	}, &types.GitRepository{})
	suite.Require().Equal(http.StatusForbidden, status)

	status, body = apiJSON(suite.T(), memberWriteKey, http.MethodPut, "/git/repositories/"+projectRepo.ID, &types.GitRepositoryUpdateRequest{
		Description: "project write user can update inherited repo",
	}, &repo)
	suite.Require().Equal(http.StatusOK, status, body)
	suite.Require().Equal("project write user can update inherited repo", repo.Description)

	status, _ = apiJSON(suite.T(), memberNoGrantKey, http.MethodGet, "/git/repositories/"+projectRepo.ID, nil, &types.GitRepository{})
	suite.Require().Equal(http.StatusForbidden, status)

	var repos []*types.GitRepository
	status, body = apiJSON(suite.T(), memberReadKey, http.MethodGet, "/git/repositories?organization_id="+url.QueryEscape(suite.organization.ID), nil, &repos)
	suite.Require().Equal(http.StatusOK, status, body)
	suite.Require().True(containsRepository(repos, projectRepo.ID), "org repo list should include project-inherited repo")
	suite.Require().False(containsRepository(repos, directReadRepo.ID), "org repo list must hide repos without direct or project access")

	status, _ = apiJSON(suite.T(), memberReadKey, http.MethodGet, "/git/repositories/"+directReadRepo.ID, nil, &types.GitRepository{})
	suite.Require().Equal(http.StatusForbidden, status)

	var directGrant types.AccessGrant
	status, body = apiJSON(suite.T(), suite.userOrgOwnerAPIKey, http.MethodPost, fmt.Sprintf("/git/repositories/%s/access-grants", directReadRepo.ID), &types.CreateAccessGrantRequest{
		UserReference: suite.userMember2.ID,
		Roles:         []string{"read"},
	}, &directGrant)
	suite.Require().Equal(http.StatusOK, status, body)

	status, body = apiJSON(suite.T(), memberReadKey, http.MethodGet, "/git/repositories/"+directReadRepo.ID, nil, &repo)
	suite.Require().Equal(http.StatusOK, status, body)
	suite.Require().Equal(directReadRepo.ID, repo.ID)

	status, _ = apiJSON(suite.T(), memberReadKey, http.MethodPut, "/git/repositories/"+directReadRepo.ID, &types.GitRepositoryUpdateRequest{
		Description: "read user should not update direct repo",
	}, &types.GitRepository{})
	suite.Require().Equal(http.StatusForbidden, status)

	status, body = apiJSON(suite.T(), suite.userOrgOwnerAPIKey, http.MethodPost, fmt.Sprintf("/git/repositories/%s/access-grants", directWriteRepo.ID), &types.CreateAccessGrantRequest{
		UserReference: suite.userMember1.ID,
		Roles:         []string{"write"},
	}, &directGrant)
	suite.Require().Equal(http.StatusOK, status, body)

	status, body = apiJSON(suite.T(), memberWriteKey, http.MethodPut, "/git/repositories/"+directWriteRepo.ID, &types.GitRepositoryUpdateRequest{
		Description: "write user can update direct repo",
	}, &repo)
	suite.Require().Equal(http.StatusOK, status, body)
	suite.Require().Equal("write user can update direct repo", repo.Description)
}

func (suite *OrganizationsRBACTestSuite) TestNonMemberCannotCreateApp() {
	// Create the app as userMember1
	userNonMemberClient, err := getAPIClient(suite.userNonMemberAPIKey)
	suite.Require().NoError(err)

	app, err := userNonMemberClient.CreateApp(suite.ctx, &types.App{
		OrganizationID: suite.organization.ID,
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Name:        "TestAppVisibilityWithoutGrantingAccess",
				Description: "TestAppVisibilityWithoutGrantingAccess-description",
				Assistants: []types.AssistantConfig{
					{
						Name:  "test-assistant-1",
						Model: "openai/gpt-oss-20b",
					},
				},
			},
		},
	})
	suite.Require().Error(err)
	suite.Nil(app)
}

func (suite *OrganizationsRBACTestSuite) createTestRepository(namePrefix, ownerID string) *types.GitRepository {
	suite.T().Helper()

	var repo types.GitRepository
	status, body := apiJSON(suite.T(), suite.userOrgOwnerAPIKey, http.MethodPost, "/git/repositories", &types.GitRepositoryCreateRequest{
		Name:           namePrefix + "-" + uuid.New().String(),
		Description:    "rbac integration test repository",
		RepoType:       types.GitRepositoryTypeCode,
		OwnerID:        ownerID,
		OrganizationID: suite.organization.ID,
		DefaultBranch:  "main",
		InitialFiles: map[string]string{
			"README.md": "# RBAC integration test repository\n",
		},
		Metadata: map[string]interface{}{},
	}, &repo)
	suite.Require().Equal(http.StatusCreated, status, body)
	return &repo
}

func (suite *OrganizationsRBACTestSuite) assertProjectVisible(apiKey, projectID string, visible bool) {
	suite.T().Helper()

	var projects []*types.Project
	status, body := apiJSON(suite.T(), apiKey, http.MethodGet, "/projects?organization_id="+url.QueryEscape(suite.organization.ID), nil, &projects)
	suite.Require().Equal(http.StatusOK, status, body)
	suite.Require().Equal(visible, containsProject(projects, projectID))

	var project types.Project
	status, _ = apiJSON(suite.T(), apiKey, http.MethodGet, "/projects/"+projectID, nil, &project)
	if visible {
		suite.Require().Equal(http.StatusOK, status)
		suite.Require().Equal(projectID, project.ID)
	} else {
		suite.Require().Equal(http.StatusForbidden, status)
	}
}

func apiJSON(t *testing.T, apiKey, method, path string, body any, out any) (int, string) {
	t.Helper()

	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
		reader = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(context.Background(), method, "http://localhost:8080/api/v1"+path, reader)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("perform request: %v", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 && out != nil && len(responseBody) > 0 {
		if err := json.Unmarshal(responseBody, out); err != nil {
			t.Fatalf("unmarshal response body %q: %v", string(responseBody), err)
		}
	}

	return resp.StatusCode, string(responseBody)
}

func containsProject(projects []*types.Project, id string) bool {
	for _, project := range projects {
		if project.ID == id {
			return true
		}
	}
	return false
}

func containsRepository(repos []*types.GitRepository, id string) bool {
	for _, repo := range repos {
		if repo.ID == id {
			return true
		}
	}
	return false
}

func containsTeam(teams []*types.Team, id string) bool {
	for _, team := range teams {
		if team.ID == id {
			return true
		}
	}
	return false
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
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Name:        "TestAppVisibilityWithoutGrantingAccess",
				Description: "TestAppVisibilityWithoutGrantingAccess-description",
				Assistants: []types.AssistantConfig{
					{
						Name:  "test-assistant-1",
						Model: "openai/gpt-oss-20b",
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
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Name:        "test-app-single-user-access",
				Description: "test-app-single-user-access-description",
				Assistants: []types.AssistantConfig{
					{
						Name:  "test-assistant-1",
						Model: "openai/gpt-oss-20b",
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

func (suite *OrganizationsRBACTestSuite) TestAppVisibility_GrantedAccessToTeam() {
	userMember1Client, err := getAPIClient(suite.userMember1APIKey)
	suite.Require().NoError(err)

	app, err := userMember1Client.CreateApp(suite.ctx, &types.App{
		OrganizationID: suite.organization.ID,
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Name:        "test-app-single-user-access",
				Description: "test-app-single-user-access-description",
				Assistants: []types.AssistantConfig{
					{
						Name:  "test-assistant-1",
						Model: "openai/gpt-oss-20b",
					},
				},
			},
		},
	})
	suite.Require().NoError(err)
	suite.Require().NotNil(app)

	// Grant access to userMember2
	_, err = userMember1Client.CreateAppAccessGrant(suite.ctx, app.ID, &types.CreateAccessGrantRequest{
		TeamID: suite.userMember3Team.ID,
		Roles:  []string{"read"},
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

	// userMember2 should not see the app (access not granted)
	userMember2Client, err := getAPIClient(suite.userMember2APIKey)
	suite.Require().NoError(err)
	suite.False(assertAppVisibility(suite, userMember2Client, suite.organization.ID, app.ID), "userMember2 should not see the app (access not granted)")

	// userMember3 should see the app (access granted)
	userMember3Client, err := getAPIClient(suite.userMember3APIKey)
	suite.Require().NoError(err)
	suite.True(assertAppVisibility(suite, userMember3Client, suite.organization.ID, app.ID), "userMember3 should see the app (access granted)")

	// userNonMember should not see the app (not in the organization, no way to grant access)
	userNonMemberClient, err := getAPIClient(suite.userNonMemberAPIKey)
	suite.Require().NoError(err)
	_, err = userNonMemberClient.ListApps(context.Background(), &client.AppFilter{
		OrganizationID: suite.organization.ID,
	})
	suite.Require().Error(err)
}

func (suite *OrganizationsRBACTestSuite) TestAppUpdate_AppOwner_OrgOwner() {
	userMember1Client, err := getAPIClient(suite.userMember1APIKey)
	suite.Require().NoError(err)

	app, err := userMember1Client.CreateApp(suite.ctx, &types.App{
		OrganizationID: suite.organization.ID,
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Name:        "test-app-single-user-access",
				Description: "test-app-single-user-access-description",
				Assistants: []types.AssistantConfig{
					{
						Name:  "test-assistant-1",
						Model: "openai/gpt-oss-20b",
					},
				},
			},
		},
	})
	suite.Require().NoError(err)
	suite.Require().NotNil(app)

	// App creator should be able to update the app
	suite.NoError(assertAppWriteAccess(userMember1Client, app.ID), "userMember1 should be able to update the app")

	// Org owner should be able to update the app
	orgOwnerClient, err := getAPIClient(suite.userOrgOwnerAPIKey)
	suite.Require().NoError(err)
	suite.NoError(assertAppWriteAccess(orgOwnerClient, app.ID), "orgOwner should be able to update the app")

	// Other member should not be able to
	userMember2Client, err := getAPIClient(suite.userMember2APIKey)
	suite.Require().NoError(err)
	suite.Error(assertAppWriteAccess(userMember2Client, app.ID), "userMember2 should not be able to update the app before access is granted")
}

func (suite *OrganizationsRBACTestSuite) TestAppUpdate_SingleUser() {
	userMember1Client, err := getAPIClient(suite.userMember1APIKey)
	suite.Require().NoError(err)

	app, err := userMember1Client.CreateApp(suite.ctx, &types.App{
		OrganizationID: suite.organization.ID,
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Name:        "test-app-single-user-access",
				Description: "test-app-single-user-access-description",
				Assistants: []types.AssistantConfig{
					{
						Name:  "test-assistant-1",
						Model: "openai/gpt-oss-20b",
					},
				},
			},
		},
	})
	suite.Require().NoError(err)
	suite.Require().NotNil(app)

	// Should not be able to
	userMember2Client, err := getAPIClient(suite.userMember2APIKey)
	suite.Require().NoError(err)
	suite.Error(assertAppWriteAccess(userMember2Client, app.ID), "userMember2 should not be able to update the app before access is granted")

	// Grant access to userMember2
	_, err = userMember1Client.CreateAppAccessGrant(suite.ctx, app.ID, &types.CreateAccessGrantRequest{
		UserReference: suite.userMember2.ID,
		Roles:         []string{"write"},
	})
	suite.Require().NoError(err)

	/*
		VALIDATE APP WRITE ACCESS
	*/

	// userMember2 should be able to update the app
	suite.NoError(assertAppWriteAccess(userMember2Client, app.ID), "userMember2 should be able to update the app")

	// userMember3 should not be able to update the app
	userMember3Client, err := getAPIClient(suite.userMember3APIKey)
	suite.Require().NoError(err)
	suite.Error(assertAppWriteAccess(userMember3Client, app.ID), "userMember3 should not be able to update the app")

	suite.T().Run("UserAccessEndpoint_Owner", func(_ *testing.T) {
		userMember1Client, err := getAPIClient(suite.userMember1APIKey)
		suite.Require().NoError(err)

		access, err := userMember1Client.GetAppUserAccess(suite.ctx, app.ID)
		suite.Require().NoError(err)

		suite.True(access.CanWrite, "userMember1 should be able to write to the app")
		suite.True(access.CanRead, "userMember1 should be able to read from the app")
		suite.True(access.IsAdmin, "userMember1 should be an admin of the app")
	})

	suite.T().Run("UserAccessEndpoint_NotOwner_ButWithAccess", func(_ *testing.T) {
		userMember2Client, err := getAPIClient(suite.userMember2APIKey)
		suite.Require().NoError(err)

		access, err := userMember2Client.GetAppUserAccess(suite.ctx, app.ID)
		suite.Require().NoError(err)

		suite.True(access.CanWrite, "userMember2 should be able to write to the app")
		suite.True(access.CanRead, "userMember2 should be able to read from the app")
		suite.False(access.IsAdmin, "userMember2 should not be an admin of the app")
	})

	suite.T().Run("UserAccessEndpoint_NotOwner_NoAccess", func(_ *testing.T) {
		userMember3Client, err := getAPIClient(suite.userMember3APIKey)
		suite.Require().NoError(err)

		access, err := userMember3Client.GetAppUserAccess(suite.ctx, app.ID)
		suite.Require().NoError(err)

		suite.False(access.CanWrite, "userMember3 should not be able to write to the app")
		suite.False(access.CanRead, "userMember3 should not be able to read from the app")
		suite.False(access.IsAdmin, "userMember3 should not be an admin of the app")
	})
}

func (suite *OrganizationsRBACTestSuite) TestAppUpdate_Team() {
	userMember1Client, err := getAPIClient(suite.userMember1APIKey)
	suite.Require().NoError(err)

	app, err := userMember1Client.CreateApp(suite.ctx, &types.App{
		OrganizationID: suite.organization.ID,
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Name:        "test-app-single-user-access",
				Description: "test-app-single-user-access-description",
				Assistants: []types.AssistantConfig{
					{
						Name:  "test-assistant-1",
						Model: "openai/gpt-oss-20b",
					},
				},
			},
		},
	})
	suite.Require().NoError(err)
	suite.Require().NotNil(app)

	// Grant access to userMember2
	_, err = userMember1Client.CreateAppAccessGrant(suite.ctx, app.ID, &types.CreateAccessGrantRequest{
		TeamID: suite.userMember3Team.ID,
		Roles:  []string{"write"},
	})
	suite.Require().NoError(err)

	/*
		VALIDATE APP WRITE ACCESS
	*/

	// userMember2 should not be able to update the app
	userMember2Client, err := getAPIClient(suite.userMember2APIKey)
	suite.Require().NoError(err)
	suite.Error(assertAppWriteAccess(userMember2Client, app.ID), "userMember2 should not be able to update the app")

	// userMember3 should see the app (access granted)
	userMember3Client, err := getAPIClient(suite.userMember3APIKey)
	suite.Require().NoError(err)
	suite.NoError(assertAppWriteAccess(userMember3Client, app.ID), "userMember3 should be able to update the app")
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

func assertAppWriteAccess(userClient *client.HelixClient, appID string) error {
	existingApp, err := userClient.GetApp(context.Background(), appID)
	if err != nil {
		return err
	}

	// Update description of the app
	existingApp.Config.Helix.Description = "new-description"

	_, err = userClient.UpdateApp(context.Background(), existingApp)
	return err
}
