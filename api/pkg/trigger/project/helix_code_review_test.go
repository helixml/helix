package project

import (
	"context"
	"os"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	gomock "go.uber.org/mock/gomock"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHelixCodeReviewTrigger_ProcessGitPushEvent(t *testing.T) {
	adoToken := os.Getenv("ADO_TOKEN")
	adoOrgURL := os.Getenv("ADO_ORG_URL")

	if adoToken == "" {
		t.Skip("ADO_TOKEN is not set")
	}
	if adoOrgURL == "" {
		t.Skip("ADO_ORG_URL is not set")
	}

	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockController := NewMockController(ctrl)

	cfg := &config.ServerConfig{}

	helixCodeReviewTrigger := New(cfg, mockStore, mockController)

	ctx := context.Background()

	specTask := &types.SpecTask{
		ID:            "test-spec-task-id",
		ProjectID:     "test-project-id",
		Name:          "Test Task",
		PullRequestID: "3",
	}

	repo := &types.GitRepository{
		ID:           "test-repo-id",
		ExternalURL:  "https://helixml@dev.azure.com/helixml/helix-agents/_git/helix-agents",
		ExternalType: types.ExternalRepositoryTypeADO,
		AzureDevOps: &types.AzureDevOps{
			OrganizationURL:     adoOrgURL,
			PersonalAccessToken: adoToken,
		},
	}

	project := &types.Project{
		ID:                            "test-project-id",
		PullRequestReviewsEnabled:     true,
		PullRequestReviewerHelixAppID: "test-app-id",
	}

	app := &types.App{
		ID:             "test-app-id",
		Owner:          "test-owner-id",
		OwnerType:      types.OwnerTypeUser,
		OrganizationID: "test-org-id",
	}

	user := &types.User{
		ID: "test-owner-id",
	}

	mockStore.EXPECT().GetProject(gomock.Any(), specTask.ProjectID).Return(project, nil)
	mockStore.EXPECT().GetApp(gomock.Any(), project.PullRequestReviewerHelixAppID).Return(app, nil)
	mockStore.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: app.Owner}).Return(user, nil)

	mockController.EXPECT().WriteSession(gomock.Any(), gomock.Any()).Return(nil)
	mockController.EXPECT().RunBlockingSession(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, _ *controller.RunSessionRequest) (*types.Interaction, error) {
			// Get azure context from
			azureCtx, ok := types.GetAzureDevopsRepositoryContext(ctx)
			require.True(t, ok)
			assert.Equal(t, repo.ExternalURL, azureCtx.RemoteURL)
			assert.Equal(t, "helix-agents", azureCtx.RepositoryID)
			assert.Equal(t, "helix-agents", azureCtx.ProjectID)
			assert.Equal(t, 3, azureCtx.PullRequestID)

			// Check commit hashes
			assert.Equal(t, "6d1165273d52ca05526f95a8bcfbb5e48e46672b", azureCtx.LastMergeSourceCommitID)
			assert.Equal(t, "ee17bac1d16e6787c8a153b8225e245a5922c0db", azureCtx.LastMergeTargetCommitID)

			// Check ref names
			assert.Equal(t, "refs/heads/feature/newtest", azureCtx.SourceRefName)
			assert.Equal(t, "refs/heads/master", azureCtx.TargetRefName)

			return &types.Interaction{
				ResponseMessage: "Review completed",
			}, nil
		},
	)

	commitHash := "6d1165273d52ca05526f95a8bcfbb5e48e46672b"

	err := helixCodeReviewTrigger.ProcessGitPushEvent(ctx, specTask, repo, commitHash)
	require.NoError(t, err)
}
