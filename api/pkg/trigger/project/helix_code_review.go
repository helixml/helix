// Package project provides a trigger for Helix project task changes
package project

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/store"
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

	// Load project
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
		// Not implemented yet
		return fmt.Errorf("unsupported external repository type: %s", repo.ExternalType)
	}
}

func (h *HelixCodeReviewTrigger) processAzureDevOpsPullRequest(ctx context.Context, specTask *types.SpecTask, project *types.Project, repo *types.GitRepository, commitHash string) error {
	log.Info().
		Str("spec_task_id", specTask.ID).
		Str("commit_hash", commitHash).
		Msg("Processing Azure DevOps pull request")
	return nil
}
