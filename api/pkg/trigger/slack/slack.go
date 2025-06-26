package slack

import (
	"context"
	"fmt"
	stdlog "log"
	"os"
	"sync"

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
	}
}

// Start - reconcile SlackBots, check which apps/agents have Slack triggers enabled
// and start the bot for each of them. Once running, they are added into the map.
// If they get disabled, the bot will be stopped and removed from the map.
func (s *Slack) Start(ctx context.Context) error {
	s.botMu.Lock()
	defer s.botMu.Unlock()

	// TODO: implement

	return nil
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

type SlackBot struct {
	cfg        *config.ServerConfig
	store      store.Store
	controller *controller.Controller

	api    *slack.Client
	client *socketmode.Client

	app *types.App // App/agent configuration

	trigger *types.SlackTrigger
}

func (s *SlackBot) RunBot(ctx context.Context) error {
	log.Info().Str("app_id", s.app.ID).Msg("starting Slack bot")
	defer log.Info().Str("app_id", s.app.ID).Msg("stopping Slack bot")

	options := []slack.Option{
		slack.OptionDebug(true),
		slack.OptionLog(stdlog.New(os.Stdout, "api: ", stdlog.Lshortfile|stdlog.LstdFlags)),
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

	<-ctx.Done()

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

	resp, err := s.startChat(context.Background(), &types.App{}, ev)
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
		Content: `You are an AI assistant Discord bot. Be concise with the replies, keep them short but informative.`,
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
