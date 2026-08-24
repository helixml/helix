package services

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func legacyCodingApp(kind string) *types.App {
	return &types.App{
		ID:        "app-legacy",
		AgentKind: kind,
		Config: types.AppConfig{Helix: types.AppHelixConfig{Assistants: []types.AssistantConfig{{
			AgentType:               types.AgentTypeZedExternal,
			CodeAgentRuntime:        types.CodeAgentRuntimeCodexCLI,
			CodeAgentCredentialType: types.CodeAgentCredentialTypeAPIKey,
			Provider:                "provider-old",
			Model:                   "model-old",
			ReasoningEffort:         "medium",
		}}}},
	}
}

func TestMigrateSpecTaskCodeAgentConfigMaterializesAndClearsLegacyIDs(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	service := &SpecDrivenTaskService{store: mockStore}
	project := &types.Project{ID: "project-1", DefaultHelixAppID: "app-legacy"}
	task := &types.SpecTask{
		ID: "task-1", ProjectID: project.ID, HelixAppID: "app-legacy", PlanningSessionID: "session-1",
		CodeAgentOverrides: &types.CodeAgentOverrides{
			ProviderRef: "provider-new", Model: "gpt-5.6-sol", ReasoningEffort: "xhigh", ServiceTier: "fast",
		},
	}
	app := legacyCodingApp(types.AgentKindCoding)
	session := &types.Session{
		ID:        "session-1",
		ParentApp: "app-legacy",
		Metadata: types.SessionMetadata{
			CodeAgentRuntime:   types.CodeAgentRuntimeQwenCode,
			CodeAgentOverrides: &types.CodeAgentOverrides{Model: "session-model"},
		},
	}

	mockStore.EXPECT().GetApp(gomock.Any(), "app-legacy").Return(app, nil).Times(2)
	mockStore.EXPECT().UpdateProject(gomock.Any(), project).DoAndReturn(func(_ context.Context, got *types.Project) error {
		require.Empty(t, got.DefaultHelixAppID)
		require.NotNil(t, got.CodeAgentConfig)
		return nil
	})
	mockStore.EXPECT().UpdateSpecTask(gomock.Any(), task).DoAndReturn(func(_ context.Context, got *types.SpecTask) error {
		require.Empty(t, got.HelixAppID)
		require.Nil(t, got.CodeAgentOverrides)
		require.Equal(t, "provider-new", got.CodeAgentConfig.ProviderRef)
		require.Equal(t, "gpt-5.6-sol", got.CodeAgentConfig.Model)
		require.Equal(t, "xhigh", got.CodeAgentConfig.ReasoningEffort)
		require.Equal(t, "fast", got.CodeAgentConfig.ServiceTier)
		return nil
	})
	mockStore.EXPECT().GetSession(gomock.Any(), "session-1").Return(session, nil)
	mockStore.EXPECT().UpdateSession(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, got types.Session) (*types.Session, error) {
		require.Empty(t, got.ParentApp)
		require.Nil(t, got.Metadata.CodeAgentOverrides)
		require.Equal(t, types.CodeAgentRuntimeCodexCLI, got.Metadata.CodeAgentRuntime)
		return &got, nil
	})

	require.NoError(t, service.migrateSpecTaskCodeAgentConfig(context.Background(), task, project))
}

func TestMigrateSpecTaskCodeAgentConfigRetainsOrgAgentProjectLink(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	service := &SpecDrivenTaskService{store: mockStore}
	project := &types.Project{ID: "worker-project", DefaultHelixAppID: "app-legacy"}
	task := &types.SpecTask{ID: "task-1", ProjectID: project.ID}
	app := legacyCodingApp(types.AgentKindOrg)

	mockStore.EXPECT().GetApp(gomock.Any(), "app-legacy").Return(app, nil)
	mockStore.EXPECT().UpdateProject(gomock.Any(), project).DoAndReturn(func(_ context.Context, got *types.Project) error {
		require.Equal(t, "app-legacy", got.DefaultHelixAppID)
		return nil
	})
	mockStore.EXPECT().UpdateSpecTask(gomock.Any(), task).DoAndReturn(func(_ context.Context, got *types.SpecTask) error {
		require.Empty(t, got.HelixAppID)
		require.NotNil(t, got.CodeAgentConfig)
		return nil
	})

	require.NoError(t, service.migrateSpecTaskCodeAgentConfig(context.Background(), task, project))
}

func TestMigrateSpecTaskCodeAgentConfigIsIdempotent(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	service := &SpecDrivenTaskService{store: mockStore}
	config := &types.CodeAgentExecutionConfig{
		Runtime: types.CodeAgentRuntimeClaudeCode, CredentialType: types.CodeAgentCredentialTypeSubscription,
		Model: "claude-opus-5",
	}
	project := &types.Project{ID: "project-1", CodeAgentConfig: config}
	task := &types.SpecTask{ID: "task-1", ProjectID: project.ID, CodeAgentConfig: config}

	require.NoError(t, service.migrateSpecTaskCodeAgentConfig(context.Background(), task, project))
}

func TestMigrateSpecTaskCodeAgentConfigClearsProjectAppAfterPartialMigration(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	service := &SpecDrivenTaskService{store: mockStore}
	config := &types.CodeAgentExecutionConfig{
		Runtime: types.CodeAgentRuntimeCodexCLI, CredentialType: types.CodeAgentCredentialTypeSubscription, Model: "gpt-5.6-sol",
	}
	project := &types.Project{ID: "project-1", DefaultHelixAppID: "app-legacy", CodeAgentConfig: config}
	task := &types.SpecTask{ID: "task-1", ProjectID: project.ID, CodeAgentConfig: config}
	mockStore.EXPECT().GetApp(gomock.Any(), "app-legacy").Return(&types.App{
		ID: "app-legacy", AgentKind: types.AgentKindCoding,
	}, nil)
	mockStore.EXPECT().UpdateProject(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, got *types.Project) error {
		require.Empty(t, got.DefaultHelixAppID)
		return nil
	})

	require.NoError(t, service.migrateSpecTaskCodeAgentConfig(context.Background(), task, project))
}

func TestMigrateSpecTaskCodeAgentConfigRecoversTaskConfigFromMissingProjectApp(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	service := &SpecDrivenTaskService{store: mockStore}
	config := &types.CodeAgentExecutionConfig{
		Runtime: types.CodeAgentRuntimeCodexCLI, CredentialType: types.CodeAgentCredentialTypeSubscription, Model: "gpt-5.6-sol",
	}
	project := &types.Project{ID: "project-1", DefaultHelixAppID: "app-missing"}
	task := &types.SpecTask{ID: "task-1", ProjectID: project.ID, CodeAgentConfig: config}

	mockStore.EXPECT().GetApp(gomock.Any(), "app-missing").Return(nil, store.ErrNotFound)
	mockStore.EXPECT().UpdateProject(gomock.Any(), project).DoAndReturn(func(_ context.Context, got *types.Project) error {
		require.Empty(t, got.DefaultHelixAppID)
		return nil
	})

	require.NoError(t, service.migrateSpecTaskCodeAgentConfig(context.Background(), task, project))
	require.Equal(t, config, task.CodeAgentConfig)
}

func TestMigrateSpecTaskCodeAgentConfigRecoversProjectConfigFromMissingProjectApp(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	service := &SpecDrivenTaskService{store: mockStore}
	config := &types.CodeAgentExecutionConfig{
		Runtime: types.CodeAgentRuntimeCodexCLI, CredentialType: types.CodeAgentCredentialTypeSubscription, Model: "gpt-5.6-sol",
	}
	project := &types.Project{ID: "project-1", DefaultHelixAppID: "app-missing", CodeAgentConfig: config}
	task := &types.SpecTask{ID: "task-1", ProjectID: project.ID}

	mockStore.EXPECT().GetApp(gomock.Any(), "app-missing").Return(nil, store.ErrNotFound)
	mockStore.EXPECT().UpdateProject(gomock.Any(), project).DoAndReturn(func(_ context.Context, got *types.Project) error {
		require.Empty(t, got.DefaultHelixAppID)
		return nil
	})
	mockStore.EXPECT().UpdateSpecTask(gomock.Any(), task).DoAndReturn(func(_ context.Context, got *types.SpecTask) error {
		require.Equal(t, config, got.CodeAgentConfig)
		return nil
	})

	require.NoError(t, service.migrateSpecTaskCodeAgentConfig(context.Background(), task, project))
}

func TestMigrateSpecTaskCodeAgentConfigRecoversLegacyTaskAppFromMissingProjectApp(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	service := &SpecDrivenTaskService{store: mockStore}
	project := &types.Project{ID: "project-1", DefaultHelixAppID: "app-missing"}
	task := &types.SpecTask{ID: "task-1", ProjectID: project.ID, HelixAppID: "app-legacy"}
	app := legacyCodingApp(types.AgentKindCoding)

	mockStore.EXPECT().GetApp(gomock.Any(), "app-missing").Return(nil, store.ErrNotFound)
	mockStore.EXPECT().UpdateProject(gomock.Any(), project).DoAndReturn(func(_ context.Context, got *types.Project) error {
		require.Empty(t, got.DefaultHelixAppID)
		return nil
	})
	mockStore.EXPECT().GetApp(gomock.Any(), "app-legacy").Return(app, nil)
	mockStore.EXPECT().UpdateSpecTask(gomock.Any(), task).DoAndReturn(func(_ context.Context, got *types.SpecTask) error {
		require.Empty(t, got.HelixAppID)
		require.NotNil(t, got.CodeAgentConfig)
		return nil
	})

	require.NoError(t, service.migrateSpecTaskCodeAgentConfig(context.Background(), task, project))
}

func TestMigrateSpecTaskCodeAgentConfigReportsMissingProjectAgent(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	service := &SpecDrivenTaskService{store: mockStore}
	project := &types.Project{ID: "project-1", DefaultHelixAppID: "app-missing"}
	task := &types.SpecTask{ID: "task-1", ProjectID: project.ID}

	mockStore.EXPECT().GetApp(gomock.Any(), "app-missing").Return(nil, store.ErrNotFound)
	mockStore.EXPECT().UpdateProject(gomock.Any(), project).DoAndReturn(func(_ context.Context, got *types.Project) error {
		require.Empty(t, got.DefaultHelixAppID)
		return nil
	})

	err := service.migrateSpecTaskCodeAgentConfig(context.Background(), task, project)
	require.EqualError(t, err, "no coding agent is configured; select one for this task or project and retry")
	require.NotContains(t, err.Error(), "app-missing")
}

func TestMigrateSpecTaskCodeAgentConfigReportsProjectlessTaskWithoutConfig(t *testing.T) {
	ctrl := gomock.NewController(t)
	service := &SpecDrivenTaskService{store: store.NewMockStore(ctrl)}
	task := &types.SpecTask{ID: "task-1"}

	err := service.migrateSpecTaskCodeAgentConfig(context.Background(), task, nil)
	require.EqualError(t, err, "no coding agent is configured; select one for this task or project and retry")
}
