package slack

import (
	"context"
	"fmt"
	"io"
	stdlog "log"
	"net/http"
	"regexp"
	"strings"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/trigger/shared"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
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
		// Convert error message to Slack format
		slackFormattedError := convertMarkdownToSlackFormat(err.Error())
		_, _, _ = client.Client.PostMessage(ev.Channel, slack.MsgOptionText(slackFormattedError, false), slack.MsgOptionTS(ev.TimeStamp))
		return
	}

	// Convert markdown to Slack format
	slackFormattedResponse := convertMarkdownToSlackFormat(agentResponse)

	// Write agent response to Slack's thread
	// Use the message timestamp as the thread timestamp to create a proper thread
	log.Debug().
		Str("app_id", s.app.ID).
		Str("channel", ev.Channel).
		Str("thread_timestamp", ev.TimeStamp).
		Str("response_length", fmt.Sprintf("%d", len(slackFormattedResponse))).
		Msg("Posting bot response in thread")

	_, _, err = client.Client.PostMessage(ev.Channel, slack.MsgOptionText(slackFormattedResponse, false), slack.MsgOptionTS(ev.TimeStamp))
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

	log.Debug().
		Str("app_id", s.app.ID).
		Str("channel", ev.Channel).
		Str("thread", threadKey).
		Str("user", ev.User).
		Msg("Received message in active thread")

	agentResponse, err := s.handleMessage(context.Background(), thread, s.app, ev.Text, ev.Channel, ev.TimeStamp, ev.ThreadTimeStamp, false)
	if err != nil {
		log.Error().Err(err).Msg("failed to continue chat")
		// Convert error message to Slack format
		slackFormattedError := convertMarkdownToSlackFormat(err.Error())
		_, _, _ = client.Client.PostMessage(ev.Channel, slack.MsgOptionText(slackFormattedError, false), slack.MsgOptionTS(ev.ThreadTimeStamp))
		return
	}

	// Convert markdown to Slack format
	slackFormattedResponse := convertMarkdownToSlackFormat(agentResponse)

	// Write agent response to Slack's thread
	// Use the thread timestamp to keep the reply in the same thread
	log.Debug().
		Str("app_id", s.app.ID).
		Str("channel", ev.Channel).
		Str("thread_timestamp", ev.ThreadTimeStamp).
		Str("response_length", fmt.Sprintf("%d", len(slackFormattedResponse))).
		Msg("Posting bot response to thread")

	_, _, err = client.Client.PostMessage(ev.Channel, slack.MsgOptionText(slackFormattedResponse, false), slack.MsgOptionTS(ev.ThreadTimeStamp))
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

		newSession := shared.NewTriggerSession(ctx, types.TriggerTypeSlack.String(), app)
		session = newSession.Session

		threadKey := threadTimestamp
		if threadKey == "" {
			threadKey = messageTimestamp
		}

		// Create the new thread
		var err error
		_, err = s.createNewThread(ctx, channel, threadKey, session.ID)
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

	resp, err := s.controller.RunBlockingSession(ctx, &controller.RunSessionRequest{
		OrganizationID: app.OrganizationID,
		App:            app,
		Session:        session,
		User:           user,
		PromptMessage:  types.MessageContent{Parts: []any{messageText}},
	})
	if err != nil {
		return "", fmt.Errorf("failed to get response from inference API: %w", err)
	}

	return resp.ResponseMessage, nil
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

func convertMarkdownToSlackFormat(markdown string) string {
	// Convert markdown to Slack format
	slackFormat := markdown

	// Convert [DOC_ID:xxx] markers to numbered citations [1], [2], etc.
	// Slack can't link to internal documents, so we just show citation numbers
	slackFormat = shared.ConvertDocIDsToNumberedCitations(slackFormat)

	// First, let's protect code blocks and inline code from other conversions
	codeBlocks := []string{}
	inlineCodes := []string{}

	// Extract code blocks
	codeBlockRegex := regexp.MustCompile("```(\\w*)\\n([\\s\\S]*?)```")
	slackFormat = codeBlockRegex.ReplaceAllStringFunc(slackFormat, func(match string) string {
		codeBlocks = append(codeBlocks, match)
		return fmt.Sprintf("__CODE_BLOCK_%d__", len(codeBlocks)-1)
	})

	// Extract inline code
	inlineCodeRegex := regexp.MustCompile("`([^`]+)`")
	slackFormat = inlineCodeRegex.ReplaceAllStringFunc(slackFormat, func(match string) string {
		inlineCodes = append(inlineCodes, match)
		return fmt.Sprintf("__INLINE_CODE_%d__", len(inlineCodes)-1)
	})

	// Convert lists: - item or * item -> • item
	listItemRegex := regexp.MustCompile(`^[\s]*[-*][\s]+`)
	lines := strings.Split(slackFormat, "\n")
	for i, line := range lines {
		if listItemRegex.MatchString(line) {
			lines[i] = listItemRegex.ReplaceAllString(line, "• ")
		}
	}
	slackFormat = strings.Join(lines, "\n")

	// Convert bold and italic using a more sophisticated approach
	slackFormat = convertBoldAndItalic(slackFormat)

	// Convert links: [text](url) -> <url|text>
	linkRegex := regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	slackFormat = linkRegex.ReplaceAllString(slackFormat, "<$2|$1>")

	// Convert strikethrough: ~~text~~ -> ~text~
	strikethroughRegex := regexp.MustCompile(`~~(.*?)~~`)
	slackFormat = strikethroughRegex.ReplaceAllString(slackFormat, "~$1~")

	// Restore code blocks
	for i, codeBlock := range codeBlocks {
		placeholder := fmt.Sprintf("__CODE_BLOCK_%d__", i)
		slackFormat = strings.Replace(slackFormat, placeholder, codeBlock, 1)
	}

	// Restore inline codes
	for i, inlineCode := range inlineCodes {
		placeholder := fmt.Sprintf("__INLINE_CODE_%d__", i)
		slackFormat = strings.Replace(slackFormat, placeholder, inlineCode, 1)
	}

	return slackFormat
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
