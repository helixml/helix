package services

import (
	"context"

	"github.com/helixml/helix/api/pkg/types"
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
