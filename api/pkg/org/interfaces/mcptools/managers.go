package mcptools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/domain/channels"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// Managers is the upward, escalation-direction read: who you report to,
// and the DM topic you escalate on. Computed live from the reporting
// graph at call time, so it stays correct as the org is reshaped — the
// fixed worker-policy prompt can refer to "your managers" abstractly and
// let a blocked Worker resolve the concrete people only when it needs
// them.
type Managers struct {
	deps Deps
}

const ManagersName tool.Name = "managers"

var managersSchema = mustSchema[managersArgs]()

type managersArgs struct{}

type managerView struct {
	ID         orgchart.WorkerID  `json:"id"`
	Role       orgchart.RoleID    `json:"role"`
	DMTopicID streaming.TopicID `json:"dmTopicId"`
}

type managersResult struct {
	Managers []managerView `json:"managers"`
}

func (t *Managers) Name() tool.Name                 { return ManagersName }
func (t *Managers) InputSchema() *jsonschema.Schema { return managersSchema }
func (t *Managers) Description() string {
	return "List the managers you report to — your escalation targets. Takes no " +
		"arguments; resolves your own reporting lines live. Each manager comes with " +
		"the deterministic DM topic id you escalate on. When you are blocked on a " +
		"decision above your authority, call this, then `dm` one manager with the " +
		"decision needed + options + your recommendation, and `read_events` (with " +
		"wait) on that dmTopicId for the reply. You already receive your managers' " +
		"team-topic briefings without asking — those arrive via your subscriptions."
}

func (t *Managers) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("managers: caller has no OrgID")
	}
	caller := orgchart.WorkerID(inv.Caller.ID())
	if !t.deps.Queries.ReportingLinesWired() {
		return json.Marshal(managersResult{Managers: []managerView{}})
	}
	managerIDs, err := t.deps.Queries.ListManagers(ctx, orgID, caller)
	if err != nil {
		return nil, fmt.Errorf("list managers: %w", err)
	}
	out := make([]managerView, 0, len(managerIDs))
	for _, m := range managerIDs {
		w, err := t.deps.Queries.GetWorker(ctx, orgID, m)
		if err != nil {
			// A line pointing at a worker that no longer exists is a
			// dangling row, not a manager — skip it rather than report a
			// phantom.
			if errors.Is(err, store.ErrNotFound) {
				continue
			}
			return nil, fmt.Errorf("get manager %q: %w", m, err)
		}
		out = append(out, managerView{
			ID:         w.ID(),
			Role:       w.RoleID(),
			DMTopicID: channels.DMTopicID(caller, m),
		})
	}
	return json.Marshal(managersResult{Managers: out})
}
