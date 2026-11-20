package mcptools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/application/activations"
	"github.com/helixml/helix/api/pkg/org/application/lifecycle"
	"github.com/helixml/helix/api/pkg/org/domain/audit"
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
	NodeID string `json:"bot_id"`
}

var startBotSchema = mustSchema[startBotArgs]()

func (t *StartBot) Name() tool.Name                 { return StartBotName }
func (t *StartBot) InputSchema() *jsonschema.Schema { return startBotSchema }
func (t *StartBot) Description() string {
	return "Start (or wake) a Bot's agent desktop sandbox. Ensures its Helix project " +
		"exists, attaches org MCP tools, and enqueues a manual activation so the " +
		"desktop comes up. Use after create_bot (or when a bot is stopped) so the bot " +
		"can work. Returns activation_id, project_id, legacy_app_id, session_id."
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
	NodeID string `json:"bot_id"`
}

var stopBotSchema = mustSchema[stopBotArgs]()

func (t *StopBot) Name() tool.Name                 { return StopBotName }
func (t *StopBot) InputSchema() *jsonschema.Schema { return stopBotSchema }
func (t *StopBot) Description() string {
	return "Stop a Bot's agent desktop sandbox without deleting its session or " +
		"transcript. Purges the bot's queued and in-flight activations. No-op if the " +
		"bot is already stopped. Use start_bot to bring it back up; use restart_bot, " +
		"not stop, to recover an unhealthy desktop without losing queued work."
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
	NodeID string `json:"bot_id"`
}

var restartBotSchema = mustSchema[restartBotArgs]()

func (t *RestartBot) Name() tool.Name                 { return RestartBotName }
func (t *RestartBot) InputSchema() *jsonschema.Schema { return restartBotSchema }
func (t *RestartBot) Description() string {
	return "Restart a Bot's agent desktop without losing anything: recreates its " +
		"container on the same session with its current tools and sandbox config. " +
		"Keeps the conversation thread, the bot home (/home/retro/work, including " +
		"browser profiles and logins), queued activations, and replays an interrupted " +
		"activation under the same delivery id. Use when tools or config changed or " +
		"the desktop is unhealthy. For a fresh session use reset_bot."
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
	return json.Marshal(botRestartView{
		Mode:      string(res.Mode),
		SessionID: res.SessionID,
		Preserves: res.Preserves,
		Activation: func() *botActivateView {
			if res.Mode != activations.RestartFirstStart {
				return nil
			}
			v := toBotActivateView(res.Activation)
			return &v
		}(),
	})
}

type botRestartView struct {
	Mode       string           `json:"mode"`
	SessionID  string           `json:"session_id,omitempty"`
	Preserves  []string         `json:"preserves"`
	Activation *botActivateView `json:"activation,omitempty"`
}

// --- reset_bot ------------------------------------------------------------

const ResetBotName tool.Name = "reset_bot"

type ResetBot struct{ deps Deps }

func NewResetBot(deps Deps) *ResetBot { return &ResetBot{deps: deps} }

type resetBotArgs struct {
	NodeID  string `json:"bot_id"`
	Scope   string `json:"scope" jsonschema:"what to erase: session (queued work, session, thread, desktop; keeps the bot home) or session_and_home (also retires the bot home, including browser profiles and logins)"`
	Confirm string `json:"confirm" jsonschema:"must equal bot_id"`
}

var resetBotSchema = mustSchema[resetBotArgs]()

func (t *ResetBot) Name() tool.Name                 { return ResetBotName }
func (t *ResetBot) InputSchema() *jsonschema.Schema { return resetBotSchema }
func (t *ResetBot) Description() string {
	return "Destructively reset a Bot. Purges its queued and in-flight activations " +
		"(they are not replayed), deletes its current session, conversation thread and " +
		"desktop, then starts a fresh session. scope=session keeps the bot home " +
		"(/home/retro/work); scope=session_and_home also retires the bot home, " +
		"including browser profiles and logins. confirm must equal bot_id. Audited. " +
		"Use restart_bot instead unless you need a fresh session."
}
func (t *ResetBot) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	botID, orgID, err := botAgentArgs(inv)
	if err != nil {
		return nil, err
	}
	var args resetBotArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	svc, err := activationsSvc(t.deps)
	if err != nil {
		return nil, err
	}
	if err := ensureBotExists(ctx, t.deps, orgID, botID); err != nil {
		return nil, err
	}
	res, err := svc.Reset(ctx, orgID, botID, activations.ResetRequest{
		Scope:   activations.ResetScope(args.Scope),
		Confirm: args.Confirm,
		Actor:   audit.ActorBot,
		ActorID: inv.Caller.ID(),
	})
	if err != nil {
		return nil, mapAgentErr(err)
	}
	return json.Marshal(toBotResetView(res))
}

type botResetView struct {
	Scope             string                   `json:"scope"`
	Erases            []string                 `json:"erases"`
	Keeps             []string                 `json:"keeps"`
	PreviousSessionID string                   `json:"previous_session_id,omitempty"`
	Home              *activations.HomeErasure `json:"home,omitempty"`
	Activation        botActivateView          `json:"activation"`
}

func toBotResetView(res activations.ResetResult) botResetView {
	return botResetView{
		Scope:             string(res.Scope),
		Erases:            res.Declaration.Erases,
		Keeps:             res.Declaration.Keeps,
		PreviousSessionID: res.PreviousSessionID,
		Home:              res.Home,
		Activation:        toBotActivateView(res.Activation),
	}
}

// --- helpers --------------------------------------------------------------

type botActivateView struct {
	ActivationID string `json:"activation_id,omitempty"`
	ProjectID    string `json:"project_id,omitempty"`
	LegacyAppID  string `json:"legacy_app_id,omitempty"`
	SessionID    string `json:"session_id,omitempty"`
}

func toBotActivateView(res activations.ActivateResult) botActivateView {
	return botActivateView{
		ActivationID: string(res.ActivationID),
		ProjectID:    res.ProjectID,
		LegacyAppID:  res.AgentID,
		SessionID:    res.SessionID,
	}
}

// botAgentArgs parses {bot_id} from the invocation and returns bot + org.
func botAgentArgs(inv tool.Invocation) (orgchart.NodeID, string, error) {
	var args struct {
		NodeID string `json:"bot_id"`
	}
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return "", "", fmt.Errorf("parse args: %w", err)
	}
	if args.NodeID == "" {
		return "", "", errors.New("bot_id is required")
	}
	if inv.Caller == nil {
		return "", "", errors.New("caller missing on invocation")
	}
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return "", "", errors.New("caller has no organization id")
	}
	return orgchart.NodeID(args.NodeID), orgID, nil
}

func activationsSvc(deps Deps) (*activations.Activations, error) {
	if deps.Activations == nil {
		return nil, activations.ErrActivateUnavailable
	}
	return deps.Activations, nil
}

// ensureBotExists also refuses a bot whose delete is in flight, so nothing
// provisions a sandbox the delete cascade is tearing down.
func ensureBotExists(ctx context.Context, deps Deps, orgID string, botID orgchart.NodeID) error {
	if deps.Lifecycle.Deleting(orgID, botID) {
		return fmt.Errorf("bot %s: %w", botID, lifecycle.ErrDeleteInProgress)
	}
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
