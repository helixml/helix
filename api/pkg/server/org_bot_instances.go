package server

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/org/application/configregistry"
	"github.com/helixml/helix/api/pkg/org/application/instances"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	helixorgstore "github.com/helixml/helix/api/pkg/org/domain/store"
	runtimehelix "github.com/helixml/helix/api/pkg/org/infrastructure/runtime/helix"
	helixorgserver "github.com/helixml/helix/api/pkg/org/interfaces/server"
	helixorgapi "github.com/helixml/helix/api/pkg/org/interfaces/server/api"
	helixstore "github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// botInstanceStartTimeout bounds the background sandbox start of a new
// instance, like an exploratory session start.
const botInstanceStartTimeout = exploratorySessionStartTimeout

// botInstances implements helixorgapi.BotInstances. An instance is a session
// with the bot's identity (instructions, project, app, code agent config) and
// role types.SessionRoleOrgBotInstance, so the org runtime never treats it as
// the bot's main session.
type botInstances struct {
	server   *HelixAPIServer
	store    *helixorgstore.Store
	projects helixorgapi.ProjectEnsurer
	configs  *configregistry.Registry
	getApp   func(ctx context.Context, id string) (types.AppConfig, error)
}

var _ instances.Manager = botInstances{}

func (b botInstances) List(ctx context.Context, orgID string, botID orgchart.NodeID) ([]*types.Session, error) {
	bot, err := b.store.Nodes.Get(ctx, orgID, botID)
	if err != nil {
		return nil, fmt.Errorf("get bot: %w", err)
	}
	return b.listSessions(ctx, bot)
}

func (b botInstances) listSessions(ctx context.Context, bot orgchart.Node) ([]*types.Session, error) {
	if bot.AgentID == "" {
		return nil, nil
	}
	sessions, _, err := b.server.Store.ListSessions(ctx, helixstore.ListSessionsQuery{
		AnyOwner:              true,
		OrganizationID:        bot.OrganizationID,
		AppID:                 bot.AgentID,
		SessionRole:           types.SessionRoleOrgBotInstance,
		IncludeExternalAgents: true,
	})
	if err != nil {
		return nil, fmt.Errorf("list instances: %w", err)
	}
	out := make([]*types.Session, 0, len(sessions))
	for _, session := range sessions {
		if session.Metadata.OrgWorkerID == string(bot.ID) {
			out = append(out, session)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return out, nil
}

func (b botInstances) Create(ctx context.Context, orgID string, botID orgchart.NodeID, params instances.Params) (*types.Session, error) {
	userID := runtimehelix.UserIDFromContext(ctx)
	if userID == "" {
		return nil, errors.New("create instance: no requesting user")
	}
	user, err := b.server.Store.GetUser(ctx, &helixstore.GetUserQuery{ID: userID})
	if err != nil {
		return nil, fmt.Errorf("get requesting user: %w", err)
	}
	bot, err := b.store.Nodes.Get(ctx, orgID, botID)
	if err != nil {
		return nil, fmt.Errorf("get bot: %w", err)
	}
	projectID, appID, _, err := b.projects.Ensure(ctx, orgID, botID)
	if err != nil {
		return nil, fmt.Errorf("ensure bot project: %w", err)
	}
	mandate, err := botMandate(ctx, b.getApp, bot)
	if err != nil {
		return nil, err
	}
	app, err := b.server.Store.GetApp(ctx, appID)
	if err != nil {
		return nil, fmt.Errorf("get bot app: %w", err)
	}
	assistant := data.GetAssistant(app, "")
	if assistant == nil {
		return nil, fmt.Errorf("bot app %s has no assistant", appID)
	}

	profile := bot.EffectiveInstanceProfile()
	orgRuntime, orgResources := b.configs.GetDefaultSandboxConfig(ctx, orgID)
	launch := runtimehelix.EffectiveLaunchConfig(bot, orgRuntime, orgResources)
	sandboxRuntime := params.SandboxRuntime
	if sandboxRuntime == "" {
		sandboxRuntime = profile.SandboxRuntime
	}
	if sandboxRuntime == "" {
		sandboxRuntime = launch.SandboxRuntime
	}
	if sandboxRuntime != types.SandboxRuntimeHeadlessUbuntu && sandboxRuntime != types.SandboxRuntimeUbuntuDesktop {
		return nil, fmt.Errorf("%w: sandbox_runtime %q is not supported", instances.ErrInvalidRequest, sandboxRuntime)
	}
	resources := launch.SandboxResources

	name := params.Name
	if name == "" {
		botName := bot.Name
		if botName == "" {
			botName = string(bot.ID)
		}
		name = fmt.Sprintf("%s · %s", botName, time.Now().UTC().Format("Jan 2 15:04"))
	}
	codeAgentRuntime := assistant.CodeAgentRuntime
	if codeAgentRuntime == "" {
		codeAgentRuntime = types.CodeAgentRuntimeZedAgent
	}
	now := time.Now()
	created, err := b.server.Store.CreateSession(ctx, types.Session{
		ID:             system.GenerateSessionID(),
		Name:           name,
		Created:        now,
		Updated:        now,
		Mode:           types.SessionModeInference,
		Type:           types.SessionTypeText,
		ModelName:      "external_agent",
		Owner:          user.ID,
		OwnerType:      types.OwnerTypeUser,
		OrganizationID: orgID,
		ProjectID:      projectID,
		ParentApp:      appID,
		Metadata: types.SessionMetadata{
			Stream:                   true,
			AgentType:                "zed_external",
			ProjectID:                projectID,
			SessionRole:              types.SessionRoleOrgBotInstance,
			OrgWorkerID:              string(bot.ID),
			RuntimeInstructions:      mandate,
			SandboxRuntime:           sandboxRuntime,
			SandboxResourceOverrides: &resources,
			BotInstance:              &profile,
			AutoRestartOnCrash:       true,
			AssistantID:              assistant.ID,
			CodeAgentRuntime:         codeAgentRuntime,
			ZedAgentName:             codeAgentRuntime.ZedAgentName(),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("create instance session: %w", err)
	}

	// Start the sandbox off the request path, like an exploratory session:
	// a container launch takes seconds and the UI shows its own starting
	// state. resumeSessionInternal builds the desktop from the session row,
	// which already carries the bot identity.
	startCtx, cancel := detachContext(ctx, botInstanceStartTimeout)
	go func() {
		defer cancel()
		if _, err := b.server.resumeSessionInternal(startCtx, user, created); err != nil {
			log.Error().Err(err).Str("session_id", created.ID).Str("bot_id", string(bot.ID)).Msg("Failed to start bot instance sandbox")
		}
	}()

	if params.Message != "" {
		if _, err := b.server.enqueueAgentMessage(ctx, created.ID, params.Message, false, "", ""); err != nil {
			return nil, fmt.Errorf("queue first message: %w", err)
		}
	}
	return created, nil
}

// createBotInstanceForChat starts a new instance of the Org Bot behind app for
// POST /api/v1/sessions/chat. Like the org REST route, any member of the
// Bot's organization may create one; it is theirs.
func (s *HelixAPIServer) createBotInstanceForChat(ctx context.Context, user *types.User, app *types.App) (*types.Session, *system.HTTPError) {
	if s.botInstances == nil {
		return nil, system.NewHTTPError400("org bots are not enabled in this deployment")
	}
	if _, err := s.authorizeOrgMember(ctx, user, app.OrganizationID); err != nil {
		return nil, system.NewHTTPError403("only members of the bot's organization can chat with it")
	}
	instance, err := s.botInstances.CreateForApp(runtimehelix.WithUserID(ctx, user.ID), app)
	if errors.Is(err, helixorgstore.ErrNotFound) {
		return nil, system.NewHTTPError404(err.Error())
	}
	if err != nil {
		log.Error().Err(err).Str("app_id", app.ID).Msg("Failed to create bot instance for chat")
		return nil, system.NewHTTPError500("failed to create bot instance: " + err.Error())
	}
	return instance, nil
}

// enqueueBotInstanceTurnWebhook announces a finished instance turn to the
// organization's webhook endpoints. The turn is already saved, so a failure
// here is logged rather than failing the turn.
func (s *HelixAPIServer) enqueueBotInstanceTurnWebhook(ctx context.Context, session *types.Session, interaction *types.Interaction) {
	if session.Metadata.SessionRole != types.SessionRoleOrgBotInstance {
		return
	}
	err := s.Store.EnqueueWebhookEvent(ctx, types.WebhookEventBotInstanceTurnCompleted, session.OrganizationID, session.ProjectID, types.BotInstanceTurnWebhookData{
		SessionID:      session.ID,
		InteractionID:  interaction.ID,
		BotID:          session.Metadata.OrgWorkerID,
		AppID:          session.ParentApp,
		ProjectID:      session.ProjectID,
		OrganizationID: session.OrganizationID,
		State:          interaction.State,
		Response:       interaction.ResponseMessage,
		Error:          interaction.Error,
	})
	if err != nil {
		log.Error().Err(err).Str("session_id", session.ID).Str("interaction_id", interaction.ID).Msg("Failed to enqueue bot instance turn webhook")
	}
}

func (s *HelixAPIServer) enqueueBotInstanceChatFailure(ctx context.Context, interaction *types.Interaction) {
	if interaction.PromptID != "" {
		return
	}
	session, err := s.Store.GetSession(ctx, interaction.SessionID)
	if err != nil || session == nil {
		log.Warn().Err(err).Str("session_id", interaction.SessionID).Msg("Failed to load session for bot instance webhook")
		return
	}
	s.enqueueBotInstanceTurnWebhook(ctx, session, interaction)
}

// CreateForApp creates an instance of the bot backed by app, for a caller
// that knows the bot only by its app id: POST /api/v1/sessions/chat.
func (b botInstances) CreateForApp(ctx context.Context, app *types.App) (*types.Session, error) {
	nodes, err := b.store.Nodes.List(ctx, app.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("list bots: %w", err)
	}
	for _, node := range nodes {
		if node.AgentID == app.ID {
			return b.Create(ctx, app.OrganizationID, node.ID, instances.Params{})
		}
	}
	return nil, fmt.Errorf("bot for agent %s: %w", app.ID, helixorgstore.ErrNotFound)
}

func (b botInstances) Delete(ctx context.Context, orgID string, botID orgchart.NodeID, sessionID string) error {
	session, err := b.server.Store.GetSession(ctx, sessionID)
	if errors.Is(err, helixstore.ErrNotFound) {
		return fmt.Errorf("instance %s: %w", sessionID, helixorgstore.ErrNotFound)
	}
	if err != nil {
		return fmt.Errorf("get instance: %w", err)
	}
	if session.OrganizationID != orgID || session.Metadata.OrgWorkerID != string(botID) ||
		session.Metadata.SessionRole != types.SessionRoleOrgBotInstance {
		return fmt.Errorf("instance %s of bot %s: %w", sessionID, botID, helixorgstore.ErrNotFound)
	}
	if session.Owner != runtimehelix.UserIDFromContext(ctx) && !helixorgserver.CanManageOrganization(ctx) {
		return fmt.Errorf("only the instance owner or an organization owner can delete it: %w", instances.ErrForbidden)
	}
	return b.deleteSession(ctx, session)
}

// deleteSession tears an instance down: its sandbox, its workspace, then its
// session row.
func (b botInstances) deleteSession(ctx context.Context, session *types.Session) error {
	executor := b.server.externalAgentExecutor
	// Read the host before stopping: a stop can clear it from the session.
	sandboxID := session.SandboxID
	if executor.HasRunningContainer(ctx, session.ID) {
		if err := executor.StopDesktop(ctx, session.ID); err != nil {
			return fmt.Errorf("stop instance sandbox: %w", err)
		}
	}
	if err := executor.DeleteWorkspace(ctx, session.ID, sandboxID); err != nil {
		return fmt.Errorf("delete instance workspace: %w", err)
	}
	if _, err := b.server.Store.DeleteSession(ctx, session.ID); err != nil {
		return fmt.Errorf("delete instance session: %w", err)
	}
	return nil
}

// DeleteAll deletes every instance of a bot. The bot delete cascade calls it
// before the bot's project and app go away.
func (b botInstances) DeleteAll(ctx context.Context, orgID string, botID orgchart.NodeID) error {
	bot, err := b.store.Nodes.Get(ctx, orgID, botID)
	if err != nil {
		return fmt.Errorf("get bot: %w", err)
	}
	sessions, err := b.listSessions(ctx, bot)
	if err != nil {
		return err
	}
	for _, session := range sessions {
		if err := b.deleteSession(ctx, session); err != nil {
			return fmt.Errorf("delete instance %s: %w", session.ID, err)
		}
	}
	return nil
}

func (b botInstances) SyncProfile(ctx context.Context, orgID string, botID orgchart.NodeID) error {
	bot, err := b.store.Nodes.Get(ctx, orgID, botID)
	if err != nil {
		return fmt.Errorf("get bot: %w", err)
	}
	sessions, err := b.listSessions(ctx, bot)
	if err != nil {
		return err
	}
	profile := bot.EffectiveInstanceProfile()
	for _, session := range sessions {
		session.Metadata.BotInstance = &profile
		if _, err := b.server.Store.UpdateSession(ctx, *session); err != nil {
			return fmt.Errorf("update instance %s: %w", session.ID, err)
		}
	}
	return nil
}

// botMandate is the bot's own instructions: the linked App's single
// assistant prompt while the bot still has one, else the bot's content.
func botMandate(ctx context.Context, getApp func(ctx context.Context, id string) (types.AppConfig, error), bot orgchart.Node) (string, error) {
	if bot.AgentID == "" {
		return bot.Content, nil
	}
	appConfig, err := getApp(ctx, bot.AgentID)
	if err != nil {
		return "", fmt.Errorf("get canonical agent instructions: %w", err)
	}
	if len(appConfig.Helix.Assistants) != 1 {
		return "", fmt.Errorf("linked agent %s must contain exactly one assistant", bot.AgentID)
	}
	return appConfig.Helix.Assistants[0].SystemPrompt, nil
}
