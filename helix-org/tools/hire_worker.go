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

// HireWorker brings a Worker into existence: a Worker row, an
// Environment row, the per-Worker markdown trio (role.md / identity.md
// / agent.md), any tool grants bundled inline, and — for AI Workers —
// a hire activation through the Dispatcher.
//
// The system owns the Worker's filesystem layout. The Environment is
// always at <Deps.EnvsDir>/<workerID>/, created here. role.md is the
// Role.Content as it stands at hire time (subsequent updates land via
// UpdateRole). identity.md is the per-hire markdown supplied by the
// caller. agent.md is a fixed stub the Spawner reads as its entry
// point.
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
	return "Hire a Worker into a Position. The system creates the Worker's Environment under " +
		"the configured envs directory and stamps role.md (from the Role) and identity.md " +
		"(from identityContent) into it. Optional `grants` are issued atomically with the hire " +
		"so the Worker is authorised before the agent process boots."
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

// agentMDStub is the fixed entry-point text written into every new
// Worker's Environment. The Spawner reads it; Claude follows it.
const agentMDStub = `Read role.md (your job) and identity.md (who you are).
Then act on the trigger described below.
Each activation is a single turn — do the work and exit.
`

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

	role, err := t.deps.Store.Roles.Get(ctx, pos.RoleID)
	if err != nil {
		return nil, fmt.Errorf("role %q: %w", pos.RoleID, err)
	}

	id := domain.WorkerID(args.ID)
	if id == "" {
		id = domain.WorkerID("w-" + t.deps.NewID())
	}
	envPath := filepath.Join(t.deps.EnvsDir, string(id))

	var worker domain.Worker
	switch args.Kind {
	case domain.WorkerKindHuman:
		w, err := domain.NewHumanWorker(id, []domain.PositionID{pos.ID})
		if err != nil {
			return nil, err
		}
		worker = w
	case domain.WorkerKindAI:
		w, err := domain.NewAIWorker(id, []domain.PositionID{pos.ID})
		if err != nil {
			return nil, err
		}
		worker = w
	default:
		return nil, fmt.Errorf("unknown worker kind %q", args.Kind)
	}

	if err := os.MkdirAll(envPath, 0o750); err != nil {
		return nil, fmt.Errorf("create env dir %q: %w", envPath, err)
	}
	if err := writeEnvFile(envPath, "role.md", role.Content); err != nil {
		return nil, err
	}
	if err := writeEnvFile(envPath, "identity.md", args.IdentityContent); err != nil {
		return nil, err
	}
	if err := writeEnvFile(envPath, "agent.md", agentMDStub); err != nil {
		return nil, err
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

// writeEnvFile writes content to a file inside a Worker's Environment
// directory. The mode is 0o600 — these files describe behaviour and
// identity and shouldn't be world-readable.
func writeEnvFile(envPath, name, content string) error {
	full := filepath.Join(envPath, name)
	if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write %q: %w", full, err)
	}
	return nil
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
