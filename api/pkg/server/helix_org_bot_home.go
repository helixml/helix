package server

import (
	"context"
	"fmt"

	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/hydra"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	helixorgstore "github.com/helixml/helix/api/pkg/org/domain/store"
	runtimehelix "github.com/helixml/helix/api/pkg/org/infrastructure/runtime/helix"
)

// orgBotHomeRegistry stores bot home records in the Org Bot runtime state,
// which belongs to the bot and is removed with it.
type orgBotHomeRegistry struct {
	st *helixorgstore.Store
}

var _ external_agent.BotHomeRegistry = orgBotHomeRegistry{}

func (r orgBotHomeRegistry) LoadBotHome(ctx context.Context, orgID, botID string) (external_agent.BotHomeRecord, error) {
	state, err := runtimehelix.LoadState(ctx, r.st, orgID, orgchart.NodeID(botID))
	if err != nil {
		return external_agent.BotHomeRecord{}, err
	}
	return external_agent.BotHomeRecord{
		State:      external_agent.BotHomeState(state.Home.State),
		Generation: state.Home.Generation,
		SandboxID:  state.Home.SandboxID,
		Detail:     state.Home.Detail,
		SessionID:  state.SessionID,
	}, nil
}

func (r orgBotHomeRegistry) SaveBotHome(ctx context.Context, orgID, botID string, rec external_agent.BotHomeRecord) error {
	return runtimehelix.SaveHome(ctx, r.st, orgID, orgchart.NodeID(botID), runtimehelix.HomeRecord{
		State:      string(rec.State),
		Generation: rec.Generation,
		SandboxID:  rec.SandboxID,
		Detail:     rec.Detail,
	})
}

func (r orgBotHomeRegistry) ListLiveBotHomes(ctx context.Context) ([]string, error) {
	ids, err := r.st.Nodes.ListAllIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list bots: %w", err)
	}
	keys := make([]string, 0, len(ids))
	for _, id := range ids {
		keys = append(keys, hydra.BotHomeKey(id.OrgID, id.ID))
	}
	return keys, nil
}
