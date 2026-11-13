package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

func TestSessionKeyScope(t *testing.T) {
	for name, tc := range map[string]struct {
		session     types.Session
		wantProject string
		wantTask    string
	}{
		"spec task": {
			session:     types.Session{ProjectID: "prj_a", Metadata: types.SessionMetadata{SpecTaskID: "spt_a"}},
			wantProject: "prj_a", wantTask: "spt_a",
		},
		"bot instance": {
			session:     types.Session{ProjectID: "prj_a", Metadata: types.SessionMetadata{SessionRole: types.SessionRoleOrgBotInstance}},
			wantProject: "prj_a",
		},
		"project exploratory desktop": {
			session:     types.Session{ProjectID: "prj_a", Metadata: types.SessionMetadata{SessionRole: "exploratory", ProjectID: "prj_a"}},
			wantProject: "prj_a",
		},
		"exploratory desktop with the project only in metadata": {
			session:     types.Session{Metadata: types.SessionMetadata{SessionRole: "exploratory", ProjectID: "prj_a"}},
			wantProject: "prj_a",
		},
		"helix-org worker keeps no project": {
			session: types.Session{ProjectID: "prj_a", Metadata: types.SessionMetadata{SessionRole: "exploratory", OrgWorkerID: "chief-of-staff"}},
		},
		"helix-org worker outside a project": {
			session: types.Session{Metadata: types.SessionMetadata{OrgWorkerID: "chief-of-staff"}},
		},
		// Sessions in a project with no spec task or exploratory role keep
		// their project: their desktops clone its repositories.
		"golden build": {
			session:     types.Session{ProjectID: "prj_a", Metadata: types.SessionMetadata{ProjectID: "prj_a"}},
			wantProject: "prj_a",
		},
		"recurring task job": {
			session:     types.Session{ProjectID: "prj_a", Metadata: types.SessionMetadata{SessionRole: "job", ProjectID: "prj_a"}},
			wantProject: "prj_a",
		},
		"project chat with an external agent and no role": {
			session:     types.Session{ProjectID: "prj_a"},
			wantProject: "prj_a",
		},
		"plain chat": {
			session: types.Session{},
		},
	} {
		t.Run(name, func(t *testing.T) {
			project, task := sessionKeyScope(&tc.session)
			require.Equal(t, tc.wantProject, project)
			require.Equal(t, tc.wantTask, task)
		})
	}
}

// A project's exploratory desktop gets a key confined to its project, and a
// key minted for it before that (with no project) is not reused.
func TestGetOrCreateSessionAPIKey_ExploratoryDesktopKeyCarriesProject(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	svc := &SpecDrivenTaskService{store: mockStore}

	mockStore.EXPECT().GetSession(gomock.Any(), "ses_explore").Return(&types.Session{
		ID: "ses_explore", ProjectID: "prj_a", Owner: "usr_owner",
		Metadata: types.SessionMetadata{SessionRole: "exploratory", ProjectID: "prj_a"},
	}, nil)
	mockStore.EXPECT().GetAPIKey(gomock.Any(), &types.ApiKey{
		OrganizationID: "org_a", Owner: "usr_owner", OwnerType: types.OwnerTypeUser,
		SessionID: "ses_explore", Type: types.APIkeytypeAPI, ProjectID: "prj_a",
	}).Return(nil, store.ErrNotFound)
	var created *types.ApiKey
	mockStore.EXPECT().CreateAPIKey(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, key *types.ApiKey) (*types.ApiKey, error) {
			created = key
			return key, nil
		})

	_, err := svc.GetOrCreateSessionAPIKey(context.Background(), &SessionAPIKeyRequest{
		OrganizationID: "org_a", UserID: "usr_owner", SessionID: "ses_explore",
	})
	require.NoError(t, err)
	require.Equal(t, "prj_a", created.ProjectID)
	require.Equal(t, "ses_explore", created.SessionID)
	require.Empty(t, created.SpecTaskID)
}
