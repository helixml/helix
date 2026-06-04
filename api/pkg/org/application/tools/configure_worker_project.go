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

// ConfigureWorkerProject applies a partial patch to a Worker's
// helix project configuration. Today: startupScript. Future:
// additional project-level config fields (skills, guidelines, …)
// — each one becomes a new optional patch field here and on the
// ProjectConfigPatch struct.
//
// Patch semantics: any field LEFT OUT of the JSON body is
// preserved as-is. Setting a field to "" (or false/empty array)
// is treated as an explicit intent to clear; the underlying port
// passes a non-nil pointer for that case so the runtime
// distinguishes "not touched" from "explicitly empty".
//
// The tool refuses an args object with no patch fields set —
// otherwise a careless caller could silently no-op when it
// expected to write. Forces the LLM to be explicit about what
// it's changing.
type ConfigureWorkerProject struct {
	deps Deps
}

const ConfigureWorkerProjectName tool.Name = "configure_worker_project"

func NewConfigureWorkerProject(deps Deps) *ConfigureWorkerProject {
	return &ConfigureWorkerProject{deps: deps}
}

var configureWorkerProjectSchema = mustSchema[configureWorkerProjectArgs]()

// configureWorkerProjectArgs uses pointers for the patchable
// fields so we can distinguish "field omitted" (nil) from
// "explicitly empty" (non-nil pointer to ""). Mirrors Helix's
// `types.ProjectUpdateRequest` shape.
type configureWorkerProjectArgs struct {
	WorkerID      string  `json:"workerId"`
	StartupScript *string `json:"startupScript,omitempty"`
	// Future fields go here (Skills, Guidelines, etc.) in the same
	// pointer-optional shape. Don't add them until the underlying
	// ProjectConfigPatch grows — schema drift is more confusing
	// than a missing knob.
}

func (t *ConfigureWorkerProject) Name() tool.Name                 { return ConfigureWorkerProjectName }
func (t *ConfigureWorkerProject) InputSchema() *jsonschema.Schema { return configureWorkerProjectSchema }
func (t *ConfigureWorkerProject) Description() string {
	return "Patch a Worker's helix project configuration. Today supports startupScript " +
		"(the bash script the desktop runs on container start); more fields land here as the " +
		"port surfaces them. Patch semantics: any field omitted is preserved as-is, an " +
		"explicitly empty string clears the field. Call get_worker_project first so you patch " +
		"a known-current state. Owner-only."
}

func (t *ConfigureWorkerProject) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args configureWorkerProjectArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.WorkerID == "" {
		return nil, errors.New("workerId is required")
	}
	patch := runtime.ProjectConfigPatch{
		StartupScript: args.StartupScript,
	}
	if !projectConfigPatchHasFields(patch) {
		return nil, errors.New("at least one configurable field must be set (e.g. startupScript)")
	}
	orgID, err := projectConfigOrgID(inv)
	if err != nil {
		return nil, err
	}
	cfg := t.deps.ProjectConfig
	if cfg == nil {
		return nil, runtime.ErrProjectConfigUnsupported
	}
	snap, err := cfg.UpdateWorkerProjectConfig(ctx, orgID, orgchart.WorkerID(args.WorkerID), patch)
	if err != nil {
		return nil, fmt.Errorf("configure worker project: %w", err)
	}
	return json.Marshal(snap)
}

// projectConfigPatchHasFields returns true when at least one
// patchable field is set on the patch. Adding a new field to
// ProjectConfigPatch requires extending this — the configure
// tool's empty-patch rejection depends on it.
func projectConfigPatchHasFields(p runtime.ProjectConfigPatch) bool {
	return p.StartupScript != nil
}
