package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/helixml/helix/api/pkg/types"
)

// OrgBotPRRef is what the pull request proposal flow needs to know about an org
// bot. Org bots live in helix-org; OrgBotPRHost adapts them so this package
// does not depend on it.
type OrgBotPRRef struct {
	OrganizationID string
	BotID          string
	Name           string
	ProjectID      string
	// HiringUserID approves on the bot's behalf when auto-approval was turned
	// on without recording who did it.
	HiringUserID              string
	AutoApprovePullRequests   bool
	AutoApprovePullRequestsBy string
}

// OrgBotPRHost is helix-org's side of an org bot's pull request proposals.
type OrgBotPRHost interface {
	PRBot(ctx context.Context, orgID, botID string) (*OrgBotPRRef, error)
	SetPRAutoApprove(ctx context.Context, orgID, botID string, enabled bool, by string) error
	// NotifyPRBot tells the bot about a decision as a message in its session.
	NotifyPRBot(ctx context.Context, orgID, botID, message string, provenance types.TurnProvenance) error
}

// SetOrgBotHost enables pull request proposals for org bots.
func (s *PRProposalService) SetOrgBotHost(h OrgBotPRHost) {
	s.orgBots = h
}

// proposalOwner is the spec task or org bot a proposal belongs to. Exactly one
// field is set.
type proposalOwner struct {
	task *types.SpecTask
	bot  *OrgBotPRRef
}

func (o proposalOwner) projectID() string {
	if o.task != nil {
		return o.task.ProjectID
	}
	return o.bot.ProjectID
}

// lockKey serialises writes to the owner's proposal settings.
func (o proposalOwner) lockKey() string {
	if o.task != nil {
		return "task:" + o.task.ID
	}
	return "org-bot:" + o.bot.OrganizationID + "/" + o.bot.BotID
}

func (o proposalOwner) logKey() (string, string) {
	if o.task != nil {
		return "task_id", o.task.ID
	}
	return "org_bot_id", o.bot.BotID
}

// filter selects this owner's proposals.
func (o proposalOwner) filter() types.SpecTaskPRProposalFilter {
	if o.task != nil {
		return types.SpecTaskPRProposalFilter{SpecTaskID: o.task.ID}
	}
	return types.SpecTaskPRProposalFilter{OrgBotID: o.bot.BotID, OrganizationID: o.bot.OrganizationID}
}

func (o proposalOwner) owns(p *types.SpecTaskPRProposal) bool {
	if o.task != nil {
		return p.SpecTaskID == o.task.ID
	}
	return p.SpecTaskID == "" && p.OrgBotID == o.bot.BotID && p.OrganizationID == o.bot.OrganizationID
}

func (o proposalOwner) stamp(p *types.SpecTaskPRProposal) {
	p.ProjectID = o.projectID()
	if o.task != nil {
		p.SpecTaskID = o.task.ID
		return
	}
	p.OrgBotID, p.OrganizationID = o.bot.BotID, o.bot.OrganizationID
}

// ownBranch is the branch the owner may always push to and propose without a
// grant: a task's working branch. Org bots have none.
func (o proposalOwner) ownBranch() string {
	if o.task != nil {
		return o.task.BranchName
	}
	return ""
}

// autoApprover is whether proposals are approved without asking, and as whom.
func (o proposalOwner) autoApprover() (bool, string) {
	if o.task != nil {
		by := o.task.AutoApprovePullRequestsBy
		if by == "" {
			by = o.task.CreatedBy
		}
		return o.task.AutoApprovePullRequests, by
	}
	by := o.bot.AutoApprovePullRequestsBy
	if by == "" {
		by = o.bot.HiringUserID
	}
	return o.bot.AutoApprovePullRequests, by
}

// ownerOf loads the spec task or org bot a stored proposal belongs to.
func (s *PRProposalService) ownerOf(ctx context.Context, p *types.SpecTaskPRProposal) (proposalOwner, error) {
	if p.SpecTaskID != "" {
		task, err := s.store.GetSpecTask(ctx, p.SpecTaskID)
		if err != nil {
			return proposalOwner{}, fmt.Errorf("get spec task: %w", err)
		}
		return proposalOwner{task: task}, nil
	}
	return s.orgBotOwner(ctx, p.OrganizationID, p.OrgBotID)
}

func (s *PRProposalService) orgBotOwner(ctx context.Context, orgID, botID string) (proposalOwner, error) {
	if orgID == "" || botID == "" {
		return proposalOwner{}, errors.New("pull request proposal has no owner")
	}
	if s.orgBots == nil {
		return proposalOwner{}, errors.New("org bot pull requests are not available on this server")
	}
	bot, err := s.orgBots.PRBot(ctx, orgID, botID)
	if err != nil {
		return proposalOwner{}, fmt.Errorf("get org bot %s: %w", botID, err)
	}
	return proposalOwner{bot: bot}, nil
}
