package mcptools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
)

const (
	ListSandboxRuntimesName tool.Name = "list_sandbox_runtimes"
	ListSandboxesName       tool.Name = "list_sandboxes"
	GetSandboxName          tool.Name = "get_sandbox"
	CreateSandboxName       tool.Name = "create_sandbox"
	UpdateSandboxName       tool.Name = "update_sandbox"
	DeleteSandboxName       tool.Name = "delete_sandbox"
)

type sandboxTool struct{ deps Deps }

type emptySandboxArgs struct{}

var emptySandboxSchema = mustSchema[emptySandboxArgs]()

type ListSandboxRuntimes struct{ sandboxTool }

func NewListSandboxRuntimes(deps Deps) *ListSandboxRuntimes {
	return &ListSandboxRuntimes{sandboxTool{deps: deps}}
}
func (t *ListSandboxRuntimes) Name() tool.Name                 { return ListSandboxRuntimesName }
func (t *ListSandboxRuntimes) InputSchema() *jsonschema.Schema { return emptySandboxSchema }
func (t *ListSandboxRuntimes) Description() string {
	return "List the standalone sandbox runtimes configured on this Helix server and identify the default runtime. Call before create_sandbox when runtime requirements matter."
}
func (t *ListSandboxRuntimes) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	value, err := t.deps.Sandboxes.ListRuntimes(ctx, inv.Caller)
	if err != nil {
		return nil, err
	}
	return json.Marshal(value)
}

type listSandboxesArgs struct {
	ProjectID string `json:"project_id,omitempty"`
}

var listSandboxesSchema = mustSchema[listSandboxesArgs]()

type ListSandboxes struct{ sandboxTool }

func NewListSandboxes(deps Deps) *ListSandboxes {
	return &ListSandboxes{sandboxTool{deps: deps}}
}
func (t *ListSandboxes) Name() tool.Name                 { return ListSandboxesName }
func (t *ListSandboxes) InputSchema() *jsonschema.Schema { return listSandboxesSchema }
func (t *ListSandboxes) Description() string {
	return "List standalone sandboxes in your organization, optionally filtered by project_id. Returns lifecycle status, runtime, resources, tags, and expiry; environment variables are intentionally omitted."
}
func (t *ListSandboxes) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args listSandboxesArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	values, err := t.deps.Sandboxes.List(ctx, inv.Caller, args.ProjectID)
	if err != nil {
		return nil, err
	}
	return json.Marshal(values)
}

type sandboxIDArgs struct {
	SandboxID string `json:"sandbox_id"`
}

var sandboxIDSchema = mustSchema[sandboxIDArgs]()

func parseSandboxID(raw json.RawMessage) (sandboxIDArgs, error) {
	var args sandboxIDArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return sandboxIDArgs{}, fmt.Errorf("parse args: %w", err)
	}
	if args.SandboxID == "" {
		return sandboxIDArgs{}, errors.New("sandbox_id is required")
	}
	return args, nil
}

type GetSandbox struct{ sandboxTool }

func NewGetSandbox(deps Deps) *GetSandbox             { return &GetSandbox{sandboxTool{deps: deps}} }
func (t *GetSandbox) Name() tool.Name                 { return GetSandboxName }
func (t *GetSandbox) InputSchema() *jsonschema.Schema { return sandboxIDSchema }
func (t *GetSandbox) Description() string {
	return "Get one standalone sandbox in your organization by sandbox_id. Environment variables are intentionally omitted from the result."
}
func (t *GetSandbox) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	args, err := parseSandboxID(inv.Args)
	if err != nil {
		return nil, err
	}
	value, err := t.deps.Sandboxes.Get(ctx, inv.Caller, args.SandboxID)
	if err != nil {
		return nil, err
	}
	return json.Marshal(value)
}

type createSandboxArgs struct {
	Name           string            `json:"name,omitempty"`
	Runtime        string            `json:"runtime,omitempty"`
	Image          string            `json:"image,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	Tags           map[string]string `json:"tags,omitempty"`
	TimeoutSeconds int               `json:"timeout_seconds,omitempty"`
	VCPUs          int               `json:"vcpus,omitempty"`
	MemoryMB       int               `json:"memory_mb,omitempty"`
	DisplayWidth   int               `json:"display_width,omitempty"`
	DisplayHeight  int               `json:"display_height,omitempty"`
	DisplayFPS     int               `json:"display_fps,omitempty"`
	ProjectID      string            `json:"project_id,omitempty"`
	Persistent     bool              `json:"persistent,omitempty"`
}

var createSandboxSchema = mustSchema[createSandboxArgs]()

type CreateSandbox struct{ sandboxTool }

func NewCreateSandbox(deps Deps) *CreateSandbox          { return &CreateSandbox{sandboxTool{deps: deps}} }
func (t *CreateSandbox) Name() tool.Name                 { return CreateSandboxName }
func (t *CreateSandbox) InputSchema() *jsonschema.Schema { return createSandboxSchema }
func (t *CreateSandbox) Description() string {
	return "Create a standalone sandbox in your organization. Omit runtime and image to use the server default; otherwise call list_sandbox_runtimes first. image works only when custom images are enabled. timeout_seconds=0 uses the one-hour default and a negative value means no expiry. The returned pending sandbox provisions asynchronously; poll get_sandbox until running or failed."
}
func (t *CreateSandbox) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args createSandboxArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	value, err := t.deps.Sandboxes.Create(ctx, inv.Caller, runtime.CreateSandboxInput{
		Name:           args.Name,
		Runtime:        args.Runtime,
		Image:          args.Image,
		Env:            args.Env,
		Tags:           args.Tags,
		TimeoutSeconds: args.TimeoutSeconds,
		VCPUs:          args.VCPUs,
		MemoryMB:       args.MemoryMB,
		DisplayWidth:   args.DisplayWidth,
		DisplayHeight:  args.DisplayHeight,
		DisplayFPS:     args.DisplayFPS,
		ProjectID:      args.ProjectID,
		Persistent:     args.Persistent,
	})
	if err != nil {
		return nil, err
	}
	return json.Marshal(value)
}

type updateSandboxArgs struct {
	SandboxID      string             `json:"sandbox_id"`
	Name           *string            `json:"name,omitempty"`
	TimeoutSeconds *int               `json:"timeout_seconds,omitempty"`
	Tags           *map[string]string `json:"tags,omitempty"`
}

var updateSandboxSchema = mustSchema[updateSandboxArgs]()

type UpdateSandbox struct{ sandboxTool }

func NewUpdateSandbox(deps Deps) *UpdateSandbox          { return &UpdateSandbox{sandboxTool{deps: deps}} }
func (t *UpdateSandbox) Name() tool.Name                 { return UpdateSandboxName }
func (t *UpdateSandbox) InputSchema() *jsonschema.Schema { return updateSandboxSchema }
func (t *UpdateSandbox) Description() string {
	return "Update a standalone sandbox's name, positive timeout_seconds, or tags. Omitted fields are unchanged."
}
func (t *UpdateSandbox) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args updateSandboxArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.SandboxID == "" {
		return nil, errors.New("sandbox_id is required")
	}
	if args.Name == nil && args.TimeoutSeconds == nil && args.Tags == nil {
		return nil, errors.New("at least one of name, timeout_seconds, or tags is required")
	}
	if args.TimeoutSeconds != nil && *args.TimeoutSeconds <= 0 {
		return nil, errors.New("timeout_seconds must be positive when updating a sandbox")
	}
	value, err := t.deps.Sandboxes.Update(ctx, inv.Caller, args.SandboxID, runtime.UpdateSandboxInput{
		Name:           args.Name,
		TimeoutSeconds: args.TimeoutSeconds,
		Tags:           args.Tags,
	})
	if err != nil {
		return nil, err
	}
	return json.Marshal(value)
}

type DeleteSandbox struct{ sandboxTool }

func NewDeleteSandbox(deps Deps) *DeleteSandbox          { return &DeleteSandbox{sandboxTool{deps: deps}} }
func (t *DeleteSandbox) Name() tool.Name                 { return DeleteSandboxName }
func (t *DeleteSandbox) InputSchema() *jsonschema.Schema { return sandboxIDSchema }
func (t *DeleteSandbox) Description() string {
	return "Delete a standalone sandbox in your organization and tear down its container. This is irreversible."
}
func (t *DeleteSandbox) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	args, err := parseSandboxID(inv.Args)
	if err != nil {
		return nil, err
	}
	if err := t.deps.Sandboxes.Delete(ctx, inv.Caller, args.SandboxID); err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		DeletedSandboxID string `json:"deleted_sandbox_id"`
	}{DeletedSandboxID: args.SandboxID})
}
