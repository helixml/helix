package server

import (
	"context"
	"errors"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/dispatch"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	helixorgstore "github.com/helixml/helix/api/pkg/org/domain/store"
	runtimehelix "github.com/helixml/helix/api/pkg/org/infrastructure/runtime/helix"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/types"
)

// orgBotPRHost is helix-org's side of an org bot's pull request proposals.
type orgBotPRHost struct {
	st         *helixorgstore.Store
	dispatcher *dispatch.Dispatcher
	now        func() time.Time
}

var _ services.OrgBotPRHost = orgBotPRHost{}

func (h orgBotPRHost) PRBot(ctx context.Context, orgID, botID string) (*services.OrgBotPRRef, error) {
	bot, err := h.st.Nodes.Get(ctx, orgID, orgchart.NodeID(botID))
	if err != nil {
		return nil, err
	}
	state, err := runtimehelix.LoadState(ctx, h.st, orgID, bot.ID)
	if err != nil {
		return nil, err
	}
	if state.ProjectID == "" {
		return nil, errors.New("the bot has no project yet; it gets one on its first run")
	}
	return &services.OrgBotPRRef{
		OrganizationID:            orgID,
		BotID:                     botID,
		Name:                      bot.Name,
		ProjectID:                 state.ProjectID,
		HiringUserID:              state.HiringUserID,
		AutoApprovePullRequests:   bot.AutoApprovePullRequests,
		AutoApprovePullRequestsBy: bot.AutoApprovePullRequestsBy,
	}, nil
}

func (h orgBotPRHost) SetPRAutoApprove(ctx context.Context, orgID, botID string, enabled bool, by string) error {
	return h.st.Nodes.UpdatePRAutoApprove(ctx, orgID, orgchart.NodeID(botID), enabled, by, h.now())
}

func (h orgBotPRHost) NotifyPRBot(ctx context.Context, orgID, botID, message string, _ types.TurnProvenance) error {
	h.dispatcher.DispatchNotice(ctx, orgID, orgchart.NodeID(botID), "pull request proposal", message)
	return nil
}
