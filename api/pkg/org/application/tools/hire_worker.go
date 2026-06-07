package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/environment"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	runtimehelix "github.com/helixml/helix/api/pkg/org/infrastructure/runtime/helix"
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
// the Role and every Worker holding it sees the new tool set on the
// next MCP request. There is no per-Worker grants table and no
// `grants` parameter on this tool — capability is the Role's
// responsibility.
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
	if err := args.Kind.Validate(); err != nil {
		return nil, err
	}
	if args.RoleID == "" {
		return nil, fmt.Errorf("roleId is required")
	}
	if args.IdentityContent == "" {
		return nil, fmt.Errorf("identityContent is required")
	}
	if t.deps.EnvsDir == "" {
		return nil, fmt.Errorf("server is not configured with an envs directory")
	}

	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("hire_worker: caller has no OrgID")
	}

	roleID := orgchart.RoleID(args.RoleID)
	if _, err := t.deps.Store.Roles.Get(ctx, orgID, roleID); err != nil {
		return nil, fmt.Errorf("role %q: %w", args.RoleID, err)
	}

	var parent *orgchart.WorkerID
	if args.ParentID != "" {
		parentID := orgchart.WorkerID(args.ParentID)
		if _, err := t.deps.Store.Workers.Get(ctx, orgID, parentID); err != nil {
			return nil, fmt.Errorf("parent worker %q: %w", args.ParentID, err)
		}
		parent = &parentID
	}

	id := orgchart.WorkerID(args.ID)
	if id == "" {
		id = orgchart.WorkerID("w-" + t.deps.NewID())
	}
	envPath := filepath.Join(t.deps.EnvsDir, string(id))

	var wkr orgchart.Worker
	switch args.Kind {
	case orgchart.WorkerKindHuman:
		w, err := orgchart.NewHumanWorker(id, roleID, args.IdentityContent, orgID)
		if err != nil {
			return nil, err
		}
		wkr = w
	case orgchart.WorkerKindAI:
		w, err := orgchart.NewAIWorker(id, roleID, args.IdentityContent, orgID)
		if err != nil {
			return nil, err
		}
		wkr = w
	default:
		// Unreachable: Validate() above already rejected unknown kinds.
		return nil, args.Kind.Validate()
	}

	if err := os.MkdirAll(envPath, 0o750); err != nil {
		return nil, fmt.Errorf("create env dir %q: %w", envPath, err)
	}

	if err := t.deps.Store.Workers.Create(ctx, wkr); err != nil {
		return nil, err
	}

	// Wire the initial reporting line (the new hire reports to parent)
	// now that both Worker rows exist. Reporting is a many-to-many
	// relation — more managers can be added later via the add-parent
	// endpoint.
	if parent != nil && t.deps.Store.ReportingLines != nil {
		line, err := orgchart.NewReportingLine(orgID, *parent, id)
		if err != nil {
			return nil, err
		}
		if err := t.deps.Store.ReportingLines.Add(ctx, line); err != nil {
			return nil, fmt.Errorf("add reporting line: %w", err)
		}
	}

	env, err := environment.New(id, envPath, t.deps.Now(), orgID)
	if err != nil {
		return nil, err
	}
	if err := t.deps.Store.Environments.Create(ctx, env); err != nil {
		return nil, fmt.Errorf("create environment: %w", err)
	}

	// Reconcile the activation/team Streams implied by the new Worker
	// and its reporting line. Topology is the single owner of those
	// Streams: this mints the hire's activation Stream (subscribers =
	// its managers) and adds the hire to the manager's team Stream
	// (creating it on the manager's first report). Replaces the inline
	// EnsureActivationStream call — same outcome for the AI case, plus
	// the team-stream wiring, derived declaratively from the graph.
	if t.deps.Topology != nil {
		if err := t.deps.Topology.Reconcile(ctx, orgID, id); err != nil {
			return nil, fmt.Errorf("reconcile topology for hire %q: %w", id, err)
		}
	}

	// Persist the hiring user's identity (if the request carried one)
	// BEFORE we dispatch the hire activation so the Spawner can pick
	// it up on its very first call. Empty userID — standalone helix-org
	// with no HTTP auth, or any path that didn't stash a user — is a
	// no-op; the Spawner then falls back to its static service api_key.
	//
	// Routes through the runtime.HireHook port so non-helix runtimes
	// (claude / dev / test) can no-op without hire_worker knowing
	// anything about helix-runtime internals.
	if uid := runtimehelix.UserIDFromContext(ctx); uid != "" && t.deps.HireHook != nil {
		if err := t.deps.HireHook.OnHire(ctx, orgID, id, uid); err != nil {
			return nil, fmt.Errorf("hire handler: %w", err)
		}
	}

	// Pre-create the hire-Activation audit row so hire_worker can
	// return the ID synchronously. The Spawner picks up this row
	// (matched by Trigger.ActivationID) and Completes it when the
	// activation finishes, rather than minting a sibling.
	var hireActID activation.ID
	if args.Kind == orgchart.WorkerKindAI && t.deps.Store.Activations != nil {
		hireActID = activation.ID("a-" + t.deps.NewID())
		hireAct, err := activation.New(
			hireActID,
			id,
			[]activation.Trigger{{Kind: activation.TriggerHire}},
			t.deps.Now(),
			orgID,
		)
		if err != nil {
			return nil, fmt.Errorf("build hire activation: %w", err)
		}
		if err := t.deps.Store.Activations.Create(ctx, hireAct); err != nil {
			return nil, fmt.Errorf("persist hire activation: %w", err)
		}
	}

	if args.Kind == orgchart.WorkerKindAI && t.deps.Dispatcher != nil {
		t.deps.Dispatcher.DispatchHire(ctx, orgID, id, envPath, hireActID)
	}

	resp := map[string]string{"id": string(id)}
	if hireActID != "" {
		resp["activation_id"] = string(hireActID)
	}
	return json.Marshal(resp)
}
