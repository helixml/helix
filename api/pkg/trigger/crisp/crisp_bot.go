package crisp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/crisp-im/go-crisp-api/crisp/v3"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/ptr"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/trigger/shared"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/rs/zerolog/log"
)

// To get the token:
// curl -H "Content-Type: application/json" -X POST -d '{"email":"email@example.com","password":"your password"}' https://api.crisp.chat/v1/user/session/login

func NewCrispBot(cfg *config.ServerConfig, store store.Store, controller *controller.Controller, app *types.App, trigger *types.CrispTrigger) *CrispBot {

	if trigger.Nickname == "" {
		trigger.Nickname = "Helix"
	}

	return &CrispBot{
		cfg:        cfg,
		store:      store,
		controller: controller,
		app:        app,
		trigger:    trigger,
	}
}

type CrispBot struct { //nolint:revive
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
		},

		func(reg *crisp.EventsRegister) {
			c.setStatus(true, "Crisp bot connected")

			// User hits send button (done typing)
			_ = reg.On("message:send/text", func(evt crisp.EventsReceiveTextMessage) {
				if evt.User == nil {
					log.Warn().Str("app_id", c.app.ID).Any("event", evt).Msg("user is nil")
					return
				}
				if evt.SessionID == nil {
					log.Warn().Str("app_id", c.app.ID).Any("event", evt).Msg("session ID is nil")
					return
				}

				if evt.WebsiteID == nil {
					log.Warn().Str("app_id", c.app.ID).Any("event", evt).Msg("website ID is nil")
					return
				}

				err := c.handleTextMessage(c.ctx, client, *evt.WebsiteID, *evt.SessionID, messageSourceUser, uint(*evt.Timestamp), *evt.Content)
				if err != nil {
					log.Error().Err(err).Msg("failed to handle text message")
				}
			})

			// When either bot or operator sends a message
			_ = reg.On("message:received/text", func(evt crisp.EventsReceiveTextMessage) {
				if evt.User == nil {
					log.Info().Str("app_id", c.app.ID).Msg("user is nil")
					return
				}

				if evt.Automated != nil && *evt.Automated {
					log.Info().Str("app_id", c.app.ID).Msg("do not reply to automated messages")
					return // Do not reply to automated messages
				}

				if *evt.User.Nickname == c.trigger.Nickname {
					log.Info().Str("app_id", c.app.ID).Msg("do not reply to own messages")
					return // Do not reply to own messages
				}

				if evt.SessionID == nil {
					return
				}
				if evt.WebsiteID == nil {
					return
				}

				// If the message is not directed to the bot, ignore it
				if !c.isMessageDirectedToBot(*evt.Content) {
					log.Info().
						Str("content", *evt.Content).
						Str("bot_nickname", c.trigger.Nickname).
						Str("app_id", c.app.ID).Msg("message is not directed to bot")
					return
				}

				err := c.handleTextMessage(c.ctx, client, *evt.WebsiteID, *evt.SessionID, messageSourceOperator, uint(*evt.Timestamp), *evt.Content)
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

type messageSource int

const (
	messageSourceOperator messageSource = iota
	messageSourceUser
)

func (c *CrispBot) handleTextMessage(ctx context.Context, client *crisp.Client, websiteID, crispSessionID string, source messageSource, messageTimestamp uint, content string) error {
	if crispSessionID == "" {
		log.Error().Str("app_id", c.app.ID).Str("crisp_session_id", crispSessionID).Msg("crisp session ID is empty")
		return fmt.Errorf("session ID is nil")
	}

	if websiteID == "" {
		log.Error().Str("app_id", c.app.ID).Str("website_id", websiteID).Msg("crisp website ID is empty")
		return fmt.Errorf("website ID is nil")
	}

	if content == "" {
		log.Error().Str("app_id", c.app.ID).Str("content", content).Msg("crisp content is empty")
		return fmt.Errorf("content is nil")
	}

	log.Debug().
		Str("app_id", c.app.ID).
		Str("session_id", crispSessionID).
		Msg("handleTextMessage")

	messages, _, err := client.Website.GetMessagesInConversationBefore(websiteID, crispSessionID, messageTimestamp)
	if err != nil {
		log.Error().Err(err).Msg("failed to get message")
		return fmt.Errorf("failed to get message: %w", err)
	}

	if isInstructedToStop(c.trigger.Nickname, *messages) {
		log.Info().Str("app_id", c.app.ID).Msg("bot is instructed to stop")
		return nil
	}

	// If user is replying to human operator, ignore it
	if source == messageSourceUser && isLastOperatorMessageHuman(c.trigger.Nickname, *messages) {
		log.Info().Str("app_id", c.app.ID).Msg("last message is from the human operator")
		return nil
	}

	// Check if we have existing crisp thread
	thread, err := c.store.GetCrispThread(ctx, c.app.ID, crispSessionID)
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

		// Always reset generation ID
		session.GenerationID++

		err = c.controller.WriteSession(ctx, session)
		if err != nil {
			log.Error().
				Err(err).
				Str("app_id", c.app.ID).
				Msg("failed to update session")
			return fmt.Errorf("failed to update session: %w", err)
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
			CrispSessionID: crispSessionID,
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

	interactionID := system.GenerateInteractionID()

	promptMessage := content

	if len(*messages) > 0 {
		summary, err := c.summarizeConversation(user, session, interactionID, *messages)
		if err != nil {
			log.Error().Err(err).Msg("failed to summarize conversation")
			return fmt.Errorf("failed to summarize conversation: %w", err)
		}

		// Reconstruct the prompt to include the summary
		promptMessage = fmt.Sprintf(`Here's a summary of the conversation so far: %s\n\nUser message:%s`, summary, content)
	}

	resp, err := c.controller.RunBlockingSession(ctx, &controller.RunSessionRequest{
		OrganizationID: c.app.OrganizationID,
		App:            c.app,
		Session:        session,
		User:           user,
		InteractionID:  interactionID,
		PromptMessage:  types.MessageContent{Parts: []any{promptMessage}},
		HistoryLimit:   -1, // Do not include any interactions
	})
	if err != nil {
		return fmt.Errorf("failed to get response from inference API: %w", err)
	}

	// Send response back to Crisp
	err = c.sendMessage(ctx, client, websiteID, crispSessionID, resp.ResponseMessage)
	if err != nil {
		return fmt.Errorf("failed to send message to Crisp: %w", err)
	}

	return nil
}

func (c *CrispBot) handleFileMessage(_ context.Context, _ *crisp.Client, _ crisp.EventsReceiveFileMessage) error {
	// TODO: handle images or just ignore it but include in the next prompt
	return nil
}

func (c *CrispBot) sendMessage(_ context.Context, client *crisp.Client, websiteID, sessionID, message string) error {
	_, _, err := client.Website.SendTextMessageInConversation(websiteID, sessionID, crisp.ConversationTextMessageNew{
		Type:      "text",
		Content:   message,
		Origin:    "chat",
		Automated: ptr.To(true),
		From:      "operator",
		User: crisp.ConversationAllMessageNewUser{
			Nickname: c.trigger.Nickname,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to send message to Crisp: %w", err)
	}

	return nil
}

// We are looking for "Hey <bot_nickname>"
func (c *CrispBot) isMessageDirectedToBot(message string) bool {
	return isMessageDirectedToBot(c.trigger.Nickname, message)
}

func isMessageDirectedToBot(nickName string, message string) bool {
	return strings.Contains(strings.ToLower(message), "hey "+strings.ToLower(nickName))
}

// isInstructedToStop checks whether there's a message from today that says
// "<bot_nickname> stop" and there is no "<bot_nickname> continue" message after it
func isInstructedToStop(nickName string, messages []crisp.ConversationMessage) bool {
	var lastInstruction string

	for _, message := range messages {
		if ptr.From(message.Type) != "text" {
			continue
		}

		content, ok := ptr.From(message.Content).(string)
		if !ok {
			continue
		}

		if ptr.From(message.From) != "operator" {
			continue
		}

		stopCmd := nickName + " stop"
		continueCmd := nickName + " continue"

		if strings.Contains(content, stopCmd) {
			lastInstruction = "stop"
		} else if strings.Contains(content, continueCmd) {
			lastInstruction = "continue"
		}
	}

	return lastInstruction == "stop"
}

// isLastOperatorMessageHuman handles cases where there's a conversation between
// human operator and the customer. We don't want to reply to the customer if the last message
// is from the human operator. To achieve that we
func isLastOperatorMessageHuman(nickName string, messages []crisp.ConversationMessage) bool {
	for i := len(messages) - 1; i >= 0; i-- {
		message := messages[i]

		if ptr.From(message.From) != "operator" {
			continue
		}

		if message.User == nil || message.User.Nickname == nil {
			continue
		}

		if *message.User.Nickname != nickName {
			return true
		}

		return false
	}

	return false
}
