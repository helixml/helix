package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	giteagit "code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/git/gitcmd"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestFromPromptInlineAttachmentReachesJDIWorkspaceAndInteraction(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockFilestore := filestore.NewMockFileStore(ctrl)
	mockExecutor := external_agent.NewMockExecutor(ctrl)
	upstreamDir, middleRepoPath := initInlineAttachmentGitFixture(t, ctx)
	repository := &types.GitRepository{
		ID:            "repo-inline-jdi",
		Name:          "inline-jdi",
		LocalPath:     middleRepoPath,
		IsExternal:    true,
		ExternalURL:   "file://" + upstreamDir,
		ExternalType:  types.ExternalRepositoryTypeGitHub,
		DefaultBranch: "main",
		Status:        types.GitRepositoryStatusActive,
	}
	user := types.User{ID: "test-user", Email: "test@example.com"}
	project := &types.Project{
		ID:            "project-inline-jdi",
		UserID:        user.ID,
		DefaultRepoID: repository.ID,
		CodeAgentConfig: &types.CodeAgentExecutionConfig{
			Runtime:        types.CodeAgentRuntimeCodexCLI,
			CredentialType: types.CodeAgentCredentialTypeAPIKey,
			ProviderRef:    "provider-1",
			Model:          "model-1",
		},
	}

	mockStore.EXPECT().GetProject(gomock.Any(), project.ID).Return(project, nil).AnyTimes()
	mockStore.EXPECT().GetGitRepository(gomock.Any(), repository.ID).Return(repository, nil).AnyTimes()
	mockStore.EXPECT().GetUser(gomock.Any(), gomock.Any()).Return(&user, nil).AnyTimes()
	mockStore.EXPECT().ListGitRepositories(gomock.Any(), gomock.Any()).Return([]*types.GitRepository{repository}, nil).AnyTimes()
	mockStore.EXPECT().UpdateGitRepository(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockStore.EXPECT().IncrementGlobalTaskNumber(gomock.Any()).Return(3265, nil)

	var createdTask *types.SpecTask
	mockStore.EXPECT().CreateSpecTask(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, task *types.SpecTask) error {
		createdTask = task
		require.Equal(t, types.TaskStatusPreparing, task.Status)
		return nil
	})
	mockStore.EXPECT().TransitionSpecTaskStatus(
		gomock.Any(), gomock.Any(),
		[]types.SpecTaskStatus{types.TaskStatusPreparing},
		types.TaskStatusQueuedImplementation,
		nil,
	).Return(true, nil)

	submittedBytes := []byte("engagement-scope-token-3265")
	var storedBytes []byte
	mockFilestore.EXPECT().CreateFolder(gomock.Any(), gomock.Any()).Return(filestore.Item{Directory: true}, nil)
	mockFilestore.EXPECT().WriteFile(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, path string, reader io.Reader) (filestore.Item, error) {
			var err error
			storedBytes, err = io.ReadAll(reader)
			require.NoError(t, err)
			return filestore.Item{Path: path, Name: "engagement-brief.md"}, nil
		},
	)
	var attachment *types.SpecTaskAttachment
	mockStore.EXPECT().CreateSpecTaskAttachment(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, row *types.SpecTaskAttachment) error {
			attachment = row
			return nil
		},
	)

	gitService := services.NewGitRepositoryService(mockStore, filepath.Dir(filepath.Dir(middleRepoPath)), "http://localhost:8080", "Test User", user.Email)
	service := services.NewSpecDrivenTaskService(
		mockStore, nil, "test-agent", []string{"test-zed-agent"}, nil,
		mockExecutor, gitService, nil, services.NewDisabledKoditService(),
	)
	service.SetTestMode(true)
	service.ReadAttachmentBlob = func(_ context.Context, path string) ([]byte, error) {
		require.NotNil(t, attachment)
		require.Equal(t, attachment.FilestorePath, path)
		return append([]byte(nil), storedBytes...), nil
	}
	cfg := &config.ServerConfig{}
	apiServer := &HelixAPIServer{
		Cfg:   cfg,
		Store: mockStore,
		Controller: &controller.Controller{
			Ctx: ctx,
			Options: controller.Options{
				Config:    cfg,
				Store:     mockStore,
				Filestore: mockFilestore,
			},
		},
		specDrivenTaskService: service,
	}

	requestBody, err := json.Marshal(types.CreateTaskRequest{
		ProjectID:      project.ID,
		Prompt:         "Run the engagement using the attached brief",
		JustDoItMode:   true,
		AutoStart:      true,
		BranchMode:     types.BranchModeNew,
		BaseBranch:     "skip-sync-in-test",
		SandboxRuntime: types.SandboxRuntimeHeadlessUbuntu,
		Attachments: []types.SpecTaskInlineAttachment{{
			Name:          "engagement-brief.md",
			ContentBase64: base64.StdEncoding.EncodeToString(submittedBytes),
			Caption:       "rules of engagement",
		}},
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/spec-tasks/from-prompt", bytes.NewReader(requestBody))
	req = req.WithContext(setRequestUser(req.Context(), user))
	response := httptest.NewRecorder()
	apiServer.createTaskFromPrompt(response, req)
	require.Equal(t, http.StatusCreated, response.Code, response.Body.String())
	require.Equal(t, submittedBytes, storedBytes)
	require.NotNil(t, createdTask)
	require.NotNil(t, attachment)

	var task types.SpecTask
	require.NoError(t, json.NewDecoder(response.Body).Decode(&task))
	require.Equal(t, types.TaskStatusQueuedImplementation, task.Status)
	require.Equal(t, task.ID, attachment.SpecTaskID)

	mockStore.EXPECT().UpdateSpecTask(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, updated *types.SpecTask) error {
		require.Equal(t, types.TaskStatusImplementation, updated.Status)
		return nil
	})
	mockStore.EXPECT().CreateSession(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, session types.Session) (*types.Session, error) { return &session, nil },
	)
	mockStore.EXPECT().SetPlanningSessionIDIfEmpty(gomock.Any(), task.ID, gomock.Any()).Return(true, nil)
	mockStore.EXPECT().ListSpecTaskAttachments(gomock.Any(), task.ID).Return([]*types.SpecTaskAttachment{attachment}, nil)
	mockStore.EXPECT().UpdateSpecTaskAttachment(gomock.Any(), attachment).Return(nil)
	var interaction *types.Interaction
	mockStore.EXPECT().CreateInteraction(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, created *types.Interaction) (*types.Interaction, error) {
			interaction = created
			return created, nil
		},
	)
	mockStore.EXPECT().GetSession(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, id string) (*types.Session, error) { return &types.Session{ID: id}, nil },
	)
	mockStore.EXPECT().GetAPIKey(gomock.Any(), gomock.Any()).Return(&types.ApiKey{Key: "test-session-key"}, nil)
	mockExecutor.EXPECT().StartDesktop(gomock.Any(), gomock.Any()).Return(&types.DesktopAgentResponse{
		DevContainerID: "dev-inline-jdi",
		ContainerName:  "desktop-inline-jdi",
	}, nil)

	service.StartJustDoItMode(ctx, &task)

	workspacePath := "design/tasks/" + task.DesignDocPath + "/attachments/engagement-brief.md"
	gitBytes, _, err := gitcmd.NewCommand("show").
		AddDynamicArguments(services.SpecsBranchName+":"+workspacePath).
		RunStdBytes(ctx, &gitcmd.RunOpts{Dir: upstreamDir})
	require.NoError(t, err)
	require.Equal(t, submittedBytes, gitBytes)
	require.NotNil(t, interaction)
	require.Contains(t, interaction.PromptMessage, workspacePath)
	require.Contains(t, interaction.PromptMessage, "rules of engagement")
}

func initInlineAttachmentGitFixture(t *testing.T, ctx context.Context) (string, string) {
	t.Helper()
	testDir := t.TempDir()
	upstreamDir := filepath.Join(testDir, "upstream")
	tempClone := filepath.Join(testDir, "seed")
	middleRepoPath := filepath.Join(testDir, "middle", "git-repositories", "repo-inline-jdi")

	require.NoError(t, os.MkdirAll(upstreamDir, 0o755))
	require.NoError(t, giteagit.InitRepository(ctx, upstreamDir, true, "sha1"))
	require.NoError(t, giteagit.InitRepository(ctx, tempClone, false, "sha1"))
	require.NoError(t, os.WriteFile(filepath.Join(tempClone, "README.md"), []byte("# Inline JDI\n"), 0o644))
	require.NoError(t, giteagit.AddChanges(ctx, tempClone, true))
	require.NoError(t, giteagit.CommitChanges(ctx, tempClone, giteagit.CommitChangesOptions{
		Committer: &giteagit.Signature{Name: "Test", Email: "test@example.com", When: time.Now()},
		Author:    &giteagit.Signature{Name: "Test", Email: "test@example.com", When: time.Now()},
		Message:   "Initial commit",
	}))
	currentBranch, err := services.GetHEADBranch(ctx, tempClone)
	require.NoError(t, err)
	if currentBranch == "master" {
		require.NoError(t, services.GitRenameBranch(ctx, tempClone, "master", "main"))
	}
	require.NoError(t, services.AddRemote(ctx, tempClone, "origin", upstreamDir))
	require.NoError(t, giteagit.Push(ctx, tempClone, giteagit.PushOptions{
		Remote: "origin",
		Branch: "refs/heads/main:refs/heads/main",
	}))
	require.NoError(t, services.SetHEAD(ctx, upstreamDir, "main"))
	require.NoError(t, os.MkdirAll(filepath.Dir(middleRepoPath), 0o755))
	require.NoError(t, giteagit.Clone(ctx, upstreamDir, middleRepoPath, giteagit.CloneRepoOptions{
		Bare:   true,
		Mirror: true,
	}))
	return upstreamDir, middleRepoPath
}
