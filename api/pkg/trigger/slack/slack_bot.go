package slack

import (
	"context"
	"errors"
	"fmt"
	"io"
	stdlog "log"
	"net/http"
	"regexp"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/trigger/shared"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

func NewSlackBot(cfg *config.ServerConfig, store store.Store, controller *controller.Controller, app *types.App, trigger *types.SlackTrigger) *SlackBot {
	return &SlackBot{
		cfg:         cfg,
		store:       store,
		controller:  controller,
		app:         app,
		trigger:     trigger,
		botUserID:   "", // Initialize botUserID
		postMessage: nil,
	}
}

// SlackBot - agent instance that connects to the Slack API
type SlackBot struct { //nolint:revive
	cfg                    *config.ServerConfig
	store                  store.Store
	controller             *controller.Controller
	postMessage            func(channelID string, options ...slack.MsgOption) (string, string, error)
	updateMessage          func(channelID, timestamp string, options ...slack.MsgOption) (string, string, string, error)
	getConversationReplies func(params *slack.GetConversationRepliesParameters) (msgs []slack.Message, hasMore bool, nextCursor string, err error)

	ctx       context.Context
	ctxCancel context.CancelFunc

	app *types.App // App/agent configuration

	trigger *types.SlackTrigger

	// Bot user ID for filtering bot messages
	botUserID string
	botID     string
}

func (s *SlackBot) Stop() {
	if s.ctxCancel != nil {
		log.Info().Str("app_id", s.app.ID).Msg("stopping Slack bot")
		s.ctxCancel()
	}
}

// Update controller status with the current status of the bot
func (s *SlackBot) setStatus(ok bool, message string) {
	s.controller.SetTriggerStatus(s.app.ID, types.TriggerTypeSlack, types.TriggerStatus{
		Type:    types.TriggerTypeSlack,
		OK:      ok,
		Message: message,
	})
}

func (s *SlackBot) RunBot(ctx context.Context) error {
	// Recover from panics
	defer func() {
		if r := recover(); r != nil {
			log.Error().Msgf("panic: %v", r)
		}
	}()

	log.Info().Str("app_id", s.app.ID).Msg("starting Slack bot")
	defer log.Info().Str("app_id", s.app.ID).Msg("stopping Slack bot")

	s.ctx, s.ctxCancel = context.WithCancel(ctx)

	options := []slack.Option{
		slack.OptionDebug(false),
		slack.OptionLog(stdlog.New(io.Discard, "api: ", stdlog.Lshortfile|stdlog.LstdFlags)),
		slack.OptionHTTPClient(http.DefaultClient),
	}

	if s.trigger.AppToken != "" {
		options = append(options, slack.OptionAppLevelToken(s.trigger.AppToken))
	}

	api := slack.New(
		s.trigger.BotToken,
		options...,
	)

	// Get bot user ID for filtering bot messages
	authTest, err := api.AuthTest()
	if err != nil {
		log.Error().Err(err).Str("app_id", s.app.ID).Msg("failed to get auth test")
		return fmt.Errorf("failed to get auth test: %w", err)
	}
	s.botUserID = authTest.UserID
	s.botID = authTest.BotID
	s.postMessage = api.PostMessage
	s.updateMessage = api.UpdateMessage
	s.getConversationReplies = api.GetConversationReplies
	log.Info().Str("app_id", s.app.ID).Str("bot_user_id", s.botUserID).Msg("bot user ID retrieved")

	client := socketmode.New(
		api,
		socketmode.OptionDebug(true),
		socketmode.OptionLog(stdlog.New(io.Discard, "socketmode: ", stdlog.Lshortfile|stdlog.LstdFlags)),
	)

	socketmodeHandler := socketmode.NewSocketmodeHandler(client)

	socketmodeHandler.Handle(socketmode.EventTypeConnecting, s.middlewareConnecting)
	socketmodeHandler.Handle(socketmode.EventTypeConnectionError, s.middlewareConnectionError)
	socketmodeHandler.Handle(socketmode.EventTypeConnected, s.middlewareConnected)

	// Handle app mention events (when bot is mentioned)
	socketmodeHandler.HandleEvents(slackevents.AppMention, s.middlewareAppMentionEvent)

	// Handle regular message events (for thread conversations)
	socketmodeHandler.HandleEvents(slackevents.Message, s.middlewareMessageEvent)

	// TODO: this is to listen to everything
	// socketmodeHandler.Handle(socketmode.EventTypeEventsAPI, s.middlewareEventsAPI

	go func() {
		err := s.postProjectUpdates(s.ctx, s.app)
		if err != nil {
			log.Error().Err(err).Msg("failed to post project updates")
		}
	}()

	log.Info().Str("app_id", s.app.ID).Msg("running event loop")
	defer log.Info().Str("app_id", s.app.ID).Msg("event loop stopped")

	err = socketmodeHandler.RunEventLoop()
	if err != nil {
		log.Error().Err(err).Msg("failed to run event loop")
	}

	// Wait for the context to be cancelled
	<-s.ctx.Done()

	return nil
}

// middlewareAppMentionEvent - works as the initial trigger for a new conversation with Helix agent
func (s *SlackBot) middlewareAppMentionEvent(evt *socketmode.Event, client *socketmode.Client) {
	eventsAPIEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
	if !ok {
		log.Info().Msgf("Ignored: %+v", evt)
		return
	}

	client.Ack(*evt.Request)

	ev, ok := eventsAPIEvent.InnerEvent.Data.(*slackevents.AppMentionEvent)
	if !ok {
		log.Info().Msgf("Ignored event: %+v", ev)
		return
	}

	log.Info().
		Str("app_id", s.app.ID).
		Str("channel", ev.Channel).
		Str("user", ev.User).
		Str("timestamp", ev.TimeStamp).
		Str("thread_timestamp", ev.ThreadTimeStamp).
		Str("text", ev.Text).
		Msg("We have been mentioned")

	s.addReaction(client, ev.Channel, ev.TimeStamp, "eyes")

	agentResponse, documentIDs, err := s.handleMessage(context.Background(), nil, s.app, ev.Text, ev.Channel, ev.TimeStamp, ev.ThreadTimeStamp, true)
	if err != nil {
		s.addReaction(client, ev.Channel, ev.TimeStamp, "x")
		log.Error().Err(err).Msg("failed to start chat")
		slackFormattedError := convertMarkdownToSlackFormat(err.Error())
		_, _, _ = client.Client.PostMessage(ev.Channel, slack.MsgOptionText(slackFormattedError, false), slack.MsgOptionTS(ev.TimeStamp))
		return
	}

	msg := formatResponseForSlack(agentResponse, documentIDs)

	log.Debug().
		Str("app_id", s.app.ID).
		Str("channel", ev.Channel).
		Str("thread_timestamp", ev.TimeStamp).
		Str("response_length", fmt.Sprintf("%d", len(msg.text))).
		Int("blocks", len(msg.blocks)).
		Msg("Posting bot response in thread")

	opts := []slack.MsgOption{
		slack.MsgOptionText(msg.text, false),
		slack.MsgOptionTS(ev.TimeStamp),
	}
	if len(msg.blocks) > 0 {
		opts = append(opts, slack.MsgOptionBlocks(msg.blocks...))
	}
	_, _, err = client.Client.PostMessage(ev.Channel, opts...)
	if err != nil {
		s.addReaction(client, ev.Channel, ev.TimeStamp, "x")
		log.Error().Err(err).Msg("failed to post message")
		return
	}

	s.addReaction(client, ev.Channel, ev.TimeStamp, "white_check_mark")
}

// middlewareMessageEvent - processes message events that are part of the thread. This ensures
// agent can continue the conversation that the user has started
func (s *SlackBot) middlewareMessageEvent(evt *socketmode.Event, client *socketmode.Client) {
	log.Debug().Str("app_id", s.app.ID).Msg("middlewareMessageEvent received")

	eventsAPIEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
	if !ok {
		log.Debug().Str("app_id", s.app.ID).Msgf("Ignored non-EventsAPIEvent: %+v", evt)
		client.Ack(*evt.Request)
		return
	}

	client.Ack(*evt.Request)

	ev, ok := eventsAPIEvent.InnerEvent.Data.(*slackevents.MessageEvent)
	if !ok {
		log.Debug().Str("app_id", s.app.ID).Msgf("Ignored non-MessageEvent: %+v", eventsAPIEvent.InnerEvent.Data)
		return
	}

	// Skip messages from the bot itself
	if ev.BotID != "" {
		log.Debug().Str("app_id", s.app.ID).Str("bot_id", ev.BotID).Msg("Skipping bot message (bot_id)")
		return
	}

	// Also skip messages from the bot user (sometimes bot messages don't have bot_id set)
	if ev.User == s.botUserID {
		log.Debug().Str("app_id", s.app.ID).Str("user", ev.User).Str("bot_user_id", s.botUserID).Msg("Skipping bot message (user_id)")
		return
	}

	// Skip messages without text
	if ev.Text == "" {
		log.Debug().Str("app_id", s.app.ID).Msg("Skipping empty message")
		return
	}

	// Determine thread key - if this is a thread reply, use the thread timestamp
	// If it's a new message, use the message timestamp
	threadKey := ev.ThreadTimeStamp
	if threadKey == "" {
		// This is a new message, not a thread reply
		log.Debug().Str("app_id", s.app.ID).Msg("Message is not in a thread, ignoring")
		return
	}

	log.Debug().
		Str("app_id", s.app.ID).
		Str("thread_key", threadKey).
		Msg("Checking for active thread")

	ownedByBot := s.isBotOwnedThread(s.ctx, ev.Channel, threadKey)
	if !ownedByBot && s.trigger.ProjectChannel != "" && s.trigger.ProjectChannel != ev.Channel {
		ownedByBot = s.isBotOwnedThread(s.ctx, s.trigger.ProjectChannel, threadKey)
	}
	if !ownedByBot {
		log.Debug().
			Str("app_id", s.app.ID).
			Str("channel", ev.Channel).
			Str("thread_key", threadKey).
			Msg("Thread is not owned by this bot, ignoring message")
		return
	}

	thread, hasActiveThread := s.getActiveThread(s.ctx, ev.Channel, threadKey)
	if !hasActiveThread && s.trigger.ProjectChannel != "" && s.trigger.ProjectChannel != ev.Channel {
		log.Debug().
			Str("app_id", s.app.ID).
			Str("event_channel", ev.Channel).
			Str("configured_channel", s.trigger.ProjectChannel).
			Str("thread_key", threadKey).
			Msg("Thread not found in event channel, trying configured project channel")
		thread, hasActiveThread = s.getActiveThread(s.ctx, s.trigger.ProjectChannel, threadKey)
	}
	if !hasActiveThread {
		thread, hasActiveThread = s.reconcileMissingThread(s.ctx, ev.Channel, threadKey)
	}
	if !hasActiveThread {
		log.Debug().
			Str("app_id", s.app.ID).
			Str("channel", ev.Channel).
			Str("thread_key", threadKey).
			Msg("No active thread found, ignoring message")
		return
	}

	log.Debug().
		Str("app_id", s.app.ID).
		Str("channel", ev.Channel).
		Str("thread", threadKey).
		Str("user", ev.User).
		Msg("Received message in active thread")

	s.addReaction(client, ev.Channel, ev.TimeStamp, "eyes")

	agentResponse, documentIDs, err := s.handleMessage(context.Background(), thread, s.app, ev.Text, ev.Channel, ev.TimeStamp, ev.ThreadTimeStamp, false)
	if err != nil {
		s.addReaction(client, ev.Channel, ev.TimeStamp, "x")
		log.Error().Err(err).Msg("failed to continue chat")
		slackFormattedError := convertMarkdownToSlackFormat(err.Error())
		_, _, _ = client.Client.PostMessage(ev.Channel, slack.MsgOptionText(slackFormattedError, false), slack.MsgOptionTS(ev.ThreadTimeStamp))
		return
	}

	msg := formatResponseForSlack(agentResponse, documentIDs)

	log.Debug().
		Str("app_id", s.app.ID).
		Str("channel", ev.Channel).
		Str("thread_timestamp", ev.ThreadTimeStamp).
		Str("response_length", fmt.Sprintf("%d", len(msg.text))).
		Int("blocks", len(msg.blocks)).
		Msg("Posting bot response to thread")

	opts := []slack.MsgOption{
		slack.MsgOptionText(msg.text, false),
		slack.MsgOptionTS(ev.ThreadTimeStamp),
	}
	if len(msg.blocks) > 0 {
		opts = append(opts, slack.MsgOptionBlocks(msg.blocks...))
	}
	_, _, err = client.Client.PostMessage(ev.Channel, opts...)
	if err != nil {
		s.addReaction(client, ev.Channel, ev.TimeStamp, "x")
		log.Error().Err(err).Msg("failed to post message")
		return
	}

	s.addReaction(client, ev.Channel, ev.TimeStamp, "white_check_mark")
}

func (s *SlackBot) addReaction(client *socketmode.Client, channel, timestamp, reaction string) {
	if client == nil || channel == "" || timestamp == "" || reaction == "" {
		return
	}

	if err := client.Client.AddReaction(reaction, slack.ItemRef{Channel: channel, Timestamp: timestamp}); err != nil {
		log.Debug().
			Err(err).
			Str("app_id", s.app.ID).
			Str("channel", channel).
			Str("timestamp", timestamp).
			Str("reaction", reaction).
			Msg("failed to add Slack reaction")
	}
}

func (s *SlackBot) handleMessage(ctx context.Context, existingThread *types.SlackThread, app *types.App, messageText, channel, messageTimestamp, threadTimestamp string, isMention bool) (string, map[string]string, error) {
	log.Debug().
		Str("app_id", app.ID).
		Str("message_timestamp", messageTimestamp).
		Str("thread_timestamp", threadTimestamp).
		Bool("is_mention", isMention).
		Msg("handleMessage called")

	var (
		session         *types.Session
		specTask        *types.SpecTask
		shouldSummarize bool
		err             error
	)

	if existingThread != nil && existingThread.SpecTaskID != "" {
		specTask, err = s.store.GetSpecTask(ctx, existingThread.SpecTaskID)
		if err != nil {
			return "", nil, fmt.Errorf("failed to get spec task '%s': %w", existingThread.SpecTaskID, err)
		}
	}

	switch {
	case isMention:
		fmt.Println("XXX MENTION")
		session, err = s.handleAppMentionThread(ctx, app, channel, messageTimestamp, threadTimestamp)
		if err != nil {
			return "", nil, err
		}

	case existingThread != nil && existingThread.SpecTaskID == "":
		fmt.Println("XXX NO SPEC TASK")
		// Regular bot reply, not talking with Helix Project spec tasks
		session, err = s.store.GetSession(ctx, existingThread.SessionID)
		if err != nil {
			return "", nil, fmt.Errorf("failed to get session '%s': %w", existingThread.SessionID, err)
		}

	case existingThread != nil && specTask != nil && isSpecTaskActive(specTask.Status):
		fmt.Println("XXX ACTIVE TASK")
		// Spec task thread, handle spec task thread - using planning session ID
		planningSession, err := s.store.GetSession(ctx, specTask.PlanningSessionID)
		if err != nil {
			return "", nil, fmt.Errorf("failed to get planning session '%s': %w", specTask.PlanningSessionID, err)
		}

		// Replace the session with planning session, this one speaks directly with the zed agent
		session = planningSession

	case existingThread != nil && specTask != nil && !isSpecTaskActive(specTask.Status):
		fmt.Println("XXX INACTIVE TASK")
		// Inactive spec task (backlog or merged/done) - using normal app route
		shouldSummarize = true

		session, err = s.store.GetSession(ctx, existingThread.SessionID)
		if err != nil {
			return "", nil, fmt.Errorf("failed to get session '%s': %w", existingThread.SessionID, err)
		}

		spew.Dump(session)

	default:
		// Log
		log.Error().
			Str("app_id", app.ID).
			Str("channel", channel).
			Str("message_timestamp", messageTimestamp).
			Str("thread_timestamp", threadTimestamp).
			Msg("handleMessage not a mention and not a thread either")
		return "", nil, fmt.Errorf("unknown thread type")
	}

	// Get user for the request
	user, err := s.store.GetUser(ctx, &store.GetUserQuery{
		ID: app.Owner,
	})
	if err != nil {
		log.Error().
			Err(err).
			Str("app_id", app.ID).
			Str("user_id", app.Owner).
			Msg("failed to get user")
		return "", nil, fmt.Errorf("failed to get user: %w", err)
	}
	user.SpecTaskID = session.Metadata.SpecTaskID
	if specTask != nil {
		user.SpecTaskID = specTask.ID
	}
	user.ProjectID = session.Metadata.ProjectID

	interactionID := system.GenerateInteractionID()
	promptMessage := messageText
	historyLimit := 0

	if threadTimestamp != "" && shouldSummarize {
		threadMessages, err := s.getSlackThreadMessages(channel, threadTimestamp)
		if err != nil {
			return "", nil, fmt.Errorf("failed to get thread messages: %w", err)
		}

		if len(threadMessages) > 0 {
			summary, err := s.summarizeConversation(ctx, user, session, interactionID, threadMessages)
			if err != nil {
				return "", nil, fmt.Errorf("failed to summarize thread conversation: %w", err)
			}

			promptMessage = fmt.Sprintf("Here's a summary of the conversation so far: %s\n\nUser message:%s", summary, messageText)
			historyLimit = -1
		}
	}
	if !shouldSummarize && specTask != nil {
		log.Info().
			Str("app_id", app.ID).
			Str("spec_task_id", specTask.ID).
			Str("status", specTask.Status.String()).
			Str("session_id", session.ID).
			Msg("running spec task thread message through planning session without Slack summarization")
	}

	spew.Dump(app)

	resp, err := s.controller.RunBlockingSession(ctx, &controller.RunSessionRequest{
		OrganizationID: app.OrganizationID,
		App:            app,
		Session:        session,
		User:           user,
		InteractionID:  interactionID,
		PromptMessage:  types.MessageContent{Parts: []any{promptMessage}},
		HistoryLimit:   historyLimit,
	})
	if err != nil {
		return "", nil, fmt.Errorf("failed to get response from inference API: %w", err)
	}

	// Fetch updated session to get document_ids (populated during RAG)
	updatedSession, err := s.store.GetSession(ctx, session.ID)
	if err != nil {
		log.Warn().Err(err).Str("session_id", session.ID).Msg("failed to fetch updated session for document IDs")
		return resp.ResponseMessage, nil, nil
	}

	return resp.ResponseMessage, updatedSession.Metadata.DocumentIDs, nil
}

// handleAppTriggerThread - handles a default app/agent path where we use a thread to provide context to the agent.
// Must use `s.app` for model configuration, etc.
func (s *SlackBot) handleAppMentionThread(ctx context.Context, app *types.App, channel, messageTimestamp, threadTimestamp string) (*types.Session, error) {
	log.Info().
		Str("app_id", app.ID).
		Str("channel", channel).
		Str("message_ts", messageTimestamp).
		Msg("starting new Slack session")

	newSession := shared.NewTriggerSession(ctx, types.TriggerTypeSlack.String(), app)
	session := newSession.Session

	threadKey := threadTimestamp
	if threadKey == "" {
		threadKey = messageTimestamp
	}

	_, err := s.createNewThread(ctx, channel, threadKey, session.ID)
	if err != nil {
		log.Error().Err(err).Msg("failed to create new thread")
		return nil, fmt.Errorf("failed to create new thread: %w", err)
	}

	log.Debug().
		Str("app_id", app.ID).
		Str("thread_key", threadKey).
		Str("session_id", session.ID).
		Msg("stored new session for thread")

	err = s.controller.WriteSession(ctx, session)
	if err != nil {
		log.Error().
			Err(err).
			Str("app_id", app.ID).
			Msg("failed to create session")
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	return session, nil
}

func isSpecTaskActive(status types.SpecTaskStatus) bool {
	switch status {
	case types.TaskStatusQueuedSpecGeneration,
		types.TaskStatusSpecGeneration,
		types.TaskStatusSpecReview,
		types.TaskStatusSpecRevision,
		types.TaskStatusSpecApproved,
		types.TaskStatusQueuedImplementation,
		types.TaskStatusImplementation,
		types.TaskStatusImplementationReview,
		types.TaskStatusPullRequest:
		return true
	default:
		return false
	}
}

func (s *SlackBot) getSlackThreadMessages(channel, threadTimestamp string) ([]slack.Message, error) {
	if s.getConversationReplies == nil {
		return nil, fmt.Errorf("slack client is not initialized")
	}

	params := &slack.GetConversationRepliesParameters{
		ChannelID: channel,
		Timestamp: threadTimestamp,
		Inclusive: true,
		Limit:     200,
	}

	var all []slack.Message
	for {
		replies, hasMore, nextCursor, err := s.getConversationReplies(params)
		if err != nil {
			return nil, err
		}

		all = append(all, replies...)

		if !hasMore || nextCursor == "" {
			break
		}
		params.Cursor = nextCursor
	}

	return all, nil
}

func (s *SlackBot) middlewareConnecting(_ *socketmode.Event, _ *socketmode.Client) {
	log.Debug().
		Str("app_id", s.app.ID).
		Str("organization_id", s.app.OrganizationID).
		Msg("Connecting to Slack with Socket Mode...")
	s.setStatus(false, "Connecting to Slack...")
}

func (s *SlackBot) middlewareConnectionError(evt *socketmode.Event, _ *socketmode.Client) {
	log.Error().
		Str("app_id", s.app.ID).
		Str("organization_id", s.app.OrganizationID).
		Any("event", evt).
		Msg("Connection failed. Retrying later...")
	s.setStatus(false, "Connection failed. Retrying later...")
}

func (s *SlackBot) middlewareConnected(_ *socketmode.Event, _ *socketmode.Client) {
	log.Debug().
		Str("app_id", s.app.ID).
		Str("organization_id", s.app.OrganizationID).
		Msg("Connected to Slack with Socket Mode.")
	s.setStatus(true, "Connected to Slack")
}

func (s *SlackBot) getActiveThread(ctx context.Context, channel, threadKey string) (*types.SlackThread, bool) {
	tread, err := s.store.GetSlackThread(ctx, s.app.ID, channel, threadKey)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, false
		}
		log.Error().Err(err).
			Str("channel", channel).
			Str("thread_key", threadKey).
			Msg("failed to get slack thread")
		return nil, false
	}
	// Active thread found
	if tread != nil {
		return tread, true
	}

	return nil, false
}

func (s *SlackBot) createNewThread(ctx context.Context, channel, threadKey, sessionID string) (*types.SlackThread, error) {
	// Check if app uses external agent - if so, enable progress updates
	isExternalAgent := s.app.Config.Helix.DefaultAgentType == "zed_external"
	if !isExternalAgent {
		for _, assistant := range s.app.Config.Helix.Assistants {
			if assistant.AgentType == types.AgentTypeZedExternal {
				isExternalAgent = true
				break
			}
		}
	}

	thread := &types.SlackThread{
		ThreadKey:           threadKey,
		AppID:               s.app.ID,
		Channel:             channel,
		SessionID:           sessionID,
		PostProgressUpdates: isExternalAgent, // Auto-enable for external agents
		IncludeScreenshots:  isExternalAgent, // Include screenshots for external agents
	}

	return s.store.CreateSlackThread(ctx, thread)
}

func (s *SlackBot) reconcileMissingThread(ctx context.Context, channel, threadKey string) (*types.SlackThread, bool) {
	if !s.isBotOwnedThread(ctx, channel, threadKey) {
		return nil, false
	}

	session := shared.NewTriggerSession(ctx, types.TriggerTypeSlack.String(), s.app).Session
	if err := s.controller.WriteSession(ctx, session); err != nil {
		log.Error().
			Err(err).
			Str("app_id", s.app.ID).
			Str("channel", channel).
			Str("thread_key", threadKey).
			Msg("failed to create session while reconciling Slack thread")
		return nil, false
	}

	thread, err := s.createNewThread(ctx, channel, threadKey, session.ID)
	if err == nil {
		log.Info().
			Str("app_id", s.app.ID).
			Str("channel", channel).
			Str("thread_key", threadKey).
			Str("session_id", session.ID).
			Msg("reconciled missing Slack thread mapping")
		return thread, true
	}

	thread, getErr := s.store.GetSlackThread(ctx, s.app.ID, channel, threadKey)
	if getErr == nil && thread != nil {
		return thread, true
	}

	log.Error().
		Err(err).
		Str("app_id", s.app.ID).
		Str("channel", channel).
		Str("thread_key", threadKey).
		Msg("failed to persist reconciled Slack thread mapping")
	return nil, false
}

func (s *SlackBot) isBotOwnedThread(ctx context.Context, channel, threadKey string) bool {
	root, ok := s.getSlackThreadRoot(ctx, channel, threadKey)
	if !ok {
		return false
	}

	if root.User == s.botUserID || (s.botID != "" && root.BotID == s.botID) {
		return true
	}

	return s.botUserID != "" && strings.Contains(root.Text, "<@"+s.botUserID+">")
}

func (s *SlackBot) getSlackThreadRoot(_ context.Context, channel, threadKey string) (*slack.Message, bool) {
	if s.getConversationReplies == nil {
		return nil, false
	}

	replies, _, _, err := s.getConversationReplies(&slack.GetConversationRepliesParameters{
		ChannelID: channel,
		Timestamp: threadKey,
		Inclusive: true,
		Oldest:    threadKey,
		Latest:    threadKey,
		Limit:     1,
	})
	if err != nil {
		log.Debug().
			Err(err).
			Str("app_id", s.app.ID).
			Str("channel", channel).
			Str("thread_key", threadKey).
			Msg("failed to load thread root from Slack")
		return nil, false
	}
	if len(replies) == 0 {
		return nil, false
	}

	return &replies[0], true
}

func convertMarkdownToSlackFormat(markdown string) string {
	return convertMarkdownToSlackFormatWithLinks(markdown, nil)
}

// convertMarkdownToSlackFormatWithLinks converts markdown to Slack format with clickable citation links
func convertMarkdownToSlackFormatWithLinks(markdown string, documentIDs map[string]string) string {
	slackFormat := markdown

	slackFormat = shared.ProcessCitationsForChatWithLinks(slackFormat, documentIDs, shared.LinkFormatSlack)

	codeBlocks := []string{}
	inlineCodes := []string{}

	codeBlockRegex := regexp.MustCompile("```(\\w*)\\n([\\s\\S]*?)```")
	slackFormat = codeBlockRegex.ReplaceAllStringFunc(slackFormat, func(match string) string {
		codeBlocks = append(codeBlocks, match)
		return fmt.Sprintf("__CODE_BLOCK_%d__", len(codeBlocks)-1)
	})

	inlineCodeRegex := regexp.MustCompile("`([^`]+)`")
	slackFormat = inlineCodeRegex.ReplaceAllStringFunc(slackFormat, func(match string) string {
		inlineCodes = append(inlineCodes, match)
		return fmt.Sprintf("__INLINE_CODE_%d__", len(inlineCodes)-1)
	})

	slackFormat = convertMarkdownTables(slackFormat)

	slackFormat = convertMarkdownHeadings(slackFormat)

	listItemRegex := regexp.MustCompile(`^[\s]*[-*][\s]+`)
	lines := strings.Split(slackFormat, "\n")
	for i, line := range lines {
		if listItemRegex.MatchString(line) {
			lines[i] = listItemRegex.ReplaceAllString(line, "• ")
		}
	}
	slackFormat = strings.Join(lines, "\n")

	slackFormat = convertBoldAndItalic(slackFormat)

	linkRegex := regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	slackFormat = linkRegex.ReplaceAllString(slackFormat, "<$2|$1>")

	strikethroughRegex := regexp.MustCompile(`~~(.*?)~~`)
	slackFormat = strikethroughRegex.ReplaceAllString(slackFormat, "~$1~")

	for i, codeBlock := range codeBlocks {
		placeholder := fmt.Sprintf("__CODE_BLOCK_%d__", i)
		slackFormat = strings.Replace(slackFormat, placeholder, codeBlock, 1)
	}

	for i, inlineCode := range inlineCodes {
		placeholder := fmt.Sprintf("__INLINE_CODE_%d__", i)
		slackFormat = strings.Replace(slackFormat, placeholder, inlineCode, 1)
	}

	return slackFormat
}

func convertMarkdownHeadings(text string) string {
	lines := strings.Split(text, "\n")
	headingRegex := regexp.MustCompile(`^(#{1,6})\s+(.+)$`)
	for i, line := range lines {
		if m := headingRegex.FindStringSubmatch(line); m != nil {
			lines[i] = "**" + strings.TrimSpace(m[2]) + "**"
		}
	}
	return strings.Join(lines, "\n")
}

func convertMarkdownTables(text string) string {
	lines := strings.Split(text, "\n")
	var result []string
	i := 0

	for i < len(lines) {
		if !isTableRow(lines[i]) {
			result = append(result, lines[i])
			i++
			continue
		}

		tableStart := i
		for i < len(lines) && isTableRow(lines[i]) {
			i++
		}
		tableEnd := i

		tableLines := lines[tableStart:tableEnd]
		converted := convertTableToKeyValue(tableLines)
		result = append(result, converted...)
	}

	return strings.Join(result, "\n")
}

func isTableRow(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "|") && strings.HasSuffix(trimmed, "|") && strings.Count(trimmed, "|") >= 2
}

func isTableSeparator(line string) bool {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "|") {
		return false
	}
	cleaned := strings.NewReplacer("|", "", "-", "", ":", "", " ", "").Replace(trimmed)
	return cleaned == ""
}

func parseTableCells(line string) []string {
	trimmed := strings.TrimSpace(line)
	trimmed = strings.TrimPrefix(trimmed, "|")
	trimmed = strings.TrimSuffix(trimmed, "|")
	parts := strings.Split(trimmed, "|")
	cells := make([]string, len(parts))
	for i, p := range parts {
		cells[i] = strings.TrimSpace(p)
	}
	return cells
}

func convertTableToKeyValue(tableLines []string) []string {
	if len(tableLines) < 2 {
		return tableLines
	}

	headers := parseTableCells(tableLines[0])

	dataStart := 1
	if dataStart < len(tableLines) && isTableSeparator(tableLines[dataStart]) {
		dataStart = 2
	}

	if dataStart >= len(tableLines) {
		return tableLines
	}

	var out []string
	for rowIdx := dataStart; rowIdx < len(tableLines); rowIdx++ {
		cells := parseTableCells(tableLines[rowIdx])
		for j, cell := range cells {
			if j < len(headers) && cell != "" {
				out = append(out, fmt.Sprintf("**%s:** %s", headers[j], cell))
			}
		}
		if rowIdx < len(tableLines)-1 {
			out = append(out, "")
		}
	}

	return out
}

// slackTableBlock implements slack.Block for Slack's native table block type.
// slack-go v0.12.2 doesn't have built-in table block support.
type slackTableBlock struct {
	Type           slack.MessageBlockType `json:"type"`
	BlockID        string                 `json:"block_id,omitempty"`
	Rows           [][]slackTableCell     `json:"rows"`
	ColumnSettings []slackColumnSetting   `json:"column_settings,omitempty"`
}

type slackTableCell struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type slackColumnSetting struct {
	Align     string `json:"align,omitempty"`
	IsWrapped bool   `json:"is_wrapped,omitempty"`
}

func (b *slackTableBlock) BlockType() slack.MessageBlockType {
	return b.Type
}

type slackFormattedMessage struct {
	text   string
	blocks []slack.Block
}

func formatResponseForSlack(markdown string, documentIDs map[string]string) slackFormattedMessage {
	text := convertMarkdownToSlackFormatWithLinks(markdown, documentIDs)

	segments := splitMarkdownByTables(markdown)
	hasTable := false
	for _, seg := range segments {
		if seg.isTable {
			hasTable = true
			break
		}
	}
	if !hasTable {
		return slackFormattedMessage{text: text}
	}

	var blocks []slack.Block
	tableUsed := false

	for _, seg := range segments {
		segText := strings.Join(seg.lines, "\n")

		if seg.isTable && !tableUsed {
			tb := buildTableBlock(seg.lines)
			if tb != nil {
				blocks = append(blocks, tb)
				tableUsed = true
				continue
			}
		}

		if seg.isTable {
			segText = strings.Join(convertTableToKeyValue(seg.lines), "\n")
		}

		converted := convertMarkdownToSlackFormatWithLinks(segText, documentIDs)
		if trimmed := strings.TrimSpace(converted); trimmed != "" {
			appendSectionBlocks(&blocks, trimmed)
		}
	}

	if len(blocks) == 0 {
		return slackFormattedMessage{text: text}
	}

	return slackFormattedMessage{text: text, blocks: blocks}
}

type markdownSegment struct {
	isTable bool
	lines   []string
}

func splitMarkdownByTables(markdown string) []markdownSegment {
	lines := strings.Split(markdown, "\n")
	var segments []markdownSegment
	var currentText []string
	i := 0

	for i < len(lines) {
		if isTableRow(lines[i]) {
			if len(currentText) > 0 {
				segments = append(segments, markdownSegment{isTable: false, lines: currentText})
				currentText = nil
			}
			var tableLines []string
			for i < len(lines) && isTableRow(lines[i]) {
				tableLines = append(tableLines, lines[i])
				i++
			}
			segments = append(segments, markdownSegment{isTable: true, lines: tableLines})
		} else {
			currentText = append(currentText, lines[i])
			i++
		}
	}

	if len(currentText) > 0 {
		segments = append(segments, markdownSegment{isTable: false, lines: currentText})
	}

	return segments
}

func buildTableBlock(tableLines []string) *slackTableBlock {
	if len(tableLines) < 2 {
		return nil
	}

	headers := parseTableCells(tableLines[0])
	if len(headers) == 0 || len(headers) > 20 {
		return nil
	}

	dataStart := 1
	if dataStart < len(tableLines) && isTableSeparator(tableLines[dataStart]) {
		dataStart = 2
	}
	if dataStart >= len(tableLines) {
		return nil
	}

	var rows [][]slackTableCell

	headerRow := make([]slackTableCell, len(headers))
	for i, h := range headers {
		headerRow[i] = slackTableCell{Type: "raw_text", Text: h}
	}
	rows = append(rows, headerRow)

	for i := dataStart; i < len(tableLines) && len(rows) < 100; i++ {
		cells := parseTableCells(tableLines[i])
		row := make([]slackTableCell, len(headers))
		for j := range headers {
			text := ""
			if j < len(cells) {
				text = cells[j]
			}
			row[j] = slackTableCell{Type: "raw_text", Text: text}
		}
		rows = append(rows, row)
	}

	settings := make([]slackColumnSetting, len(headers))
	for i := range settings {
		settings[i] = slackColumnSetting{IsWrapped: true}
	}

	return &slackTableBlock{
		Type:           "table",
		Rows:           rows,
		ColumnSettings: settings,
	}
}

const slackSectionMaxChars = 3000

func appendSectionBlocks(blocks *[]slack.Block, text string) {
	for len(text) > 0 {
		chunk := text
		if len(chunk) > slackSectionMaxChars {
			idx := strings.LastIndex(chunk[:slackSectionMaxChars], "\n")
			if idx > 0 {
				chunk = chunk[:idx]
			} else {
				chunk = chunk[:slackSectionMaxChars]
			}
		}
		*blocks = append(*blocks, slack.NewSectionBlock(
			slack.NewTextBlockObject(slack.MarkdownType, chunk, false, false),
			nil, nil,
		))
		text = text[len(chunk):]
		text = strings.TrimLeft(text, "\n")
	}
}

// convertBoldAndItalic handles the conversion of bold and italic markers
// using a state machine approach to avoid conflicts
func convertBoldAndItalic(text string) string {
	var result strings.Builder
	i := 0

	for i < len(text) {
		if i+1 < len(text) && text[i] == '*' && text[i+1] == '*' {
			// Found ** - this is bold
			result.WriteString("*")
			i += 2

			// Find the closing **
			for i < len(text)-1 {
				if text[i] == '*' && text[i+1] == '*' {
					result.WriteString("*")
					i += 2
					break
				}
				result.WriteByte(text[i])
				i++
			}
		} else if text[i] == '*' {
			// Found single * - this is italic
			result.WriteString("_")
			i++

			// Find the closing *
			for i < len(text) {
				if text[i] == '*' {
					result.WriteString("_")
					i++
					break
				}
				result.WriteByte(text[i])
				i++
			}
		} else {
			result.WriteByte(text[i])
			i++
		}
	}

	return result.String()
}
