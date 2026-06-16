package mcptools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

type workerView struct {
	ID        orgchart.WorkerID   `json:"id"`
	Kind      orgchart.WorkerKind `json:"kind"`
	RoleID    orgchart.RoleID     `json:"roleId,omitempty"`
	ParentIDs []orgchart.WorkerID `json:"parentIds,omitempty"`
}

func workerViewOf(w orgchart.Worker, managers []orgchart.WorkerID) workerView {
	return workerView{ID: w.ID(), Kind: w.Kind(), RoleID: w.RoleID(), ParentIDs: managers}
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
	return "List every Worker: id, kind (human|ai), Role, and reporting parent."
}

func (t *ListWorkers) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("list_workers: caller has no OrgID")
	}
	workers, err := t.deps.Queries.ListWorkers(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("list workers: %w", err)
	}
	// One List call builds the report → managers index, so we don't
	// issue a ListManagers per worker.
	managersByReport := map[orgchart.WorkerID][]orgchart.WorkerID{}
	if t.deps.Queries.ReportingLinesWired() {
		lines, err := t.deps.Queries.ListReportingLines(ctx, orgID)
		if err != nil {
			return nil, fmt.Errorf("list reporting lines: %w", err)
		}
		for _, l := range lines {
			managersByReport[l.ReportID] = append(managersByReport[l.ReportID], l.ManagerID)
		}
	}
	out := make([]workerView, 0, len(workers))
	for _, w := range workers {
		out = append(out, workerViewOf(w, managersByReport[w.ID()]))
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

func (t *GetWorker) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args getWorkerArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.ID == "" {
		return nil, fmt.Errorf("id is required")
	}
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("get_worker: caller has no OrgID")
	}
	w, err := t.deps.Queries.GetWorker(ctx, orgID, orgchart.WorkerID(args.ID))
	if err != nil {
		return nil, fmt.Errorf("get worker %q: %w", args.ID, err)
	}
	var managers []orgchart.WorkerID
	if t.deps.Queries.ReportingLinesWired() {
		managers, err = t.deps.Queries.ListManagers(ctx, orgID, w.ID())
		if err != nil {
			return nil, fmt.Errorf("list managers for %q: %w", args.ID, err)
		}
	}
	return json.Marshal(workerViewOf(w, managers))
}
