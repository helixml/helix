package mcptools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/application/lifecycle"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// HireWorker brings a Worker into existence: a Worker row carrying the
// per-hire IdentityContent, an Environment row pointing at
// <Deps.EnvsDir>/<workerID>/, and — for AI Workers — a hire activation
// through the Dispatcher.
//
// State lives in the domain (DB), not on disk. role.md / identity.md /
// agent.md are projected into the Worker's Environment by the Spawner
// at activation time. This keeps every mutation a single DB write and
// lets the env layer evolve (local files today, remote workspaces
// tomorrow) without touching the tools.
//
// A Worker's MCP tool surface is derived live from Role.Tools: change
// the Role and every Worker filling it sees the new tool set on the
// next MCP request. There is no per-Worker tool record and no
// `tools` parameter on this tool — the Role's tool list is the whole
// story.
//
// hire_worker does not subscribe to Streams; the hiring Worker does
// that explicitly after the Worker is alive, typically via the Worker's
// own on-hire activation. (Per ADR-0001 §1 the canonical term is
// Stream, not Channel.)
//
// For AI Workers, hire_worker also creates the per-Worker activation
// Stream (s-activations-<workerID>) and subscribes the hiring Worker to
// it. The Spawner publishes one event per assistant message, tool call,
// and tool result to that Stream — the hiring Worker can audit their
// hires by calling read_events on it. The new Worker themselves is
// intentionally never subscribed to their own activation Stream
// (otherwise self-published events would re-trigger them indefinitely).
type HireWorker struct {
	deps Deps
}

// NewHireWorker constructs the tool with its dependencies. Exported so
// non-MCP callers (the REST POST /workers handler in
// api/pkg/org/server/api) can drive the same hire path the MCP
// surface uses.
func NewHireWorker(deps Deps) *HireWorker {
	return &HireWorker{deps: deps}
}

const HireWorkerName tool.Name = "hire_worker"

var hireWorkerSchema = mustSchema[hireWorkerArgs]()

func (t *HireWorker) Name() tool.Name                 { return HireWorkerName }
func (t *HireWorker) InputSchema() *jsonschema.Schema { return hireWorkerSchema }
func (t *HireWorker) Description() string {
	return "Hire a Worker into a Role. The Worker's identityContent (per-hire persona / " +
		"profile) is stored in the domain alongside the Worker row; the spawner projects " +
		"role and identity into the Environment at activation time. The Worker's MCP tool " +
		"surface comes from Role.Tools — to change a Worker's capabilities, edit the Role.\n\n" +
		"`parentId` is the manager Worker this hire reports to. Omit it only for the org " +
		"owner (bootstrap creates that); every other Worker has a manager.\n\n" +
		"Always supply `id` as a short, real-sounding handle: a lowercase given name " +
		"prefixed with `w-`, e.g. `w-mark`, `w-priya`, `w-jordan`. Pick a name that fits " +
		"the Role and isn't already taken. Do NOT pass a UUID and do NOT omit `id` " +
		"to let the server invent one — the auto-generated `w-<uuid>` form is reserved as " +
		"a last-resort fallback and is unpleasant to read in logs and UIs. If your first " +
		"choice collides, try a variant (`w-mark-2`, `w-marko`) rather than falling back " +
		"to a UUID."
}

type hireWorkerArgs struct {
	ID              string              `json:"id,omitempty"`
	RoleID          string              `json:"roleId"`
	ParentID        string              `json:"parentId,omitempty"`
	Kind            orgchart.WorkerKind `json:"kind"`
	IdentityContent string              `json:"identityContent"`
}

func (t *HireWorker) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args hireWorkerArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("hire_worker: caller has no OrgID")
	}
	res, err := t.deps.lifecycleService().Hire(ctx, orgID, lifecycle.HireParams{
		ID:              args.ID,
		RoleID:          orgchart.RoleID(args.RoleID),
		ParentID:        orgchart.WorkerID(args.ParentID),
		Kind:            args.Kind,
		IdentityContent: args.IdentityContent,
	})
	if err != nil {
		return nil, err
	}
	resp := map[string]string{"id": string(res.WorkerID)}
	if res.ActivationID != "" {
		resp["activation_id"] = string(res.ActivationID)
	}
	return json.Marshal(resp)
}
