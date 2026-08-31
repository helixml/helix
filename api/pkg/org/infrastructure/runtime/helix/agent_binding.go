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

// ErrNoBoundAgent reports that no single agent Node owns the project —
// either none does or the ownership is ambiguous. Both cases fail closed:
// delegated task access derives from an unambiguous bond or not at all.
var ErrNoBoundAgent = errors.New("helix: no agent bound to project")

// BoundAgentForProject returns the single agent Node whose Helix runtime
// state names projectID as its home project. This is the org's standing
// agent↔project bond: a Worker's project is created with it at hire time
// (SaveProject), so the mapping is one-way-derived from live state and
// disappears automatically when the Worker is deleted or re-homed.
//
// Ownership means exactly LoadState(node).ProjectID == projectID — never
// an entry in node.ProjectIDs, which is the *managed* cross-project
// allowlist (a manager bot supervising another bot's project must not
// grant its org-wide tool/credential surface to tasks in that project).
//
// A store error or missing runtime state on a node is treated as "not an
// owner" (fail closed); a transient failure across all nodes yields
// ErrNoBoundAgent, never a wrong agent.
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
		// Ambiguous ownership should be impossible (one ProjectID per hire);
		// fail closed loudly rather than guess which bot's grants apply.
		log.Warn().Str("org_id", orgID).Str("project_id", projectID).
			Int("owners", matches).Msg("org project claimed by multiple agent nodes; resolving as unbound")
	}
	return "", ErrNoBoundAgent
}

// AgentToolNames returns the live tool surface of an agent Node. Node.Tools
// is the canonical live MCP surface (the reconciler prunes names the
// registry no longer knows), so tasks resolve whatever their Agent holds
// right now — attach_tool/detach_tool propagate with no copies.
func AgentToolNames(ctx context.Context, st *store.Store, orgID string, nodeID orgchart.NodeID) []tool.Name {
	if st == nil || st.Nodes == nil || nodeID == "" {
		return nil
	}
	node, err := st.Nodes.Get(ctx, orgID, nodeID)
	if err != nil {
		// The agent may have been fired between resolution and here; treat as
		// no surface rather than failing the whole tool list.
		log.Debug().Err(err).Str("org_id", orgID).Str("node_id", string(nodeID)).
			Msg("agent tool names: node unreadable; serving empty surface")
		return nil
	}
	return node.Tools
}
