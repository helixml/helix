package services

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	giteagit "code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/git/gitcmd"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// proposalStore is an in-memory slice of store.Store covering what the
// proposal flow touches; any other method panics via the nil embedded store.
type proposalStore struct {
	store.Store
	mu        sync.Mutex
	tasks     map[string]*types.SpecTask
	repos     map[string]*types.GitRepository
	project   *types.Project
	proposals map[string]*types.SpecTaskPRProposal
	dismissed []string
}

func clone[T any](v *T) *T { c := *v; return &c }

func (s *proposalStore) GetProject(_ context.Context, _ string) (*types.Project, error) {
	return clone(s.project), nil
}
func (s *proposalStore) GetSpecTask(_ context.Context, id string) (*types.SpecTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tasks[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	c := clone(t)
	c.RepoPullRequests = append([]types.RepoPR(nil), t.RepoPullRequests...)
	return c, nil
}
func (s *proposalStore) UpdateSpecTask(_ context.Context, t *types.SpecTask) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks[t.ID] = clone(t)
	return nil
}
func (s *proposalStore) ListSpecTasks(_ context.Context, f *types.SpecTaskFilters) ([]*types.SpecTask, error) {
	var out []*types.SpecTask
	for _, t := range s.tasks {
		if f.BranchName == "" || t.BranchName == f.BranchName {
			out = append(out, clone(t))
		}
	}
	return out, nil
}
func (s *proposalStore) ListGitRepositories(_ context.Context, _ *types.ListGitRepositoriesRequest) ([]*types.GitRepository, error) {
	var out []*types.GitRepository
	for _, r := range s.repos {
		out = append(out, clone(r))
	}
	return out, nil
}
func (s *proposalStore) GetGitRepository(_ context.Context, id string) (*types.GitRepository, error) {
	return clone(s.repos[id]), nil
}
func (s *proposalStore) GetUser(_ context.Context, q *store.GetUserQuery) (*types.User, error) {
	return &types.User{ID: q.ID, Email: q.ID + "@example.com"}, nil
}
func (s *proposalStore) DismissAttentionEventByKey(_ context.Context, key string) error {
	s.dismissed = append(s.dismissed, key)
	return nil
}
func (s *proposalStore) CreateSpecTaskPRProposal(_ context.Context, p *types.SpecTaskPRProposal) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.proposals[p.ID] = clone(p)
	return nil
}
func (s *proposalStore) GetSpecTaskPRProposal(_ context.Context, id string) (*types.SpecTaskPRProposal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.proposals[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	return clone(p), nil
}
func (s *proposalStore) ListSpecTaskPRProposals(_ context.Context, f *types.SpecTaskPRProposalFilter) ([]*types.SpecTaskPRProposal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*types.SpecTaskPRProposal
	for _, p := range s.proposals {
		if (f.SpecTaskID != "" && p.SpecTaskID != f.SpecTaskID) ||
			(f.RepositoryID != "" && p.RepositoryID != f.RepositoryID) ||
			(f.HeadBranch != "" && p.HeadBranch != f.HeadBranch) {
			continue
		}
		if len(f.Statuses) > 0 {
			match := false
			for _, st := range f.Statuses {
				match = match || st == p.Status
			}
			if !match {
				continue
			}
		}
		out = append(out, clone(p))
	}
	return out, nil
}
func (s *proposalStore) UpdateSpecTaskPRProposal(_ context.Context, p *types.SpecTaskPRProposal, from ...types.SpecTaskPRProposalStatus) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.proposals[p.ID]
	if !ok {
		return false, nil
	}
	if len(from) > 0 {
		match := false
		for _, st := range from {
			match = match || st == cur.Status
		}
		if !match {
			return false, nil
		}
	}
	s.proposals[p.ID] = clone(p)
	return true, nil
}

// fakeProvider stands in for the external git provider behind GitRepositoryService.
type fakeProvider struct {
	pushed    []string
	created   []string
	createErr error
	oauthErr  error
	nextPR    int
}

func (f *fakeProvider) WithRepoLock(_ string, fn func() error) error { return fn() }
func (f *fakeProvider) PushBranchToRemote(_ context.Context, _ string, branch string, _ bool, _ ...string) error {
	f.pushed = append(f.pushed, branch)
	return nil
}
func (f *fakeProvider) ListPullRequests(context.Context, string) ([]*types.PullRequest, error) {
	return nil, nil
}
func (f *fakeProvider) GetPullRequest(_ context.Context, _ string, id string) (*types.PullRequest, error) {
	n, _ := strconv.Atoi(id)
	return &types.PullRequest{ID: id, Number: n, URL: "https://github.example/pull/" + id, State: types.PullRequestStateOpen}, nil
}
func (f *fakeProvider) CreatePullRequest(_ context.Context, _ string, title, _, head, _ string, _ string) (string, error) {
	if f.createErr != nil {
		return "", f.createErr
	}
	f.nextPR++
	f.created = append(f.created, head+":"+title)
	return strconv.Itoa(f.nextPR), nil
}
func (f *fakeProvider) ValidateUserOAuth(context.Context, *types.GitRepository, string) error {
	return f.oauthErr
}

type PRProposalSuite struct {
	suite.Suite
	ctx      context.Context
	store    *proposalStore
	provider *fakeProvider
	svc      *PRProposalService
	messages []string
	repoPath string
	base     string
	user     *types.User
}

func TestPRProposalSuite(t *testing.T) { suite.Run(t, new(PRProposalSuite)) }

func (s *PRProposalSuite) SetupTest() {
	s.ctx = context.Background()
	t := s.T()
	s.repoPath = filepath.Join(t.TempDir(), "repo")
	require.NoError(t, giteagit.InitRepository(s.ctx, s.repoPath, false, "sha1"))
	commit(t, s.ctx, s.repoPath, "README.md", "# initial", "initial")
	base, _, err := gitcmd.NewCommand().AddArguments("rev-parse", "--abbrev-ref", "HEAD").
		RunStdString(s.ctx, &gitcmd.RunOpts{Dir: s.repoPath})
	require.NoError(t, err)
	s.base = trimNewline(base)
	s.git("checkout", "-b", "feature/000001-task")
	commit(t, s.ctx, s.repoPath, "work.md", "task work", "task work")
	s.git("checkout", s.base)

	s.store = &proposalStore{
		tasks: map[string]*types.SpecTask{
			"spt_1": {ID: "spt_1", ProjectID: "prj_1", Name: "Task one", BranchName: "feature/000001-task", Status: types.TaskStatusImplementation},
			"spt_2": {ID: "spt_2", ProjectID: "prj_1", Name: "Task two", BranchName: "feature/000002-other", Status: types.TaskStatusImplementation},
		},
		repos: map[string]*types.GitRepository{
			"repo_ext": {ID: "repo_ext", Name: "app", LocalPath: s.repoPath, ExternalURL: "https://github.com/acme/app", IsExternal: true, DefaultBranch: s.base},
			"repo_int": {ID: "repo_int", Name: "playbook", LocalPath: s.repoPath, DefaultBranch: s.base},
		},
		project:   &types.Project{ID: "prj_1", DefaultRepoID: "repo_ext"},
		proposals: map[string]*types.SpecTaskPRProposal{},
	}
	s.provider = &fakeProvider{}
	s.messages = nil
	s.svc = NewPRProposalService(s.store, s.provider, nil, "http://helix.test")
	s.svc.SetMessageEnqueuer(func(_ context.Context, _ *types.SpecTask, msg string, interrupt bool, _ string) error {
		s.False(interrupt, "proposal outcomes must not interrupt the agent")
		s.messages = append(s.messages, msg)
		return nil
	})
	s.user = &types.User{ID: "usr_reviewer", Email: "reviewer@example.com"}
}

func (s *PRProposalSuite) git(args ...string) {
	_, _, err := gitcmd.NewCommand().AddArguments(gitcmd.ToTrustedCmdArgs(args)...).
		RunStdString(s.ctx, &gitcmd.RunOpts{Dir: s.repoPath})
	s.Require().NoError(err)
}

func (s *PRProposalSuite) task(id string) *types.SpecTask {
	t, err := s.store.GetSpecTask(s.ctx, id)
	s.Require().NoError(err)
	return t
}

func (s *PRProposalSuite) propose(in ProposePRInput) (*types.SpecTaskPRProposal, error) {
	if in.Reason == "" {
		in.Reason = "ready for review"
	}
	return s.svc.Propose(s.ctx, s.task("spt_1"), in)
}

func (s *PRProposalSuite) TestFirstPROnTaskBranchOpensOnApproval() {
	p, err := s.propose(ProposePRInput{Title: "Add work"})
	s.Require().NoError(err)
	s.Equal("feature/000001-task", p.HeadBranch, "head defaults to the task branch")
	s.Equal(s.base, p.BaseBranch, "base defaults to the task target branch")
	s.Equal(types.PRProposalStatusPending, p.Status)
	s.Empty(s.provider.pushed, "nothing reaches the provider before approval")

	decided, err := s.svc.Decide(s.ctx, s.user, p.ID, &types.PRProposalDecisionRequest{Decision: types.PRProposalDecisionApprove})
	s.Require().NoError(err)
	s.Equal(types.PRProposalStatusOpened, decided.Status)
	s.Equal(1, decided.PRNumber)
	s.Equal([]string{"feature/000001-task"}, s.provider.pushed)

	task := s.task("spt_1")
	s.Equal(types.TaskStatusPullRequest, task.Status)
	s.Require().Len(task.RepoPullRequests, 1)
	s.Equal(p.ID, task.RepoPullRequests[0].ProposalID)
	s.Equal("feature/000001-task", task.RepoPullRequests[0].HeadBranch)
	s.Equal("usr_reviewer", task.ImplementationApprovedBy)

	s.Require().Len(s.messages, 1)
	s.Contains(s.messages[0], "Pull request #1 is open")
	s.Contains(s.messages[0], "https://github.example/pull/1")
	s.Len(s.store.dismissed, 1, "the approval request leaves Needs Attention once decided")
}

func (s *PRProposalSuite) TestNewBranchNeedsApprovalBeforePushAndOpensOnPush() {
	allowed, err := s.svc.AllowedPushBranches(s.ctx, "spt_1", "repo_ext")
	s.Require().NoError(err)
	s.Empty(allowed)

	p, err := s.propose(ProposePRInput{HeadBranch: "feature/000001-part-2", Title: "Part 2"})
	s.Require().NoError(err)
	allowed, _ = s.svc.AllowedPushBranches(s.ctx, "spt_1", "repo_ext")
	s.Empty(allowed, "a pending proposal grants no push rights")

	decided, err := s.svc.Decide(s.ctx, s.user, p.ID, &types.PRProposalDecisionRequest{Decision: types.PRProposalDecisionApprove})
	s.Require().NoError(err)
	s.Equal(types.PRProposalStatusApproved, decided.Status, "no commits yet, so the PR waits for the push")
	s.Empty(s.provider.created)
	s.Require().Len(s.messages, 1)
	s.Contains(s.messages[0], "git push origin feature/000001-part-2")

	allowed, _ = s.svc.AllowedPushBranches(s.ctx, "spt_1", "repo_ext")
	s.Equal([]string{"feature/000001-part-2"}, allowed)
	allowed, _ = s.svc.AllowedPushBranches(s.ctx, "spt_1", "repo_int")
	s.Empty(allowed, "grants are per repository")

	// The agent pushes the approved branch.
	s.git("checkout", "-b", "feature/000001-part-2")
	commit(s.T(), s.ctx, s.repoPath, "part2.md", "second slice", "part 2")
	s.git("checkout", s.base)
	s.svc.OnBranchPushed(s.ctx, "repo_ext", "feature/000001-part-2")

	got, _ := s.store.GetSpecTaskPRProposal(s.ctx, p.ID)
	s.Equal(types.PRProposalStatusOpened, got.Status)
	s.Equal([]string{"feature/000001-part-2:Part 2"}, s.provider.created)
	s.Require().Len(s.messages, 2)
	s.Contains(s.messages[1], "Pull request #1 is open")
}

func (s *PRProposalSuite) TestSeveralPRsPerTask() {
	first, err := s.propose(ProposePRInput{Title: "Slice 1"})
	s.Require().NoError(err)
	_, err = s.svc.Decide(s.ctx, s.user, first.ID, &types.PRProposalDecisionRequest{Decision: types.PRProposalDecisionApprove})
	s.Require().NoError(err)

	_, err = s.propose(ProposePRInput{Title: "Again"})
	s.ErrorIs(err, ErrPRProposalConflict, "the open PR on the task branch is updated by pushing, not re-proposed")

	s.git("checkout", "-b", "feature/000001-slice-2", "feature/000001-task")
	commit(s.T(), s.ctx, s.repoPath, "slice2.md", "slice 2", "slice 2")
	s.git("checkout", s.base)
	// Branch exists but was never granted: proposing it must still be possible
	// only if it is the agent's own... it is not, so it is rejected.
	_, err = s.propose(ProposePRInput{HeadBranch: "feature/000001-slice-2", Title: "Slice 2"})
	s.ErrorIs(err, ErrPRProposalConflict, "an existing branch the task was never granted cannot be claimed")

	second, err := s.propose(ProposePRInput{HeadBranch: "feature/000001-slice-3", Title: "Slice 3"})
	s.Require().NoError(err)
	_, err = s.svc.Decide(s.ctx, s.user, second.ID, &types.PRProposalDecisionRequest{
		Decision: types.PRProposalDecisionApprove, HeadBranch: "feature/000001-renamed",
	})
	s.Require().NoError(err)
	s.Contains(s.messages[len(s.messages)-1], "`feature/000001-slice-3` → `feature/000001-renamed`", "the agent is told to push to the name the user approved")

	s.git("branch", "feature/000001-renamed", "feature/000001-slice-2")
	s.svc.OnBranchPushed(s.ctx, "repo_ext", "feature/000001-renamed")

	task := s.task("spt_1")
	s.Len(task.RepoPullRequests, 2)
	s.Equal(types.TaskStatusPullRequest, task.Status)
}

func (s *PRProposalSuite) TestRejectWithdrawsPushRights() {
	p, err := s.propose(ProposePRInput{HeadBranch: "feature/000001-extra"})
	s.Require().NoError(err)
	_, err = s.svc.Decide(s.ctx, s.user, p.ID, &types.PRProposalDecisionRequest{Decision: types.PRProposalDecisionApprove})
	s.Require().NoError(err)
	allowed, _ := s.svc.AllowedPushBranches(s.ctx, "spt_1", "repo_ext")
	s.Equal([]string{"feature/000001-extra"}, allowed)

	rejected, err := s.svc.Decide(s.ctx, s.user, p.ID, &types.PRProposalDecisionRequest{Decision: types.PRProposalDecisionReject, Comment: "not needed"})
	s.Require().NoError(err)
	s.Equal(types.PRProposalStatusRejected, rejected.Status)
	allowed, _ = s.svc.AllowedPushBranches(s.ctx, "spt_1", "repo_ext")
	s.Empty(allowed)
	s.Contains(s.messages[len(s.messages)-1], "**Reviewer note:** not needed")

	_, err = s.svc.Decide(s.ctx, s.user, p.ID, &types.PRProposalDecisionRequest{Decision: types.PRProposalDecisionApprove})
	s.ErrorIs(err, ErrPRProposalConflict, "a rejected proposal cannot be approved later")
}

func (s *PRProposalSuite) TestProviderFailureIsRetryable() {
	p, err := s.propose(ProposePRInput{})
	s.Require().NoError(err)
	s.provider.createErr = errors.New("422 validation failed")
	failed, err := s.svc.Decide(s.ctx, s.user, p.ID, &types.PRProposalDecisionRequest{Decision: types.PRProposalDecisionApprove})
	s.Require().NoError(err)
	s.Equal(types.PRProposalStatusFailed, failed.Status)
	s.Contains(failed.Error, "422 validation failed")
	s.Contains(s.messages[0], "could not be opened")
	outstanding, _ := s.svc.HasOutstanding(s.ctx, "spt_1")
	s.True(outstanding, "a failed proposal keeps the task open")

	s.provider.createErr = nil
	opened, err := s.svc.Decide(s.ctx, s.user, p.ID, &types.PRProposalDecisionRequest{Decision: types.PRProposalDecisionApprove})
	s.Require().NoError(err)
	s.Equal(types.PRProposalStatusOpened, opened.Status)
	outstanding, _ = s.svc.HasOutstanding(s.ctx, "spt_1")
	s.False(outstanding)
}

func (s *PRProposalSuite) TestMissingOAuthGrantsNothing() {
	p, err := s.propose(ProposePRInput{})
	s.Require().NoError(err)
	s.provider.oauthErr = &OAuthRequiredError{ProviderType: "github"}
	_, err = s.svc.Decide(s.ctx, s.user, p.ID, &types.PRProposalDecisionRequest{Decision: types.PRProposalDecisionApprove})
	var oauthErr *OAuthRequiredError
	s.ErrorAs(err, &oauthErr)
	got, _ := s.store.GetSpecTaskPRProposal(s.ctx, p.ID)
	s.Equal(types.PRProposalStatusPending, got.Status)
}

func (s *PRProposalSuite) TestValidation() {
	cases := []struct {
		name string
		in   ProposePRInput
		want error
	}{
		{"internal repo has no PRs", ProposePRInput{RepositoryID: "playbook"}, ErrPRProposalInvalid},
		{"unknown repo", ProposePRInput{RepositoryID: "nope"}, ErrPRProposalInvalid},
		{"invalid branch name", ProposePRInput{HeadBranch: "bad..name"}, ErrPRProposalInvalid},
		{"head equals base", ProposePRInput{HeadBranch: "x", BaseBranch: "x"}, ErrPRProposalInvalid},
		{"cannot propose from helix-specs", ProposePRInput{HeadBranch: SpecsBranchName}, ErrPRProposalInvalid},
		{"cannot propose from the default branch", ProposePRInput{HeadBranch: "", BaseBranch: "feature/000001-task"}, ErrPRProposalInvalid},
		{"missing base", ProposePRInput{BaseBranch: "does-not-exist"}, ErrPRProposalInvalid},
		{"another task's branch", ProposePRInput{HeadBranch: "feature/000002-other"}, ErrPRProposalConflict},
		{"reason required", ProposePRInput{Reason: " "}, ErrPRProposalInvalid},
	}
	for _, tc := range cases {
		s.Run(tc.name, func() {
			in := tc.in
			if tc.name == "cannot propose from the default branch" {
				in.HeadBranch = s.base
			}
			if in.Reason == "" {
				in.Reason = "why"
			}
			_, err := s.svc.Propose(s.ctx, s.task("spt_1"), in)
			s.ErrorIs(err, tc.want)
		})
	}

	backlog := s.task("spt_1")
	backlog.Status = types.TaskStatusBacklog
	_, err := s.svc.Propose(s.ctx, backlog, ProposePRInput{Reason: "why"})
	s.ErrorIs(err, ErrPRProposalInvalid, "no PRs before implementation starts")
}

func (s *PRProposalSuite) TestBranchClaimedByAnotherTasksProposal() {
	other := s.task("spt_2")
	_, err := s.svc.Propose(s.ctx, other, ProposePRInput{HeadBranch: "feature/shared", Reason: "mine"})
	s.Require().NoError(err)
	_, err = s.propose(ProposePRInput{HeadBranch: "feature/shared"})
	s.ErrorIs(err, ErrPRProposalConflict)
}

func (s *PRProposalSuite) TestReproposingPendingUpdatesInPlace() {
	first, err := s.propose(ProposePRInput{Title: "v1"})
	s.Require().NoError(err)
	second, err := s.propose(ProposePRInput{Title: "v2"})
	s.Require().NoError(err)
	s.Equal(first.ID, second.ID)
	s.Equal("v2", second.Title)
	s.Len(s.store.proposals, 1)
}

func (s *PRProposalSuite) TestCompletedTaskReopensForFollowUpPR() {
	t := s.task("spt_1")
	t.Status = types.TaskStatusDone
	t.RepoPullRequests = []types.RepoPR{{RepositoryID: "repo_ext", PRID: "7", PRNumber: 7, PRState: "merged", HeadBranch: "feature/000001-task"}}
	s.Require().NoError(s.store.UpdateSpecTask(s.ctx, t))

	p, err := s.propose(ProposePRInput{HeadBranch: "feature/000001-follow-up", Title: "Follow-up"})
	s.Require().NoError(err)
	_, err = s.svc.Decide(s.ctx, s.user, p.ID, &types.PRProposalDecisionRequest{Decision: types.PRProposalDecisionApprove})
	s.Require().NoError(err)
	s.git("branch", "feature/000001-follow-up", "feature/000001-task")
	s.svc.OnBranchPushed(s.ctx, "repo_ext", "feature/000001-follow-up")

	t = s.task("spt_1")
	s.Equal(types.TaskStatusPullRequest, t.Status, "a new PR reopens a completed task")
	s.Nil(t.CompletedAt)
	s.Len(t.RepoPullRequests, 2, "earlier PRs are kept")
}

func (s *PRProposalSuite) TestConcurrentDecisionsOnlyOneWins() {
	p, err := s.propose(ProposePRInput{HeadBranch: "feature/000001-race"})
	s.Require().NoError(err)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, d := range []string{types.PRProposalDecisionApprove, types.PRProposalDecisionReject} {
		wg.Add(1)
		go func(decision string) {
			defer wg.Done()
			_, err := s.svc.Decide(s.ctx, s.user, p.ID, &types.PRProposalDecisionRequest{Decision: decision})
			results <- err
		}(d)
	}
	wg.Wait()
	close(results)
	var failures int
	for err := range results {
		if err != nil {
			s.ErrorIs(err, ErrPRProposalConflict)
			failures++
		}
	}
	// Approve-then-reject is a legitimate sequence (revoking an approval), so
	// either both succeed in order or one loses the compare-and-set.
	got, _ := s.store.GetSpecTaskPRProposal(s.ctx, p.ID)
	s.Contains([]types.SpecTaskPRProposalStatus{types.PRProposalStatusApproved, types.PRProposalStatusRejected}, got.Status)
	s.LessOrEqual(failures, 1)
}
