package mcptools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/application/instances"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/types"
)

// Bot instance tools — create / list / delete a Bot's instances: extra
// sessions with the Bot's identity, each with its own sandbox. Same port as
// the REST /bots/{id}/instances endpoints. Granted on OwnerBotTools.

var errInstancesUnavailable = errors.New("bot instances are not wired in this deployment")

type botInstanceView struct {
	SessionID      string               `json:"session_id"`
	BotID          string               `json:"bot_id"`
	Name           string               `json:"name"`
	SandboxRuntime types.SandboxRuntime `json:"sandbox_runtime"`
	SandboxStatus  string               `json:"sandbox_status,omitempty"`
}

func toBotInstanceView(session *types.Session) botInstanceView {
	return botInstanceView{
		SessionID:      session.ID,
		BotID:          session.Metadata.OrgWorkerID,
		Name:           session.Name,
		SandboxRuntime: session.Metadata.SandboxRuntime,
		SandboxStatus:  session.Metadata.ExternalAgentStatus,
	}
}

type deletedBotInstanceView struct {
	Deleted string `json:"deleted"`
}

// --- create_bot_instance --------------------------------------------------

const CreateBotInstanceName tool.Name = "create_bot_instance"

type CreateBotInstance struct{ deps Deps }

func NewCreateBotInstance(deps Deps) *CreateBotInstance { return &CreateBotInstance{deps: deps} }

type createBotInstanceArgs struct {
	NodeID         string `json:"bot_id"`
	Name           string `json:"name,omitempty"`
	SandboxRuntime string `json:"sandbox_runtime,omitempty"`
	Message        string `json:"message,omitempty"`
}

var createBotInstanceSchema = mustSchema[createBotInstanceArgs]()

func (t *CreateBotInstance) Name() tool.Name                 { return CreateBotInstanceName }
func (t *CreateBotInstance) InputSchema() *jsonschema.Schema { return createBotInstanceSchema }
func (t *CreateBotInstance) Description() string {
	return "Start a new instance of a Bot: a separate session with the bot's instructions, " +
		"project and model, running in its own sandbox. Use one instance per end user or " +
		"case so they never share a browser or workspace. Instances get only what the bot's " +
		"instance profile enables (by default a browser, no helix tools). Returns session_id."
}
func (t *CreateBotInstance) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args createBotInstanceArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	botID, orgID, err := botAgentArgs(inv)
	if err != nil {
		return nil, err
	}
	if t.deps.Instances == nil {
		return nil, errInstancesUnavailable
	}
	if err := ensureBotExists(ctx, t.deps, orgID, botID); err != nil {
		return nil, err
	}
	session, err := t.deps.Instances.Create(ctx, orgID, botID, instances.Params{
		Name:           args.Name,
		SandboxRuntime: types.SandboxRuntime(args.SandboxRuntime),
		Message:        args.Message,
	})
	if err != nil {
		return nil, err
	}
	return json.Marshal(toBotInstanceView(session))
}

// --- list_bot_instances ---------------------------------------------------

const ListBotInstancesName tool.Name = "list_bot_instances"

type ListBotInstances struct{ deps Deps }

func NewListBotInstances(deps Deps) *ListBotInstances { return &ListBotInstances{deps: deps} }

type listBotInstancesArgs struct {
	NodeID string `json:"bot_id"`
}

var listBotInstancesSchema = mustSchema[listBotInstancesArgs]()

func (t *ListBotInstances) Name() tool.Name                 { return ListBotInstancesName }
func (t *ListBotInstances) InputSchema() *jsonschema.Schema { return listBotInstancesSchema }
func (t *ListBotInstances) Description() string {
	return "List a Bot's instances, newest first, with each one's session_id, name, " +
		"sandbox runtime and sandbox status."
}
func (t *ListBotInstances) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	botID, orgID, err := botAgentArgs(inv)
	if err != nil {
		return nil, err
	}
	if t.deps.Instances == nil {
		return nil, errInstancesUnavailable
	}
	if err := ensureBotExists(ctx, t.deps, orgID, botID); err != nil {
		return nil, err
	}
	sessions, err := t.deps.Instances.List(ctx, orgID, botID)
	if err != nil {
		return nil, err
	}
	out := make([]botInstanceView, 0, len(sessions))
	for _, session := range sessions {
		out = append(out, toBotInstanceView(session))
	}
	return json.Marshal(out)
}

// --- delete_bot_instance --------------------------------------------------

const DeleteBotInstanceName tool.Name = "delete_bot_instance"

type DeleteBotInstance struct{ deps Deps }

func NewDeleteBotInstance(deps Deps) *DeleteBotInstance { return &DeleteBotInstance{deps: deps} }

type deleteBotInstanceArgs struct {
	NodeID    string `json:"bot_id"`
	SessionID string `json:"session_id"`
}

var deleteBotInstanceSchema = mustSchema[deleteBotInstanceArgs]()

func (t *DeleteBotInstance) Name() tool.Name                 { return DeleteBotInstanceName }
func (t *DeleteBotInstance) InputSchema() *jsonschema.Schema { return deleteBotInstanceSchema }
func (t *DeleteBotInstance) Description() string {
	return "Delete a Bot instance: stops its sandbox and deletes its workspace and chat. " +
		"The bot and its other instances are untouched. This cannot be undone."
}
func (t *DeleteBotInstance) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args deleteBotInstanceArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.SessionID == "" {
		return nil, errors.New("session_id is required")
	}
	botID, orgID, err := botAgentArgs(inv)
	if err != nil {
		return nil, err
	}
	if t.deps.Instances == nil {
		return nil, errInstancesUnavailable
	}
	if err := t.deps.Instances.Delete(ctx, orgID, botID, args.SessionID); err != nil {
		return nil, err
	}
	return json.Marshal(deletedBotInstanceView{Deleted: args.SessionID})
}
