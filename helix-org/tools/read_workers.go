package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/position"
	"github.com/helixml/helix/api/pkg/org/tool"
	"github.com/helixml/helix/api/pkg/org/worker"
	"github.com/helixml/helix/helix-org/domain"
)

type workerView struct {
	ID       worker.ID   `json:"id"`
	Kind     worker.Kind `json:"kind"`
	Position position.ID `json:"position,omitempty"`
}

func workerViewOf(w domain.Worker) workerView {
	return workerView{ID: w.ID(), Kind: w.Kind(), Position: w.Position()}
}

// ListWorkers returns every Worker — humans and AIs.
type ListWorkers struct {
	deps Deps
}

const ListWorkersName tool.Name = "list_workers"

var listWorkersSchema = mustSchema[listWorkersArgs]()

type listWorkersArgs struct{}

func (t *ListWorkers) Name() tool.Name                 { return ListWorkersName }
func (t *ListWorkers) InputSchema() *jsonschema.Schema { return listWorkersSchema }
func (t *ListWorkers) Description() string {
	return "List every Worker: id, kind (human|ai), and Positions held."
}

func (t *ListWorkers) Invoke(ctx context.Context, _ domain.Invocation) (json.RawMessage, error) {
	workers, err := t.deps.Store.Workers.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list workers: %w", err)
	}
	out := make([]workerView, 0, len(workers))
	for _, w := range workers {
		out = append(out, workerViewOf(w))
	}
	return json.Marshal(map[string]any{"workers": out})
}

// GetWorker returns one Worker by ID.
type GetWorker struct {
	deps Deps
}

const GetWorkerName tool.Name = "get_worker"

var getWorkerSchema = mustSchema[getWorkerArgs]()

type getWorkerArgs struct {
	ID string `json:"id"`
}

func (t *GetWorker) Name() tool.Name                 { return GetWorkerName }
func (t *GetWorker) InputSchema() *jsonschema.Schema { return getWorkerSchema }
func (t *GetWorker) Description() string {
	return "Fetch one Worker by id."
}

func (t *GetWorker) Invoke(ctx context.Context, inv domain.Invocation) (json.RawMessage, error) {
	var args getWorkerArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.ID == "" {
		return nil, fmt.Errorf("id is required")
	}
	w, err := t.deps.Store.Workers.Get(ctx, worker.ID(args.ID))
	if err != nil {
		return nil, fmt.Errorf("get worker %q: %w", args.ID, err)
	}
	return json.Marshal(workerViewOf(w))
}

// ListWorkerGrants returns every ToolGrant held by one Worker.
type ListWorkerGrants struct {
	deps Deps
}

const ListWorkerGrantsName tool.Name = "list_worker_grants"

var listWorkerGrantsSchema = mustSchema[listWorkerGrantsArgs]()

type listWorkerGrantsArgs struct {
	WorkerID string `json:"workerId"`
}

func (t *ListWorkerGrants) Name() tool.Name                 { return ListWorkerGrantsName }
func (t *ListWorkerGrants) InputSchema() *jsonschema.Schema { return listWorkerGrantsSchema }
func (t *ListWorkerGrants) Description() string {
	return "List the ToolGrants held by a Worker — i.e. the tools they may invoke over MCP."
}

func (t *ListWorkerGrants) Invoke(ctx context.Context, inv domain.Invocation) (json.RawMessage, error) {
	var args listWorkerGrantsArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.WorkerID == "" {
		return nil, fmt.Errorf("workerId is required")
	}
	grants, err := t.deps.Store.Grants.ListByWorker(ctx, worker.ID(args.WorkerID))
	if err != nil {
		return nil, fmt.Errorf("list grants for %q: %w", args.WorkerID, err)
	}
	out := make([]grantView, 0, len(grants))
	for _, g := range grants {
		out = append(out, grantViewOf(g))
	}
	return json.Marshal(map[string]any{"grants": out})
}

// GetWorkerEnvironment returns the on-disk Environment record for a Worker.
type GetWorkerEnvironment struct {
	deps Deps
}

const GetWorkerEnvironmentName tool.Name = "get_worker_environment"

var getWorkerEnvironmentSchema = mustSchema[getWorkerEnvironmentArgs]()

type getWorkerEnvironmentArgs struct {
	WorkerID string `json:"workerId"`
}

type environmentView struct {
	WorkerID  worker.ID `json:"workerId"`
	Path      string    `json:"path"`
	CreatedAt time.Time `json:"createdAt"`
}

func (t *GetWorkerEnvironment) Name() tool.Name { return GetWorkerEnvironmentName }
func (t *GetWorkerEnvironment) InputSchema() *jsonschema.Schema {
	return getWorkerEnvironmentSchema
}
func (t *GetWorkerEnvironment) Description() string {
	return "Fetch a Worker's Environment record: the path on disk where their role.md, " +
		"identity.md, and agent.md live."
}

func (t *GetWorkerEnvironment) Invoke(ctx context.Context, inv domain.Invocation) (json.RawMessage, error) {
	var args getWorkerEnvironmentArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.WorkerID == "" {
		return nil, fmt.Errorf("workerId is required")
	}
	env, err := t.deps.Store.Environments.Get(ctx, worker.ID(args.WorkerID))
	if err != nil {
		return nil, fmt.Errorf("get environment for %q: %w", args.WorkerID, err)
	}
	return json.Marshal(environmentView{
		WorkerID:  env.WorkerID,
		Path:      env.Path,
		CreatedAt: env.CreatedAt,
	})
}
