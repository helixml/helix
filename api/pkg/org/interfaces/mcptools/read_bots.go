package mcptools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// botView is the on-the-wire shape returned by list_bots / get_bot. A
// Bot is the single org-chart aggregate (the former Role and Worker
// merged), so this carries both its definition (content, tools) and its
// place in the reporting graph (parentIds). A Bot's subscriptions are not
// on the bot — read them via the topic/subscription read tools.
type botView struct {
	ID   orgchart.BotID `json:"id"`
	Name string         `json:"name,omitempty"`
	// Kind is "" for an agent bot or "human" for a person (a human node).
	// Use ask_human to reach a person; do not try to dm/manage them.
	Kind      string           `json:"kind,omitempty"`
	Content   string           `json:"content"`
	Tools     []tool.Name      `json:"tools,omitempty"`
	ParentIDs []orgchart.BotID `json:"parentIds,omitempty"`
	CreatedAt time.Time        `json:"createdAt"`
	UpdatedAt time.Time        `json:"updatedAt"`
}

func botViewOf(b orgchart.Bot, managers []orgchart.BotID) botView {
	return botView{
		ID:        b.ID,
		Name:      b.Name,
		Kind:      b.Kind,
		Content:   b.Content,
		Tools:     b.Tools,
		ParentIDs: managers,
		CreatedAt: b.CreatedAt,
		UpdatedAt: b.UpdatedAt,
	}
}

// ListBots returns every Bot in the org.
type ListBots struct {
	deps Deps
}

const ListBotsName tool.Name = "list_bots"

var listBotsSchema = mustSchema[listBotsArgs]()

type listBotsArgs struct{}

func (t *ListBots) Name() tool.Name                 { return ListBotsName }
func (t *ListBots) InputSchema() *jsonschema.Schema { return listBotsSchema }
func (t *ListBots) Description() string {
	return "List every Bot: id, name, kind, markdown content, tools, reporting parents, " +
		"and timestamps. Use this to discover what bots exist. `kind` is \"\" for an agent " +
		"bot or \"human\" for a person (a human node) — reach a person with `ask_human`."
}

func (t *ListBots) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("list_bots: caller has no OrgID")
	}
	all, err := t.deps.Queries.ListBots(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("list bots: %w", err)
	}
	// One List call builds the report → managers index, so we don't
	// issue a ListManagers per bot.
	managersByReport := map[orgchart.BotID][]orgchart.BotID{}
	if t.deps.Queries.ReportingLinesWired() {
		lines, err := t.deps.Queries.ListReportingLines(ctx, orgID)
		if err != nil {
			return nil, fmt.Errorf("list reporting lines: %w", err)
		}
		for _, l := range lines {
			managersByReport[l.ReportID] = append(managersByReport[l.ReportID], l.ManagerID)
		}
	}
	out := make([]botView, 0, len(all))
	for _, b := range all {
		out = append(out, botViewOf(b, managersByReport[b.ID]))
	}
	return json.Marshal(map[string]any{"bots": out})
}

// GetBot returns one Bot by ID.
type GetBot struct {
	deps Deps
}

const GetBotName tool.Name = "get_bot"

var getBotSchema = mustSchema[getBotArgs]()

type getBotArgs struct {
	ID string `json:"id"`
}

func (t *GetBot) Name() tool.Name                 { return GetBotName }
func (t *GetBot) InputSchema() *jsonschema.Schema { return getBotSchema }
func (t *GetBot) Description() string {
	return "Fetch one Bot by id and return its content, tools, and reporting parents."
}

func (t *GetBot) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args getBotArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.ID == "" {
		return nil, fmt.Errorf("id is required")
	}
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("get_bot: caller has no OrgID")
	}
	b, err := t.deps.Queries.GetBot(ctx, orgID, orgchart.BotID(args.ID))
	if err != nil {
		return nil, fmt.Errorf("get bot %q: %w", args.ID, err)
	}
	var managers []orgchart.BotID
	if t.deps.Queries.ReportingLinesWired() {
		managers, err = t.deps.Queries.ListManagers(ctx, orgID, b.ID)
		if err != nil {
			return nil, fmt.Errorf("list managers for %q: %w", args.ID, err)
		}
	}
	return json.Marshal(botViewOf(b, managers))
}
