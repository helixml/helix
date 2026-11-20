package services

import (
	"context"
	"errors"

	"github.com/helixml/helix/api/pkg/types"
)

// fakeOrgBotHost is helix-org's side of an org bot's proposals.
type fakeOrgBotHost struct {
	bots     map[string]*OrgBotPRRef
	notified []string
}

func (h *fakeOrgBotHost) PRBot(_ context.Context, orgID, botID string) (*OrgBotPRRef, error) {
	b, ok := h.bots[orgID+"/"+botID]
	if !ok {
		return nil, errors.New("no such bot")
	}
	c := *b
	return &c, nil
}

func (h *fakeOrgBotHost) SetPRAutoApprove(_ context.Context, orgID, botID string, enabled bool, by string) error {
	b := h.bots[orgID+"/"+botID]
	b.AutoApprovePullRequests = enabled
	if enabled {
		b.AutoApprovePullRequestsBy = by
	}
	return nil
}

func (h *fakeOrgBotHost) NotifyPRBot(_ context.Context, _, _, message string, _ types.TurnProvenance) error {
	h.notified = append(h.notified, message)
	return nil
}

const botCraftBranch = "feature/recruiter-seeds"

// setupBot gives the suite an org bot in prj_1 that has already pushed its
// craft changes to botCraftBranch — the state luke's recruiter bot was in.
func (s *PRProposalSuite) setupBot() *fakeOrgBotHost {
	s.git("checkout", "-b", botCraftBranch)
	commit(s.T(), s.ctx, s.repoPath, "playbook.md", "new rules", "craft")
	s.git("checkout", s.base)
	host := &fakeOrgBotHost{bots: map[string]*OrgBotPRRef{
		"org_1/b-recruiter": {OrganizationID: "org_1", BotID: "b-recruiter", Name: "Recruiter", ProjectID: "prj_1", HiringUserID: "usr_owner"},
	}}
	s.svc.SetOrgBotHost(host)
	return host
}

func (s *PRProposalSuite) proposeAsBot(in ProposePRInput) (*types.SpecTaskPRProposal, error) {
	if in.Reason == "" {
		in.Reason = "ship today's playbook rules"
	}
	return s.svc.ProposeForOrgBot(s.ctx, "org_1", "b-recruiter", in)
}

func (s *PRProposalSuite) TestOrgBotProposesItsPushedBranchAndItOpensOnApproval() {
	host := s.setupBot()
	p, err := s.proposeAsBot(ProposePRInput{HeadBranch: botCraftBranch, Title: "Playbook rules"})
	s.Require().NoError(err, "a bot's already-pushed branch is proposable")
	s.Equal("b-recruiter", p.OrgBotID)
	s.Equal("org_1", p.OrganizationID)
	s.Empty(p.SpecTaskID)
	s.Equal(s.base, p.BaseBranch, "base defaults to the repository's default branch")

	opened, err := s.svc.Decide(s.ctx, s.user, p.ID, &types.PRProposalDecisionRequest{Decision: types.PRProposalDecisionApprove})
	s.Require().NoError(err)
	s.Equal(types.PRProposalStatusOpened, opened.Status)
	s.Equal([]string{botCraftBranch}, s.provider.pushed)
	s.Require().Len(host.notified, 1, "the bot hears the outcome in its own session")
	s.Contains(host.notified[0], "Pull request #1 is open")
	s.Empty(s.messages, "nothing goes to a spec task")
	s.Empty(s.store.dismissed, "a bot proposal raises no attention event to dismiss")

	listed, err := s.svc.ListForOrgBot(s.ctx, "org_1", "b-recruiter")
	s.Require().NoError(err)
	s.Len(listed, 1)
}

func (s *PRProposalSuite) TestOrgBotMustNameBranchAndTitle() {
	s.setupBot()
	_, err := s.proposeAsBot(ProposePRInput{Title: "x"})
	s.ErrorIs(err, ErrPRProposalInvalid)
	_, err = s.proposeAsBot(ProposePRInput{HeadBranch: botCraftBranch})
	s.ErrorIs(err, ErrPRProposalInvalid)
}

func (s *PRProposalSuite) TestOrgBotCannotClaimATasksBranchAndViceVersa() {
	s.setupBot()
	_, err := s.proposeAsBot(ProposePRInput{HeadBranch: "feature/000001-task", Title: "x"})
	s.ErrorIs(err, ErrPRProposalConflict, "a task's working branch is not the bot's")

	_, err = s.proposeAsBot(ProposePRInput{HeadBranch: botCraftBranch, Title: "x"})
	s.Require().NoError(err)
	_, err = s.propose(ProposePRInput{HeadBranch: botCraftBranch})
	s.ErrorIs(err, ErrPRProposalConflict, "a task cannot take over a bot's proposed branch")
}

func (s *PRProposalSuite) TestOrgBotAutoApprovesAsTheUserWhoEnabledIt() {
	host := s.setupBot()
	s.Require().NoError(s.svc.SetOrgBotAutoApprove(s.ctx, "org_1", "b-recruiter", true, "usr_reviewer"))
	s.Equal("usr_reviewer", host.bots["org_1/b-recruiter"].AutoApprovePullRequestsBy)

	p, err := s.proposeAsBot(ProposePRInput{HeadBranch: botCraftBranch, Title: "Playbook rules"})
	s.Require().NoError(err)
	s.Equal(types.PRProposalStatusOpened, p.Status)
	s.True(p.AutoApproved)
	s.Equal("usr_reviewer", p.DecidedBy)
}

func (s *PRProposalSuite) TestOrgBotAutoApproveFallsBackToHiringUser() {
	host := s.setupBot()
	host.bots["org_1/b-recruiter"].AutoApprovePullRequests = true

	p, err := s.proposeAsBot(ProposePRInput{HeadBranch: botCraftBranch, Title: "Playbook rules"})
	s.Require().NoError(err)
	s.Equal(types.PRProposalStatusOpened, p.Status)
	s.Equal("usr_owner", p.DecidedBy)
}

func (s *PRProposalSuite) TestOrgBotCanReproposeBranchOnceItsPRMerged() {
	s.setupBot()
	p, err := s.proposeAsBot(ProposePRInput{HeadBranch: botCraftBranch, Title: "Playbook rules"})
	s.Require().NoError(err)
	opened, err := s.svc.Decide(s.ctx, s.user, p.ID, &types.PRProposalDecisionRequest{Decision: types.PRProposalDecisionApprove})
	s.Require().NoError(err)

	_, err = s.proposeAsBot(ProposePRInput{HeadBranch: botCraftBranch, Title: "More rules"})
	s.ErrorIs(err, ErrPRProposalConflict, "the PR is still open")

	s.provider.states = map[string]types.PullRequestState{opened.PRID: types.PullRequestStateMerged}
	_, err = s.proposeAsBot(ProposePRInput{HeadBranch: botCraftBranch, Title: "More rules"})
	s.NoError(err, "a long-lived craft branch is proposable again once its PR merged")
}

func (s *PRProposalSuite) TestOrgBotRejectionReachesTheBot() {
	host := s.setupBot()
	p, err := s.proposeAsBot(ProposePRInput{HeadBranch: botCraftBranch, Title: "Playbook rules"})
	s.Require().NoError(err)
	_, err = s.svc.Decide(s.ctx, s.user, p.ID, &types.PRProposalDecisionRequest{Decision: types.PRProposalDecisionReject, Comment: "split it"})
	s.Require().NoError(err)
	s.Require().Len(host.notified, 1)
	s.Contains(host.notified[0], "rejected")
	s.Contains(host.notified[0], "split it")
	s.Empty(s.provider.pushed)
}

func (s *PRProposalSuite) TestOrgBotProposalsUnavailableWithoutHost() {
	_, err := s.svc.ProposeForOrgBot(s.ctx, "org_1", "b-recruiter", ProposePRInput{HeadBranch: botCraftBranch, Title: "x", Reason: "r"})
	s.Error(err)
}
