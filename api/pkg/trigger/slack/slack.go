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
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

type Slack struct {
	cfg        *config.ServerConfig
	store      store.Store
	controller *controller.Controller

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
		cfg:        cfg,
		store:      store,
		controller: controller,
		app:        app,
		trigger:    trigger,
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
}

func (s *SlackBot) Stop() {
	if s.ctxCancel != nil {
		s.ctxCancel()
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

	client := socketmode.New(
		api,
		socketmode.OptionDebug(true),
		socketmode.OptionLog(stdlog.New(os.Stdout, "socketmode: ", stdlog.Lshortfile|stdlog.LstdFlags)),
	)

	socketmodeHandler := socketmode.NewSocketmodeHandler(client)

	socketmodeHandler.Handle(socketmode.EventTypeConnecting, middlewareConnecting)
	socketmodeHandler.Handle(socketmode.EventTypeConnectionError, middlewareConnectionError)
	socketmodeHandler.Handle(socketmode.EventTypeConnected, middlewareConnected)

	// Handle a specific event from EventsAPI
	socketmodeHandler.HandleEvents(slackevents.AppMention, s.middlewareAppMentionEvent)

	// TODO: this is to listen to everything
	// socketmodeHandler.Handle(socketmode.EventTypeEventsAPI, s.middlewareEventsAPI)

	err := socketmodeHandler.RunEventLoop()
	if err != nil {
		log.Error().Err(err).Msg("failed to run event loop")
	}

	// Wait for the context to be cancelled
	<-s.ctx.Done()

	return nil
}

func (s *Slack) middlewareEventsAPI(evt *socketmode.Event, client *socketmode.Client) {
	fmt.Println("middlewareEventsAPI")
	eventsAPIEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
	if !ok {
		fmt.Printf("Ignored %+v\n", evt)
		return
	}

	fmt.Printf("Event received: %+v\n", eventsAPIEvent)

	client.Ack(*evt.Request)

	switch eventsAPIEvent.Type {
	case slackevents.CallbackEvent:
		innerEvent := eventsAPIEvent.InnerEvent
		switch ev := innerEvent.Data.(type) {
		case *slackevents.AppMentionEvent:
			fmt.Printf("We have been mentionned in %v", ev.Channel)
			_, _, err := client.Client.PostMessage(ev.Channel, slack.MsgOptionText("Yes, hello.", false))
			if err != nil {
				fmt.Printf("failed posting message: %v", err)
			}
		case *slackevents.MemberJoinedChannelEvent:
			fmt.Printf("user %q joined to channel %q", ev.User, ev.Channel)
		}
	default:
		client.Debugf("unsupported Events API event received")
	}
}

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

	log.Info().Str("channel", ev.Channel).Msg("We have been mentioned")

	resp, err := s.startChat(context.Background(), s.app, ev)
	if err != nil {
		log.Error().Err(err).Msg("failed to start chat")
		_, _, _ = client.Client.PostMessage(ev.Channel, slack.MsgOptionText(err.Error(), false))
		return
	}

	_, _, err = client.Client.PostMessage(ev.Channel, slack.MsgOptionText(resp, false))
	if err != nil {
		log.Error().Err(err).Msg("failed to post message")
	}
}

func (s *SlackBot) startChat(ctx context.Context, app *types.App, event *slackevents.AppMentionEvent) (string, error) {
	system := openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleSystem,
		Content: `You are an AI assistant Slack bot. Be concise with the replies, keep them short but informative.`,
	}

	messages := []openai.ChatCompletionMessage{
		system,
	}

	// TODO: Add history from a thread
	// for _, msg := range history {
	// 	switch {
	// 	case msg.Author.ID == s.State.User.ID:
	// 		messages = append(messages, openai.ChatCompletionMessage{
	// 			Role:    openai.ChatMessageRoleAssistant,
	// 			Content: msg.Content,
	// 		})
	// 	default:
	// 		messages = append(messages, openai.ChatCompletionMessage{
	// 			Role:    openai.ChatMessageRoleUser,
	// 			Content: msg.Content,
	// 		})
	// 	}
	// }

	userMessage := openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: event.Text,
	}

	messages = append(messages, userMessage)

	resp, _, err := s.controller.ChatCompletion(
		ctx,
		&types.User{},
		openai.ChatCompletionRequest{
			Stream:   false,
			Messages: messages,
		},
		&controller.ChatCompletionOptions{
			AppID: app.ID,
		},
	)
	if err != nil {
		return "", fmt.Errorf("failed to get response from inference API: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	return resp.Choices[0].Message.Content, nil
}

func middlewareConnecting(evt *socketmode.Event, client *socketmode.Client) {
	log.Info().Msg("Connecting to Slack with Socket Mode...")
}

func middlewareConnectionError(evt *socketmode.Event, client *socketmode.Client) {
	log.Error().Msg("Connection failed. Retrying later...")
}

func middlewareConnected(evt *socketmode.Event, client *socketmode.Client) {
	log.Info().Msg("Connected to Slack with Socket Mode.")
}
