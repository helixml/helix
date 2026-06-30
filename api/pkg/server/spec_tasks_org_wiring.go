package server

import (
	"context"

	"github.com/helixml/helix/api/pkg/types"
)

// specTaskWorkflow adapts the HelixAPIServer's spec-task workflow code to
// the runtimehelix.SpecTaskWorkflow port. It reuses the exact canonical
// code the REST UI drives: SpecDrivenTaskService.ApproveSpecs for spec
// approval, and ensurePullRequestsForAllRepos for opening one PR per
// repo attached to the project. No spec-task logic is duplicated here.
type specTaskWorkflow struct {
	apiServer *HelixAPIServer
}

func (w specTaskWorkflow) ApproveSpecs(ctx context.Context, task *types.SpecTask) error {
	return w.apiServer.specDrivenTaskService.ApproveSpecs(ctx, task)
}

func (w specTaskWorkflow) EnsurePullRequests(ctx context.Context, task *types.SpecTask, primaryRepoID, userID string) error {
	return w.apiServer.ensurePullRequestsForAllRepos(ctx, task, primaryRepoID, userID)
}
