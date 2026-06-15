package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
)

// GetWorkerProject reads the Worker's helix project configuration —
// the startup script today, with skills / guidelines following the
// same shape as Helix exposes more of ProjectUpdateRequest through
// the ProjectConfig port. The owner uses this before
// configure_worker_project to read what's currently set, so the
// LLM can produce a sensible patch ("add `apt install -y gh` to
// the existing script") rather than blindly overwriting it.
type GetWorkerProject struct {
	deps Deps
}

const GetWorkerProjectName tool.Name = "get_worker_project"

// NewGetWorkerProject is the public constructor — exported so
// tests can stand up a tool with a fake ProjectConfig without
// going through the full RegisterBuiltins path.
func NewGetWorkerProject(deps Deps) *GetWorkerProject {
	return &GetWorkerProject{deps: deps}
}

var getWorkerProjectSchema = mustSchema[getWorkerProjectArgs]()

type getWorkerProjectArgs struct {
	WorkerID string `json:"workerId"`
}

func (t *GetWorkerProject) Name() tool.Name                 { return GetWorkerProjectName }
func (t *GetWorkerProject) InputSchema() *jsonschema.Schema { return getWorkerProjectSchema }
func (t *GetWorkerProject) Description() string {
	return "Read the helix project configuration for a Worker — the startup script today, " +
		"plus any other configurable project field as the port adds them. Use this before " +
		"configure_worker_project so you can patch a known-current state rather than blindly " +
		"overwriting. Owner-only."
}

func (t *GetWorkerProject) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args getWorkerProjectArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.WorkerID == "" {
		return nil, errors.New("workerId is required")
	}
	orgID, err := projectConfigOrgID(inv)
	if err != nil {
		return nil, err
	}
	cfg := t.deps.ProjectConfig
	if cfg == nil {
		// Defensive: production wiring always sets ProjectConfig
		// (NoopProjectConfig by default), but if a test wires Deps
		// from scratch and forgets, we want a clear message rather
		// than a nil-deref.
		return nil, runtime.ErrProjectConfigUnsupported
	}
	snap, err := cfg.GetWorkerProjectConfig(ctx, orgID, orgchart.WorkerID(args.WorkerID))
	if err != nil {
		return nil, fmt.Errorf("get worker project: %w", err)
	}
	return json.Marshal(snap)
}

// projectConfigOrgID extracts the caller's orgID from the
// Invocation. Shared between the get + configure tools so they
// stay in lockstep on the auth assumption.
func projectConfigOrgID(inv tool.Invocation) (string, error) {
	if inv.Caller == nil {
		return "", errors.New("caller missing on invocation")
	}
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return "", errors.New("caller has no organization id")
	}
	return orgID, nil
}
