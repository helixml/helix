package slack

import (
	"context"
	"fmt"
	stdlog "log"
	"net/http"
	"os"
	"sync"
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

type Slack struct {
	cfg        *config.ServerConfig
	store      store.Store
	controller *controller.Controller

	// TODO: add method to controller to set trigger status, for example:
	// - bot is running
	// - bot is stopped (trigger disabled)
	// - bot is erroring, such as API key is invalid

	botMu sync.Mutex
	bot   map[string]*SlackBot // Slack bots for each Helix app/agent
}

func New(cfg *config.ServerConfig, store store.Store, controller *controller.Controller) *Slack {
	return &Slack{
		cfg:        cfg,
		store:      store,
		controller: controller,
		bot:        make(map[string]*SlackBot),
	}
}

// Start - reconcile SlackBots, check which apps/agents have Slack triggers enabled
// and start the bot for each of them. Once running, they are added into the map.
// If they get disabled, the bot will be stopped and removed from the map.
func (s *Slack) Start(ctx context.Context) error {
	// Reconcile bots
	for {
		err := s.reconcile(ctx)
		if err != nil {
			log.Error().Err(err).Msg("failed to reconcile Slack bots")
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
			continue
		}
	}
}

func (s *Slack) reconcile(ctx context.Context) error {
	// Get all apps from the store
	apps, err := s.store.ListApps(ctx, &store.ListAppsQuery{})
	if err != nil {
		return fmt.Errorf("failed to list apps: %w", err)
	}

	// Find apps with Slack triggers
	slackApps := make(map[string]*types.SlackTrigger)
	for _, app := range apps {
		for _, trigger := range app.Config.Helix.Triggers {
			if trigger.Slack != nil && trigger.Slack.BotToken != "" {
				slackApps[app.ID] = trigger.Slack
				break
			}
		}
	}

	s.botMu.Lock()
	defer s.botMu.Unlock()

	// Stop bots that are no longer needed
	for appID, bot := range s.bot {
		if _, exists := slackApps[appID]; !exists {
			log.Info().Str("app_id", appID).Msg("stopping Slack bot - no longer configured")
			bot.Stop()
			delete(s.bot, appID)
		}
	}

	// Start or update bots for configured apps
	for appID, slackTrigger := range slackApps {
		app, err := s.store.GetApp(ctx, appID)
		if err != nil {
			log.Error().Err(err).Str("app_id", appID).Msg("failed to get app for Slack bot")
			continue
		}

		existingBot, exists := s.bot[appID]
		if !exists {
			// Create new bot
			log.Info().Str("app_id", appID).Msg("starting new Slack bot")
			bot := NewSlackBot(s.cfg, s.store, s.controller, app, slackTrigger)
			s.bot[appID] = bot

			// Start the bot in a goroutine
			go func(bot *SlackBot, appID string) {
				if err := bot.RunBot(ctx); err != nil {
					log.Error().Err(err).Str("app_id", appID).Msg("Slack bot exited with error")
				}
			}(bot, appID)
		} else {
			// Check if the trigger configuration has changed
			if !s.triggerConfigEqual(existingBot.trigger, slackTrigger) {
				log.Info().Str("app_id", appID).Msg("updating Slack bot configuration")

				// Stop the existing bot
				existingBot.Stop()

				// Create new bot with updated configuration
				bot := NewSlackBot(s.cfg, s.store, s.controller, app, slackTrigger)
				s.bot[appID] = bot

				// Start the new bot in a goroutine
				go func(bot *SlackBot, appID string) {
					if err := bot.RunBot(ctx); err != nil {
						log.Error().Err(err).Str("app_id", appID).Msg("Slack bot exited with error")
					}
				}(bot, appID)
			}
		}
	}

	return nil
}

// triggerConfigEqual compares two SlackTrigger configurations for equality
func (s *Slack) triggerConfigEqual(a, b *types.SlackTrigger) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	return a.AppToken == b.AppToken &&
		a.BotToken == b.BotToken &&
		slicesEqual(a.Channels, b.Channels)
}

// Stop stops all running Slack bots
func (s *Slack) Stop() {
	s.botMu.Lock()
	defer s.botMu.Unlock()

	log.Info().Msg("stopping all Slack bots")

	for appID, bot := range s.bot {
		log.Info().Str("app_id", appID).Msg("stopping Slack bot")
		bot.Stop()
	}

	// Clear the bot map
	s.bot = make(map[string]*SlackBot)
}

// slicesEqual compares two string slices for equality
func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func NewSlackBot(cfg *config.ServerConfig, store store.Store, controller *controller.Controller, app *types.App, trigger *types.SlackTrigger) *SlackBot {
	return &SlackBot{
		cfg:           cfg,
		store:         store,
		controller:    controller,
		app:           app,
		trigger:       trigger,
		activeThreads: make(map[string]*types.Session), // Track active threads by thread timestamp
		botUserID:     "",                              // Initialize botUserID
	}
}

// SlackBot - agent instance that connects to the Slack API
type SlackBot struct { //nolint:revive
	cfg        *config.ServerConfig
	store      store.Store
	controller *controller.Controller

	ctx       context.Context
	ctxCancel context.CancelFunc

	// api    *slack.Client
	// client *socketmode.Client

	app *types.App // App/agent configuration

	trigger *types.SlackTrigger

	// Track active threads to continue conversations
	activeThreads map[string]*types.Session
	threadsMu     sync.RWMutex

	// Bot user ID for filtering bot messages
	botUserID string

	statusMu sync.RWMutex
	message  string
	ok       bool
}

func (s *SlackBot) Stop() {
	if s.ctxCancel != nil {
		s.ctxCancel()
	}
}

func (s *SlackBot) SetStatus(ok bool, message string) {
	s.statusMu.Lock()
	defer s.statusMu.Unlock()

	s.ok = ok
	s.message = message
}

func (s *SlackBot) GetStatus() (bool, string) {
	s.statusMu.RLock()
	defer s.statusMu.RUnlock()

	return s.ok, s.message
}

// cleanupOldThreads removes threads that haven't been active for more than 24 hours
func (s *SlackBot) cleanupOldThreads() {
	s.threadsMu.Lock()
	defer s.threadsMu.Unlock()

	cutoff := time.Now().Add(-24 * time.Hour)
	removed := 0

	for threadKey, session := range s.activeThreads {
		if session.Updated.Before(cutoff) {
			log.Debug().
				Str("app_id", s.app.ID).
				Str("thread_key", threadKey).
				Str("session_id", session.ID).
				Msg("removing old thread")
			delete(s.activeThreads, threadKey)
			removed++
		}
	}

	if removed > 0 {
		log.Info().
			Str("app_id", s.app.ID).
			Int("removed_threads", removed).
			Msg("cleaned up old threads")
	}
}

func (s *SlackBot) RunBot(ctx context.Context) error {
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

	// Start periodic cleanup of old threads
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-s.ctx.Done():
				return
			case <-ticker.C:
				s.cleanupOldThreads()
			}
		}
	}()

	err = socketmodeHandler.RunEventLoop()
	if err != nil {
		log.Error().Err(err).Msg("failed to run event loop")
	}

	// Wait for the context to be cancelled
	<-s.ctx.Done()

	return nil
}

// func (s *SlackBot) middlewareEventsAPI(evt *socketmode.Event, client *socketmode.Client) {
// 	fmt.Println("middlewareEventsAPI")
// 	eventsAPIEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
// 	if !ok {
// 		fmt.Printf("Ignored %+v\n", evt)
// 		return
// 	}

// 	fmt.Printf("Event received: %+v\n", eventsAPIEvent)

// 	client.Ack(*evt.Request)

// 	switch eventsAPIEvent.Type {
// 	case slackevents.CallbackEvent:
// 		innerEvent := eventsAPIEvent.InnerEvent
// 		switch ev := innerEvent.Data.(type) {
// 		case *slackevents.AppMentionEvent:
// 			fmt.Printf("We have been mentionned in %v", ev.Channel)
// 			_, _, err := client.Client.PostMessage(ev.Channel, slack.MsgOptionText("Yes, hello.", false))
// 			if err != nil {
// 				fmt.Printf("failed posting message: %v", err)
// 			}
// 		case *slackevents.MemberJoinedChannelEvent:
// 			fmt.Printf("user %q joined to channel %q", ev.User, ev.Channel)
// 		}
// 	default:
// 		client.Debugf("unsupported Events API event received")
// 	}
// }

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

	// Debug: log all active threads before processing
	s.debugActiveThreads()

	agentResponse, err := s.handleMessage(context.Background(), s.app, ev.Text, ev.Channel, ev.TimeStamp, ev.ThreadTimeStamp, true)
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

	// Debug: log all active threads after processing
	s.debugActiveThreads()
}

// debugActiveThreads logs all currently active threads for debugging
func (s *SlackBot) debugActiveThreads() {
	s.threadsMu.RLock()
	defer s.threadsMu.RUnlock()

	log.Debug().Str("app_id", s.app.ID).Msg("=== Active Threads ===")
	for threadKey, session := range s.activeThreads {
		log.Debug().
			Str("app_id", s.app.ID).
			Str("thread_key", threadKey).
			Str("session_id", session.ID).
			Msg("active thread")
	}
	log.Debug().Str("app_id", s.app.ID).Msg("=== End Active Threads ===")
}

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

	log.Debug().
		Str("app_id", s.app.ID).
		Str("channel", ev.Channel).
		Str("user", ev.User).
		Str("bot_id", ev.BotID).
		Str("timestamp", ev.TimeStamp).
		Str("thread_timestamp", ev.ThreadTimeStamp).
		Str("text", ev.Text).
		Msg("Received message event")

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

	// Debug: log all active threads
	s.debugActiveThreads()

	s.threadsMu.RLock()
	_, hasActiveThread := s.activeThreads[threadKey]
	s.threadsMu.RUnlock()

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

	agentResponse, err := s.handleMessage(context.Background(), s.app, ev.Text, ev.Channel, ev.TimeStamp, ev.ThreadTimeStamp, false)
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

func (s *SlackBot) handleMessage(ctx context.Context, app *types.App, messageText, channel, messageTimestamp, threadTimestamp string, isMention bool) (string, error) {
	threadKey := threadTimestamp
	if threadKey == "" {
		threadKey = messageTimestamp
	}

	log.Debug().
		Str("app_id", app.ID).
		Str("message_timestamp", messageTimestamp).
		Str("thread_timestamp", threadTimestamp).
		Str("thread_key", threadKey).
		Bool("is_mention", isMention).
		Msg("handleMessage called")

	s.threadsMu.Lock()
	defer s.threadsMu.Unlock()

	var session *types.Session
	var err error

	if isMention {
		// This is a new conversation (mention), create a new session
		log.Info().
			Str("app_id", app.ID).
			Str("channel", channel).
			Str("thread_id", threadKey).
			Str("message_ts", messageTimestamp).
			Msg("starting new Slack session")

		newSession := shared.NewTriggerSession(ctx, "Slack", app, messageText)
		session = newSession.Session

		// Store the session for this thread
		s.activeThreads[threadKey] = session

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
		var exists bool

		session, exists = s.activeThreads[threadKey]
		if !exists {
			log.Error().
				Str("app_id", app.ID).
				Str("thread_key", threadKey).
				Msg("no active session found for thread")
			return "", fmt.Errorf("no active session found for thread %s", threadKey)
		}

		log.Info().
			Str("app_id", app.ID).
			Str("channel", channel).
			Str("thread_id", threadKey).
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

func (s *SlackBot) middlewareConnecting(evt *socketmode.Event, client *socketmode.Client) {
	log.Debug().Msg("Connecting to Slack with Socket Mode...")
	s.SetStatus(false, "Connecting to Slack...")
}

func (s *SlackBot) middlewareConnectionError(evt *socketmode.Event, client *socketmode.Client) {
	log.Error().Msg("Connection failed. Retrying later...")
	s.SetStatus(false, "Connection failed. Retrying later...")
}

func (s *SlackBot) middlewareConnected(evt *socketmode.Event, client *socketmode.Client) {
	log.Debug().Msg("Connected to Slack with Socket Mode.")
	s.SetStatus(true, "Connected to Slack")
}
