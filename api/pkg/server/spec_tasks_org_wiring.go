package server

import (
	"context"

	"github.com/helixml/helix/api/pkg/services"
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

// RequestChanges delivers the reviewer's comment to the task's agent as a
// revision instruction — the exact mechanism the REST design-review
// "request_changes" branch uses (BuildRevisionInstructionPrompt +
// enqueueSpecTaskAgentMessage, interrupt=true). The status transition itself
// is already persisted by the runtime impl; this only carries the comment.
func (w specTaskWorkflow) RequestChanges(ctx context.Context, task *types.SpecTask, comment, userID string) error {
	message := services.BuildRevisionInstructionPrompt(task, comment)
	return w.apiServer.enqueueSpecTaskAgentMessage(ctx, task, message, true, userID)
}
