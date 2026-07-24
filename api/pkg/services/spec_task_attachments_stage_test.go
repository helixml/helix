package services

import (
	"context"

	"code.gitea.io/gitea/modules/git/gitcmd"
	"github.com/helixml/helix/api/pkg/types"
	"go.uber.org/mock/gomock"
)

// stageEnqueue captures a call to the fake EnqueueMessageToAgent so tests can assert
// whether (and how) the agent was notified about a late-arriving attachment.
type stageEnqueue struct {
	message      string
	interrupt    bool
	notifyUserID string
}

// newStageService builds a SpecDrivenTaskService wired to the suite's real git repo and
// mock store, with a fake blob reader and a capturing enqueuer. Enqueue calls are appended
// to *captured.
func (s *GitIntegrationSuite) newStageService(captured *[]stageEnqueue) *SpecDrivenTaskService {
	svc := &SpecDrivenTaskService{
		store:                s.mockStore,
		gitRepositoryService: s.gitRepoService,
	}
	svc.ReadAttachmentBlob = func(_ context.Context, path string) ([]byte, error) {
		return []byte("fake-bytes-for-" + path), nil
	}
	svc.EnqueueMessageToAgent = func(_ context.Context, _ *types.SpecTask, message string, interrupt bool, notifyUserID string) error {
		*captured = append(*captured, stageEnqueue{message: message, interrupt: interrupt, notifyUserID: notifyUserID})
		return nil
	}
	return svc
}

// upstreamFileOnBranch returns the content of a file on a branch in the upstream bare repo,
// and whether it exists. Used to prove the attachment actually reached the pushed repo.
func (s *GitIntegrationSuite) upstreamFileOnBranch(branch, path string) (string, bool) {
	stdout, _, err := gitcmd.NewCommand("show").
		AddDynamicArguments(branch+":"+path).
		RunStdString(s.ctx, &gitcmd.RunOpts{Dir: s.upstreamDir})
	if err != nil {
		return "", false
	}
	return stdout, true
}

// TestStageUploadedAttachments_RacingUploadAfterPlanning reproduces the bug: an attachment
// is uploaded AFTER the planning session already exists (its prompt was built without the
// file). StageUploadedAttachments must (a) commit the file to the helix-specs branch and
// push it upstream, (b) set CommittedSHA, and (c) enqueue a non-interrupt note so the agent
// is told to read it.
func (s *GitIntegrationSuite) TestStageUploadedAttachments_RacingUploadAfterPlanning() {
	require := s.Require()

	project := &types.Project{ID: "proj-1", DefaultRepoID: "test-repo"}
	task := &types.SpecTask{
		ID:                "task-1",
		ProjectID:         "proj-1",
		CreatedBy:         "test-user",
		DesignDocPath:     "000123_racing-task",
		PlanningSessionID: "ses-1", // planning already running — the bug's precondition
	}
	att := &types.SpecTaskAttachment{
		ID:            "att-1",
		SpecTaskID:    "task-1",
		Filename:      "screenshot.png",
		MimeType:      "image/png",
		SizeBytes:     2250000,
		FilestorePath: "/filestore/att-1__screenshot.png",
	}

	s.mockStore.EXPECT().GetSpecTask(gomock.Any(), "task-1").Return(task, nil).AnyTimes()
	s.mockStore.EXPECT().GetProject(gomock.Any(), "proj-1").Return(project, nil).AnyTimes()
	s.mockStore.EXPECT().ListSpecTaskAttachments(gomock.Any(), "task-1").
		Return([]*types.SpecTaskAttachment{att}, nil).AnyTimes()
	s.mockStore.EXPECT().UpdateSpecTaskAttachment(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	var enqueued []stageEnqueue
	svc := s.newStageService(&enqueued)

	err := svc.StageUploadedAttachments(s.ctx, "task-1")
	require.NoError(err)

	// (a) file reached the pushed upstream repo under the task's attachments dir
	content, ok := s.upstreamFileOnBranch(SpecsBranchName, "design/tasks/000123_racing-task/attachments/screenshot.png")
	require.True(ok, "attachment must exist on helix-specs branch upstream")
	require.Equal("fake-bytes-for-/filestore/att-1__screenshot.png", content)

	// (b) CommittedSHA persisted on the row (idempotency marker)
	require.NotEmpty(att.CommittedSHA, "CommittedSHA must be set after staging")

	// (c) agent notified exactly once, non-interrupt, to the task creator, pointing at the dir
	require.Len(enqueued, 1, "agent must be notified about the late attachment")
	require.False(enqueued[0].interrupt, "note must not interrupt in-flight work")
	require.Equal("test-user", enqueued[0].notifyUserID)
	require.Contains(enqueued[0].message, "design/tasks/000123_racing-task/attachments/screenshot.png")
}

// TestStageUploadedAttachments_BacklogNoSession covers the case where planning has not
// started yet (no PlanningSessionID). The file must still be committed so it is present
// when the planning prompt is later built — but no note is enqueued (the prompt will list
// it via ListSpecTaskAttachments).
func (s *GitIntegrationSuite) TestStageUploadedAttachments_BacklogNoSession() {
	require := s.Require()

	project := &types.Project{ID: "proj-2", DefaultRepoID: "test-repo"}
	task := &types.SpecTask{
		ID:            "task-2",
		ProjectID:     "proj-2",
		CreatedBy:     "test-user",
		DesignDocPath: "000124_backlog-task",
		// PlanningSessionID intentionally empty
	}
	att := &types.SpecTaskAttachment{
		ID:            "att-2",
		SpecTaskID:    "task-2",
		Filename:      "notes.md",
		MimeType:      "text/markdown",
		SizeBytes:     42,
		FilestorePath: "/filestore/att-2__notes.md",
	}

	s.mockStore.EXPECT().GetSpecTask(gomock.Any(), "task-2").Return(task, nil).AnyTimes()
	s.mockStore.EXPECT().GetProject(gomock.Any(), "proj-2").Return(project, nil).AnyTimes()
	s.mockStore.EXPECT().ListSpecTaskAttachments(gomock.Any(), "task-2").
		Return([]*types.SpecTaskAttachment{att}, nil).AnyTimes()
	s.mockStore.EXPECT().UpdateSpecTaskAttachment(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	var enqueued []stageEnqueue
	svc := s.newStageService(&enqueued)

	err := svc.StageUploadedAttachments(s.ctx, "task-2")
	require.NoError(err)

	_, ok := s.upstreamFileOnBranch(SpecsBranchName, "design/tasks/000124_backlog-task/attachments/notes.md")
	require.True(ok, "attachment must be committed even before planning starts")
	require.NotEmpty(att.CommittedSHA)
	require.Empty(enqueued, "no note should be enqueued when there is no planning session")
}

// TestStageUploadedAttachments_Idempotent proves double-staging is safe: an attachment whose
// CommittedSHA is already set is not re-committed, and — because there is nothing new to
// stage — the agent is not re-notified.
func (s *GitIntegrationSuite) TestStageUploadedAttachments_Idempotent() {
	require := s.Require()

	project := &types.Project{ID: "proj-3", DefaultRepoID: "test-repo"}
	task := &types.SpecTask{
		ID:                "task-3",
		ProjectID:         "proj-3",
		CreatedBy:         "test-user",
		DesignDocPath:     "000125_idem-task",
		PlanningSessionID: "ses-3",
	}
	att := &types.SpecTaskAttachment{
		ID:            "att-3",
		SpecTaskID:    "task-3",
		Filename:      "already.png",
		MimeType:      "image/png",
		SizeBytes:     100,
		FilestorePath: "/filestore/att-3__already.png",
	}

	s.mockStore.EXPECT().GetSpecTask(gomock.Any(), "task-3").Return(task, nil).AnyTimes()
	s.mockStore.EXPECT().GetProject(gomock.Any(), "proj-3").Return(project, nil).AnyTimes()
	s.mockStore.EXPECT().ListSpecTaskAttachments(gomock.Any(), "task-3").
		Return([]*types.SpecTaskAttachment{att}, nil).AnyTimes()
	s.mockStore.EXPECT().UpdateSpecTaskAttachment(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	var enqueued []stageEnqueue
	svc := s.newStageService(&enqueued)

	// First call stages + notifies.
	require.NoError(svc.StageUploadedAttachments(s.ctx, "task-3"))
	require.Len(enqueued, 1)
	firstSHA := att.CommittedSHA
	require.NotEmpty(firstSHA)

	// Second call: nothing pending → no re-commit, no re-notify. The note branch is gated
	// on there being un-staged work, so a fully-committed set is a no-op for notification.
	require.NoError(svc.StageUploadedAttachments(s.ctx, "task-3"))
	require.Equal(firstSHA, att.CommittedSHA, "CommittedSHA must not change on re-stage")
	require.Len(enqueued, 1, "a fully-staged attachment must not re-notify the agent")
}
