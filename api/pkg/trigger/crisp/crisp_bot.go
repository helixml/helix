package crisp

import (
	"context"
	"errors"
	"fmt"

	"github.com/crisp-im/go-crisp-api/crisp/v3"
	"github.com/davecgh/go-spew/spew"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/ptr"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/trigger/shared"
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

	c.setStatus(false, "Crisp bot stopped")
}

func (c *CrispBot) RunBot(ctx context.Context) error {
	// Recover from panics
	defer func() {
		if r := recover(); r != nil {
			log.Error().Msgf("panic: %v", r)
		}
	}()

	client := crisp.New()

	c.setStatus(false, "Crisp bot connecting")

	c.ctx, c.ctxCancel = context.WithCancel(ctx)

	// Authenticate to API with your plugin token (identifier, key)
	// eg. client.AuthenticateTier("plugin", "43f34724-9eeb-4474-9cec-560250754dec", "d12e60c5d2aa264b90997a641b6474ffd6602b66d8e8abc49634c404f06fa7d0")
	client.AuthenticateTier("plugin", c.trigger.Identifier, c.trigger.Token)

	// Subscribe to realtime events (RTM API over WebSockets)
	err := client.Events.Listen(
		crisp.EventsModeWebSockets,

		[]string{
			"message:send",
			"message:received",
			"message:compose:send",
			"message:compose:receive",
		},

		func(reg *crisp.EventsRegister) {
			c.setStatus(true, "Crisp bot connected")

			fmt.Print("XXX WebSocket channel is connected: now listening for events\n")

			// User hits send button
			// [message:send/text] crisp.EventsReceiveTextMessage{EventsGeneric:crisp.EventsGeneric{RoutingIDs:[]},
			// EventsReceiveGenericMessage:crisp.EventsReceiveGenericMessage{EventsReceiveGenericMessageType:crisp.EventsReceiveGenericMessageType{Type:"text"},
			// EventsSessionGeneric:crisp.EventsSessionGeneric{EventsWebsiteGeneric:crisp.EventsWebsiteGeneric{WebsiteID:"ac7f9077-a943-4521-a259-cff0e877a49b"},
			// EventsSessionGenericUnbound:crisp.EventsSessionGenericUnbound{SessionID:"session_b3269776-df9c-483b-a894-f762148da1c0"}},
			// From:"user", Origin:"chat", Stamped:true, Timestamp:1760201391530, Fingerprint:176020139095962,
			// User:crisp.EventsReceiveCommonMessageUser{UserID:"session_b3269776-df9c-483b-a894-f762148da1c0", Nickname:"visitor523"}},
			// Content:"I have a problem with installation"}
			reg.On("message:send/text", func(evt crisp.EventsReceiveTextMessage) {
				fmt.Printf("[message:send/text] XXXXXX\n")

				spew.Dump(evt)

				// TODO:
				// 1. Load history of the conversation
				// 2. If needed - summarize it first

				err := c.handleTextMessage(c.ctx, client, evt)
				if err != nil {
					log.Error().Err(err).Msg("failed to handle text message")
				}

			})

			reg.On("message:send/file", func(evt crisp.EventsReceiveFileMessage) {
				fmt.Printf("[message:send/file] %s\n", evt)

				err := c.handleFileMessage(c.ctx, client, evt)
				if err != nil {
					log.Error().Err(err).Msg("failed to handle file message")
				}

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

			// User typing message
			reg.On("message:compose:send", func(evt crisp.EventsReceiveMessageComposeSend) {
				fmt.Printf("[message:compose:send] %s\n", evt)
				spew.Dump(evt)
			})

			reg.On("message:compose:receive", func(evt crisp.EventsReceiveMessageComposeReceive) {
				fmt.Printf("[message:compose:receive] %s\n", evt)
			})
		},

		func() {
			fmt.Print("XXX WebSocket channel is disconnected: will try to reconnect\n")
			c.setStatus(false, "Crisp bot disconnected, reconnecting...")
		},

		func(err error) {
			fmt.Printf("XXXWebSocket channel error: %+v\n", err)
			c.setStatus(false, fmt.Sprintf("Crisp bot websocket error: %v", err))
		},
	)
	if err != nil {
		log.Error().Err(err).Str("app_id", c.app.ID).Msg("failed to listen to Crisp events")
		c.setStatus(false, fmt.Sprintf("Crisp bot exited with error: %v", err))
		return fmt.Errorf("failed to listen to Crisp events: %w", err)
	}

	// Block until the context is done
	<-c.ctx.Done()

	return nil
}

func (c *CrispBot) handleTextMessage(ctx context.Context, client *crisp.Client, evt crisp.EventsReceiveTextMessage) error {
	if evt.SessionID == nil {
		log.Error().Str("app_id", c.app.ID).Any("event", evt).Msg("crisp session ID is nil")
		return fmt.Errorf("session ID is nil")
	}

	if evt.Content == nil {
		log.Error().Str("app_id", c.app.ID).Any("event", evt).Msg("crisp content is nil")
		return fmt.Errorf("content is nil")
	}

	conversation, _, err := client.Website.GetConversation("ac7f9077-a943-4521-a259-cff0e877a49b", *evt.SessionID)
	if err != nil {
		log.Error().Err(err).Msg("failed to get conversation")
		return fmt.Errorf("failed to get conversation: %w", err)
	}

	spew.Dump(conversation)

	// Check if we have existing crisp thread
	thread, err := c.store.GetCrispThread(ctx, c.app.ID, *evt.SessionID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		log.Error().Err(err).Msg("failed to get crisp thread")
		return fmt.Errorf("failed to get crisp thread: %w", err)
	}

	var session *types.Session

	if thread != nil {
		// Existing thread, load helix session
		session, err = c.store.GetSession(ctx, thread.SessionID)
		if err != nil {
			log.Error().Err(err).Msg("failed to get helix session")
			return fmt.Errorf("failed to get helix session: %w", err)
		}
	} else {
		newSession := shared.NewTriggerSession(ctx, types.TriggerTypeCrisp.String(), c.app)
		session = newSession.Session

		err = c.controller.WriteSession(ctx, session)
		if err != nil {
			log.Error().
				Err(err).
				Str("app_id", c.app.ID).
				Msg("failed to create session")
			return fmt.Errorf("failed to create session: %w", err)
		}

		// Create the new thread
		_, err = c.store.CreateCrispThread(ctx, &types.CrispThread{
			CrispSessionID: *evt.SessionID,
			AppID:          c.app.ID,
			SessionID:      session.ID,
		})
		if err != nil {
			log.Error().Err(err).Msg("failed to create crisp thread")
			return fmt.Errorf("failed to create crisp thread: %w", err)
		}
	}

	// Get user for the request (app owner pays)
	user, err := c.store.GetUser(ctx, &store.GetUserQuery{
		ID: c.app.Owner,
	})
	if err != nil {
		log.Error().Err(err).Msg("failed to get user")
		return fmt.Errorf("failed to get user: %w", err)
	}

	resp, err := c.controller.RunBlockingSession(ctx, &controller.RunSessionRequest{
		OrganizationID: c.app.OrganizationID,
		App:            c.app,
		Session:        session,
		User:           user,
		PromptMessage:  types.MessageContent{Parts: []any{*evt.Content}},
	})
	if err != nil {
		return fmt.Errorf("failed to get response from inference API: %w", err)
	}

	// Send response back to Crisp
	_, _, err = client.Website.SendTextMessageInConversation("ac7f9077-a943-4521-a259-cff0e877a49b", *evt.SessionID, crisp.ConversationTextMessageNew{
		Type:      "text",
		Content:   resp.ResponseMessage,
		Origin:    "chat",
		Automated: ptr.To(true),
		From:      "operator",
		User: crisp.ConversationAllMessageNewUser{
			Nickname: "Helix",
		},
	})
	if err != nil {
		return fmt.Errorf("failed to send message to Crisp: %w", err)
	}

	return nil
}

func (c *CrispBot) handleFileMessage(ctx context.Context, client *crisp.Client, evt crisp.EventsReceiveFileMessage) error {
	return nil
}
