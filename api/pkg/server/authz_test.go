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

// --- No org: owner-only access ---

func (s *AuthzRepositorySuite) TestNoOrg_OwnerAllowed() {
	repo := &types.GitRepository{
		ID:      "repo1",
		OwnerID: s.userID,
		// OrganizationID is empty
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

// --- With org: owner with membership ---

func (s *AuthzRepositorySuite) TestWithOrg_RepoOwnerAllowed() {
	repo := &types.GitRepository{
		ID:             "repo1",
		OwnerID:        s.userID,
		OrganizationID: s.orgID,
	}
	user := &types.User{ID: s.userID}

	s.mockStore.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: s.orgID,
		UserID:         s.userID,
	}).Return(&types.OrganizationMembership{
		OrganizationID: s.orgID,
		UserID:         s.userID,
		Role:           types.OrganizationRoleMember,
	}, nil)

	err := s.server.authorizeUserToRepository(context.Background(), user, repo, types.ActionGet)
	s.NoError(err)
}

// --- With org: org owner gets access ---

func (s *AuthzRepositorySuite) TestWithOrg_OrgOwnerAllowed() {
	repo := &types.GitRepository{
		ID:             "repo1",
		OwnerID:        "someone_else",
		OrganizationID: s.orgID,
	}
	user := &types.User{ID: s.userID}

	s.mockStore.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: s.orgID,
		UserID:         s.userID,
	}).Return(&types.OrganizationMembership{
		OrganizationID: s.orgID,
		UserID:         s.userID,
		Role:           types.OrganizationRoleOwner,
	}, nil)

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

	s.mockStore.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: s.orgID,
		UserID:         s.userID,
	}).Return(nil, store.ErrNotFound)

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

	// User is org member
	s.mockStore.EXPECT().GetOrganizationMembership(gomock.Any(), gomock.Any()).Return(&types.OrganizationMembership{
		OrganizationID: s.orgID,
		UserID:         s.userID,
		Role:           types.OrganizationRoleMember,
	}, nil)

	// Direct access grant on the repo resource
	s.mockStore.EXPECT().ListTeams(gomock.Any(), gomock.Any()).Return([]*types.Team{}, nil)
	s.mockStore.EXPECT().ListAccessGrants(gomock.Any(), &store.ListAccessGrantsQuery{
		OrganizationID: s.orgID,
		UserID:         s.userID,
		ResourceID:     "repo1",
		TeamIDs:        nil,
	}).Return([]*types.AccessGrant{
		{
			ID:         "grant1",
			ResourceID: "repo1",
			UserID:     s.userID,
			Roles: []types.Role{
				{
					Config: types.Config{
						Rules: []types.Rule{
							{
								Resources: []types.Resource{types.ResourceGitRepository},
								Actions:   []types.Action{types.ActionGet},
								Effect:    types.EffectAllow,
							},
						},
					},
				},
			},
		},
	}, nil)

	// After direct access succeeds, it proceeds to project check
	s.mockStore.EXPECT().ListProjects(gomock.Any(), &store.ListProjectsQuery{
		OrganizationID: s.orgID,
		UserID:         s.userID,
	}).Return([]*types.Project{}, nil)

	err := s.server.authorizeUserToRepository(context.Background(), user, repo, types.ActionGet)
	s.NoError(err)
}

// --- With org: no direct access, returns error before project check ---

func (s *AuthzRepositorySuite) TestWithOrg_NoDirectAccess_DeniedBeforeProjectCheck() {
	repo := &types.GitRepository{
		ID:             "repo1",
		OwnerID:        "someone_else",
		OrganizationID: s.orgID,
	}
	user := &types.User{ID: s.userID}

	// User is org member
	s.mockStore.EXPECT().GetOrganizationMembership(gomock.Any(), gomock.Any()).Return(&types.OrganizationMembership{
		OrganizationID: s.orgID,
		UserID:         s.userID,
		Role:           types.OrganizationRoleMember,
	}, nil)

	// No direct access grant
	s.mockStore.EXPECT().ListTeams(gomock.Any(), gomock.Any()).Return([]*types.Team{}, nil)
	s.mockStore.EXPECT().ListAccessGrants(gomock.Any(), gomock.Any()).Return([]*types.AccessGrant{}, nil)

	// authorizeUserToResource returns "not authorized" -> function returns immediately
	// ListProjects is NOT called because err != nil on line 236

	err := s.server.authorizeUserToRepository(context.Background(), user, repo, types.ActionGet)
	s.Error(err)
	s.Contains(err.Error(), "not authorized")
}

// --- With org: direct access + project-based access via attached repository ---

func (s *AuthzRepositorySuite) TestWithOrg_AccessViaProjectRepository() {
	repo := &types.GitRepository{
		ID:             "repo1",
		OwnerID:        "someone_else",
		OrganizationID: s.orgID,
	}
	user := &types.User{ID: s.userID}

	// User is org member
	s.mockStore.EXPECT().GetOrganizationMembership(gomock.Any(), gomock.Any()).Return(&types.OrganizationMembership{
		OrganizationID: s.orgID,
		UserID:         s.userID,
		Role:           types.OrganizationRoleMember,
	}, nil)

	// Direct access grant on repo succeeds (required for project check to be reached)
	s.mockStore.EXPECT().ListTeams(gomock.Any(), gomock.Any()).Return([]*types.Team{}, nil).Times(2)
	s.mockStore.EXPECT().ListAccessGrants(gomock.Any(), &store.ListAccessGrantsQuery{
		OrganizationID: s.orgID,
		UserID:         s.userID,
		ResourceID:     "repo1",
		TeamIDs:        nil,
	}).Return([]*types.AccessGrant{
		{
			Roles: []types.Role{{
				Config: types.Config{Rules: []types.Rule{{
					Resources: []types.Resource{types.ResourceGitRepository},
					Actions:   []types.Action{types.ActionGet},
					Effect:    types.EffectAllow,
				}}},
			}},
		},
	}, nil)

	// User has a project with the repo attached
	s.mockStore.EXPECT().ListProjects(gomock.Any(), &store.ListProjectsQuery{
		OrganizationID: s.orgID,
		UserID:         s.userID,
	}).Return([]*types.Project{
		{
			ID:             "proj1",
			OrganizationID: s.orgID,
			UserID:         s.userID,
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

	// Project-level authz check
	s.mockStore.EXPECT().ListAccessGrants(gomock.Any(), &store.ListAccessGrantsQuery{
		OrganizationID: s.orgID,
		UserID:         s.userID,
		ResourceID:     "proj1",
		TeamIDs:        nil,
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

	err := s.server.authorizeUserToRepository(context.Background(), user, repo, types.ActionGet)
	s.NoError(err)
}

// --- Admin bypasses org membership check ---

func (s *AuthzRepositorySuite) TestWithOrg_AdminAllowed() {
	repo := &types.GitRepository{
		ID:             "repo1",
		OwnerID:        "someone_else",
		OrganizationID: s.orgID,
	}
	user := &types.User{ID: s.userID, Admin: true}

	// Admin gets temporary owner membership
	s.mockStore.EXPECT().GetOrganizationMembership(gomock.Any(), gomock.Any()).
		Return(nil, store.ErrNotFound)

	err := s.server.authorizeUserToRepository(context.Background(), user, repo, types.ActionGet)
	s.NoError(err) // Admin gets org owner role -> allowed
}
