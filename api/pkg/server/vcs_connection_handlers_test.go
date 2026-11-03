package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestProjectVCSConnectionsUsesRepositoryOAuthConnection(t *testing.T) {
	for _, test := range []struct {
		name       string
		connection *types.OAuthConnection
		github     *types.GitHub
		state      types.VCSConnectionState
		hasAccess  bool
		verified   bool
	}{
		{name: "missing", state: types.VCSConnectionNeedsAttention, verified: true},
		{name: "empty", connection: &types.OAuthConnection{ID: "github-connection"}, state: types.VCSConnectionNeedsAttention, verified: true},
		{name: "PAT fallback", github: &types.GitHub{PersonalAccessToken: "pat"}, state: types.VCSConnectionVerified, hasAccess: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			projectStore := store.NewMockStore(ctrl)
			server := &HelixAPIServer{Store: projectStore}
			project := &types.Project{ID: "project", UserID: "viewer"}
			repo := &types.GitRepository{
				Name:              "private-repo",
				IsExternal:        true,
				ExternalURL:       "https://github.com/acme/private-repo.git",
				ExternalType:      types.ExternalRepositoryTypeGitHub,
				OAuthConnectionID: "github-connection",
				GitHub:            test.github,
			}

			projectStore.EXPECT().GetProject(gomock.Any(), project.ID).Return(project, nil)
			projectStore.EXPECT().ListGitRepositories(gomock.Any(), &types.ListGitRepositoriesRequest{ProjectID: project.ID}).Return([]*types.GitRepository{repo}, nil)
			if test.connection == nil {
				projectStore.EXPECT().GetOAuthConnection(gomock.Any(), repo.OAuthConnectionID).Return(nil, store.ErrNotFound)
			} else {
				projectStore.EXPECT().GetOAuthConnection(gomock.Any(), repo.OAuthConnectionID).Return(test.connection, nil)
			}

			request := httptest.NewRequest(http.MethodGet, "/api/v1/projects/project/vcs-connections", http.NoBody)
			request = request.WithContext(setRequestUser(request.Context(), types.User{ID: "viewer"}))
			request = mux.SetURLVars(request, map[string]string{"id": project.ID})
			got, httpErr := server.getProjectVCSConnections(nil, request)

			require.Nil(t, httpErr)
			require.Len(t, got, 1)
			require.Equal(t, test.state, got[0].State)
			require.Nil(t, got[0].PushingAs)
			require.Equal(t, []types.VCSRepoAccess{{Repo: "acme/private-repo", HasAccess: test.hasAccess, Verified: test.verified}}, got[0].Repos)
		})
	}
}

func TestProjectVCSConnectionsSelfHealsDanglingOAuthPin(t *testing.T) {
	ctrl := gomock.NewController(t)
	projectStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: projectStore}
	project := &types.Project{ID: "project", UserID: "viewer"}
	repo := &types.GitRepository{
		Name:              "private-repo",
		OwnerID:           "repo-owner",
		IsExternal:        true,
		ExternalURL:       "https://github.com/private-repo",
		ExternalType:      types.ExternalRepositoryTypeGitHub,
		OAuthConnectionID: "deleted-connection",
	}
	connections := []*types.OAuthConnection{
		{ID: "older", UserID: repo.OwnerID, AccessToken: "older-token", UpdatedAt: time.Unix(1, 0), Provider: types.OAuthProvider{Type: types.OAuthProviderTypeGitHub}},
		{ID: "newest", UserID: repo.OwnerID, AccessToken: "newest-token", UpdatedAt: time.Unix(2, 0), ProviderUsername: "chocobar", Provider: types.OAuthProvider{Type: types.OAuthProviderTypeGitHub}},
	}

	projectStore.EXPECT().GetProject(gomock.Any(), project.ID).Return(project, nil)
	projectStore.EXPECT().ListGitRepositories(gomock.Any(), &types.ListGitRepositoriesRequest{ProjectID: project.ID}).Return([]*types.GitRepository{repo}, nil)
	projectStore.EXPECT().GetOAuthConnection(gomock.Any(), repo.OAuthConnectionID).Return(nil, store.ErrNotFound)
	projectStore.EXPECT().ListOAuthConnections(gomock.Any(), &store.ListOAuthConnectionsQuery{UserID: repo.OwnerID}).Return(connections, nil)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/projects/project/vcs-connections", http.NoBody)
	request = request.WithContext(setRequestUser(request.Context(), types.User{ID: "viewer"}))
	request = mux.SetURLVars(request, map[string]string{"id": project.ID})
	got, httpErr := server.getProjectVCSConnections(nil, request)

	require.Nil(t, httpErr)
	require.Len(t, got, 1)
	require.Equal(t, types.VCSConnectionVerified, got[0].State)
	require.NotNil(t, got[0].PushingAs)
	require.Equal(t, "newest", got[0].PushingAs.ConnectionID)
	require.Equal(t, "@chocobar", got[0].PushingAs.Username)
}

func TestProjectVCSConnectionsPreservesPinnedAndUnpinnedRepositories(t *testing.T) {
	ctrl := gomock.NewController(t)
	projectStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: projectStore}
	project := &types.Project{ID: "project", UserID: "viewer"}
	repos := []*types.GitRepository{
		{
			Name:              "private-repo",
			IsExternal:        true,
			ExternalURL:       "https://github.com/private-repo",
			ExternalType:      types.ExternalRepositoryTypeGitHub,
			OAuthConnectionID: "github-connection",
		},
		{
			Name:         "public-or-pat-repo",
			IsExternal:   true,
			ExternalURL:  "https://gitlab.com/acme/public-or-pat-repo.git",
			ExternalType: types.ExternalRepositoryTypeGitLab,
			GitLab:       &types.GitLab{PersonalAccessToken: "pat"},
		},
		{
			Name:         "public-repo",
			IsExternal:   true,
			ExternalURL:  "https://gitlab.com/acme/public-repo.git",
			ExternalType: types.ExternalRepositoryTypeGitLab,
		},
	}
	connection := &types.OAuthConnection{
		ID:               "github-connection",
		AccessToken:      "github-token",
		ProviderUsername: "chocobar",
	}

	projectStore.EXPECT().GetProject(gomock.Any(), project.ID).Return(project, nil)
	projectStore.EXPECT().ListGitRepositories(gomock.Any(), &types.ListGitRepositoriesRequest{ProjectID: project.ID}).Return(repos, nil)
	projectStore.EXPECT().GetOAuthConnection(gomock.Any(), repos[0].OAuthConnectionID).Return(connection, nil)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/projects/project/vcs-connections", http.NoBody)
	request = request.WithContext(setRequestUser(request.Context(), types.User{ID: "viewer"}))
	request = mux.SetURLVars(request, map[string]string{"id": project.ID})
	got, httpErr := server.getProjectVCSConnections(nil, request)

	require.Nil(t, httpErr)
	require.Len(t, got, 2)
	require.Equal(t, types.VCSConnectionVerified, got[0].State)
	require.NotNil(t, got[0].PushingAs)
	require.Equal(t, "@chocobar", got[0].PushingAs.Username)
	require.Equal(t, types.VCSConnectionVerified, got[1].State)
	require.Equal(t, []types.VCSRepoAccess{
		{Repo: "acme/public-or-pat-repo", HasAccess: true, Verified: false},
		{Repo: "acme/public-repo", HasAccess: true, Verified: false},
	}, got[1].Repos)
}

func TestRepositoryStaticCredentialState(t *testing.T) {
	for _, test := range []struct {
		name       string
		repo       *types.GitRepository
		configured bool
		usable     bool
	}{
		{name: "public", repo: &types.GitRepository{ExternalType: types.ExternalRepositoryTypeGitHub}},
		{name: "PAT", repo: &types.GitRepository{ExternalType: types.ExternalRepositoryTypeGitHub, GitHub: &types.GitHub{PersonalAccessToken: "pat"}}, configured: true, usable: true},
		{name: "missing saved PAT", repo: &types.GitRepository{ExternalType: types.ExternalRepositoryTypeGitHub, GitProviderConnectionID: "connection"}, configured: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			configured, usable := repositoryStaticCredentialState(test.repo)
			require.Equal(t, test.configured, configured)
			require.Equal(t, test.usable, usable)
		})
	}
}
