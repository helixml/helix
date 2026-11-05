package services

import (
	"context"
	"time"

	"github.com/helixml/helix/api/pkg/types"
)

func (s *proposalStore) DismissAttentionEventsForTask(_ context.Context, taskID string) (int64, error) {
	s.dismissed = append(s.dismissed, "task:"+taskID)
	return 0, nil
}

func (s *PRProposalSuite) TestCompletionNeedsTheUsersConfirmation() {
	out, err := s.svc.RequestCompletion(s.ctx, "spt_1", "Shipped the parser fix")
	s.Require().NoError(err)
	s.False(out.Done)
	task := s.task("spt_1")
	s.Equal(types.TaskStatusImplementation, task.Status, "asking does not finish the task")
	s.Require().NotNil(task.CompletionRequestedAt)
	s.Equal(task.CompletionRequestedAt.Truncate(time.Microsecond), *task.CompletionRequestedAt,
		"the attention key derives from this time, so it must survive Postgres's microsecond precision")
	s.Equal("Shipped the parser fix", task.CompletionRequestSummary)

	// Asking again updates the summary of the same request.
	requestedAt := *task.CompletionRequestedAt
	_, err = s.svc.RequestCompletion(s.ctx, "spt_1", "Shipped the parser fix and docs")
	s.Require().NoError(err)
	task = s.task("spt_1")
	s.Equal(requestedAt, *task.CompletionRequestedAt)
	s.Equal("Shipped the parser fix and docs", task.CompletionRequestSummary)

	done, err := s.svc.DecideCompletion(s.ctx, s.user, "spt_1", &types.CompletionDecisionRequest{Decision: types.CompletionDecisionApprove})
	s.Require().NoError(err)
	s.Equal(types.TaskStatusDone, done.Status)
	task = s.task("spt_1")
	s.Equal(types.TaskStatusDone, task.Status)
	s.NotNil(task.CompletedAt)
	s.Nil(task.CompletionRequestedAt)
	s.Contains(s.store.dismissed, "task:spt_1", "finishing clears the task's Needs Attention entries")
	s.Empty(s.messages, "a finished task's agent is not woken up")
}

func (s *PRProposalSuite) TestSendingCompletionBackTellsTheAgent() {
	_, err := s.svc.RequestCompletion(s.ctx, "spt_1", "All done")
	s.Require().NoError(err)
	key := CompletionRequestAttentionKey(s.task("spt_1"))

	_, err = s.svc.DecideCompletion(s.ctx, s.user, "spt_1", &types.CompletionDecisionRequest{
		Decision: types.CompletionDecisionReject, Comment: "The deploy still fails in staging",
	})
	s.Require().NoError(err)
	task := s.task("spt_1")
	s.Equal(types.TaskStatusImplementation, task.Status)
	s.Nil(task.CompletionRequestedAt)
	s.Empty(task.CompletionRequestSummary)
	s.Contains(s.store.dismissed, key)
	s.Require().Len(s.messages, 1)
	s.Contains(s.messages[0], "The deploy still fails in staging")
	s.Contains(s.messages[0], "mark_task_complete")

	_, err = s.svc.DecideCompletion(s.ctx, s.user, "spt_1", &types.CompletionDecisionRequest{Decision: types.CompletionDecisionReject})
	s.ErrorIs(err, ErrPRProposalConflict, "there is no request left to send back")
}

func (s *PRProposalSuite) TestUserCanMarkDoneWithoutARequest() {
	done, err := s.svc.DecideCompletion(s.ctx, s.user, "spt_1", &types.CompletionDecisionRequest{Decision: types.CompletionDecisionApprove})
	s.Require().NoError(err)
	s.Equal(types.TaskStatusDone, done.Status)
	s.NotNil(s.task("spt_1").CompletedAt)
}

func (s *PRProposalSuite) TestAutoApprovingTaskCompletesAtOnce() {
	s.enableAutoApprove("usr_owner")
	out, err := s.svc.RequestCompletion(s.ctx, "spt_1", "Finished")
	s.Require().NoError(err)
	s.True(out.Done)
	task := s.task("spt_1")
	s.Equal(types.TaskStatusDone, task.Status)
	s.Nil(task.CompletionRequestedAt)
}

func (s *PRProposalSuite) TestCompletionRequestRules() {
	_, err := s.svc.RequestCompletion(s.ctx, "spt_1", "  ")
	s.ErrorIs(err, ErrPRProposalInvalid, "a summary is required")

	backlog := s.task("spt_2")
	backlog.Status = types.TaskStatusBacklog
	s.Require().NoError(s.store.UpdateSpecTask(s.ctx, backlog))
	_, err = s.svc.RequestCompletion(s.ctx, "spt_2", "Finished")
	s.ErrorIs(err, ErrPRProposalInvalid, "only a task being worked on can finish")

	_, err = s.propose(ProposePRInput{HeadBranch: "feature/000001-slice"})
	s.Require().NoError(err)
	_, err = s.svc.RequestCompletion(s.ctx, "spt_1", "Finished")
	s.ErrorIs(err, ErrPRProposalConflict, "a pending pull request proposal blocks finishing")
}

func (s *PRProposalSuite) TestCompletionReportsOpenPullRequests() {
	task := s.task("spt_1")
	task.Status = types.TaskStatusPullRequest
	task.RepoPullRequests = []types.RepoPR{
		{RepositoryID: "repo_ext", PRID: "1", PRNumber: 1, PRState: "merged"},
		{RepositoryID: "repo_ext", PRID: "2", PRNumber: 2, PRState: "open"},
	}
	s.Require().NoError(s.store.UpdateSpecTask(s.ctx, task))

	out, err := s.svc.RequestCompletion(s.ctx, "spt_1", "Finished")
	s.Require().NoError(err)
	s.Require().Len(out.OpenPRs, 1)
	s.Equal(2, out.OpenPRs[0].PRNumber)
}

func (s *PRProposalSuite) TestBotCanCompleteWithoutARequest() {
	done, err := s.svc.CompleteTask(s.ctx, "spt_1")
	s.Require().NoError(err)
	s.Equal(types.TaskStatusDone, done.Status)
	_, err = s.svc.CompleteTask(s.ctx, "spt_1")
	s.ErrorIs(err, ErrPRProposalConflict)
}

func (s *PRProposalSuite) TestPRSettledMessage() {
	task := s.task("spt_1")
	task.RepoPullRequests = []types.RepoPR{
		{RepositoryName: "app", PRNumber: 1, PRURL: "https://github.example/pull/1", PRState: "merged"},
		{RepositoryName: "app", PRNumber: 2, PRURL: "https://github.example/pull/2", PRState: "open"},
	}
	msg := BuildPRSettledPrompt(task, task.RepoPullRequests[0])
	s.Contains(msg, "pull request #1 in app (https://github.example/pull/1) was merged")
	s.Contains(msg, "Still open: pull request #2")
	s.NotContains(msg, "mark_task_complete", "with PRs still open the agent is not invited to finish")

	task.RepoPullRequests[1].PRState = "closed"
	msg = BuildPRSettledPrompt(task, task.RepoPullRequests[1])
	s.Contains(msg, "was closed without being merged")
	s.Contains(msg, "mark_task_complete")
}

// A done task's desktop is stopped on its next update, so starting its agent
// must move it back to work: to pull_request when it has PRs.
func (s *PRProposalSuite) TestStartingADoneTasksAgentReopensIt() {
	_, err := s.svc.CompleteTask(s.ctx, "spt_1")
	s.Require().NoError(err)
	s.Require().NoError(s.svc.ReopenForAgent(s.ctx, "spt_1"))
	task := s.task("spt_1")
	s.Equal(types.TaskStatusImplementation, task.Status, "no PRs: back to implementation")
	s.Nil(task.CompletedAt)

	task.RepoPullRequests = []types.RepoPR{{RepositoryID: "repo_ext", PRID: "1", PRNumber: 1, PRState: "merged"}}
	s.Require().NoError(s.store.UpdateSpecTask(s.ctx, task))
	_, err = s.svc.CompleteTask(s.ctx, "spt_1")
	s.Require().NoError(err)
	s.Require().NoError(s.svc.ReopenForAgent(s.ctx, "spt_1"))
	s.Equal(types.TaskStatusPullRequest, s.task("spt_1").Status, "with PRs: back to pull_request")

	// Any other status is left alone.
	s.Require().NoError(s.svc.ReopenForAgent(s.ctx, "spt_1"))
	s.Equal(types.TaskStatusPullRequest, s.task("spt_1").Status)
}
