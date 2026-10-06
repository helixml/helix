package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
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

func (w specTaskWorkflow) SendAgentMessage(ctx context.Context, task *types.SpecTask, message string, interrupt bool, userID string) (string, error) {
	return w.apiServer.enqueueAgentMessage(ctx, task.PlanningSessionID, message, interrupt, userID, task.ID)
}

func (w specTaskWorkflow) StartAgent(ctx context.Context, task *types.SpecTask, userID string) error {
	user, session, err := w.loadActorAndSession(ctx, task, userID)
	if err != nil {
		return err
	}
	if w.apiServer.externalAgentExecutor == nil {
		return errors.New("external agent executor not available")
	}
	if _, err := w.apiServer.resumeSessionInternal(ctx, user, session); err != nil {
		return fmt.Errorf("resume session: %w", err)
	}
	return nil
}

func (w specTaskWorkflow) StopAgent(ctx context.Context, task *types.SpecTask) error {
	if task.PlanningSessionID == "" || w.apiServer.externalAgentExecutor == nil {
		return nil
	}
	return w.apiServer.externalAgentExecutor.StopDesktop(ctx, task.PlanningSessionID)
}

func (w specTaskWorkflow) RestartAgent(ctx context.Context, task *types.SpecTask, userID string) (int, bool, error) {
	user, session, err := w.loadActorAndSession(ctx, task, userID)
	if err != nil {
		return 0, false, err
	}
	if w.apiServer.externalAgentExecutor == nil {
		return 0, false, errors.New("external agent executor not available")
	}
	resetThread := w.apiServer.threadIsWedged(ctx, session)
	threadReset := resetThread && session.Metadata.ZedThreadID != ""
	promptsReset, httpErr := w.apiServer.restartSessionContainer(ctx, user, session, resetThread)
	if httpErr != nil {
		return 0, false, fmt.Errorf("restart session: %w", httpErr)
	}
	return promptsReset, threadReset, nil
}

func (w specTaskWorkflow) loadActorAndSession(ctx context.Context, task *types.SpecTask, userID string) (*types.User, *types.Session, error) {
	user, err := w.apiServer.Store.GetUser(ctx, &store.GetUserQuery{ID: userID})
	if err != nil {
		return nil, nil, fmt.Errorf("get acting user: %w", err)
	}
	if user == nil {
		return nil, nil, fmt.Errorf("acting user %s not found", userID)
	}
	session, err := w.apiServer.Store.GetSession(ctx, task.PlanningSessionID)
	if err != nil {
		return nil, nil, fmt.Errorf("get planning session: %w", err)
	}
	if session == nil {
		return nil, nil, fmt.Errorf("planning session %s not found", task.PlanningSessionID)
	}
	return user, session, nil
}
