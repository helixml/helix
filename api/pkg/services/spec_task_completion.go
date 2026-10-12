package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/types"
)

// A spec task is done only when someone says so: the agent calls
// mark_task_complete and the user confirms (or the task auto-approves), or the
// user moves the task to done. Merged pull requests never complete a task —
// an agent often has follow-up work after a merge.

// completableStatuses are the statuses in which the agent is working on the
// task and may say it is finished — including during planning, for tasks that
// ship no code.
var completableStatuses = map[types.SpecTaskStatus]bool{
	types.TaskStatusSpecGeneration:       true,
	types.TaskStatusSpecReview:           true,
	types.TaskStatusSpecRevision:         true,
	types.TaskStatusImplementation:       true,
	types.TaskStatusImplementationReview: true,
	types.TaskStatusPullRequest:          true,
}

// CompletionOutcome is the result of an agent's completion request.
type CompletionOutcome struct {
	Task *types.SpecTask
	// Done is true when the task was marked done immediately (auto-approval).
	Done bool
	// OpenPRs lists the task's pull requests that are still open.
	OpenPRs []types.RepoPR
}

// RequestCompletion records the agent's request to mark the task done. A
// task that auto-approves pull requests is completed at once, as the user who
// enabled it; otherwise the user is asked. Asking again while a request is
// pending updates its summary.
func (s *PRProposalService) RequestCompletion(ctx context.Context, taskID, summary string) (*CompletionOutcome, error) {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return nil, invalidProposal("summary is required: tell the user what was done and why the task is finished")
	}
	var out *CompletionOutcome
	var newRequest bool
	err := s.git.WithRepoLock("task:"+taskID, func() error {
		task, err := s.store.GetSpecTask(ctx, taskID)
		if err != nil {
			return err
		}
		if !completableStatuses[task.Status] {
			return invalidProposal("the task is in %q; it can only be marked complete while you are working on it", task.Status)
		}
		outstanding, err := s.HasOutstanding(ctx, task.ID)
		if err != nil {
			return err
		}
		if outstanding {
			return conflictingProposal("a pull request proposal is still pending, approved but not opened, or failed; wait for it to be resolved (or ask the user to reject it) before marking the task complete")
		}
		out = &CompletionOutcome{Task: task, OpenPRs: openPRs(task)}
		// Postgres keeps microseconds; the request's attention key is derived
		// from this time, so it must survive the round trip unchanged.
		now := time.Now().Truncate(time.Microsecond)
		if task.AutoApprovePullRequests {
			completeTask(task, now)
			out.Done = true
		} else {
			newRequest = task.CompletionRequestedAt == nil
			if newRequest {
				task.CompletionRequestedAt = &now
			}
			task.CompletionRequestSummary = summary
			task.UpdatedAt = now
		}
		return s.store.UpdateSpecTask(ctx, task)
	})
	if err != nil {
		return nil, err
	}
	if out.Done {
		DismissTaskAttentionEvents(ctx, s.store, taskID)
		log.Info().Str("task_id", taskID).Msg("Task auto-completed at the agent's request")
	} else if newRequest {
		s.emitAttention(out.Task, types.AttentionEventCompletionRequest, completionQualifier(out.Task), map[string]interface{}{
			"summary": summary,
		})
	}
	return out, nil
}

// DecideCompletion applies the user's decision. Approving marks the task done
// (stopping its desktop) whether or not the agent asked — the user may finish a
// task at any time. Rejecting a pending request clears it and passes the
// user's feedback to the agent.
func (s *PRProposalService) DecideCompletion(ctx context.Context, user *types.User, taskID string, req *types.CompletionDecisionRequest) (*types.SpecTask, error) {
	switch req.Decision {
	case types.CompletionDecisionApprove:
		return s.CompleteTask(ctx, taskID)
	case types.CompletionDecisionReject:
	default:
		return nil, invalidProposal("decision must be %q or %q", types.CompletionDecisionApprove, types.CompletionDecisionReject)
	}
	var task *types.SpecTask
	var key string
	err := s.git.WithRepoLock("task:"+taskID, func() error {
		var err error
		if task, err = s.store.GetSpecTask(ctx, taskID); err != nil {
			return err
		}
		if task.CompletionRequestedAt == nil {
			return conflictingProposal("the agent has not asked to complete this task")
		}
		key = CompletionRequestAttentionKey(task)
		task.CompletionRequestedAt = nil
		task.CompletionRequestSummary = ""
		task.UpdatedAt = time.Now()
		return s.store.UpdateSpecTask(ctx, task)
	})
	if err != nil {
		return nil, err
	}
	if err := s.store.DismissAttentionEventByKey(ctx, key); err != nil {
		log.Warn().Err(err).Str("task_id", task.ID).Msg("completion: failed to dismiss request")
	}
	if s.enqueue != nil {
		if err := s.enqueue(ctx, task, BuildCompletionSentBackPrompt(user, req.Comment), false, ""); err != nil {
			log.Error().Err(err).Str("task_id", task.ID).Msg("completion: failed to notify agent")
		}
	}
	return task, nil
}

// CompleteTask marks a task done on a person's or org Bot's say-so, with or
// without a pending request from the agent.
func (s *PRProposalService) CompleteTask(ctx context.Context, taskID string) (*types.SpecTask, error) {
	var task *types.SpecTask
	err := s.git.WithRepoLock("task:"+taskID, func() error {
		var err error
		if task, err = s.store.GetSpecTask(ctx, taskID); err != nil {
			return err
		}
		if task.Status == types.TaskStatusDone {
			return conflictingProposal("the task is already done")
		}
		if task.Archived {
			return conflictingProposal("the task is archived")
		}
		completeTask(task, time.Now())
		return s.store.UpdateSpecTask(ctx, task)
	})
	if err != nil {
		return nil, err
	}
	DismissTaskAttentionEvents(ctx, s.store, task.ID)
	return task, nil
}

// NotifyPRSettled tells the agent that one of its pull requests was merged or
// closed upstream, so it can carry on or ask to complete the task.
func (s *PRProposalService) NotifyPRSettled(ctx context.Context, task *types.SpecTask, pr types.RepoPR) {
	if s.enqueue == nil {
		return
	}
	if err := s.enqueue(ctx, task, BuildPRSettledPrompt(task, pr), false, ""); err != nil {
		log.Error().Err(err).Str("task_id", task.ID).Str("pr_id", pr.PRID).Msg("completion: failed to tell agent about settled PR")
	}
}

// ReopenForAgent moves a done task back to work when its agent starts again:
// a done task's desktop is stopped on its next update, so a running agent and
// done status cannot coexist. It returns to pull_request when the task has
// pull requests, else to implementation. Other statuses are left alone.
func (s *PRProposalService) ReopenForAgent(ctx context.Context, taskID string) error {
	var reopened *types.SpecTask
	err := s.git.WithRepoLock("task:"+taskID, func() error {
		task, err := s.store.GetSpecTask(ctx, taskID)
		if err != nil {
			return err
		}
		if task.Status != types.TaskStatusDone || task.Archived {
			return nil
		}
		target := types.TaskStatusImplementation
		if task.HasAnyPR() {
			target = types.TaskStatusPullRequest
		}
		setTaskStatus(task, target, time.Now())
		reopened = task
		return s.store.UpdateSpecTask(ctx, task)
	})
	if err == nil && reopened != nil {
		log.Info().Str("task_id", taskID).Str("status", string(reopened.Status)).Msg("Reopened done task because its agent started")
	}
	return err
}

// setTaskStatus moves a task to status, clearing completion when it leaves done.
func setTaskStatus(task *types.SpecTask, status types.SpecTaskStatus, now time.Time) {
	if task.Status == types.TaskStatusDone && status != types.TaskStatusDone {
		task.CompletedAt = nil
		task.MergedToMain = false
		task.MergedAt = nil
		task.MergeCommitHash = ""
	}
	task.Status = status
	task.StatusUpdatedAt = &now
	task.UpdatedAt = now
}

func completeTask(task *types.SpecTask, now time.Time) {
	task.Status = types.TaskStatusDone
	task.StatusUpdatedAt = &now
	task.CompletedAt = &now
	task.CompletionRequestedAt = nil
	task.CompletionRequestSummary = ""
	task.UpdatedAt = now
}

func completionQualifier(task *types.SpecTask) string {
	return fmt.Sprintf("completion-%d", task.CompletionRequestedAt.UnixMicro())
}

// CompletionRequestAttentionKey identifies the Needs Attention entry of the
// task's pending completion request.
func CompletionRequestAttentionKey(task *types.SpecTask) string {
	return types.BuildAttentionEventIdempotencyKey(task.ID, types.AttentionEventCompletionRequest, completionQualifier(task))
}

func openPRs(task *types.SpecTask) []types.RepoPR {
	var open []types.RepoPR
	for _, pr := range task.RepoPullRequests {
		if pr.PRState == string(types.PullRequestStateOpen) {
			open = append(open, pr)
		}
	}
	return open
}

func prLabel(pr types.RepoPR) string {
	label := "pull request"
	if pr.PRNumber > 0 {
		label = fmt.Sprintf("pull request #%d", pr.PRNumber)
	}
	if pr.RepositoryName != "" {
		label += " in " + pr.RepositoryName
	}
	if pr.PRURL != "" {
		label += " (" + pr.PRURL + ")"
	}
	return label
}

// MarkTaskCompleteGuidance is the prompt section that tells the agent how its
// task finishes.
func MarkTaskCompleteGuidance(hasPullRequests bool) string {
	confirm := "The user confirms it (unless the task auto-approves), the task moves to Done and your desktop shuts down; " +
		"if they send it back instead, their feedback arrives as a message here — keep working and call it again when ready."
	if !hasPullRequests {
		return "**Finishing the task:** The user lands your branch with Accept in Helix, which also finishes the task. " +
			"If the task needs no code changes (research, analysis, docs on the helix-specs branch), call `mark_task_complete` with a summary of what you delivered instead. " +
			confirm
	}
	return "**Finishing the task:** The task is never marked done automatically — not even when your pull requests merge, because work often continues after a merge. " +
		"When the work is finished (or the user tells you it is), call `mark_task_complete` with a summary of what was delivered. " + confirm + " " +
		"Do not call it while your pull requests are still open: users merge them on GitHub/GitLab/Azure DevOps themselves, so tell the user which ones are waiting to be merged. " +
		"You get a message here when each one is merged or closed; after a merge, check whether follow-up work is needed before you finish. " +
		"Tasks that need no code changes (research, analysis, docs on the helix-specs branch) finish the same way, with no pull request."
}

// BuildCompletionSentBackPrompt tells the agent the user did not accept its
// request to finish.
func BuildCompletionSentBackPrompt(user *types.User, comment string) string {
	who := "The user"
	if user != nil {
		if user.Email != "" {
			who = user.Email
		} else if user.FullName != "" {
			who = user.FullName
		}
	}
	msg := who + " sent the task back instead of marking it done, so it is not finished yet."
	if c := strings.TrimSpace(comment); c != "" {
		msg += "\n\nTheir feedback:\n" + c
	}
	return msg + "\n\nKeep working, and call `mark_task_complete` again when the work is finished."
}

// BuildPRSettledPrompt tells the agent a pull request was merged or closed.
func BuildPRSettledPrompt(task *types.SpecTask, pr types.RepoPR) string {
	var msg string
	if pr.PRState == string(types.PullRequestStateMerged) {
		msg = "Your " + prLabel(pr) + " was merged."
	} else {
		msg = "Your " + prLabel(pr) + " was closed without being merged."
	}
	if open := openPRs(task); len(open) > 0 {
		labels := make([]string, len(open))
		for i, o := range open {
			labels[i] = prLabel(o)
		}
		msg += "\n\nStill open: " + strings.Join(labels, ", ") + "."
	} else {
		msg += "\n\nNone of your pull requests are open now. If the task's work is finished, call `mark_task_complete`; " +
			"if follow-up work is needed, carry on and propose another pull request."
	}
	return msg
}
