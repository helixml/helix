package types

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
)

// TurnOrigin says who caused a user-role turn (a queued prompt or an
// interaction's prompt message).
type TurnOrigin string

const (
	// TurnOriginUser is a person signed in through the browser (OIDC or
	// password session).
	TurnOriginUser TurnOrigin = "user"
	// TurnOriginAPI is a request authenticated with an API key.
	TurnOriginAPI TurnOrigin = "api"
	// TurnOriginSystem is Helix itself: templated prompts, webhooks, CI and
	// PR notifications, triggers and org bots. SystemKind says which.
	TurnOriginSystem TurnOrigin = "system"
	// TurnOriginExternalAgent is a turn reported by the agent's own desktop
	// (a message typed into, or a thread opened in, Zed). Helix relays it but
	// cannot tell who typed it.
	TurnOriginExternalAgent TurnOrigin = "external_agent"
	// TurnOriginUnknown is returned for rows written before provenance was
	// recorded. It is never stored by a creation path.
	TurnOriginUnknown TurnOrigin = "unknown"
)

// SystemTurnKind names the Helix code path that wrote a system turn. Values
// are stable: external verifiers match on them.
type SystemTurnKind string

const (
	SystemTurnPlanningPrompt           SystemTurnKind = "planning_prompt"
	SystemTurnImplementationPrompt     SystemTurnKind = "implementation_prompt"
	SystemTurnRevisionRequested        SystemTurnKind = "revision_requested"
	SystemTurnDesignReviewComment      SystemTurnKind = "design_review_comment"
	SystemTurnAttachmentAdded          SystemTurnKind = "attachment_added"
	SystemTurnRebaseRequired           SystemTurnKind = "rebase_required"
	SystemTurnPushRequired             SystemTurnKind = "push_required"
	SystemTurnCompletionSentBack       SystemTurnKind = "completion_sent_back"
	SystemTurnPRSettled                SystemTurnKind = "pr_settled"
	SystemTurnPRProposalOutcome        SystemTurnKind = "pr_proposal_outcome"
	SystemTurnCIStatus                 SystemTurnKind = "ci_status"
	SystemTurnReviewComment            SystemTurnKind = "review_comment"
	SystemTurnSecretIntakeSubmitted    SystemTurnKind = "secret_intake_submitted"
	SystemTurnSessionForked            SystemTurnKind = "session_forked"
	SystemTurnForkHandoff              SystemTurnKind = "fork_handoff"
	SystemTurnAgentSwitched            SystemTurnKind = "agent_switched"
	SystemTurnAgentSwitchHandoff       SystemTurnKind = "agent_switch_handoff"
	SystemTurnOrgActivation            SystemTurnKind = "org_activation"
	SystemTurnOrgBotSpecTaskMessage    SystemTurnKind = "org_bot_spec_task_message"
	SystemTurnOrgBotInstanceMessage    SystemTurnKind = "org_bot_instance_message"
	SystemTurnCronTrigger              SystemTurnKind = "cron_trigger"
	SystemTurnSlackMessage             SystemTurnKind = "slack_message"
	SystemTurnTeamsMessage             SystemTurnKind = "teams_message"
	SystemTurnCrispMessage             SystemTurnKind = "crisp_message"
	SystemTurnAzureDevOpsEvent         SystemTurnKind = "azure_devops_event"
	SystemTurnCodeReviewTrigger        SystemTurnKind = "code_review_trigger"
	SystemTurnEvaluationQuestion       SystemTurnKind = "evaluation_question"
	SystemTurnExternalAgentTestHarness SystemTurnKind = "external_agent_test_harness"
	// SystemTurnThreadReseed: Helix prepended a context preamble to a
	// replayed turn because the agent could not reopen its session.
	SystemTurnThreadReseed SystemTurnKind = "thread_reseed"
)

// Actor kinds appear only inside TriggeredBy, naming a non-human caller.
const (
	// SystemTurnOrgBotAction: an org bot (OrgBotID) asked for the work.
	SystemTurnOrgBotAction SystemTurnKind = "org_bot_action"
	// SystemTurnHelixAgentTool: a Helix chat agent's tool asked for the work.
	SystemTurnHelixAgentTool SystemTurnKind = "helix_agent_tool"
	// SystemTurnTriggerConfiguration: a configured trigger or integration
	// (TriggerID, TriggerAppID, TriggerOwner) fired. The external sender is
	// not known to Helix and is not recorded.
	SystemTurnTriggerConfiguration SystemTurnKind = "trigger_configuration"
	// SystemTurnPRAutoApprove: Helix approved a pull request proposal under
	// the task's auto-approve setting.
	SystemTurnPRAutoApprove SystemTurnKind = "pr_auto_approve"
)

// APIKeyScope classifies the API key a turn was sent with.
type APIKeyScope string

const (
	APIKeyScopePersonal    APIKeyScope = "personal"
	APIKeyScopeOrg         APIKeyScope = "org"
	APIKeyScopeProject     APIKeyScope = "project"
	APIKeyScopeSpecTask    APIKeyScope = "spec_task"
	APIKeyScopeSession     APIKeyScope = "session"
	APIKeyScopeSandbox     APIKeyScope = "sandbox"
	APIKeyScopeApp         APIKeyScope = "app"
	APIKeyScopeEmbed       APIKeyScope = "embed"
	APIKeyScopeBotInstance APIKeyScope = "bot_instance"
)

// TurnProvenance records who caused a user-role turn. It is set only by
// server code at creation, from the authenticated request or the system call
// site; no request body field maps onto it, and it is immutable afterwards.
type TurnProvenance struct {
	Origin TurnOrigin `json:"origin"`

	// UserID is the signed-in user for origin=user.
	UserID string `json:"user_id,omitempty"`

	// API key fields, for origin=api. APIKeyID is the non-secret key_… id
	// (see APIKeyID), never the secret.
	APIKeyOwner      string      `json:"api_key_owner,omitempty"`
	APIKeyType       APIKeyScope `json:"api_key_type,omitempty"`
	APIKeyID         string      `json:"api_key_id,omitempty"`
	APIKeyOrgID      string      `json:"api_key_org_id,omitempty"`
	APIKeyProjectID  string      `json:"api_key_project_id,omitempty"`
	APIKeySpecTaskID string      `json:"api_key_spec_task_id,omitempty"`
	APIKeySessionID  string      `json:"api_key_session_id,omitempty"`
	// APIKeyTaskConfined: the key reaches only APIKeySpecTaskID.
	APIKeyTaskConfined bool `json:"api_key_task_confined,omitempty"`

	// SystemKind names the code path, for origin=system.
	SystemKind SystemTurnKind `json:"system_kind,omitempty"`
	// OrgBotID is the org bot that caused an org_bot_* system turn.
	OrgBotID string `json:"org_bot_id,omitempty"`
	// Trigger fields, for a trigger_configuration actor: the trigger's id
	// when it has one, the app it is configured on, and its configuring user.
	TriggerID    string `json:"trigger_id,omitempty"`
	TriggerAppID string `json:"trigger_app_id,omitempty"`
	TriggerOwner string `json:"trigger_owner,omitempty"`
	// TriggeredBy is the authenticated caller whose request made Helix write
	// this system turn (an operator clicking "request changes", say). Nil
	// when the system turn came from a background event.
	TriggeredBy *TurnProvenance `json:"triggered_by,omitempty"`
}

// MarshalJSON reports rows written before provenance existed as
// origin "unknown" rather than omitting the origin.
func (p TurnProvenance) MarshalJSON() ([]byte, error) {
	type plain TurnProvenance
	if p.Origin == "" {
		p.Origin = TurnOriginUnknown
	}
	return json.Marshal(plain(p))
}

// Value stores provenance as JSON, so map-based updates write it too.
func (p TurnProvenance) Value() (driver.Value, error) {
	b, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

// Scan reads stored provenance; NULL (rows from before provenance existed)
// leaves it empty, which marshals as origin "unknown".
func (p *TurnProvenance) Scan(src any) error {
	*p = TurnProvenance{}
	switch v := src.(type) {
	case nil:
		return nil
	case []byte:
		return json.Unmarshal(v, p)
	case string:
		return json.Unmarshal([]byte(v), p)
	default:
		return fmt.Errorf("turn provenance: cannot scan %T", src)
	}
}

// SystemTurn is the provenance of a turn Helix writes from a background event.
func SystemTurn(kind SystemTurnKind) TurnProvenance {
	return TurnProvenance{Origin: TurnOriginSystem, SystemKind: kind}
}

// OrgBotSystemTurn is the provenance of a turn an org bot caused.
func OrgBotSystemTurn(kind SystemTurnKind, botID string) TurnProvenance {
	p := SystemTurn(kind)
	p.OrgBotID = botID
	return p
}

// TriggerSource identifies a configured trigger or integration.
type TriggerSource struct {
	TriggerID string
	AppID     string
	Owner     string
}

// TriggerActor names a configured trigger as the caller of deferred work.
func TriggerActor(source TriggerSource) TurnProvenance {
	return TurnProvenance{
		Origin:       TurnOriginSystem,
		SystemKind:   SystemTurnTriggerConfiguration,
		TriggerID:    source.TriggerID,
		TriggerAppID: source.AppID,
		TriggerOwner: source.Owner,
	}
}

// TriggerTurn is the provenance of a turn an external-platform trigger
// caused: a system turn triggered by the trigger's configuration.
func TriggerTurn(kind SystemTurnKind, source TriggerSource) TurnProvenance {
	return TriggeredBySystemTurn(kind, TriggerActor(source))
}

// OrgBotActor names an org bot as the caller of deferred work.
func OrgBotActor(botID string) TurnProvenance {
	return OrgBotSystemTurn(SystemTurnOrgBotAction, botID)
}

// HelixAgentToolActor names a Helix chat agent's tool as the caller of
// deferred work.
func HelixAgentToolActor() TurnProvenance {
	return SystemTurn(SystemTurnHelixAgentTool)
}

// SystemTurnTriggeredBy is the provenance of a templated turn Helix writes
// because the authenticated caller asked for it (an operator clicking
// "request changes", say).
func SystemTurnTriggeredBy(kind SystemTurnKind, caller *User) (TurnProvenance, error) {
	actor, err := RequestTurnProvenance(caller)
	if err != nil {
		return TurnProvenance{}, err
	}
	return TriggeredBySystemTurn(kind, actor), nil
}

// TriggeredBySystemTurn is SystemTurnTriggeredBy for a caller whose
// provenance was already derived.
func TriggeredBySystemTurn(kind SystemTurnKind, actor TurnProvenance) TurnProvenance {
	p := SystemTurn(kind)
	p.TriggeredBy = &actor
	return p
}

// Validate rejects provenance a creation path must not write: a missing or
// "unknown" origin, or a system turn without a kind.
func (p TurnProvenance) Validate() error {
	switch p.Origin {
	case TurnOriginUser, TurnOriginAPI, TurnOriginExternalAgent:
		return nil
	case TurnOriginSystem:
		if p.SystemKind == "" {
			return fmt.Errorf("system turn provenance needs a system kind")
		}
		return nil
	default:
		return fmt.Errorf("turn provenance has no valid origin (got %q)", p.Origin)
	}
}

// SandboxIDPrefix starts every sandbox id (see system.GenerateSandboxID).
const SandboxIDPrefix = "sbx_"

// RequestTurnProvenance derives the provenance of a turn sent by the
// authenticated request user. It fails for credentials that cannot send
// turns (runner, socket, anonymous) instead of guessing an origin.
func RequestTurnProvenance(user *User) (TurnProvenance, error) {
	if user == nil {
		return TurnProvenance{}, fmt.Errorf("turn provenance: no authenticated user")
	}
	switch user.TokenType {
	case TokenTypeOIDC, TokenTypeSession:
		return TurnProvenance{Origin: TurnOriginUser, UserID: user.ID}, nil
	case TokenTypeAPIKey:
		if user.Token == "" {
			return TurnProvenance{}, fmt.Errorf("turn provenance: API key request carries no key")
		}
		return TurnProvenance{
			Origin:             TurnOriginAPI,
			APIKeyOwner:        user.ID,
			APIKeyType:         apiKeyScope(user),
			APIKeyID:           APIKeyID(user.Token),
			APIKeyOrgID:        user.OrganizationID,
			APIKeyProjectID:    user.ProjectID,
			APIKeySpecTaskID:   user.SpecTaskID,
			APIKeySessionID:    user.SessionID,
			APIKeyTaskConfined: user.TaskConfined,
		}, nil
	default:
		return TurnProvenance{}, fmt.Errorf("turn provenance: credential type %q cannot send turns", user.TokenType)
	}
}

func apiKeyScope(user *User) APIKeyScope {
	switch user.APIKeyType {
	case APIkeytypeApp:
		return APIKeyScopeApp
	case APIkeytypeEmbed:
		return APIKeyScopeEmbed
	case APIkeytypeBotInstance:
		return APIKeyScopeBotInstance
	}
	switch {
	case user.TaskConfined:
		// A task-confined session key reaches only its spec task.
		return APIKeyScopeSpecTask
	case strings.HasPrefix(user.SessionID, SandboxIDPrefix):
		return APIKeyScopeSandbox
	case user.SessionID != "":
		return APIKeyScopeSession
	case user.SpecTaskID != "":
		return APIKeyScopeSpecTask
	case user.ProjectID != "":
		return APIKeyScopeProject
	case user.OrganizationID != "":
		return APIKeyScopeOrg
	default:
		return APIKeyScopePersonal
	}
}
