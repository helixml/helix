package server

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type AuthzRepositorySuite struct {
	suite.Suite
	ctrl      *gomock.Controller
	mockStore *store.MockStore
	server    *HelixAPIServer

	orgID  string
	userID string
}

func TestAuthzRepositorySuite(t *testing.T) {
	suite.Run(t, new(AuthzRepositorySuite))
}

func (s *AuthzRepositorySuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockStore = store.NewMockStore(s.ctrl)
	s.orgID = "org1"
	s.userID = "user1"

	s.server = &HelixAPIServer{
		Cfg:   &config.ServerConfig{},
		Store: s.mockStore,
	}
}

// expectOrgMember sets up the mock to return a membership with the given role.
func (s *AuthzRepositorySuite) expectOrgMember(role types.OrganizationRole) {
	s.mockStore.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: s.orgID,
		UserID:         s.userID,
	}).Return(&types.OrganizationMembership{
		OrganizationID: s.orgID,
		UserID:         s.userID,
		Role:           role,
	}, nil).AnyTimes()
}

// expectNoDirectRepoAccess sets up mocks so authorizeUserToResource for the repo returns "not authorized".
func (s *AuthzRepositorySuite) expectNoDirectRepoAccess(repoID string) {
	s.mockStore.EXPECT().ListTeams(gomock.Any(), &store.ListTeamsQuery{
		OrganizationID: s.orgID,
		UserID:         s.userID,
	}).Return([]*types.Team{}, nil)
	s.mockStore.EXPECT().ListAccessGrants(gomock.Any(), &store.ListAccessGrantsQuery{
		OrganizationID: s.orgID,
		UserID:         s.userID,
		ResourceID:     repoID,
	}).Return([]*types.AccessGrant{}, nil)
}

// expectProjectAccess sets up mocks so authorizeUserToProject succeeds via RBAC.
func (s *AuthzRepositorySuite) expectProjectAccess(projectID string) {
	s.mockStore.EXPECT().ListTeams(gomock.Any(), &store.ListTeamsQuery{
		OrganizationID: s.orgID,
		UserID:         s.userID,
	}).Return([]*types.Team{}, nil)
	s.mockStore.EXPECT().ListAccessGrants(gomock.Any(), &store.ListAccessGrantsQuery{
		OrganizationID: s.orgID,
		UserID:         s.userID,
		ResourceID:     projectID,
	}).Return([]*types.AccessGrant{
		{
			Roles: []types.Role{{
				Config: types.Config{Rules: []types.Rule{{
					Resources: []types.Resource{types.ResourceProject},
					Actions:   []types.Action{types.ActionGet},
					Effect:    types.EffectAllow,
				}}},
			}},
		},
	}, nil)
}

// expectNoProjectAccess sets up mocks so authorizeUserToProject fails via RBAC.
func (s *AuthzRepositorySuite) expectNoProjectAccess(projectID string) {
	s.mockStore.EXPECT().ListTeams(gomock.Any(), &store.ListTeamsQuery{
		OrganizationID: s.orgID,
		UserID:         s.userID,
	}).Return([]*types.Team{}, nil)
	s.mockStore.EXPECT().ListAccessGrants(gomock.Any(), &store.ListAccessGrantsQuery{
		OrganizationID: s.orgID,
		UserID:         s.userID,
		ResourceID:     projectID,
	}).Return([]*types.AccessGrant{}, nil)
}

// expectDirectRepoAccess sets up mocks so authorizeUserToResource for the repo succeeds.
func (s *AuthzRepositorySuite) expectDirectRepoAccess(repoID string, action types.Action) {
	s.mockStore.EXPECT().ListTeams(gomock.Any(), &store.ListTeamsQuery{
		OrganizationID: s.orgID,
		UserID:         s.userID,
	}).Return([]*types.Team{}, nil)
	s.mockStore.EXPECT().ListAccessGrants(gomock.Any(), &store.ListAccessGrantsQuery{
		OrganizationID: s.orgID,
		UserID:         s.userID,
		ResourceID:     repoID,
	}).Return([]*types.AccessGrant{
		{
			Roles: []types.Role{{
				Config: types.Config{Rules: []types.Rule{{
					Resources: []types.Resource{types.ResourceGitRepository},
					Actions:   []types.Action{action},
					Effect:    types.EffectAllow,
				}}},
			}},
		},
	}, nil)
}

// --- No org: owner-only access ---

func (s *AuthzRepositorySuite) TestNoOrg_OwnerAllowed() {
	repo := &types.GitRepository{
		ID:      "repo1",
		OwnerID: s.userID,
	}
	user := &types.User{ID: s.userID}

	err := s.server.authorizeUserToRepository(context.Background(), user, repo, types.ActionGet)
	s.NoError(err)
}

func (s *AuthzRepositorySuite) TestNoOrg_NonOwnerDenied() {
	repo := &types.GitRepository{
		ID:      "repo1",
		OwnerID: "someone_else",
	}
	user := &types.User{ID: s.userID}

	err := s.server.authorizeUserToRepository(context.Background(), user, repo, types.ActionGet)
	s.Error(err)
	s.Contains(err.Error(), "not the owner")
}

// --- With org: repo owner ---

func (s *AuthzRepositorySuite) TestWithOrg_RepoOwnerAllowed() {
	repo := &types.GitRepository{
		ID:             "repo1",
		OwnerID:        s.userID,
		OrganizationID: s.orgID,
	}
	user := &types.User{ID: s.userID}

	s.expectOrgMember(types.OrganizationRoleMember)

	err := s.server.authorizeUserToRepository(context.Background(), user, repo, types.ActionGet)
	s.NoError(err)
}

// --- With org: org owner ---

func (s *AuthzRepositorySuite) TestWithOrg_OrgOwnerAllowed() {
	repo := &types.GitRepository{
		ID:             "repo1",
		OwnerID:        "someone_else",
		OrganizationID: s.orgID,
	}
	user := &types.User{ID: s.userID}

	s.expectOrgMember(types.OrganizationRoleOwner)

	err := s.server.authorizeUserToRepository(context.Background(), user, repo, types.ActionGet)
	s.NoError(err)
}

// --- With org: non-member denied ---

func (s *AuthzRepositorySuite) TestWithOrg_NonMemberDenied() {
	repo := &types.GitRepository{
		ID:             "repo1",
		OwnerID:        "someone_else",
		OrganizationID: s.orgID,
	}
	user := &types.User{ID: s.userID}

	s.mockStore.EXPECT().GetOrganizationMembership(gomock.Any(), gomock.Any()).
		Return(nil, store.ErrNotFound)

	err := s.server.authorizeUserToRepository(context.Background(), user, repo, types.ActionGet)
	s.Error(err)
}

// --- With org: direct access grant to repository ---

func (s *AuthzRepositorySuite) TestWithOrg_DirectAccessGrantAllowed() {
	repo := &types.GitRepository{
		ID:             "repo1",
		OwnerID:        "someone_else",
		OrganizationID: s.orgID,
	}
	user := &types.User{ID: s.userID}

	s.expectOrgMember(types.OrganizationRoleMember)
	s.expectDirectRepoAccess("repo1", types.ActionGet)

	// Direct access succeeds -> returns nil immediately, no project check
	err := s.server.authorizeUserToRepository(context.Background(), user, repo, types.ActionGet)
	s.NoError(err)
}

// --- With org: no direct access, no projects -> denied ---

func (s *AuthzRepositorySuite) TestWithOrg_NoDirectAccess_NoProjects_Denied() {
	repo := &types.GitRepository{
		ID:             "repo1",
		OwnerID:        "someone_else",
		OrganizationID: s.orgID,
	}
	user := &types.User{ID: s.userID}

	s.expectOrgMember(types.OrganizationRoleMember)
	s.expectNoDirectRepoAccess("repo1")

	s.mockStore.EXPECT().ListProjects(gomock.Any(), &store.ListProjectsQuery{
		OrganizationID: s.orgID,
	}).Return([]*types.Project{}, nil)

	err := s.server.authorizeUserToRepository(context.Background(), user, repo, types.ActionGet)
	s.Error(err)
	s.Contains(err.Error(), "does not have access")
}

// --- With org: no direct repo access, but has access via project ---

func (s *AuthzRepositorySuite) TestWithOrg_NoDirectAccess_AccessViaProject() {
	repo := &types.GitRepository{
		ID:             "repo1",
		OwnerID:        "someone_else",
		OrganizationID: s.orgID,
	}
	user := &types.User{ID: s.userID}

	s.expectOrgMember(types.OrganizationRoleMember)
	s.expectNoDirectRepoAccess("repo1")

	// Falls through to project check — lists ALL org projects
	s.mockStore.EXPECT().ListProjects(gomock.Any(), &store.ListProjectsQuery{
		OrganizationID: s.orgID,
	}).Return([]*types.Project{
		{
			ID:             "proj1",
			OrganizationID: s.orgID,
			UserID:         "someone_else",
		},
	}, nil)

	s.mockStore.EXPECT().ListProjectRepositories(gomock.Any(), &types.ListProjectRepositoriesQuery{
		ProjectID:    "proj1",
		RepositoryID: "repo1",
	}).Return([]*types.ProjectRepository{
		{
			ProjectID:    "proj1",
			RepositoryID: "repo1",
		},
	}, nil)

	// Repo is attached to project — now check if user has access to the project via RBAC
	s.expectProjectAccess("proj1")

	err := s.server.authorizeUserToRepository(context.Background(), user, repo, types.ActionGet)
	s.NoError(err)
}

// --- With org: no direct access, project exists but repo not attached -> denied ---

func (s *AuthzRepositorySuite) TestWithOrg_NoDirectAccess_RepoNotAttachedToProject_Denied() {
	repo := &types.GitRepository{
		ID:             "repo1",
		OwnerID:        "someone_else",
		OrganizationID: s.orgID,
	}
	user := &types.User{ID: s.userID}

	s.expectOrgMember(types.OrganizationRoleMember)
	s.expectNoDirectRepoAccess("repo1")

	// Project exists but repo is not attached to it
	s.mockStore.EXPECT().ListProjects(gomock.Any(), &store.ListProjectsQuery{
		OrganizationID: s.orgID,
	}).Return([]*types.Project{
		{
			ID:             "proj1",
			OrganizationID: s.orgID,
		},
	}, nil)

	s.mockStore.EXPECT().ListProjectRepositories(gomock.Any(), &types.ListProjectRepositoriesQuery{
		ProjectID:    "proj1",
		RepositoryID: "repo1",
	}).Return([]*types.ProjectRepository{}, nil) // Not attached

	err := s.server.authorizeUserToRepository(context.Background(), user, repo, types.ActionGet)
	s.Error(err)
	s.Contains(err.Error(), "does not have access")
}

// --- With org: repo attached to project but user has no project access -> denied ---

func (s *AuthzRepositorySuite) TestWithOrg_NoDirectAccess_RepoAttachedButNoProjectAccess_Denied() {
	repo := &types.GitRepository{
		ID:             "repo1",
		OwnerID:        "someone_else",
		OrganizationID: s.orgID,
	}
	user := &types.User{ID: s.userID}

	s.expectOrgMember(types.OrganizationRoleMember)
	s.expectNoDirectRepoAccess("repo1")

	s.mockStore.EXPECT().ListProjects(gomock.Any(), &store.ListProjectsQuery{
		OrganizationID: s.orgID,
	}).Return([]*types.Project{
		{
			ID:             "proj1",
			OrganizationID: s.orgID,
			UserID:         "someone_else",
		},
	}, nil)

	s.mockStore.EXPECT().ListProjectRepositories(gomock.Any(), &types.ListProjectRepositoriesQuery{
		ProjectID:    "proj1",
		RepositoryID: "repo1",
	}).Return([]*types.ProjectRepository{
		{ProjectID: "proj1", RepositoryID: "repo1"},
	}, nil)

	// User does NOT have access to the project
	s.expectNoProjectAccess("proj1")

	err := s.server.authorizeUserToRepository(context.Background(), user, repo, types.ActionGet)
	s.Error(err)
	s.Contains(err.Error(), "does not have access")
}

// --- Admin bypasses org membership check ---

func (s *AuthzRepositorySuite) TestWithOrg_AdminAllowed() {
	repo := &types.GitRepository{
		ID:             "repo1",
		OwnerID:        "someone_else",
		OrganizationID: s.orgID,
	}
	user := &types.User{ID: s.userID, Admin: true}

	// Admin without org membership gets synthetic owner role
	s.mockStore.EXPECT().GetOrganizationMembership(gomock.Any(), gomock.Any()).
		Return(nil, store.ErrNotFound)

	err := s.server.authorizeUserToRepository(context.Background(), user, repo, types.ActionGet)
	s.NoError(err)
}

// ===== App Authorization Suite =====

type AuthzAppSuite struct {
	suite.Suite
	ctrl      *gomock.Controller
	mockStore *store.MockStore
	server    *HelixAPIServer

	orgID  string
	userID string
}

func TestAuthzAppSuite(t *testing.T) {
	suite.Run(t, new(AuthzAppSuite))
}

func (s *AuthzAppSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockStore = store.NewMockStore(s.ctrl)
	s.orgID = "org1"
	s.userID = "user1"

	s.server = &HelixAPIServer{
		Cfg:   &config.ServerConfig{},
		Store: s.mockStore,
	}
}

func (s *AuthzAppSuite) expectOrgMember(role types.OrganizationRole) {
	s.mockStore.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: s.orgID,
		UserID:         s.userID,
	}).Return(&types.OrganizationMembership{
		OrganizationID: s.orgID,
		UserID:         s.userID,
		Role:           role,
	}, nil).AnyTimes()
}

func (s *AuthzAppSuite) expectNoDirectAppAccess(appID string) {
	s.mockStore.EXPECT().ListTeams(gomock.Any(), &store.ListTeamsQuery{
		OrganizationID: s.orgID,
		UserID:         s.userID,
	}).Return([]*types.Team{}, nil)
	s.mockStore.EXPECT().ListAccessGrants(gomock.Any(), &store.ListAccessGrantsQuery{
		OrganizationID: s.orgID,
		UserID:         s.userID,
		ResourceID:     appID,
	}).Return([]*types.AccessGrant{}, nil)
}

func (s *AuthzAppSuite) expectProjectAccess(projectID string) {
	s.mockStore.EXPECT().ListTeams(gomock.Any(), &store.ListTeamsQuery{
		OrganizationID: s.orgID,
		UserID:         s.userID,
	}).Return([]*types.Team{}, nil)
	s.mockStore.EXPECT().ListAccessGrants(gomock.Any(), &store.ListAccessGrantsQuery{
		OrganizationID: s.orgID,
		UserID:         s.userID,
		ResourceID:     projectID,
	}).Return([]*types.AccessGrant{
		{
			Roles: []types.Role{{
				Config: types.Config{Rules: []types.Rule{{
					Resources: []types.Resource{types.ResourceProject},
					Actions:   []types.Action{types.ActionGet},
					Effect:    types.EffectAllow,
				}}},
			}},
		},
	}, nil)
}

func (s *AuthzAppSuite) expectNoProjectAccess(projectID string) {
	s.mockStore.EXPECT().ListTeams(gomock.Any(), &store.ListTeamsQuery{
		OrganizationID: s.orgID,
		UserID:         s.userID,
	}).Return([]*types.Team{}, nil)
	s.mockStore.EXPECT().ListAccessGrants(gomock.Any(), &store.ListAccessGrantsQuery{
		OrganizationID: s.orgID,
		UserID:         s.userID,
		ResourceID:     projectID,
	}).Return([]*types.AccessGrant{}, nil)
}

// --- No org: owner-only access ---

func (s *AuthzAppSuite) TestNoOrg_OwnerAllowed() {
	app := &types.App{ID: "app1", Owner: s.userID}
	user := &types.User{ID: s.userID}

	err := s.server.authorizeUserToApp(context.Background(), user, app, types.ActionGet)
	s.NoError(err)
}

func (s *AuthzAppSuite) TestNoOrg_NonOwnerDenied() {
	app := &types.App{ID: "app1", Owner: "someone_else"}
	user := &types.User{ID: s.userID}

	err := s.server.authorizeUserToApp(context.Background(), user, app, types.ActionGet)
	s.Error(err)
}

// --- With org: app owner ---

func (s *AuthzAppSuite) TestWithOrg_AppOwnerAllowed() {
	app := &types.App{ID: "app1", Owner: s.userID, OrganizationID: s.orgID}
	user := &types.User{ID: s.userID}

	s.expectOrgMember(types.OrganizationRoleMember)

	err := s.server.authorizeUserToApp(context.Background(), user, app, types.ActionGet)
	s.NoError(err)
}

// --- With org: org owner ---

func (s *AuthzAppSuite) TestWithOrg_OrgOwnerAllowed() {
	app := &types.App{ID: "app1", Owner: "someone_else", OrganizationID: s.orgID}
	user := &types.User{ID: s.userID}

	s.expectOrgMember(types.OrganizationRoleOwner)

	err := s.server.authorizeUserToApp(context.Background(), user, app, types.ActionGet)
	s.NoError(err)
}

// --- With org: non-member denied ---

func (s *AuthzAppSuite) TestWithOrg_NonMemberDenied() {
	app := &types.App{ID: "app1", Owner: "someone_else", OrganizationID: s.orgID}
	user := &types.User{ID: s.userID}

	s.mockStore.EXPECT().GetOrganizationMembership(gomock.Any(), gomock.Any()).
		Return(nil, store.ErrNotFound)

	err := s.server.authorizeUserToApp(context.Background(), user, app, types.ActionGet)
	s.Error(err)
}

// --- With org: no direct access, no projects -> denied ---

func (s *AuthzAppSuite) TestWithOrg_NoDirectAccess_NoProjects_Denied() {
	app := &types.App{ID: "app1", Owner: "someone_else", OrganizationID: s.orgID}
	user := &types.User{ID: s.userID}

	s.expectOrgMember(types.OrganizationRoleMember)
	s.expectNoDirectAppAccess("app1")

	s.mockStore.EXPECT().ListProjects(gomock.Any(), &store.ListProjectsQuery{
		OrganizationID: s.orgID,
	}).Return([]*types.Project{}, nil)

	err := s.server.authorizeUserToApp(context.Background(), user, app, types.ActionGet)
	s.Error(err)
	s.Contains(err.Error(), "does not have access")
}

// --- With org: no direct access, app is project's DefaultHelixAppID -> allowed ---

func (s *AuthzAppSuite) TestWithOrg_NoDirectAccess_AccessViaProject_DefaultApp() {
	app := &types.App{ID: "app1", Owner: "someone_else", OrganizationID: s.orgID}
	user := &types.User{ID: s.userID}

	s.expectOrgMember(types.OrganizationRoleMember)
	s.expectNoDirectAppAccess("app1")

	s.mockStore.EXPECT().ListProjects(gomock.Any(), &store.ListProjectsQuery{
		OrganizationID: s.orgID,
	}).Return([]*types.Project{
		{
			ID:                "proj1",
			OrganizationID:    s.orgID,
			UserID:            "someone_else",
			DefaultHelixAppID: "app1",
		},
	}, nil)

	// App is referenced by project — user needs project access via RBAC
	s.expectProjectAccess("proj1")

	err := s.server.authorizeUserToApp(context.Background(), user, app, types.ActionGet)
	s.NoError(err)
}

// --- With org: no direct access, app is project's ProjectManagerHelixAppID -> allowed ---

func (s *AuthzAppSuite) TestWithOrg_NoDirectAccess_AccessViaProject_ProjectManagerApp() {
	app := &types.App{ID: "app1", Owner: "someone_else", OrganizationID: s.orgID}
	user := &types.User{ID: s.userID}

	s.expectOrgMember(types.OrganizationRoleMember)
	s.expectNoDirectAppAccess("app1")

	s.mockStore.EXPECT().ListProjects(gomock.Any(), &store.ListProjectsQuery{
		OrganizationID: s.orgID,
	}).Return([]*types.Project{
		{
			ID:                       "proj1",
			OrganizationID:           s.orgID,
			UserID:                   "someone_else",
			ProjectManagerHelixAppID: "app1",
		},
	}, nil)

	s.expectProjectAccess("proj1")

	err := s.server.authorizeUserToApp(context.Background(), user, app, types.ActionGet)
	s.NoError(err)
}

// --- With org: no direct access, app is project's PullRequestReviewerHelixAppID -> allowed ---

func (s *AuthzAppSuite) TestWithOrg_NoDirectAccess_AccessViaProject_PRReviewerApp() {
	app := &types.App{ID: "app1", Owner: "someone_else", OrganizationID: s.orgID}
	user := &types.User{ID: s.userID}

	s.expectOrgMember(types.OrganizationRoleMember)
	s.expectNoDirectAppAccess("app1")

	s.mockStore.EXPECT().ListProjects(gomock.Any(), &store.ListProjectsQuery{
		OrganizationID: s.orgID,
	}).Return([]*types.Project{
		{
			ID:                            "proj1",
			OrganizationID:                s.orgID,
			UserID:                        "someone_else",
			PullRequestReviewerHelixAppID: "app1",
		},
	}, nil)

	s.expectProjectAccess("proj1")

	err := s.server.authorizeUserToApp(context.Background(), user, app, types.ActionGet)
	s.NoError(err)
}

// --- With org: no direct access, project exists but app not referenced -> denied ---

func (s *AuthzAppSuite) TestWithOrg_NoDirectAccess_AppNotReferencedByProject_Denied() {
	app := &types.App{ID: "app1", Owner: "someone_else", OrganizationID: s.orgID}
	user := &types.User{ID: s.userID}

	s.expectOrgMember(types.OrganizationRoleMember)
	s.expectNoDirectAppAccess("app1")

	s.mockStore.EXPECT().ListProjects(gomock.Any(), &store.ListProjectsQuery{
		OrganizationID: s.orgID,
	}).Return([]*types.Project{
		{
			ID:                "proj1",
			OrganizationID:    s.orgID,
			DefaultHelixAppID: "other_app",
		},
	}, nil)

	err := s.server.authorizeUserToApp(context.Background(), user, app, types.ActionGet)
	s.Error(err)
	s.Contains(err.Error(), "does not have access")
}

// --- With org: app referenced by project but user has no project access -> denied ---

func (s *AuthzAppSuite) TestWithOrg_NoDirectAccess_AppReferencedButNoProjectAccess_Denied() {
	app := &types.App{ID: "app1", Owner: "someone_else", OrganizationID: s.orgID}
	user := &types.User{ID: s.userID}

	s.expectOrgMember(types.OrganizationRoleMember)
	s.expectNoDirectAppAccess("app1")

	s.mockStore.EXPECT().ListProjects(gomock.Any(), &store.ListProjectsQuery{
		OrganizationID: s.orgID,
	}).Return([]*types.Project{
		{
			ID:                "proj1",
			OrganizationID:    s.orgID,
			UserID:            "someone_else",
			DefaultHelixAppID: "app1",
		},
	}, nil)

	// User does NOT have access to the project
	s.expectNoProjectAccess("proj1")

	err := s.server.authorizeUserToApp(context.Background(), user, app, types.ActionGet)
	s.Error(err)
	s.Contains(err.Error(), "does not have access")
}

// --- Admin bypasses org membership check ---

func (s *AuthzAppSuite) TestWithOrg_AdminAllowed() {
	app := &types.App{ID: "app1", Owner: "someone_else", OrganizationID: s.orgID}
	user := &types.User{ID: s.userID, Admin: true}

	s.mockStore.EXPECT().GetOrganizationMembership(gomock.Any(), gomock.Any()).
		Return(nil, store.ErrNotFound)

	err := s.server.authorizeUserToApp(context.Background(), user, app, types.ActionGet)
	s.NoError(err)
}

// ===== Project-via-team Authorization Suite =====
//
// Regression tests for the case where an org member belongs to a team that holds
// an access grant on a project. Before the fix, RoleRead/RoleWrite omitted
// ResourceProject so team-granted users could not see projects unless the team
// had RoleAdmin (which uses ResourceAny).

type AuthzProjectViaTeamSuite struct {
	suite.Suite
	ctrl      *gomock.Controller
	mockStore *store.MockStore
	server    *HelixAPIServer

	orgID   string
	userID  string
	teamID  string
	projID  string
	project *types.Project
	user    *types.User
}

func TestAuthzProjectViaTeamSuite(t *testing.T) {
	suite.Run(t, new(AuthzProjectViaTeamSuite))
}

func (s *AuthzProjectViaTeamSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockStore = store.NewMockStore(s.ctrl)
	s.orgID = "org1"
	s.userID = "user1"
	s.teamID = "team1"
	s.projID = "proj1"

	s.server = &HelixAPIServer{
		Cfg:   &config.ServerConfig{},
		Store: s.mockStore,
	}

	s.project = &types.Project{
		ID:             s.projID,
		OrganizationID: s.orgID,
		UserID:         "someone_else",
	}
	s.user = &types.User{ID: s.userID}
}

func (s *AuthzProjectViaTeamSuite) expectOrgMember() {
	s.mockStore.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: s.orgID,
		UserID:         s.userID,
	}).Return(&types.OrganizationMembership{
		OrganizationID: s.orgID,
		UserID:         s.userID,
		Role:           types.OrganizationRoleMember,
	}, nil).AnyTimes()
}

// expectTeamGrant mocks the lookup chain so the user is in `teamID` and that
// team holds a single grant on `projID` carrying the given canonical role config.
func (s *AuthzProjectViaTeamSuite) expectTeamGrant(cfg types.Config) {
	s.mockStore.EXPECT().ListTeams(gomock.Any(), &store.ListTeamsQuery{
		OrganizationID: s.orgID,
		UserID:         s.userID,
	}).Return([]*types.Team{{ID: s.teamID, OrganizationID: s.orgID}}, nil)

	s.mockStore.EXPECT().ListAccessGrants(gomock.Any(), &store.ListAccessGrantsQuery{
		OrganizationID: s.orgID,
		UserID:         s.userID,
		ResourceID:     s.projID,
		TeamIDs:        []string{s.teamID},
	}).Return([]*types.AccessGrant{
		{
			ID:             "grant1",
			OrganizationID: s.orgID,
			ResourceID:     s.projID,
			TeamID:         s.teamID,
			Roles:          []types.Role{{Config: cfg}},
		},
	}, nil)
}

// expectTeamNoGrant: user is in a team but ListAccessGrants returns nothing.
func (s *AuthzProjectViaTeamSuite) expectTeamNoGrant() {
	s.mockStore.EXPECT().ListTeams(gomock.Any(), &store.ListTeamsQuery{
		OrganizationID: s.orgID,
		UserID:         s.userID,
	}).Return([]*types.Team{{ID: s.teamID, OrganizationID: s.orgID}}, nil)

	s.mockStore.EXPECT().ListAccessGrants(gomock.Any(), &store.ListAccessGrantsQuery{
		OrganizationID: s.orgID,
		UserID:         s.userID,
		ResourceID:     s.projID,
		TeamIDs:        []string{s.teamID},
	}).Return([]*types.AccessGrant{}, nil)
}

// TestTeamAdminGrant_AllowsProjectGet — ResourceAny path. This is the case the
// user originally reported as broken. With autoMigrateRoleConfig syncing the
// stored config from types.RoleAdmin at startup, this path works today, but it
// must not silently regress.
func (s *AuthzProjectViaTeamSuite) TestTeamAdminGrant_AllowsProjectGet() {
	s.expectOrgMember()
	s.expectTeamGrant(types.RoleAdmin)

	err := s.server.authorizeUserToProject(context.Background(), s.user, s.project, types.ActionGet)
	s.NoError(err)
}

// TestTeamReadGrant_AllowsProjectGet — regression guard for the actual bug fix:
// RoleRead must include ResourceProject so a team with read access can see the project.
func (s *AuthzProjectViaTeamSuite) TestTeamReadGrant_AllowsProjectGet() {
	s.expectOrgMember()
	s.expectTeamGrant(types.RoleRead)

	err := s.server.authorizeUserToProject(context.Background(), s.user, s.project, types.ActionGet)
	s.NoError(err)
}

// TestTeamWriteGrant_AllowsProjectGet — same regression guard for RoleWrite.
func (s *AuthzProjectViaTeamSuite) TestTeamWriteGrant_AllowsProjectGet() {
	s.expectOrgMember()
	s.expectTeamGrant(types.RoleWrite)

	err := s.server.authorizeUserToProject(context.Background(), s.user, s.project, types.ActionGet)
	s.NoError(err)
}

// TestTeamMembership_NoGrant_Denied — negative case. User is in a team but the
// team holds no grant on this project; must not see the project.
func (s *AuthzProjectViaTeamSuite) TestTeamMembership_NoGrant_Denied() {
	s.expectOrgMember()
	s.expectTeamNoGrant()

	err := s.server.authorizeUserToProject(context.Background(), s.user, s.project, types.ActionGet)
	s.Error(err)
}

func (s *AuthzProjectViaTeamSuite) TestOrgMembersAccess_AllowsMemberGetButNotUpdate() {
	s.project.Metadata.OrgMembersAccess = true
	s.expectOrgMember()
	s.expectTeamNoGrant()

	s.NoError(s.server.authorizeUserToProject(context.Background(), s.user, s.project, types.ActionGet))
	s.Error(s.server.authorizeUserToProject(context.Background(), s.user, s.project, types.ActionUpdate))
}

func (s *AuthzProjectViaTeamSuite) TestOrgMembersAccess_DoesNotAllowDelete() {
	s.project.Metadata.OrgMembersAccess = true
	s.expectOrgMember()
	s.expectTeamNoGrant()

	s.Error(s.server.authorizeUserToProject(context.Background(), s.user, s.project, types.ActionDelete))
}

func (s *AuthzProjectViaTeamSuite) TestOrgMembersAccess_DoesNotAllowNonMember() {
	s.project.Metadata.OrgMembersAccess = true
	s.mockStore.EXPECT().GetOrganizationMembership(gomock.Any(), gomock.Any()).
		Return(nil, store.ErrNotFound)

	s.Error(s.server.authorizeUserToProject(context.Background(), s.user, s.project, types.ActionGet))
}

// AuthzSessionSuite pins authorizeUserToSession, which startChatSessionHandler
// uses to gate POST /sessions/chat on an existing session. The regression this
// guards: the chat handler used a strict `session.Owner != user.ID` check, so an
// org owner (or project grantee) who was NOT the literal session owner got a 401
// when chatting into an org-shared session — e.g. a helix-org Worker's "Human
// Desktop" session, which is owned by whoever bootstrapped the org, not the
// operator driving the worker. The read path always used this RBAC; the write
// path now matches it (with ActionUpdate so read-only members can't drive the
// agent).
type AuthzSessionSuite struct {
	suite.Suite
	ctrl      *gomock.Controller
	mockStore *store.MockStore
	server    *HelixAPIServer

	orgID  string
	userID string
}

func TestAuthzSessionSuite(t *testing.T) {
	suite.Run(t, new(AuthzSessionSuite))
}

func (s *AuthzSessionSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockStore = store.NewMockStore(s.ctrl)
	s.orgID = "org1"
	s.userID = "user1"

	s.server = &HelixAPIServer{
		Cfg:   &config.ServerConfig{},
		Store: s.mockStore,
	}
}

func (s *AuthzSessionSuite) expectOrgMember(role types.OrganizationRole) {
	s.mockStore.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: s.orgID,
		UserID:         s.userID,
	}).Return(&types.OrganizationMembership{
		OrganizationID: s.orgID,
		UserID:         s.userID,
		Role:           role,
	}, nil).AnyTimes()
}

func (s *AuthzSessionSuite) expectProjectGrant(projectID string, action types.Action) {
	s.mockStore.EXPECT().ListTeams(gomock.Any(), &store.ListTeamsQuery{
		OrganizationID: s.orgID,
		UserID:         s.userID,
	}).Return([]*types.Team{}, nil)
	s.mockStore.EXPECT().ListAccessGrants(gomock.Any(), &store.ListAccessGrantsQuery{
		OrganizationID: s.orgID,
		UserID:         s.userID,
		ResourceID:     projectID,
	}).Return([]*types.AccessGrant{
		{
			Roles: []types.Role{{
				Config: types.Config{Rules: []types.Rule{{
					Resources: []types.Resource{types.ResourceProject},
					Actions:   []types.Action{action},
					Effect:    types.EffectAllow,
				}}},
			}},
		},
	}, nil)
}

func (s *AuthzSessionSuite) expectNoProjectGrant(projectID string) {
	s.mockStore.EXPECT().ListTeams(gomock.Any(), &store.ListTeamsQuery{
		OrganizationID: s.orgID,
		UserID:         s.userID,
	}).Return([]*types.Team{}, nil)
	s.mockStore.EXPECT().ListAccessGrants(gomock.Any(), &store.ListAccessGrantsQuery{
		OrganizationID: s.orgID,
		UserID:         s.userID,
		ResourceID:     projectID,
	}).Return([]*types.AccessGrant{}, nil)
}

func (s *AuthzSessionSuite) expectProject(project *types.Project) {
	s.mockStore.EXPECT().GetProject(gomock.Any(), project.ID).Return(project, nil)
}

// The literal session owner can always write to their session.
func (s *AuthzSessionSuite) TestSessionOwnerAllowed() {
	session := &types.Session{ID: "ses1", Owner: s.userID, OrganizationID: s.orgID, ProjectID: "prj1"}
	user := &types.User{ID: s.userID}

	s.expectOrgMember(types.OrganizationRoleMember)

	err := s.server.authorizeUserToSession(context.Background(), user, session, types.ActionUpdate)
	s.NoError(err)
}

// THE regression: org owner chatting into a session owned by another member
// (e.g. a helix-org Worker "Human Desktop") must be allowed for ActionUpdate.
func (s *AuthzSessionSuite) TestOrgOwnerCanUpdateOthersSession() {
	session := &types.Session{ID: "ses1", Owner: "someone_else", OrganizationID: s.orgID, ProjectID: "prj1"}
	user := &types.User{ID: s.userID}

	s.expectOrgMember(types.OrganizationRoleOwner)

	err := s.server.authorizeUserToSession(context.Background(), user, session, types.ActionUpdate)
	s.NoError(err)
}

// A plain member with a project update grant can drive the session.
func (s *AuthzSessionSuite) TestMemberWithProjectGrantAllowed() {
	session := &types.Session{ID: "ses1", Owner: "someone_else", OrganizationID: s.orgID, ProjectID: "prj1"}
	user := &types.User{ID: s.userID}

	s.expectOrgMember(types.OrganizationRoleMember)
	s.expectProject(&types.Project{ID: "prj1", OrganizationID: s.orgID, UserID: "someone_else"})
	s.expectProjectGrant("prj1", types.ActionUpdate)

	err := s.server.authorizeUserToSession(context.Background(), user, session, types.ActionUpdate)
	s.NoError(err)
}

// A member without a grant on the session's project is denied — ActionUpdate
// keeps read-only members from driving someone else's agent.
func (s *AuthzSessionSuite) TestMemberWithoutGrantDenied() {
	session := &types.Session{ID: "ses1", Owner: "someone_else", OrganizationID: s.orgID, ProjectID: "prj1"}
	user := &types.User{ID: s.userID}

	s.expectOrgMember(types.OrganizationRoleMember)
	s.expectProject(&types.Project{ID: "prj1", OrganizationID: s.orgID, UserID: "someone_else"})
	s.expectNoProjectGrant("prj1")

	err := s.server.authorizeUserToSession(context.Background(), user, session, types.ActionUpdate)
	s.Error(err)
}

func (s *AuthzSessionSuite) TestOrgMembersAccess_AllowsMemberGetAndUpdate() {
	session := &types.Session{ID: "ses1", Owner: "someone_else", OrganizationID: s.orgID, ProjectID: "prj1"}
	user := &types.User{ID: s.userID}
	project := &types.Project{
		ID:             "prj1",
		OrganizationID: s.orgID,
		UserID:         "someone_else",
		Metadata:       types.ProjectMetadata{OrgMembersAccess: true},
	}

	s.expectOrgMember(types.OrganizationRoleMember)
	s.mockStore.EXPECT().GetProject(gomock.Any(), "prj1").Return(project, nil).Times(2)

	s.NoError(s.server.authorizeUserToSession(context.Background(), user, session, types.ActionGet))
	s.NoError(s.server.authorizeUserToSession(context.Background(), user, session, types.ActionUpdate))
}

// A non-member of the org is denied outright.
func (s *AuthzSessionSuite) TestNonMemberDenied() {
	session := &types.Session{ID: "ses1", Owner: "someone_else", OrganizationID: s.orgID, ProjectID: "prj1"}
	user := &types.User{ID: s.userID}

	s.mockStore.EXPECT().GetOrganizationMembership(gomock.Any(), gomock.Any()).
		Return(nil, store.ErrNotFound)

	err := s.server.authorizeUserToSession(context.Background(), user, session, types.ActionUpdate)
	s.Error(err)
}
