package helix

import (
	"context"
	"errors"
	"fmt"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// ErrNoBoundAgent reports that no single agent Node owns the project.
var ErrNoBoundAgent = errors.New("helix: no agent bound to project")

// BoundAgentForProject returns the one agent Node whose runtime state names
// projectID as its home. Ownership is LoadState().ProjectID only, never the
// managed node.ProjectIDs allowlist; zero or ambiguous owners fail closed.
func BoundAgentForProject(ctx context.Context, st *store.Store, orgID, projectID string) (orgchart.NodeID, error) {
	if st == nil || st.Nodes == nil || orgID == "" || projectID == "" {
		return "", ErrNoBoundAgent
	}
	nodes, err := st.Nodes.List(ctx, orgID)
	if err != nil {
		return "", fmt.Errorf("list org nodes: %w", err)
	}
	var match orgchart.NodeID
	var matches int
	for _, n := range nodes {
		if n.Kind == orgchart.NodeKindHuman || n.ID == "" {
			continue
		}
		state, stateErr := LoadState(ctx, st, orgID, n.ID)
		if stateErr != nil || state.ProjectID == "" {
			continue
		}
		if state.ProjectID == projectID {
			matches++
			match = n.ID
		}
	}
	if matches == 1 {
		return match, nil
	}
	if matches > 1 {
		log.Warn().Str("org_id", orgID).Str("project_id", projectID).
			Int("owners", matches).Msg("org project claimed by multiple agent nodes; resolving as unbound")
	}
	return "", ErrNoBoundAgent
}

// AgentToolNames returns the live tool surface of an agent Node.
func AgentToolNames(ctx context.Context, st *store.Store, orgID string, nodeID orgchart.NodeID) []tool.Name {
	if st == nil || st.Nodes == nil || nodeID == "" {
		return nil
	}
	node, err := st.Nodes.Get(ctx, orgID, nodeID)
	if err != nil {
		log.Debug().Err(err).Str("org_id", orgID).Str("node_id", string(nodeID)).
			Msg("agent tool names: node unreadable; serving empty surface")
		return nil
	}
	return node.Tools
}
