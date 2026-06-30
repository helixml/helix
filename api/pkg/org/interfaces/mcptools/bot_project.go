package mcptools

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

// GetBotProject reads a Bot's helix project configuration — the startup
// script today, with skills / guidelines following the same shape as
// Helix exposes more of ProjectUpdateRequest through the ProjectConfig
// port. The owner uses this before configure_bot_project to read what's
// currently set, so the LLM can produce a sensible patch ("add `apt
// install -y gh` to the existing script") rather than blindly
// overwriting it.
type GetBotProject struct {
	deps Deps
}

const GetBotProjectName tool.Name = "get_bot_project"

// NewGetBotProject is the public constructor — exported so tests can
// stand up a tool with a fake ProjectConfig without going through the
// full RegisterBuiltins path.
func NewGetBotProject(deps Deps) *GetBotProject {
	return &GetBotProject{deps: deps}
}

var getBotProjectSchema = mustSchema[getBotProjectArgs]()

type getBotProjectArgs struct {
	BotID string `json:"botId"`
}

func (t *GetBotProject) Name() tool.Name                 { return GetBotProjectName }
func (t *GetBotProject) InputSchema() *jsonschema.Schema { return getBotProjectSchema }
func (t *GetBotProject) Description() string {
	return "Read the helix project configuration for a Bot — the startup script today, " +
		"plus any other configurable project field as the port adds them. Use this before " +
		"configure_bot_project so you can patch a known-current state rather than blindly " +
		"overwriting. Owner-only."
}

func (t *GetBotProject) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args getBotProjectArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.BotID == "" {
		return nil, errors.New("botId is required")
	}
	orgID, err := projectConfigOrgID(inv)
	if err != nil {
		return nil, err
	}
	cfg := t.deps.ProjectConfig
	if cfg == nil {
		// Defensive: production wiring always sets ProjectConfig
		// (NoopProjectConfig by default), but if a test wires Deps from
		// scratch and forgets, we want a clear message rather than a
		// nil-deref.
		return nil, runtime.ErrProjectConfigUnsupported
	}
	snap, err := cfg.GetWorkerProjectConfig(ctx, orgID, orgchart.BotID(args.BotID))
	if err != nil {
		return nil, fmt.Errorf("get bot project: %w", err)
	}
	return json.Marshal(snap)
}

// projectConfigOrgID extracts the caller's orgID from the Invocation.
// Shared between the get + configure tools so they stay in lockstep on
// the auth assumption.
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

// ConfigureBotProject applies a partial patch to a Bot's helix project
// configuration. Today: startupScript. Future: additional project-level
// config fields (skills, guidelines, …) — each one becomes a new
// optional patch field here and on the ProjectConfigPatch struct.
//
// Patch semantics: any field LEFT OUT of the JSON body is preserved
// as-is. Setting a field to "" (or false/empty array) is treated as an
// explicit intent to clear; the underlying port passes a non-nil pointer
// for that case so the runtime distinguishes "not touched" from
// "explicitly empty".
//
// The tool refuses an args object with no patch fields set — otherwise a
// careless caller could silently no-op when it expected to write. Forces
// the LLM to be explicit about what it's changing.
type ConfigureBotProject struct {
	deps Deps
}

const ConfigureBotProjectName tool.Name = "configure_bot_project"

func NewConfigureBotProject(deps Deps) *ConfigureBotProject {
	return &ConfigureBotProject{deps: deps}
}

var configureBotProjectSchema = mustSchema[configureBotProjectArgs]()

// configureBotProjectArgs uses pointers for the patchable fields so we
// can distinguish "field omitted" (nil) from "explicitly empty" (non-nil
// pointer to ""). Mirrors Helix's `types.ProjectUpdateRequest` shape.
type configureBotProjectArgs struct {
	BotID         string  `json:"botId"`
	StartupScript *string `json:"startupScript,omitempty"`
	// Future fields go here (Skills, Guidelines, etc.) in the same
	// pointer-optional shape. Don't add them until the underlying
	// ProjectConfigPatch grows — schema drift is more confusing than a
	// missing knob.
}

func (t *ConfigureBotProject) Name() tool.Name { return ConfigureBotProjectName }
func (t *ConfigureBotProject) InputSchema() *jsonschema.Schema {
	return configureBotProjectSchema
}
func (t *ConfigureBotProject) Description() string {
	return "Patch a Bot's helix project configuration. Today supports startupScript " +
		"(the bash script the desktop runs on container start); more fields land here as the " +
		"port surfaces them. Patch semantics: any field omitted is preserved as-is, an " +
		"explicitly empty string clears the field. Call get_bot_project first so you patch " +
		"a known-current state. Owner-only."
}

func (t *ConfigureBotProject) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args configureBotProjectArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.BotID == "" {
		return nil, errors.New("botId is required")
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
	snap, err := cfg.UpdateWorkerProjectConfig(ctx, orgID, orgchart.BotID(args.BotID), patch)
	if err != nil {
		return nil, fmt.Errorf("configure bot project: %w", err)
	}
	return json.Marshal(snap)
}

// projectConfigPatchHasFields returns true when at least one patchable
// field is set on the patch. Adding a new field to ProjectConfigPatch
// requires extending this — the configure tool's empty-patch rejection
// depends on it.
func projectConfigPatchHasFields(p runtime.ProjectConfigPatch) bool {
	return p.StartupScript != nil
}
