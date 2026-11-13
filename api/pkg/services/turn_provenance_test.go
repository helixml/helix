package services

import (
	"context"
	"strings"
	"testing"

	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func (s *PRProposalSuite) TestPRProposalOutcomeNamesTheDecidingUser() {
	p, err := s.propose(ProposePRInput{HeadBranch: "feature/000001-extra"})
	s.Require().NoError(err)
	_, err = s.svc.Decide(s.ctx, s.user, p.ID, &types.PRProposalDecisionRequest{Decision: types.PRProposalDecisionReject})
	s.Require().NoError(err)

	s.Require().NotEmpty(s.provenances)
	got := s.provenances[len(s.provenances)-1]
	s.Equal(types.TurnOriginSystem, got.Origin)
	s.Equal(types.SystemTurnPRProposalOutcome, got.SystemKind)
	s.Require().NotNil(got.TriggeredBy)
	s.Equal(types.TurnProvenance{Origin: types.TurnOriginUser, UserID: "usr_reviewer"}, *got.TriggeredBy)
}

func (s *PRProposalSuite) TestCompletionSentBackNamesTheDecidingUser() {
	_, err := s.svc.RequestCompletion(s.ctx, "spt_1", "All done")
	s.Require().NoError(err)
	_, err = s.svc.DecideCompletion(s.ctx, s.user, "spt_1", &types.CompletionDecisionRequest{
		Decision: types.CompletionDecisionReject, Comment: "not yet",
	})
	s.Require().NoError(err)

	s.Require().Len(s.provenances, 1)
	want := types.TriggeredBySystemTurn(types.SystemTurnCompletionSentBack,
		types.TurnProvenance{Origin: types.TurnOriginUser, UserID: "usr_reviewer"})
	s.Equal(want, s.provenances[0])
}

func (s *PRProposalSuite) TestPRSettledIsASystemTurn() {
	s.svc.NotifyPRSettled(s.ctx, s.task("spt_1"), types.RepoPR{PRID: "7", PRState: "merged"})
	s.Require().Len(s.provenances, 1)
	s.Equal(types.SystemTurn(types.SystemTurnPRSettled), s.provenances[0])
}

func (s *PRProposalSuite) TestDecisionByCredentialThatCannotSendTurnsIsRefused() {
	p, err := s.propose(ProposePRInput{HeadBranch: "feature/000001-extra"})
	s.Require().NoError(err)
	runner := &types.User{ID: "usr_reviewer", TokenType: types.TokenTypeRunner}
	_, err = s.svc.Decide(s.ctx, runner, p.ID, &types.PRProposalDecisionRequest{Decision: types.PRProposalDecisionReject})
	s.Require().Error(err)
	s.Empty(s.provenances)
}

func (s *PRProposalSuite) TestCINotifierIsASystemTurn() {
	var got []types.TurnProvenance
	n := NewEnqueueCINotifier(func(_ context.Context, _ *types.SpecTask, _ string, interrupt bool, _ string, provenance types.TurnProvenance) error {
		s.True(interrupt)
		got = append(got, provenance)
		return nil
	})
	s.Require().NoError(n.NotifyCIResult(s.ctx, s.task("spt_1"), nil, "CI failed"))
	s.Equal([]types.TurnProvenance{types.SystemTurn(types.SystemTurnCIStatus)}, got)
}

// testUploader is an API key caller, the shape the PTaaS portal must be able
// to tell apart from Helix's own turns.
var testUploader = types.TurnProvenance{
	Origin: types.TurnOriginAPI, APIKeyOwner: "usr_key_owner", APIKeyType: types.APIKeyScopeProject,
	APIKeyID: "key_000000000000000000000001", APIKeyProjectID: "prj_1",
}

func (s *PRProposalSuite) TestPROutcomeSentAfterLaterPushNamesTheDecider() {
	key := &types.User{ID: "usr_reviewer", TokenType: types.TokenTypeAPIKey,
		Token: types.APIKeyPrefix + strings.Repeat("dc1d", 10), APIKeyType: types.APIkeytypeAPI}
	decider, err := types.RequestTurnProvenance(key)
	s.Require().NoError(err)

	p, err := s.propose(ProposePRInput{HeadBranch: "feature/000001-part-2", Title: "Part 2"})
	s.Require().NoError(err)
	_, err = s.svc.Decide(s.ctx, key, p.ID, &types.PRProposalDecisionRequest{Decision: types.PRProposalDecisionApprove})
	s.Require().NoError(err)

	// The PR opens, and the agent hears about it, only once commits land.
	s.git("checkout", "-b", "feature/000001-part-2")
	commit(s.T(), s.ctx, s.repoPath, "part2.md", "second slice", "part 2")
	s.git("checkout", s.base)
	s.provenances = nil
	s.svc.OnBranchPushed(s.ctx, "repo_ext", "feature/000001-part-2")

	s.Require().Len(s.provenances, 1)
	s.Equal(types.TriggeredBySystemTurn(types.SystemTurnPRProposalOutcome, decider), s.provenances[0])
}

func (s *PRProposalSuite) TestAutoApprovedOutcomeIsHelixsOwn() {
	s.enableAutoApprove("usr_owner")
	_, err := s.propose(ProposePRInput{HeadBranch: "feature/000001-auto", Title: "Auto"})
	s.Require().NoError(err)
	s.git("branch", "feature/000001-auto", "feature/000001-task")
	s.svc.OnBranchPushed(s.ctx, "repo_ext", "feature/000001-auto")

	s.Require().Len(s.provenances, 1)
	s.Equal(types.TriggeredBySystemTurn(types.SystemTurnPRProposalOutcome, types.SystemTurn(types.SystemTurnPRAutoApprove)),
		s.provenances[0], "no person decided this one, and the turn does not pretend one did")
}

// TestPlanningPromptNamesTheCallerWhoQueuedIt: the orchestrator writes the
// planning prompt long after the request that queued it; the turn must still
// name that caller.
func TestPlanningPromptNamesTheCallerWhoQueuedIt(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockExecutor := external_agent.NewMockExecutor(ctrl)
	project := &types.Project{ID: "project-prov", OrganizationID: "org-1", CodeAgentConfig: testSpecTaskCodeAgentConfig()}
	task := &types.SpecTask{
		ID: "task-prov", ProjectID: project.ID, CodeAgentConfig: testSpecTaskCodeAgentConfig(),
		Status: types.TaskStatusQueuedSpecGeneration, CreatedBy: "user-1", BranchMode: types.BranchModeNew,
		BaseBranch: "main", Name: "prov task", QueuedBy: testUploader,
	}
	mockStore.EXPECT().GetProject(gomock.Any(), project.ID).Return(project, nil).AnyTimes()
	mockStore.EXPECT().GetOrganization(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockStore.EXPECT().GetSpecTask(gomock.Any(), task.ID).Return(&types.SpecTask{ID: task.ID}, nil).AnyTimes()
	mockStore.EXPECT().UpdateSpecTask(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockStore.EXPECT().CreateSession(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, s types.Session) (*types.Session, error) { return &s, nil }).AnyTimes()
	mockStore.EXPECT().SetPlanningSessionIDIfEmpty(gomock.Any(), task.ID, gomock.Any()).Return(true, nil).AnyTimes()
	var created *types.Interaction
	mockStore.EXPECT().CreateInteraction(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, i *types.Interaction) (*types.Interaction, error) {
			created = i
			return i, nil
		}).AnyTimes()
	mockStore.EXPECT().ListGitRepositories(gomock.Any(), gomock.Any()).Return([]*types.GitRepository{}, nil).AnyTimes()
	mockStore.EXPECT().ListSpecTaskAttachments(gomock.Any(), gomock.Any()).Return([]*types.SpecTaskAttachment{}, nil).AnyTimes()
	mockExecutor.EXPECT().StartDesktop(gomock.Any(), gomock.Any()).
		Return(&types.DesktopAgentResponse{DevContainerID: "dev", ContainerName: "c"}, nil).AnyTimes()

	service := NewSpecDrivenTaskService(mockStore, nil, "test-helix-agent", []string{"test-zed-agent"}, nil, mockExecutor,
		NewGitRepositoryService(mockStore, t.TempDir(), "http://test", "test", "test@test"), nil, NewDisabledKoditService())
	service.SetTestMode(true)
	service.StartSpecGeneration(context.Background(), task)

	require.NotNil(t, created)
	require.Equal(t, types.TriggeredBySystemTurn(types.SystemTurnPlanningPrompt, testUploader), created.Provenance)
}
