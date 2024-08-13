package slack

import (
	"fmt"
	stdlog "log"
	"os"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/store"

	"github.com/rs/zerolog/log"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

type Slack struct {
	cfg        *config.ServerConfig
	store      store.Store
	controller *controller.Controller
}

func New(cfg *config.ServerConfig, store store.Store, controller *controller.Controller) *Slack {
	return &Slack{
		cfg:        cfg,
		store:      store,
		controller: controller,
	}
}

func (s *Slack) Start() error {

	api := slack.New(
		s.cfg.Triggers.Slack.BotToken,
		slack.OptionDebug(true),
		slack.OptionLog(stdlog.New(os.Stdout, "api: ", stdlog.Lshortfile|stdlog.LstdFlags)),
		slack.OptionAppLevelToken(s.cfg.Triggers.Slack.AppToken),
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

	return nil
}

func (s *Slack) middlewareAppMentionEvent(evt *socketmode.Event, client *socketmode.Client) {

	eventsAPIEvent, ok := evt.Data.(slackevents.EventsAPIEvent)
	if !ok {
		fmt.Printf("Ignored %+v\n", evt)
		return
	}

	client.Ack(*evt.Request)

	ev, ok := eventsAPIEvent.InnerEvent.Data.(*slackevents.AppMentionEvent)
	if !ok {
		fmt.Printf("Ignored %+v\n", ev)
		return
	}

	fmt.Printf("We have been mentionned in %v\n", ev.Channel)
	_, _, err := client.Client.PostMessage(ev.Channel, slack.MsgOptionText("Yes, hello.", false))
	if err != nil {
		fmt.Printf("failed posting message: %v", err)
	}
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
