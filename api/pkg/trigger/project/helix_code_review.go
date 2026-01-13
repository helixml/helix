// Package project provides a trigger for Helix project task changes
package project

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	azuredevops "github.com/helixml/helix/api/pkg/agent/skill/azure_devops"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// HelixCodeReviewTrigger is triggered by code changes in the project, for example
// agent pushes code when implementing the spec task and the PR is opened
type HelixCodeReviewTrigger struct { //nolint:revive
	cfg        *config.ServerConfig
	store      store.Store
	controller *controller.Controller
}

func New(cfg *config.ServerConfig, store store.Store, controller *controller.Controller) *HelixCodeReviewTrigger {
	return &HelixCodeReviewTrigger{
		cfg:        cfg,
		store:      store,
		controller: controller,
	}
}

func (h *HelixCodeReviewTrigger) ProcessGitPushEvent(ctx context.Context, specTask *types.SpecTask, repo *types.GitRepository, commitHash string) error {
	if specTask.PullRequestID == "" {
		log.Debug().Str("spec_task_id", specTask.ID).Msg("No pull request ID found for spec task, skipping")
		return nil
	}

	project, err := h.store.GetProject(ctx, specTask.ProjectID)
	if err != nil {
		return fmt.Errorf("failed to get project: %w", err)
	}

	if !project.PullRequestReviewsEnabled {
		log.Debug().Str("spec_task_id", specTask.ID).Msg("Pull request reviews are not enabled for project, skipping")
		return nil
	}

	switch repo.ExternalType {
	case types.ExternalRepositoryTypeADO:
		return h.processAzureDevOpsPullRequest(ctx, specTask, project, repo, commitHash)
	default:
		return fmt.Errorf("unsupported external repository type: %s", repo.ExternalType)
	}
}

func (h *HelixCodeReviewTrigger) processAzureDevOpsPullRequest(ctx context.Context, specTask *types.SpecTask, project *types.Project, repo *types.GitRepository, commitHash string) error {
	log.Info().
		Str("spec_task_id", specTask.ID).
		Str("commit_hash", commitHash).
		Msg("Processing Azure DevOps pull request")

	if repo.AzureDevOps == nil {
		return fmt.Errorf("azure devops configuration not found for repository %s", repo.ID)
	}

	client, err := h.getAzureDevOpsClient(ctx, repo)
	if err != nil {
		return fmt.Errorf("failed to create azure devops client: %w", err)
	}

	adoProject, err := h.getAzureDevOpsProject(repo)
	if err != nil {
		return fmt.Errorf("failed to get azure devops project: %w", err)
	}

	repositoryName, err := h.getAzureDevOpsRepositoryName(repo)
	if err != nil {
		return fmt.Errorf("failed to get azure devops repository name: %w", err)
	}

	prID, err := strconv.Atoi(specTask.PullRequestID)
	if err != nil {
		return fmt.Errorf("failed to parse pull request ID: %w", err)
	}

	pr, err := client.GetPullRequest(ctx, repositoryName, adoProject, prID)
	if err != nil {
		return fmt.Errorf("failed to get pull request: %w", err)
	}

	var sourceCommitID, targetCommitID, sourceRefName, targetRefName string
	if pr.LastMergeSourceCommit != nil && pr.LastMergeSourceCommit.CommitId != nil {
		sourceCommitID = *pr.LastMergeSourceCommit.CommitId
	}
	if pr.LastMergeTargetCommit != nil && pr.LastMergeTargetCommit.CommitId != nil {
		targetCommitID = *pr.LastMergeTargetCommit.CommitId
	}
	if pr.SourceRefName != nil {
		sourceRefName = *pr.SourceRefName
	}
	if pr.TargetRefName != nil {
		targetRefName = *pr.TargetRefName
	}

	ctx = types.SetAzureDevopsRepositoryContext(ctx, types.AzureDevopsRepositoryContext{
		RemoteURL:               repo.ExternalURL,
		RepositoryID:            repositoryName,
		PullRequestID:           prID,
		ProjectID:               adoProject,
		LastMergeSourceCommitID: sourceCommitID,
		LastMergeTargetCommitID: targetCommitID,
		SourceRefName:           sourceRefName,
		TargetRefName:           targetRefName,
	})

	return h.runReviewSession(ctx, project, specTask, commitHash)
}

func (h *HelixCodeReviewTrigger) runReviewSession(ctx context.Context, project *types.Project, specTask *types.SpecTask, commitHash string) error {
	if project.PullRequestReviewerHelixAppID == "" {
		return fmt.Errorf("no pull request reviewer agent configured for project %s", project.ID)
	}

	app, err := h.store.GetApp(ctx, project.PullRequestReviewerHelixAppID)
	if err != nil {
		return fmt.Errorf("failed to get reviewer app: %w", err)
	}

	session := &types.Session{
		ID:             system.GenerateSessionID(),
		Name:           fmt.Sprintf("PR Review - %s", specTask.Name),
		Created:        time.Now(),
		Updated:        time.Now(),
		Mode:           types.SessionModeInference,
		Type:           types.SessionTypeText,
		ParentApp:      app.ID,
		OrganizationID: app.OrganizationID,
		Owner:          app.Owner,
		OwnerType:      app.OwnerType,
		Metadata: types.SessionMetadata{
			Stream:       false,
			HelixVersion: data.GetHelixVersion(),
		},
	}

	err = h.controller.WriteSession(ctx, session)
	if err != nil {
		log.Error().
			Err(err).
			Str("app_id", app.ID).
			Str("spec_task_id", specTask.ID).
			Msg("failed to create review session")
		return fmt.Errorf("failed to create session: %w", err)
	}

	user, err := h.store.GetUser(ctx, &store.GetUserQuery{
		ID: app.Owner,
	})
	if err != nil {
		log.Error().
			Err(err).
			Str("app_id", app.ID).
			Str("user_id", app.Owner).
			Msg("failed to get user")
		return fmt.Errorf("failed to get user: %w", err)
	}

	prompt := fmt.Sprintf("Review the pull request changes for task: %s\nCommit: %s", specTask.Name, commitHash)

	resp, err := h.controller.RunBlockingSession(ctx, &controller.RunSessionRequest{
		OrganizationID: app.OrganizationID,
		App:            app,
		Session:        session,
		User:           user,
		PromptMessage:  types.MessageContent{Parts: []any{prompt}},
	})
	if err != nil {
		log.Warn().
			Err(err).
			Str("app_id", app.ID).
			Str("spec_task_id", specTask.ID).
			Msg("failed to run review session")
		return fmt.Errorf("failed to run review session: %w", err)
	}

	log.Info().
		Str("app_id", app.ID).
		Str("spec_task_id", specTask.ID).
		Str("response", resp.ResponseMessage).
		Msg("Pull request review completed")

	return nil
}

func (h *HelixCodeReviewTrigger) getAzureDevOpsClient(ctx context.Context, repo *types.GitRepository) (*azuredevops.AzureDevOpsClient, error) {
	if repo.AzureDevOps == nil {
		return nil, fmt.Errorf("azure devops configuration not found")
	}

	if repo.AzureDevOps.OrganizationURL == "" {
		return nil, fmt.Errorf("azure devops organization URL not found")
	}

	if repo.AzureDevOps.TenantID != "" && repo.AzureDevOps.ClientID != "" && repo.AzureDevOps.ClientSecret != "" {
		client, err := azuredevops.NewAzureDevOpsClientWithServicePrincipal(
			ctx,
			repo.AzureDevOps.OrganizationURL,
			repo.AzureDevOps.TenantID,
			repo.AzureDevOps.ClientID,
			repo.AzureDevOps.ClientSecret,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create Service Principal client: %w", err)
		}
		return client, nil
	}

	if repo.AzureDevOps.PersonalAccessToken != "" {
		return azuredevops.NewAzureDevOpsClient(repo.AzureDevOps.OrganizationURL, repo.AzureDevOps.PersonalAccessToken), nil
	}

	return nil, fmt.Errorf("no Azure DevOps authentication configured")
}

func (h *HelixCodeReviewTrigger) getAzureDevOpsProject(repo *types.GitRepository) (string, error) {
	u, err := url.Parse(repo.ExternalURL)
	if err != nil {
		return "", fmt.Errorf("invalid external URL: %w", err)
	}

	pathParts := strings.Split(strings.Trim(u.Path, "/"), "/")

	for i, part := range pathParts {
		if part == "_git" {
			if i > 0 {
				return pathParts[i-1], nil
			}
			break
		}
	}

	return "", fmt.Errorf("could not parse project from URL: %s", repo.ExternalURL)
}

func (h *HelixCodeReviewTrigger) getAzureDevOpsRepositoryName(repo *types.GitRepository) (string, error) {
	u, err := url.Parse(repo.ExternalURL)
	if err != nil {
		return "", fmt.Errorf("invalid external URL: %w", err)
	}

	pathParts := strings.Split(strings.Trim(u.Path, "/"), "/")

	for i, part := range pathParts {
		if part == "_git" && i+1 < len(pathParts) {
			return pathParts[i+1], nil
		}
	}

	return "", fmt.Errorf("could not parse repository name from URL: %s", repo.ExternalURL)
}
