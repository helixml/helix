package mcptools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/application/lifecycle"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// CreateBot brings a Bot into existence in a single call: the bot row (its
// markdown content and MCP tool surface), the initial reporting line to
// its manager, subscriptions to the topics named at creation, and — through
// the lifecycle service — a create activation dispatched to the Spawner.
//
// It completes the whole of "create a Bot" so the caller doesn't need a
// chain of follow-ups: `tools` grants the Bot's initial tools (unioned
// with the universal read baseline; use attach_tool/detach_tool to change
// them later) and `topics` subscribes the new Bot to each listed (already
// existing) Topic at creation (use subscribe/unsubscribe to change them
// later). `content` is the bot's prompt; `parentId` is the manager this
// bot reports to — omit it only for the org owner.
//
// Both `tools` and `topics` are required arrays: pass `[]` for none. The
// creation-time subscription reuses the same subscriptions use case the
// standalone subscribe tool drives (see lifecycle.Create) — one
// implementation, no duplicated logic.
type CreateBot struct {
	deps Deps
}

// NewCreateBot constructs the tool with its dependencies. Exported so
// non-MCP callers (the REST POST /bots handler) can drive the same
// create path the MCP surface uses.
func NewCreateBot(deps Deps) *CreateBot {
	return &CreateBot{deps: deps}
}

const CreateBotName tool.Name = "create_bot"

// createBotSchema is the reflected base (object shape + required — neither
// tools nor topics is omitempty). InputSchema swaps in the dynamic `tools`
// enum and the non-nullable `topics` array at serve time.
var createBotSchema = mustSchema[createBotArgs]()

func (t *CreateBot) Name() tool.Name { return CreateBotName }
func (t *CreateBot) InputSchema() *jsonschema.Schema {
	s := withProperty(createBotSchema, "tools",
		enumStringArrayProperty(t.deps.ToolNames(),
			"MCP tools to grant the new Bot (one or many; pass [] for the read baseline only). The universal read baseline is always added."))
	s = withProperty(s, "topics",
		stringArrayProperty(
			"Existing Topic ids to subscribe the new Bot to at creation (pass [] for none). Topics must already exist — create_topic first."))
	return s
}
func (t *CreateBot) Description() string {
	return "Create a new Bot in one call. `content` is the bot's prompt. `name` is the " +
		"human-readable display label shown in the UI (e.g. \"Chief of Staff\", \"Sales " +
		"Lead\"). `tools` is an array of MCP tool names to grant (the universal read " +
		"baseline is added automatically; pass [] for baseline only) — use " +
		"attach_tool/detach_tool to change them later. `topics` is an array of existing " +
		"Topic ids to subscribe the new Bot to immediately (pass [] for none) — use " +
		"subscribe/unsubscribe to change them later. `parentId` is the manager this bot " +
		"reports to — omit it only for the org owner.\n\n" +
		"Supply `id` as a short, real-sounding handle: a lowercase given name prefixed " +
		"with `b-`, e.g. `b-mark`, `b-priya`. Pick a name that fits and isn't taken. Do " +
		"NOT pass a UUID and do NOT omit `id` to let the server invent one. If your first " +
		"choice collides, try a variant (`b-mark-2`, `b-marko`)."
}

type createBotArgs struct {
	ID       string   `json:"id,omitempty"`
	Name     string   `json:"name,omitempty"`
	Content  string   `json:"content"`
	Tools    []string `json:"tools"`
	Topics   []string `json:"topics"`
	ParentID string   `json:"parentId,omitempty"`
}

func (t *CreateBot) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args createBotArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.Content == "" {
		return nil, fmt.Errorf("content is required")
	}
	if err := validateRegisteredTools(args.Tools, t.deps.ToolNames); err != nil {
		return nil, err
	}
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("create_bot: caller has no OrgID")
	}
	// Lifecycle.Create writes the bot row (via the bots service, so the
	// base-read-tool union is applied), wires the reporting line,
	// subscribes the new bot to the requested topics (validated first), and
	// dispatches the create activation through the Spawner.
	res, err := t.deps.Lifecycle.Create(ctx, orgID, lifecycle.CreateParams{
		ID:       args.ID,
		Name:     args.Name,
		Content:  args.Content,
		Tools:    args.Tools,
		Topics:   args.Topics,
		ParentID: orgchart.BotID(args.ParentID),
	})
	if err != nil {
		return nil, err
	}
	resp := map[string]string{"id": string(res.Bot.ID)}
	if res.ActivationID != "" {
		resp["activation_id"] = string(res.ActivationID)
	}
	return json.Marshal(resp)
}
