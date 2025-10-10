package crisp

import (
	"context"
	"fmt"

	"github.com/crisp-im/go-crisp-api/crisp/v3"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/rs/zerolog/log"
)

// To get the token:
// curl -H "Content-Type: application/json" -X POST -d '{"email":"email@example.com","password":"your password"}' https://api.crisp.chat/v1/user/session/login

func NewCrispBot(cfg *config.ServerConfig, store store.Store, controller *controller.Controller, app *types.App, trigger *types.CrispTrigger) *CrispBot {
	return &CrispBot{
		cfg:        cfg,
		store:      store,
		controller: controller,
		app:        app,
		trigger:    trigger,
	}
}

type CrispBot struct {
	cfg        *config.ServerConfig
	store      store.Store
	controller *controller.Controller
	app        *types.App // App/agent configuration
	client     *crisp.Client

	ctx       context.Context
	ctxCancel context.CancelFunc

	trigger *types.CrispTrigger
}

func (c *CrispBot) setStatus(ok bool, message string) {
	c.controller.SetTriggerStatus(c.app.ID, types.TriggerTypeCrisp, types.TriggerStatus{
		Type:    types.TriggerTypeCrisp,
		OK:      ok,
		Message: message,
	})
}

func (c *CrispBot) Stop() {
	if c.ctxCancel != nil {
		log.Info().Str("app_id", c.app.ID).Msg("stopping Crisp bot")
		c.ctxCancel()
	}

	c.controller.SetTriggerStatus(c.app.ID, types.TriggerTypeCrisp, types.TriggerStatus{
		Type:    types.TriggerTypeCrisp,
		OK:      false,
		Message: "Crisp bot stopped",
	})
}

func (c *CrispBot) RunBot(ctx context.Context) error {
	// Recover from panics
	defer func() {
		if r := recover(); r != nil {
			log.Error().Msgf("panic: %v", r)
		}
	}()

	client := crisp.New()

	c.ctx, c.ctxCancel = context.WithCancel(ctx)

	// Authenticate to API with your plugin token (identifier, key)
	// eg. client.AuthenticateTier("plugin", "43f34724-9eeb-4474-9cec-560250754dec", "d12e60c5d2aa264b90997a641b6474ffd6602b66d8e8abc49634c404f06fa7d0")
	client.AuthenticateTier("plugin", c.trigger.Identifier, c.trigger.Token)

	// Subscribe to realtime events (RTM API over WebSockets)
	client.Events.Listen(
		crisp.EventsModeWebSockets,

		[]string{
			"message:send",
			"message:received",
			"message:compose:send",
			"message:compose:receive",
		},

		func(reg *crisp.EventsRegister) {
			fmt.Print("WebSocket channel is connected: now listening for events\n")

			reg.On("message:send/text", func(evt crisp.EventsReceiveTextMessage) {
				fmt.Printf("[message:send/text] %s\n", evt)
			})

			reg.On("message:send/file", func(evt crisp.EventsReceiveFileMessage) {
				fmt.Printf("[message:send/file] %s\n", evt)
			})

			reg.On("message:send/animation", func(evt crisp.EventsReceiveAnimationMessage) {
				fmt.Printf("[message:send/animation] %s\n", evt)
			})

			reg.On("message:send/audio", func(evt crisp.EventsReceiveAudioMessage) {
				fmt.Printf("[message:send/audio] %s\n", evt)
			})

			reg.On("message:send/picker", func(evt crisp.EventsReceivePickerMessage) {
				fmt.Printf("[message:send/picker] %s\n", evt)
			})

			reg.On("message:send/field", func(evt crisp.EventsReceiveFieldMessage) {
				fmt.Printf("[message:send/field] %s\n", evt)
			})

			reg.On("message:send/carousel", func(evt crisp.EventsReceiveCarouselMessage) {
				fmt.Printf("[message:send/carousel] %s\n", evt)
			})

			reg.On("message:send/note", func(evt crisp.EventsReceiveNoteMessage) {
				fmt.Printf("[message:send/note] %s\n", evt)
			})

			reg.On("message:send/event", func(evt crisp.EventsReceiveEventMessage) {
				fmt.Printf("[message:send/event] %s\n", evt)
			})

			reg.On("message:received/text", func(evt crisp.EventsReceiveTextMessage) {
				fmt.Printf("[message:received/text] %s\n", evt)
			})

			reg.On("message:received/file", func(evt crisp.EventsReceiveFileMessage) {
				fmt.Printf("[message:received/file] %s\n", evt)
			})

			reg.On("message:received/animation", func(evt crisp.EventsReceiveAnimationMessage) {
				fmt.Printf("[message:received/animation] %s\n", evt)
			})

			reg.On("message:received/audio", func(evt crisp.EventsReceiveAudioMessage) {
				fmt.Printf("[message:received/audio] %s\n", evt)
			})

			reg.On("message:received/picker", func(evt crisp.EventsReceivePickerMessage) {
				fmt.Printf("[message:received/picker] %s\n", evt)
			})

			reg.On("message:received/field", func(evt crisp.EventsReceiveFieldMessage) {
				fmt.Printf("[message:received/field] %s\n", evt)
			})

			reg.On("message:received/carousel", func(evt crisp.EventsReceiveCarouselMessage) {
				fmt.Printf("[message:received/carousel] %s\n", evt)
			})

			reg.On("message:received/note", func(evt crisp.EventsReceiveNoteMessage) {
				fmt.Printf("[message:received/note] %s\n", evt)
			})

			reg.On("message:received/event", func(evt crisp.EventsReceiveEventMessage) {
				fmt.Printf("[message:received/event] %s\n", evt)
			})

			reg.On("message:compose:send", func(evt crisp.EventsReceiveMessageComposeSend) {
				fmt.Printf("[message:compose:send] %s\n", evt)
			})

			reg.On("message:compose:receive", func(evt crisp.EventsReceiveMessageComposeReceive) {
				fmt.Printf("[message:compose:receive] %s\n", evt)
			})
		},

		func() {
			fmt.Print("WebSocket channel is disconnected: will try to reconnect\n")
		},

		func(err error) {
			fmt.Printf("WebSocket channel error: %+v\n", err)
		},
	)

	// Block until the context is done
	<-c.ctx.Done()

	return nil
}
