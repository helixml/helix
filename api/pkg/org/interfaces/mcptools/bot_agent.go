package mcptools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/application/activations"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// Bot agent lifecycle tools — start / stop / restart a Bot's desktop
// sandbox. Same semantics as the REST activate / stop-agent /
// restart-agent endpoints and the chart ⋮ menu. Granted on OwnerBotTools
// (Chief of Staff) by default.

// --- start_bot ------------------------------------------------------------

const StartBotName tool.Name = "start_bot"

type StartBot struct{ deps Deps }

func NewStartBot(deps Deps) *StartBot { return &StartBot{deps: deps} }

type startBotArgs struct {
	BotID string `json:"bot_id"`
}

var startBotSchema = mustSchema[startBotArgs]()

func (t *StartBot) Name() tool.Name                 { return StartBotName }
func (t *StartBot) InputSchema() *jsonschema.Schema { return startBotSchema }
func (t *StartBot) Description() string {
	return "Start (or wake) a Bot's agent desktop sandbox. Ensures its Helix project " +
		"exists, attaches org MCP tools, and enqueues a manual activation so the " +
		"desktop comes up. Use after create_bot (or when a bot is stopped) so the bot " +
		"can work. Returns activation_id, project_id, agent_app_id, session_id."
}
func (t *StartBot) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	botID, orgID, err := botAgentArgs(inv)
	if err != nil {
		return nil, err
	}
	svc, err := activationsSvc(t.deps)
	if err != nil {
		return nil, err
	}
	if err := ensureBotExists(ctx, t.deps, orgID, botID); err != nil {
		return nil, err
	}
	res, err := svc.Activate(ctx, orgID, botID)
	if err != nil {
		return nil, mapAgentErr(err)
	}
	return json.Marshal(toBotActivateView(res))
}

// --- stop_bot -------------------------------------------------------------

const StopBotName tool.Name = "stop_bot"

type StopBot struct{ deps Deps }

func NewStopBot(deps Deps) *StopBot { return &StopBot{deps: deps} }

type stopBotArgs struct {
	BotID string `json:"bot_id"`
}

var stopBotSchema = mustSchema[stopBotArgs]()

func (t *StopBot) Name() tool.Name                 { return StopBotName }
func (t *StopBot) InputSchema() *jsonschema.Schema { return stopBotSchema }
func (t *StopBot) Description() string {
	return "Stop a Bot's agent desktop sandbox without deleting its session or " +
		"transcript. No-op if the bot is already stopped. Use start_bot to bring it " +
		"back up, or restart_bot for a fully fresh session."
}
func (t *StopBot) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	botID, orgID, err := botAgentArgs(inv)
	if err != nil {
		return nil, err
	}
	svc, err := activationsSvc(t.deps)
	if err != nil {
		return nil, err
	}
	if err := ensureBotExists(ctx, t.deps, orgID, botID); err != nil {
		return nil, err
	}
	res, err := svc.Stop(ctx, orgID, botID)
	if err != nil {
		return nil, mapAgentErr(err)
	}
	return json.Marshal(res)
}

// --- restart_bot ----------------------------------------------------------

const RestartBotName tool.Name = "restart_bot"

type RestartBot struct{ deps Deps }

func NewRestartBot(deps Deps) *RestartBot { return &RestartBot{deps: deps} }

type restartBotArgs struct {
	BotID string `json:"bot_id"`
}

var restartBotSchema = mustSchema[restartBotArgs]()

func (t *RestartBot) Name() tool.Name                 { return RestartBotName }
func (t *RestartBot) InputSchema() *jsonschema.Schema { return restartBotSchema }
func (t *RestartBot) Description() string {
	return "Restart a Bot's agent with a brand-new session and desktop (stops and " +
		"deletes the current session, then starts fresh). Use this when the bot's " +
		"tools or project config changed and you need them picked up, or when the " +
		"session is stuck. Prefer start_bot for a first start or a simple wake."
}
func (t *RestartBot) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	botID, orgID, err := botAgentArgs(inv)
	if err != nil {
		return nil, err
	}
	svc, err := activationsSvc(t.deps)
	if err != nil {
		return nil, err
	}
	if err := ensureBotExists(ctx, t.deps, orgID, botID); err != nil {
		return nil, err
	}
	res, err := svc.Restart(ctx, orgID, botID)
	if err != nil {
		return nil, mapAgentErr(err)
	}
	return json.Marshal(toBotActivateView(res))
}

// --- helpers --------------------------------------------------------------

type botActivateView struct {
	ActivationID string `json:"activation_id,omitempty"`
	ProjectID    string `json:"project_id,omitempty"`
	AgentAppID   string `json:"agent_app_id,omitempty"`
	SessionID    string `json:"session_id,omitempty"`
}

func toBotActivateView(res activations.ActivateResult) botActivateView {
	return botActivateView{
		ActivationID: string(res.ActivationID),
		ProjectID:    res.ProjectID,
		AgentAppID:   res.AgentAppID,
		SessionID:    res.SessionID,
	}
}

// botAgentArgs parses {bot_id} from the invocation and returns bot + org.
func botAgentArgs(inv tool.Invocation) (orgchart.BotID, string, error) {
	var args struct {
		BotID string `json:"bot_id"`
	}
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return "", "", fmt.Errorf("parse args: %w", err)
	}
	if args.BotID == "" {
		return "", "", errors.New("bot_id is required")
	}
	if inv.Caller == nil {
		return "", "", errors.New("caller missing on invocation")
	}
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return "", "", errors.New("caller has no organization id")
	}
	return orgchart.BotID(args.BotID), orgID, nil
}

func activationsSvc(deps Deps) (*activations.Activations, error) {
	if deps.Activations == nil {
		return nil, activations.ErrActivateUnavailable
	}
	return deps.Activations, nil
}

func ensureBotExists(ctx context.Context, deps Deps, orgID string, botID orgchart.BotID) error {
	if deps.Queries == nil {
		return nil
	}
	if _, err := deps.Queries.GetBot(ctx, orgID, botID); err != nil {
		return fmt.Errorf("get bot %s: %w", botID, err)
	}
	return nil
}

func mapAgentErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, activations.ErrActivateUnavailable) || errors.Is(err, activations.ErrStopUnavailable) {
		return err
	}
	return err
}
