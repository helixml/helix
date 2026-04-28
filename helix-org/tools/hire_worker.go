package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix-org/domain"
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
// hire_worker does not subscribe to Channels; the manager does that
// explicitly after the Worker is alive, typically via the Worker's own
// on-hire activation.
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

const HireWorkerName domain.ToolName = "hire_worker"

var hireWorkerSchema = mustSchema[hireWorkerArgs]()

func (t *HireWorker) Name() domain.ToolName           { return HireWorkerName }
func (t *HireWorker) InputSchema() *jsonschema.Schema { return hireWorkerSchema }
func (t *HireWorker) Description() string {
	return "Hire a Worker into a Position. The Worker's identityContent (per-hire persona / " +
		"profile) is stored in the domain alongside the Worker row; the spawner projects " +
		"role and identity into the Environment at activation time. Optional `grants` are " +
		"issued atomically with the hire so the Worker is authorised before the agent " +
		"process boots."
}

type hireWorkerGrant struct {
	ToolName string `json:"toolName"`
}

type hireWorkerArgs struct {
	ID              string            `json:"id,omitempty"`
	PositionID      string            `json:"positionId"`
	Kind            domain.WorkerKind `json:"kind"`
	IdentityContent string            `json:"identityContent"`
	Grants          []hireWorkerGrant `json:"grants,omitempty"`
}

func (t *HireWorker) Invoke(ctx context.Context, inv domain.Invocation) (json.RawMessage, error) {
	var args hireWorkerArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.IdentityContent == "" {
		return nil, fmt.Errorf("identityContent is required")
	}
	if t.deps.EnvsDir == "" {
		return nil, fmt.Errorf("server is not configured with an envs directory")
	}

	pos, err := t.deps.Store.Positions.Get(ctx, domain.PositionID(args.PositionID))
	if err != nil {
		return nil, fmt.Errorf("position %q: %w", args.PositionID, err)
	}

	id := domain.WorkerID(args.ID)
	if id == "" {
		id = domain.WorkerID("w-" + t.deps.NewID())
	}
	envPath := filepath.Join(t.deps.EnvsDir, string(id))

	var worker domain.Worker
	switch args.Kind {
	case domain.WorkerKindHuman:
		w, err := domain.NewHumanWorker(id, []domain.PositionID{pos.ID}, args.IdentityContent)
		if err != nil {
			return nil, err
		}
		worker = w
	case domain.WorkerKindAI:
		w, err := domain.NewAIWorker(id, []domain.PositionID{pos.ID}, args.IdentityContent)
		if err != nil {
			return nil, err
		}
		worker = w
	default:
		return nil, fmt.Errorf("unknown worker kind %q", args.Kind)
	}

	// The env directory exists so it can be the Worker's cwd at
	// activation; the spawner writes role.md / identity.md / agent.md
	// into it just before exec'ing claude. Nothing on disk is the
	// source of truth.
	if err := os.MkdirAll(envPath, 0o750); err != nil {
		return nil, fmt.Errorf("create env dir %q: %w", envPath, err)
	}

	if err := t.deps.Store.Workers.Create(ctx, worker); err != nil {
		return nil, err
	}

	env, err := domain.NewEnvironment(id, envPath, t.deps.Now())
	if err != nil {
		return nil, err
	}
	if err := t.deps.Store.Environments.Create(ctx, env); err != nil {
		return nil, fmt.Errorf("create environment: %w", err)
	}

	// Issue bundled grants before the Spawner runs. An AI Worker that
	// comes up without its grants immediately fails on its first tool
	// call.
	for i, g := range args.Grants {
		if g.ToolName == "" {
			return nil, fmt.Errorf("grants[%d]: toolName is required", i)
		}
		grantID := domain.GrantID("g-" + t.deps.NewID())
		grant, err := domain.NewToolGrant(grantID, id, domain.ToolName(g.ToolName))
		if err != nil {
			return nil, fmt.Errorf("grants[%d]: %w", i, err)
		}
		if err := t.deps.Store.Grants.Create(ctx, grant); err != nil {
			return nil, fmt.Errorf("grants[%d] (%s): %w", i, g.ToolName, err)
		}
	}

	if args.Kind == domain.WorkerKindAI {
		if err := createActivationStream(ctx, t.deps, id, inv.Caller.ID()); err != nil {
			return nil, err
		}
	}

	if args.Kind == domain.WorkerKindAI && t.deps.Dispatcher != nil {
		t.deps.Dispatcher.DispatchHire(ctx, id, envPath)
	}

	return json.Marshal(map[string]string{"id": string(id)})
}

// createActivationStream creates the per-Worker activation Stream and
// subscribes the hiring Worker to it. The Stream ID is deterministic
// (s-activations-<workerID>) so the Spawner can find it without an
// extra lookup.
func createActivationStream(ctx context.Context, deps Deps, workerID, hiringWorkerID domain.WorkerID) error {
	streamID := activationStreamID(workerID)
	stream, err := domain.NewStream(
		streamID,
		"Activations: "+string(workerID),
		"Per-message activation transcript for "+string(workerID)+
			" — assistant text, tool calls, tool results. "+
			"Read with read_events to audit a hire.",
		hiringWorkerID,
		deps.Now(),
		domain.Transport{},
	)
	if err != nil {
		return fmt.Errorf("activation stream: %w", err)
	}
	if err := deps.Store.Streams.Create(ctx, stream); err != nil {
		return fmt.Errorf("create activation stream: %w", err)
	}
	sub, err := domain.NewSubscription(hiringWorkerID, streamID, deps.Now())
	if err != nil {
		return fmt.Errorf("activation subscription: %w", err)
	}
	if err := deps.Store.Subscriptions.Create(ctx, sub); err != nil {
		return fmt.Errorf("subscribe %q to activation stream: %w", hiringWorkerID, err)
	}
	return nil
}
