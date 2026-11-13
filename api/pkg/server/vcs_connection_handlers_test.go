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

func TestProjectVCSConnectionsUsesRepositoryCredentials(t *testing.T) {
	for _, test := range []struct {
		name        string
		connection  *types.OAuthConnection
		github      *types.GitHub
		externalURL string
		state       types.VCSConnectionState
		hasAccess   bool
		verified    bool
	}{
		{name: "missing pinned OAuth", state: types.VCSConnectionNeedsAttention, verified: true},
		{name: "empty pinned OAuth", connection: &types.OAuthConnection{ID: "github-connection"}, state: types.VCSConnectionNeedsAttention, verified: true},
		{name: "PAT fallback", github: &types.GitHub{PersonalAccessToken: "pat"}, state: types.VCSConnectionVerified, hasAccess: true},
		{name: "legacy URL fallback", externalURL: "https://legacy:token@github.com/acme/private-repo.git", state: types.VCSConnectionVerified, hasAccess: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			projectStore := store.NewMockStore(ctrl)
			server := &HelixAPIServer{Store: projectStore}
			project := &types.Project{ID: "project", UserID: "viewer"}
			externalURL := test.externalURL
			if externalURL == "" {
				externalURL = "https://github.com/acme/private-repo.git"
			}
			repo := &types.GitRepository{
				Name: "private-repo", IsExternal: true, ExternalURL: externalURL,
				ExternalType: types.ExternalRepositoryTypeGitHub, OAuthConnectionID: "github-connection", GitHub: test.github,
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

func TestProjectVCSConnectionsSelfHealsRepositoryOAuth(t *testing.T) {
	ctrl := gomock.NewController(t)
	projectStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: projectStore}
	project := &types.Project{ID: "project", UserID: "viewer"}
	repo := &types.GitRepository{
		Name: "private-repo", OwnerID: "repo-owner", IsExternal: true, ExternalURL: "https://github.com/private-repo",
		ExternalType: types.ExternalRepositoryTypeGitHub, OAuthConnectionID: "deleted-connection",
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
	require.Equal(t, "newest", got[0].PushingAs.ConnectionID)
	require.Equal(t, "@chocobar", got[0].PushingAs.Username)
}

func TestProjectVCSConnectionsPreservesAnonymousAndStaticRepositories(t *testing.T) {
	ctrl := gomock.NewController(t)
	projectStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: projectStore}
	project := &types.Project{ID: "project", UserID: "viewer"}
	repos := []*types.GitRepository{
		{Name: "pat-repo", IsExternal: true, ExternalURL: "https://gitlab.com/acme/pat-repo.git", ExternalType: types.ExternalRepositoryTypeGitLab, GitLab: &types.GitLab{PersonalAccessToken: "pat"}},
		{Name: "public-repo", IsExternal: true, ExternalURL: "https://gitlab.com/acme/public-repo.git", ExternalType: types.ExternalRepositoryTypeGitLab},
	}

	projectStore.EXPECT().GetProject(gomock.Any(), project.ID).Return(project, nil)
	projectStore.EXPECT().ListGitRepositories(gomock.Any(), &types.ListGitRepositoriesRequest{ProjectID: project.ID}).Return(repos, nil)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/projects/project/vcs-connections", http.NoBody)
	request = request.WithContext(setRequestUser(request.Context(), types.User{ID: "viewer"}))
	request = mux.SetURLVars(request, map[string]string{"id": project.ID})
	got, httpErr := server.getProjectVCSConnections(nil, request)

	require.Nil(t, httpErr)
	require.Len(t, got, 1)
	require.Equal(t, types.VCSConnectionVerified, got[0].State)
	require.Equal(t, []types.VCSRepoAccess{
		{Repo: "acme/pat-repo", HasAccess: true},
		{Repo: "acme/public-repo", HasAccess: true},
	}, got[0].Repos)
}

func TestProjectVCSConnectionsRejectsUnavailableStaticCredential(t *testing.T) {
	ctrl := gomock.NewController(t)
	projectStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: projectStore}
	project := &types.Project{ID: "project", UserID: "viewer"}
	repo := &types.GitRepository{
		Name: "private-repo", IsExternal: true, ExternalURL: "https://bitbucket.org/acme/private-repo.git",
		ExternalType: types.ExternalRepositoryTypeBitbucket, GitProviderConnectionID: "missing",
		Bitbucket: &types.Bitbucket{Username: "partial"},
	}

	projectStore.EXPECT().GetProject(gomock.Any(), project.ID).Return(project, nil)
	projectStore.EXPECT().ListGitRepositories(gomock.Any(), &types.ListGitRepositoriesRequest{ProjectID: project.ID}).Return([]*types.GitRepository{repo}, nil)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/projects/project/vcs-connections", http.NoBody)
	request = request.WithContext(setRequestUser(request.Context(), types.User{ID: "viewer"}))
	request = mux.SetURLVars(request, map[string]string{"id": project.ID})
	got, httpErr := server.getProjectVCSConnections(nil, request)

	require.Nil(t, httpErr)
	require.Len(t, got, 1)
	require.Equal(t, types.VCSConnectionNeedsAttention, got[0].State)
	require.Equal(t, []types.VCSRepoAccess{{Repo: "acme/private-repo", Verified: true}}, got[0].Repos)
}
