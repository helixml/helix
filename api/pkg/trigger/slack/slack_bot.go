package slack

import (
	"context"
	"fmt"
	stdlog "log"
	"net/http"
	"os"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/trigger/shared"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	"github.com/sashabaranov/go-openai"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

func NewSlackBot(cfg *config.ServerConfig, store store.Store, controller *controller.Controller, app *types.App, trigger *types.SlackTrigger) *SlackBot {
	return &SlackBot{
		cfg:        cfg,
		store:      store,
		controller: controller,
		app:        app,
		trigger:    trigger,
		botUserID:  "", // Initialize botUserID
	}
}

// SlackBot - agent instance that connects to the Slack API
type SlackBot struct { //nolint:revive
	cfg        *config.ServerConfig
	store      store.Store
	controller *controller.Controller

	ctx       context.Context
	ctxCancel context.CancelFunc

	app *types.App // App/agent configuration

	trigger *types.SlackTrigger

	// Bot user ID for filtering bot messages
	botUserID string
}

func (s *SlackBot) Stop() {
	if s.ctxCancel != nil {
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
		slack.OptionDebug(true),
		slack.OptionLog(stdlog.New(os.Stdout, "api: ", stdlog.Lshortfile|stdlog.LstdFlags)),
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
	log.Info().Str("app_id", s.app.ID).Str("bot_user_id", s.botUserID).Msg("bot user ID retrieved")

	client := socketmode.New(
		api,
		socketmode.OptionDebug(true),
		// socketmode.OptionLog(stdlog.New(os.Stdout, "socketmode: ", stdlog.Lshortfile|stdlog.LstdFlags)),
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
	// socketmodeHandler.Handle(socketmode.EventTypeEventsAPI, s.middlewareEventsAPI)

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

	agentResponse, err := s.handleMessage(context.Background(), nil, s.app, ev.Text, ev.Channel, ev.TimeStamp, ev.ThreadTimeStamp, true)
	if err != nil {
		log.Error().Err(err).Msg("failed to start chat")
		_, _, _ = client.Client.PostMessage(ev.Channel, slack.MsgOptionText(err.Error(), false), slack.MsgOptionTS(ev.TimeStamp))
		return
	}

	// Write agent response to Slack's thread
	// Use the message timestamp as the thread timestamp to create a proper thread
	log.Debug().
		Str("app_id", s.app.ID).
		Str("channel", ev.Channel).
		Str("thread_timestamp", ev.TimeStamp).
		Str("response_length", fmt.Sprintf("%d", len(agentResponse))).
		Msg("Posting bot response in thread")

	_, _, err = client.Client.PostMessage(ev.Channel, slack.MsgOptionText(agentResponse, false), slack.MsgOptionTS(ev.TimeStamp))
	if err != nil {
		log.Error().Err(err).Msg("failed to post message")
	}
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

	thread, hasActiveThread := s.getActiveThread(s.ctx, ev.Channel, threadKey)
	if !hasActiveThread {
		log.Debug().
			Str("app_id", s.app.ID).
			Str("thread_key", threadKey).
			Msg("No active thread found, ignoring message")
		return
	}

	log.Info().
		Str("app_id", s.app.ID).
		Str("channel", ev.Channel).
		Str("thread", threadKey).
		Str("user", ev.User).
		Msg("Received message in active thread")

	agentResponse, err := s.handleMessage(context.Background(), thread, s.app, ev.Text, ev.Channel, ev.TimeStamp, ev.ThreadTimeStamp, false)
	if err != nil {
		log.Error().Err(err).Msg("failed to continue chat")
		_, _, _ = client.Client.PostMessage(ev.Channel, slack.MsgOptionText(err.Error(), false), slack.MsgOptionTS(ev.ThreadTimeStamp))
		return
	}

	// Write agent response to Slack's thread
	// Use the thread timestamp to keep the reply in the same thread
	log.Debug().
		Str("app_id", s.app.ID).
		Str("channel", ev.Channel).
		Str("thread_timestamp", ev.ThreadTimeStamp).
		Str("response_length", fmt.Sprintf("%d", len(agentResponse))).
		Msg("Posting bot response to thread")

	_, _, err = client.Client.PostMessage(ev.Channel, slack.MsgOptionText(agentResponse, false), slack.MsgOptionTS(ev.ThreadTimeStamp))
	if err != nil {
		log.Error().Err(err).Msg("failed to post message")
	}
}

func (s *SlackBot) handleMessage(ctx context.Context, existingThread *types.SlackThread, app *types.App, messageText, channel, messageTimestamp, threadTimestamp string, isMention bool) (string, error) {
	log.Debug().
		Str("app_id", app.ID).
		Str("message_timestamp", messageTimestamp).
		Str("thread_timestamp", threadTimestamp).
		Bool("is_mention", isMention).
		Msg("handleMessage called")

	var (
		session *types.Session
		err     error
	)

	if isMention {
		// This is a new conversation (mention), create a new session and a thread

		log.Info().
			Str("app_id", app.ID).
			Str("channel", channel).
			Str("message_ts", messageTimestamp).
			Msg("starting new Slack session")

		newSession := shared.NewTriggerSession(ctx, "Slack", app, messageText)
		session = newSession.Session

		threadKey := threadTimestamp
		if threadKey == "" {
			threadKey = messageTimestamp
		}

		// Create the new thread
		var err error
		_, err = s.createNewThread(s.ctx, channel, threadKey, session.ID)
		if err != nil {
			log.Error().Err(err).Msg("failed to create new thread")
			return "", fmt.Errorf("failed to create new thread: %w", err)
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
			return "", fmt.Errorf("failed to create session: %w", err)
		}
	} else {
		// This is a continuation of an existing conversation
		session, err = s.store.GetSession(ctx, existingThread.SessionID)
		if err != nil {
			log.Error().Err(err).Msg("failed to get session")
			return "", fmt.Errorf("failed to get session, error: %w", err)
		}

		log.Info().
			Str("app_id", app.ID).
			Str("channel", channel).
			Str("thread_id", existingThread.ThreadKey).
			Str("message_ts", messageTimestamp).
			Str("session_id", session.ID).
			Msg("continuing existing Slack session")

		// Add the new message to the existing session
		userInteraction := &types.Interaction{
			ID:       system.GenerateUUID(),
			Created:  time.Now(),
			Updated:  time.Now(),
			Mode:     types.SessionModeInference,
			Creator:  types.CreatorTypeUser,
			State:    types.InteractionStateComplete,
			Finished: true,
			Message:  messageText,
			Content: types.MessageContent{
				ContentType: types.MessageContentTypeText,
				Parts:       []any{messageText},
			},
		}

		assistantInteraction := &types.Interaction{
			ID:       system.GenerateUUID(),
			Created:  time.Now(),
			Updated:  time.Now(),
			Creator:  types.CreatorTypeAssistant,
			Mode:     types.SessionModeInference,
			Message:  "",
			State:    types.InteractionStateWaiting,
			Finished: false,
			Metadata: map[string]string{},
		}

		session.Interactions = append(session.Interactions, userInteraction, assistantInteraction)
		session.Updated = time.Now()

		err = s.controller.WriteSession(ctx, session)
		if err != nil {
			log.Error().
				Err(err).
				Str("app_id", app.ID).
				Str("session_id", session.ID).
				Msg("failed to update session")
			return "", fmt.Errorf("failed to update session: %w", err)
		}
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
		return "", fmt.Errorf("failed to get user: %w", err)
	}

	// Prepare chat completion request
	messages := []openai.ChatCompletionMessage{}
	for _, interaction := range session.Interactions {
		if interaction.Creator == types.CreatorTypeUser {
			messages = append(messages, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleUser,
				Content: interaction.Message,
			})
		} else if interaction.Creator == types.CreatorTypeAssistant && interaction.State == types.InteractionStateComplete {
			messages = append(messages, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleAssistant,
				Content: interaction.Message,
			})
		}
	}

	request := openai.ChatCompletionRequest{
		Stream:   false,
		Messages: messages,
	}

	// Set up context for the request
	ctx = oai.SetContextSessionID(ctx, session.ID)
	ctx = oai.SetContextAppID(ctx, app.ID)

	// Get the last assistant interaction
	lastAssistantInteraction := session.Interactions[len(session.Interactions)-1]
	ctx = oai.SetContextValues(ctx, &oai.ContextValues{
		OwnerID:       app.Owner,
		SessionID:     session.ID,
		InteractionID: lastAssistantInteraction.ID,
	})

	resp, _, err := s.controller.ChatCompletion(
		ctx,
		user,
		request,
		&controller.ChatCompletionOptions{
			AppID:          app.ID,
			Conversational: true,
		},
	)
	if err != nil {
		lastAssistantInteraction.Error = err.Error()
		lastAssistantInteraction.State = types.InteractionStateError
		lastAssistantInteraction.Finished = true
		lastAssistantInteraction.Completed = time.Now()
		err = s.controller.WriteSession(ctx, session)
		if err != nil {
			log.Error().
				Err(err).
				Str("app_id", app.ID).
				Str("user_id", app.Owner).
				Str("session_id", session.ID).
				Msg("failed to update session")
		}

		return "", fmt.Errorf("failed to get response from inference API: %w", err)
	}

	var respContent string
	if len(resp.Choices) > 0 {
		respContent = resp.Choices[0].Message.Content
	}

	// Update session with response
	lastAssistantInteraction.Message = respContent
	lastAssistantInteraction.State = types.InteractionStateComplete
	lastAssistantInteraction.Finished = true
	lastAssistantInteraction.Completed = time.Now()

	err = s.controller.WriteSession(ctx, session)
	if err != nil {
		log.Error().
			Err(err).
			Str("app_id", app.ID).
			Str("user_id", app.Owner).
			Str("session_id", session.ID).
			Msg("failed to update session")
	}

	return respContent, nil
}

func (s *SlackBot) middlewareConnecting(_ *socketmode.Event, _ *socketmode.Client) {
	log.Debug().Msg("Connecting to Slack with Socket Mode...")
	s.setStatus(false, "Connecting to Slack...")
}

func (s *SlackBot) middlewareConnectionError(evt *socketmode.Event, _ *socketmode.Client) {
	log.Error().Any("event", evt).Msg("Connection failed. Retrying later...")
	s.setStatus(false, "Connection failed. Retrying later...")
}

func (s *SlackBot) middlewareConnected(_ *socketmode.Event, _ *socketmode.Client) {
	log.Debug().Msg("Connected to Slack with Socket Mode.")
	s.setStatus(true, "Connected to Slack")
}

func (s *SlackBot) getActiveThread(ctx context.Context, channel, threadKey string) (*types.SlackThread, bool) {
	tread, err := s.store.GetSlackThread(ctx, s.app.ID, channel, threadKey)
	if err != nil {
		log.Error().Err(err).Msg("failed to get slack thread")
		return nil, false
	}
	// Active thread found
	if tread != nil {
		return tread, true
	}

	return nil, false
}

func (s *SlackBot) createNewThread(ctx context.Context, channel, threadKey, sessionID string) (*types.SlackThread, error) {
	thread := &types.SlackThread{
		ThreadKey: threadKey,
		AppID:     s.app.ID,
		Channel:   channel,
		SessionID: sessionID,
	}

	return s.store.CreateSlackThread(ctx, thread)
}
