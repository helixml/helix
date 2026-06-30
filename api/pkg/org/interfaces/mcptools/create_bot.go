package mcptools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/application/lifecycle"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// CreateBot brings a Bot into existence: the bot row (its markdown
// content and MCP tool surface), the initial reporting line to its
// manager, and — through the lifecycle service — a create activation
// dispatched to the Spawner. It is the merge of the former create_role
// (define the job description) and hire_worker (instantiate someone in
// it): now that a Bot IS its own job description, those are one
// operation.
//
// A Bot's MCP tool surface is derived live from Bot.Tools: change the
// bot and it sees the new tool set on its next MCP request. `content`
// is the bot's prompt; `tools` is its live MCP surface (unioned with the
// universal read baseline by the service); `topics` is the typed
// manifest of Topic IDs its prompt expects to operate on (the creator
// still drives create_topic/subscribe explicitly). `parentId` is the
// manager this bot reports to — omit it only for the org owner
// (bootstrap creates that); every other bot has a manager.
//
// State lives in the domain (DB), not on disk. role.md / agent.md are
// projected into the bot's Environment by the Spawner at activation
// time. This keeps every mutation a single DB write and lets the env
// layer evolve without touching the tools.
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

var createBotSchema = mustSchema[createBotArgs]()

func (t *CreateBot) Name() tool.Name                 { return CreateBotName }
func (t *CreateBot) InputSchema() *jsonschema.Schema { return createBotSchema }
func (t *CreateBot) Description() string {
	return "Create a new Bot with markdown content. The content is the bot's prompt — " +
		"what it reads on every activation. `tools` is the bot's live MCP surface; " +
		"populate it with every MCP tool the bot needs (the universal read baseline is " +
		"added automatically). `topics` is a typed manifest of Topic IDs the bot's " +
		"prompt expects to operate on (you still drive create_topic/subscribe " +
		"explicitly). `parentId` is the manager this bot reports to — omit it only for " +
		"the org owner.\n\n" +
		"Supply `id` as a short, real-sounding handle: a lowercase given name prefixed " +
		"with `b-`, e.g. `b-mark`, `b-priya`, `b-jordan`. Pick a name that fits the bot " +
		"and isn't already taken. Do NOT pass a UUID and do NOT omit `id` to let the " +
		"server invent one — the auto-generated `b-<uuid>` form is reserved as a " +
		"last-resort fallback. If your first choice collides, try a variant (`b-mark-2`, " +
		"`b-marko`). Use update_bot to amend content or tools later — a tools change " +
		"propagates to the bot on its next MCP request."
}

type createBotArgs struct {
	ID       string              `json:"id,omitempty"`
	Content  string              `json:"content"`
	Tools    []tool.Name         `json:"tools,omitempty"`
	Topics   []streaming.TopicID `json:"topics,omitempty"`
	ParentID string              `json:"parentId,omitempty"`
}

func (t *CreateBot) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args createBotArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.Content == "" {
		return nil, fmt.Errorf("content is required")
	}
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("create_bot: caller has no OrgID")
	}
	// Lifecycle.Create writes the bot row (via the bots service, so the
	// base-read-tool union is applied), wires the reporting line, and
	// dispatches the create activation through the Spawner.
	res, err := t.deps.Lifecycle.Create(ctx, orgID, lifecycle.CreateParams{
		ID:       args.ID,
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
