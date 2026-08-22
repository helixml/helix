package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
)

// A sandbox agent authenticates with a session key minted for one spec task.
// That key belongs to the human who created the task, so plain user RBAC would
// let the agent reach every project its owner is a member of. These tests pin
// the scoping that stops it, using artifacts as the project sub-resource — the
// agent-facing write surface where it matters most.
func TestArtifactsProjectScopeTestSuite(t *testing.T) {
	suite.Run(t, new(ArtifactsProjectScopeTestSuite))
}

type ArtifactsProjectScopeTestSuite struct {
	suite.Suite
	ctx           context.Context
	db            *store.PostgresStore
	authenticator auth.Authenticator

	user       *types.User
	userAPIKey string

	organization *types.Organization
	defaultApp   *types.App

	projectA *types.Project // the project the agent was dispatched to
	projectB *types.Project // another project the same user owns

	// scopedKey mimics services.GetOrCreateSessionAPIKey: same owner as
	// userAPIKey, but carrying session/project/spec-task attribution.
	scopedKey string
}

func (suite *ArtifactsProjectScopeTestSuite) SetupTest() {
	suite.ctx = context.Background()

	db, err := getStoreClient()
	suite.Require().NoError(err)
	suite.db = db

	authenticator, err := auth.NewHelixAuthenticator(&config.ServerConfig{}, suite.db, "test-secret", nil)
	suite.Require().NoError(err)
	suite.authenticator = authenticator

	email := fmt.Sprintf("artifact-scope-%s@test.com", uuid.New().String())
	user, apiKey, err := createUser(suite.T(), suite.db, suite.authenticator, email)
	suite.Require().NoError(err)
	suite.user = user
	suite.userAPIKey = apiKey

	ownerClient, err := getAPIClient(suite.userAPIKey)
	suite.Require().NoError(err)

	organization, err := ownerClient.CreateOrganization(suite.ctx, &types.Organization{
		Name: "artifact-scope-" + time.Now().Format("2006-01-02-15-04-05-06"),
	})
	suite.Require().NoError(err)
	suite.organization = organization

	suite.T().Cleanup(func() {
		_ = ownerClient.DeleteOrganization(suite.ctx, suite.organization.ID)
	})

	// A project needs either a code-agent config or a default Agent. An Agent
	// keeps this suite focused on authorization rather than execution config.
	defaultApp, err := ownerClient.CreateApp(suite.ctx, &types.App{
		OrganizationID: suite.organization.ID,
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Name:        "artifact-scope-agent-" + uuid.New().String(),
				Description: "artifact project scope test agent",
				Assistants: []types.AssistantConfig{{
					Name:  "assistant",
					Model: "openai/gpt-oss-20b",
				}},
			},
		},
	})
	suite.Require().NoError(err)

	defaultApp.AgentKind = types.AgentKindOrg
	defaultApp, err = suite.db.UpdateApp(suite.ctx, defaultApp)
	suite.Require().NoError(err)
	suite.defaultApp = defaultApp

	suite.projectA = suite.createProject("scope-a-" + uuid.New().String())
	suite.projectB = suite.createProject("scope-b-" + uuid.New().String())

	suite.scopedKey = suite.createSessionScopedKey(suite.projectA.ID)
}

func (suite *ArtifactsProjectScopeTestSuite) createProject(name string) *types.Project {
	suite.T().Helper()

	// A project needs a primary repository; artifacts do not use it, but
	// project creation rejects the request without one.
	repo := suite.createTestRepository(name)

	var project types.Project
	status, body := apiJSON(suite.T(), suite.userAPIKey, http.MethodPost, "/projects", &types.ProjectCreateRequest{
		OrganizationID:    suite.organization.Name,
		Name:              name,
		Description:       "artifact project scope test",
		DefaultRepoID:     repo.ID,
		DefaultHelixAppID: suite.defaultApp.ID,
	}, &project)
	suite.Require().Equal(http.StatusOK, status, body)
	suite.Require().NotEmpty(project.ID)

	return &project
}

func (suite *ArtifactsProjectScopeTestSuite) createTestRepository(namePrefix string) *types.GitRepository {
	suite.T().Helper()

	var repo types.GitRepository
	status, body := apiJSON(suite.T(), suite.userAPIKey, http.MethodPost, "/git/repositories", &types.GitRepositoryCreateRequest{
		Name:           namePrefix + "-repo",
		Description:    "artifact project scope test repository",
		RepoType:       types.GitRepositoryTypeCode,
		OwnerID:        suite.user.ID,
		OrganizationID: suite.organization.ID,
		DefaultBranch:  "main",
		InitialFiles: map[string]string{
			"README.md": "# artifact project scope test\n",
		},
		Metadata: map[string]interface{}{},
	}, &repo)
	suite.Require().Equal(http.StatusCreated, status, body)
	return &repo
}

// createSessionScopedKey mirrors what the spec-task service mints for a sandbox:
// an ordinary API key owned by the task creator, tagged with the session, the
// project and the spec task.
func (suite *ArtifactsProjectScopeTestSuite) createSessionScopedKey(projectID string) string {
	suite.T().Helper()

	key, err := system.GenerateAPIKey()
	suite.Require().NoError(err)

	_, err = suite.db.CreateAPIKey(suite.ctx, &types.ApiKey{
		Name:           "Session key - scope test",
		Key:            key,
		Owner:          suite.user.ID,
		OwnerType:      types.OwnerTypeUser,
		Type:           types.APIkeytypeAPI,
		OrganizationID: suite.organization.ID,
		SessionID:      "ses_" + strings.ReplaceAll(uuid.New().String(), "-", ""),
		ProjectID:      projectID,
		SpecTaskID:     "spt_" + strings.ReplaceAll(uuid.New().String(), "-", ""),
	})
	suite.Require().NoError(err)

	return key
}

func (suite *ArtifactsProjectScopeTestSuite) createArtifact(apiClient *client.HelixClient, projectID, name string) (*types.Artifact, error) {
	suite.T().Helper()

	visibility := types.ArtifactVisibilityProject
	return apiClient.CreateArtifact(suite.ctx, projectID, &client.ArtifactUploadRequest{
		Name:       &name,
		Visibility: &visibility,
		Content: &client.ArtifactContent{
			Filename: "index.html",
			Reader:   strings.NewReader("<h1>scope test</h1>"),
		},
	})
}

// The agent must keep full use of the project it was dispatched to.
func (suite *ArtifactsProjectScopeTestSuite) TestScopedKeyRetainsOwnProject() {
	scopedClient, err := getAPIClient(suite.scopedKey)
	suite.Require().NoError(err)

	artifact, err := suite.createArtifact(scopedClient, suite.projectA.ID, "own-project artifact")
	suite.Require().NoError(err, "scoped key must still write to its own project")
	suite.Require().Equal(suite.projectA.ID, artifact.ProjectID)

	artifacts, err := scopedClient.ListArtifacts(suite.ctx, suite.projectA.ID)
	suite.Require().NoError(err, "scoped key must still read its own project")
	suite.Require().Len(artifacts, 1)

	suite.Require().NoError(scopedClient.DeleteArtifact(suite.ctx, artifact.ID),
		"scoped key must still delete in its own project")
}

// The same key must not reach a sibling project, even though its owner may.
func (suite *ArtifactsProjectScopeTestSuite) TestScopedKeyCannotReachAnotherProject() {
	scopedClient, err := getAPIClient(suite.scopedKey)
	suite.Require().NoError(err)

	_, err = scopedClient.ListArtifacts(suite.ctx, suite.projectB.ID)
	suite.Require().Error(err, "scoped key must not list another project's artifacts")

	_, err = suite.createArtifact(scopedClient, suite.projectB.ID, "cross-project artifact")
	suite.Require().Error(err, "scoped key must not create artifacts in another project")
}

// Deleting is the destructive case: an artifact the agent never created, in a
// project it was never dispatched to.
func (suite *ArtifactsProjectScopeTestSuite) TestScopedKeyCannotDeleteAnotherProjectsArtifact() {
	ownerClient, err := getAPIClient(suite.userAPIKey)
	suite.Require().NoError(err)

	victim, err := suite.createArtifact(ownerClient, suite.projectB.ID, "owner artifact in B")
	suite.Require().NoError(err)
	suite.T().Cleanup(func() {
		_ = ownerClient.DeleteArtifact(suite.ctx, victim.ID)
	})

	scopedClient, err := getAPIClient(suite.scopedKey)
	suite.Require().NoError(err)

	suite.Require().Error(scopedClient.DeleteArtifact(suite.ctx, victim.ID),
		"scoped key must not delete an artifact in another project")

	// Still there, via a credential that is allowed to see it.
	remaining, err := ownerClient.ListArtifacts(suite.ctx, suite.projectB.ID)
	suite.Require().NoError(err)
	suite.Require().Len(remaining, 1, "artifact must survive the refused delete")
}

// The restriction comes from the key's scope, not from the user losing access:
// the same human, on an unscoped key, still reaches both projects.
func (suite *ArtifactsProjectScopeTestSuite) TestUnscopedKeyReachesBothProjects() {
	ownerClient, err := getAPIClient(suite.userAPIKey)
	suite.Require().NoError(err)

	for _, projectID := range []string{suite.projectA.ID, suite.projectB.ID} {
		_, err := ownerClient.ListArtifacts(suite.ctx, projectID)
		suite.Require().NoError(err, "unscoped user key must reach project %s", projectID)
	}

	artifact, err := suite.createArtifact(ownerClient, suite.projectB.ID, "unscoped write")
	suite.Require().NoError(err)
	suite.Require().NoError(ownerClient.DeleteArtifact(suite.ctx, artifact.ID))
}
