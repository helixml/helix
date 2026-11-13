package services

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

func taskConfinedKeyFixture(t *testing.T, confined bool) (*SpecDrivenTaskService, *store.MockStore) {
	t.Helper()
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().GetSession(gomock.Any(), "ses_finder").Return(&types.Session{
		ID: "ses_finder", ProjectID: "prj_a", Owner: "usr_owner",
		Metadata: types.SessionMetadata{SpecTaskID: "spt_finder", ProjectID: "prj_a"},
	}, nil)
	mockStore.EXPECT().GetSpecTask(gomock.Any(), "spt_finder").Return(&types.SpecTask{
		ID: "spt_finder", ProjectID: "prj_a", ConfineAgentKey: confined,
	}, nil)
	mockStore.EXPECT().GetAPIKey(gomock.Any(), &types.ApiKey{
		OrganizationID: "org_a", Owner: "usr_owner", OwnerType: types.OwnerTypeUser,
		SessionID: "ses_finder", Type: types.APIkeytypeAPI, ProjectID: "prj_a",
	}).DoAndReturn(func(context.Context, *types.ApiKey) (*types.ApiKey, error) {
		return nil, store.ErrNotFound
	}).MaxTimes(1)
	return &SpecDrivenTaskService{store: mockStore}, mockStore
}

func mintFinderKey(t *testing.T, svc *SpecDrivenTaskService) (string, error) {
	t.Helper()
	return svc.GetOrCreateSessionAPIKey(context.Background(), &SessionAPIKeyRequest{
		OrganizationID: "org_a", UserID: "usr_owner", SessionID: "ses_finder",
	})
}

func expectCreatedKey(mockStore *store.MockStore) *types.ApiKey {
	created := &types.ApiKey{}
	mockStore.EXPECT().CreateAPIKey(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, key *types.ApiKey) (*types.ApiKey, error) {
			*created = *key
			return key, nil
		})
	return created
}

// A task created with confine_agent_key gets keys that carry the flag.
func TestGetOrCreateSessionAPIKey_ConfinedTaskMintsConfinedKey(t *testing.T) {
	svc, mockStore := taskConfinedKeyFixture(t, true)
	created := expectCreatedKey(mockStore)

	_, err := mintFinderKey(t, svc)
	require.NoError(t, err)
	require.True(t, created.TaskConfined)
	require.Equal(t, "spt_finder", created.SpecTaskID)
	require.Equal(t, "prj_a", created.ProjectID)
}

// An ordinary task's keys stay project-scoped.
func TestGetOrCreateSessionAPIKey_UnconfinedTaskMintsProjectKey(t *testing.T) {
	svc, mockStore := taskConfinedKeyFixture(t, false)
	created := expectCreatedKey(mockStore)

	_, err := mintFinderKey(t, svc)
	require.NoError(t, err)
	require.False(t, created.TaskConfined)
}

// A re-minted key for a confined task keeps the flag: an existing key
// without it (GORM ignores a false bool in the lookup) is replaced, never
// handed out.
func TestGetOrCreateSessionAPIKey_ConfinedTaskReplacesUnconfinedKey(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().GetSession(gomock.Any(), "ses_finder").Return(&types.Session{
		ID: "ses_finder", ProjectID: "prj_a", Owner: "usr_owner",
		Metadata: types.SessionMetadata{SpecTaskID: "spt_finder"},
	}, nil)
	mockStore.EXPECT().GetSpecTask(gomock.Any(), "spt_finder").Return(&types.SpecTask{
		ID: "spt_finder", ProjectID: "prj_a", ConfineAgentKey: true,
	}, nil)
	mockStore.EXPECT().GetAPIKey(gomock.Any(), gomock.Any()).Return(&types.ApiKey{
		Key: "hl-old", SessionID: "ses_finder", ProjectID: "prj_a", SpecTaskID: "spt_finder",
	}, nil)
	mockStore.EXPECT().DeleteAPIKey(gomock.Any(), "hl-old").Return(nil)
	created := expectCreatedKey(mockStore)

	key, err := mintFinderKey(t, &SpecDrivenTaskService{store: mockStore})
	require.NoError(t, err)
	require.NotEqual(t, "hl-old", key)
	require.True(t, created.TaskConfined)
}

// An existing confined key is reused as it is.
func TestGetOrCreateSessionAPIKey_ConfinedTaskReusesConfinedKey(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().GetSession(gomock.Any(), "ses_finder").Return(&types.Session{
		ID: "ses_finder", ProjectID: "prj_a", Metadata: types.SessionMetadata{SpecTaskID: "spt_finder"},
	}, nil)
	mockStore.EXPECT().GetSpecTask(gomock.Any(), "spt_finder").Return(&types.SpecTask{
		ID: "spt_finder", ProjectID: "prj_a", ConfineAgentKey: true,
	}, nil)
	mockStore.EXPECT().GetAPIKey(gomock.Any(), gomock.Any()).Return(&types.ApiKey{
		Key: "hl-confined", TaskConfined: true,
	}, nil)

	key, err := mintFinderKey(t, &SpecDrivenTaskService{store: mockStore})
	require.NoError(t, err)
	require.Equal(t, "hl-confined", key)
}

// Without the task the confinement is unknown: no key is minted.
func TestGetOrCreateSessionAPIKey_TaskLookupFailureMintsNothing(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().GetSession(gomock.Any(), "ses_finder").Return(&types.Session{
		ID: "ses_finder", ProjectID: "prj_a", Metadata: types.SessionMetadata{SpecTaskID: "spt_finder"},
	}, nil)
	mockStore.EXPECT().GetSpecTask(gomock.Any(), "spt_finder").Return(nil, errors.New("db down"))

	_, err := mintFinderKey(t, &SpecDrivenTaskService{store: mockStore})
	require.Error(t, err)
}

// The flag on the create request reaches the stored task.
func TestCreateTaskFromPrompt_PersistsConfineAgentKey(t *testing.T) {
	require.True(t, createdTaskConfinement(t, true))
	require.False(t, createdTaskConfinement(t, false))
}

func createdTaskConfinement(t *testing.T, confine bool) bool {
	t.Helper()
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	service := NewSpecDrivenTaskService(mockStore, nil, "test-helix-agent", nil, nil, nil, nil, nil, NewDisabledKoditService())
	service.SetTestMode(true)
	config := &types.CodeAgentExecutionConfig{
		Runtime: types.CodeAgentRuntimeCodexCLI, CredentialType: types.CodeAgentCredentialTypeSubscription,
		Model: "gpt-5.6-sol", ReasoningEffort: "high",
	}
	mockStore.EXPECT().GetProject(gomock.Any(), "prj_a").Return(&types.Project{ID: "prj_a", CodeAgentConfig: config}, nil)
	mockStore.EXPECT().IncrementGlobalTaskNumber(gomock.Any()).Return(1, nil)
	var stored *types.SpecTask
	mockStore.EXPECT().CreateSpecTask(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, task *types.SpecTask) error {
		stored = task
		return nil
	})
	_, err := service.CreateTaskFromPrompt(context.Background(), &types.CreateTaskRequest{
		ProjectID: "prj_a", Prompt: "find bugs", Type: "feature", Priority: types.SpecTaskPriorityHigh,
		UserID: "usr_owner", CodeAgentConfig: config, ConfineAgentKey: confine,
	}, testUploader)
	require.NoError(t, err)
	return stored.ConfineAgentKey
}
