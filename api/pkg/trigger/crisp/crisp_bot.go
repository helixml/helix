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

			// User hits send button (done typing)
			_ = reg.On("message:send/text", func(evt crisp.EventsReceiveTextMessage) {
				err := c.handleTextMessage(c.ctx, client, evt)
				if err != nil {
					log.Error().Err(err).Msg("failed to handle text message")
				}

			})

			_ = reg.On("message:send/file", func(evt crisp.EventsReceiveFileMessage) {
				err := c.handleFileMessage(c.ctx, client, evt)
				if err != nil {
					log.Error().Err(err).Msg("failed to handle file message")
				}
			})

			_ = reg.On("message:send/animation", func(_ crisp.EventsReceiveAnimationMessage) {
				// Nothing to do
			})

			_ = reg.On("message:send/audio", func(_ crisp.EventsReceiveAudioMessage) {
				// Nothing to do
			})

			_ = reg.On("message:send/picker", func(_ crisp.EventsReceivePickerMessage) {
				// Nothing to do
			})

			_ = reg.On("message:send/field", func(_ crisp.EventsReceiveFieldMessage) {
				// Nothing to do
			})

			_ = reg.On("message:send/carousel", func(_ crisp.EventsReceiveCarouselMessage) {
				// Nothing to do
			})

			_ = reg.On("message:send/note", func(_ crisp.EventsReceiveNoteMessage) {
				// Nothing to do
			})

			_ = reg.On("message:send/event", func(_ crisp.EventsReceiveEventMessage) {
				// Nothing to do
			})

			_ = reg.On("message:received/text", func(_ crisp.EventsReceiveTextMessage) {
				// Nothing to do
			})

			_ = reg.On("message:received/file", func(_ crisp.EventsReceiveFileMessage) {
				// Nothing to do
			})

			_ = reg.On("message:received/animation", func(_ crisp.EventsReceiveAnimationMessage) {
				// Nothing to do
			})

			_ = reg.On("message:received/audio", func(_ crisp.EventsReceiveAudioMessage) {
				// Nothing to do
			})

			_ = reg.On("message:received/picker", func(_ crisp.EventsReceivePickerMessage) {
				// Nothing to do
			})

			_ = reg.On("message:received/field", func(_ crisp.EventsReceiveFieldMessage) {
				// Nothing to do
			})

			_ = reg.On("message:received/carousel", func(_ crisp.EventsReceiveCarouselMessage) {
				// Nothing to do
			})

			_ = reg.On("message:received/note", func(_ crisp.EventsReceiveNoteMessage) {
				// Nothing to do
			})

			_ = reg.On("message:received/event", func(_ crisp.EventsReceiveEventMessage) {
				// Nothing to do
			})

			// User typing message
			_ = reg.On("message:compose:send", func(_ crisp.EventsReceiveMessageComposeSend) {
				// Nothing to do
			})

			_ = reg.On("message:compose:receive", func(_ crisp.EventsReceiveMessageComposeReceive) {
				// Nothing to do
			})
		},

		func() {
			c.setStatus(false, "Crisp bot disconnected, reconnecting...")
		},

		func(err error) {
			log.Error().Err(err).Str("app_id", c.app.ID).Msg("Crisp bot websocket error")
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

	if evt.EventsWebsiteGeneric.WebsiteID == nil {
		log.Error().Str("app_id", c.app.ID).Any("event", evt).Msg("crisp website ID is nil")
		return fmt.Errorf("website ID is nil")
	}

	if evt.Content == nil {
		log.Error().Str("app_id", c.app.ID).Any("event", evt).Msg("crisp content is nil")
		return fmt.Errorf("content is nil")
	}

	websiteID := *evt.EventsWebsiteGeneric.WebsiteID

	conversation, _, err := client.Website.GetConversation(websiteID, *evt.SessionID)
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
	err = c.sendMessage(ctx, client, websiteID, *evt.SessionID, resp.ResponseMessage)
	if err != nil {
		return fmt.Errorf("failed to send message to Crisp: %w", err)
	}

	return nil
}

func (c *CrispBot) handleFileMessage(ctx context.Context, client *crisp.Client, evt crisp.EventsReceiveFileMessage) error {
	return nil
}

func (c *CrispBot) sendMessage(_ context.Context, client *crisp.Client, websiteID, sessionID, message string) error {
	nickname := c.trigger.Nickname
	if nickname == "" {
		nickname = "Helix"
	}

	_, _, err := client.Website.SendTextMessageInConversation(websiteID, sessionID, crisp.ConversationTextMessageNew{
		Type:      "text",
		Content:   message,
		Origin:    "chat",
		Automated: ptr.To(true),
		From:      "operator",
		User: crisp.ConversationAllMessageNewUser{
			Nickname: nickname,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to send message to Crisp: %w", err)
	}

	return nil

}
