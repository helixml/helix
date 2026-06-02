package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/activation"
	"github.com/helixml/helix/api/pkg/org/grant"
	"github.com/helixml/helix/api/pkg/org/position"
	runtimehelix "github.com/helixml/helix/api/pkg/org/runtime/helix"
	"github.com/helixml/helix/api/pkg/org/tool"
	"github.com/helixml/helix/api/pkg/org/worker"
	"github.com/helixml/helix/api/pkg/org/domain"
)

// HireWorker brings a Worker into existence: a Worker row carrying the
// per-hire IdentityContent, an Environment row pointing at
// <Deps.EnvsDir>/<workerID>/, any tool grants bundled inline, and — for
// AI Workers — a hire activation through the Dispatcher.
//
// State lives in the domain (DB), not on disk. role.md / identity.md /
// agent.md are projected into the Worker's Environment by the Spawner
// at activation time. This keeps every mutation a single DB write and
// lets the env layer evolve (local files today, remote workspaces
// tomorrow) without touching the tools.
//
// Grants are passed inline so the Worker is fully-authorised before the
// Spawner starts their process. Without this, claude would race the
// owner's follow-up grant_tool calls and hit 403s on its first action.
// Grants are data; the tool does not decide what to grant. The
// separate grant_tool tool stays for granting to Workers that already
// exist.
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
	return "Hire a Worker into a Position. The Worker's identityContent (per-hire persona / " +
		"profile) is stored in the domain alongside the Worker row; the spawner projects " +
		"role and identity into the Environment at activation time. Optional `grants` are " +
		"issued atomically with the hire so the Worker is authorised before the agent " +
		"process boots.\n\n" +
		"Always supply `id` as a short, real-sounding handle: a lowercase given name " +
		"prefixed with `w-`, e.g. `w-mark`, `w-priya`, `w-jordan`. Pick a name that fits " +
		"the Position and isn't already taken. Do NOT pass a UUID and do NOT omit `id` " +
		"to let the server invent one — the auto-generated `w-<uuid>` form is reserved as " +
		"a last-resort fallback and is unpleasant to read in logs and UIs. If your first " +
		"choice collides, try a variant (`w-mark-2`, `w-marko`) rather than falling back " +
		"to a UUID."
}

type hireWorkerGrant struct {
	ToolName string `json:"toolName"`
}

type hireWorkerArgs struct {
	ID              string            `json:"id,omitempty"`
	PositionID      string            `json:"positionId"`
	Kind            worker.Kind       `json:"kind"`
	IdentityContent string            `json:"identityContent"`
	Grants          []hireWorkerGrant `json:"grants,omitempty"`
}

// UnmarshalJSON tolerates LLM tool-call quirks where the `grants`
// field arrives as a JSON-encoded string instead of an inline array.
// Sonnet does this intermittently when nested arrays appear in tool
// schemas. We accept either form so callers don't have to retry-and-
// fall-back. Anything else still fails the standard way.
func (a *hireWorkerArgs) UnmarshalJSON(data []byte) error {
	type plain hireWorkerArgs
	type tolerant struct {
		*plain
		Grants json.RawMessage `json:"grants,omitempty"`
	}
	t := tolerant{plain: (*plain)(a)}
	if err := json.Unmarshal(data, &t); err != nil {
		return err
	}
	if len(t.Grants) == 0 || string(t.Grants) == "null" {
		a.Grants = nil
		return nil
	}
	if err := json.Unmarshal(t.Grants, &a.Grants); err == nil {
		return nil
	}
	// Try once more by unwrapping a string-encoded payload.
	var s string
	if err := json.Unmarshal(t.Grants, &s); err != nil {
		return fmt.Errorf("grants: not an array or string: %w", err)
	}
	if err := json.Unmarshal([]byte(s), &a.Grants); err != nil {
		return fmt.Errorf("grants (string-wrapped): %w", err)
	}
	return nil
}

func (t *HireWorker) Invoke(ctx context.Context, inv domain.Invocation) (json.RawMessage, error) {
	var args hireWorkerArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if err := args.Kind.Validate(); err != nil {
		return nil, err
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

	pos, err := t.deps.Store.Positions.Get(ctx, orgID, position.ID(args.PositionID))
	if err != nil {
		return nil, fmt.Errorf("position %q: %w", args.PositionID, err)
	}

	id := worker.ID(args.ID)
	if id == "" {
		id = worker.ID("w-" + t.deps.NewID())
	}
	envPath := filepath.Join(t.deps.EnvsDir, string(id))

	var wkr domain.Worker
	switch args.Kind {
	case worker.KindHuman:
		w, err := domain.NewHumanWorker(id, pos.ID, args.IdentityContent, orgID)
		if err != nil {
			return nil, err
		}
		wkr = w
	case worker.KindAI:
		w, err := domain.NewAIWorker(id, pos.ID, args.IdentityContent, orgID)
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

	env, err := domain.NewEnvironment(id, envPath, t.deps.Now(), orgID)
	if err != nil {
		return nil, err
	}
	if err := t.deps.Store.Environments.Create(ctx, env); err != nil {
		return nil, fmt.Errorf("create environment: %w", err)
	}

	for i, g := range args.Grants {
		if g.ToolName == "" {
			return nil, fmt.Errorf("grants[%d]: toolName is required", i)
		}
		grantID := grant.ID("g-" + t.deps.NewID())
		grantRow, err := domain.NewToolGrant(grantID, id, tool.Name(g.ToolName), orgID)
		if err != nil {
			return nil, fmt.Errorf("grants[%d]: %w", i, err)
		}
		if err := t.deps.Store.Grants.Create(ctx, grantRow); err != nil {
			return nil, fmt.Errorf("grants[%d] (%s): %w", i, g.ToolName, err)
		}
	}

	if args.Kind == worker.KindAI {
		if err := EnsureActivationStream(ctx, t.deps.Store, orgID, id, inv.Caller.ID(), t.deps.Now()); err != nil {
			return nil, err
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
	if args.Kind == worker.KindAI && t.deps.Store.Activations != nil {
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

	if args.Kind == worker.KindAI && t.deps.Dispatcher != nil {
		t.deps.Dispatcher.DispatchHire(ctx, orgID, id, envPath, hireActID)
	}

	resp := map[string]string{"id": string(id)}
	if hireActID != "" {
		resp["activation_id"] = string(hireActID)
	}
	return json.Marshal(resp)
}
