package discord

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/bwmarrin/discordgo"
	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
)

const (
	// TODO: take from assistant
	discordModel = string("meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo")
	// Users will be redirected to this URL to install the bot
	installationDocsURL = "https://docs.helixml.tech/helix/"
	// history limit
	historyLimit = 30
)

type Discord struct {
	cfg        *config.ServerConfig
	store      store.Store
	controller *controller.Controller

	botID string

	threadsMu sync.Mutex
	threads   map[string]bool

	appsMu sync.Mutex
	apps   map[string]*types.App // Guild name -> App
}

func New(cfg *config.ServerConfig, store store.Store, controller *controller.Controller) *Discord {
	return &Discord{
		cfg:        cfg,
		store:      store,
		controller: controller,
		threads:    make(map[string]bool), // TODO: store this in the database
	}
}

func (d *Discord) Start(ctx context.Context) error {
	s, err := discordgo.New("Bot " + d.cfg.Triggers.Discord.BotToken)
	if err != nil {
		return fmt.Errorf("failed to create discord session: %w", err)
	}

	u, err := s.User("@me")
	if err != nil {
		return fmt.Errorf("error obtaining account details: %w", err)
	}
	d.botID = u.ID

	logger := log.With().Str("trigger", "discord").Logger()

	logger.Info().Msg("starting Discord bot")

	s.AddHandler(d.messageHandler)

	s.Identify.Intents = discordgo.MakeIntent(discordgo.IntentsAllWithoutPrivileged)

	// Start the app sync loop, this will load the apps and keep them in sync
	// and make sure we know which apps are configured for which guilds (discord servers)
	go func() {
		if err := d.syncApps(ctx); err != nil {
			logger.Error().Msgf("app sync fool exited with error: %v", err)
		}
	}()

	err = s.Open()
	if err != nil {
		return fmt.Errorf("failed to open discord session: %w", err)
	}
	defer s.Close()

	<-ctx.Done()

	return nil
}

func (d *Discord) syncApps(ctx context.Context) error {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := d.syncAppsOnce(ctx); err != nil {
				return fmt.Errorf("failed syncing app: %v", err)
			}
		}
	}
}

func (d *Discord) syncAppsOnce(ctx context.Context) error {
	// Load all apps, check for discord configuration
	apps, err := d.store.ListApps(ctx, &store.ListAppsQuery{})
	if err != nil {
		return fmt.Errorf("failed to list apps: %w", err)
	}

	discordApps := make(map[string]*types.App)

	for _, app := range apps {
		for _, trigger := range app.Config.Helix.Triggers {
			if trigger.Discord != nil {
				discordApps[trigger.Discord.ServerName] = app
			}
		}
	}

	d.appsMu.Lock()
	d.apps = discordApps
	d.appsMu.Unlock()

	return nil
}

func (d *Discord) isDirectedAtBot(s *discordgo.Session, m *discordgo.MessageCreate) bool {
	if strings.Contains(m.Content, "<@"+s.State.User.ID+">") {
		return true
	}

	// TODO: remove
	log.Info().Str("content", m.Content).
		Msg("message is directed at bot")

	d.threadsMu.Lock()
	defer d.threadsMu.Unlock()

	if _, ok := d.threads[m.ChannelID]; ok {
		return true
	}

	if m.MessageReference != nil && m.MessageReference.MessageID != "" {
		return true
	}

	return false
}

func (d *Discord) messageHandler(s *discordgo.Session, m *discordgo.MessageCreate) {
	logger := log.With().Str("trigger", "discord").Logger()
	if m.Author.ID == s.State.User.ID {
		return
	}

	guild, err := s.Guild(m.GuildID)
	if err != nil || guild == nil {
		log.
			Err(err).
			Str("guild_id", m.GuildID).
			Msg("failed to get guild")

		_, err = s.ChannelMessageSendReply(m.ChannelID, "Failed to get guild, maybe I am lacking permissions?", m.Reference())
		if err != nil {
			log.Err(err).Msg("failed to send message")
		}
		return
	}

	d.appsMu.Lock()
	app, ok := d.apps[guild.Name]
	if !ok {
		d.appsMu.Unlock()
		log.Warn().Str("guild_name", guild.Name).Msg("no app configured for guild")

		_, err = s.ChannelMessageSendReply(
			m.ChannelID,
			fmt.Sprintf("I am not yet configured to respond in this Discord channel. Please visit %s to install me.", installationDocsURL),
			m.Reference(),
		)
		if err != nil {
			log.Err(err).Msg("failed to send message")
		}
		return
	}
	d.appsMu.Unlock()

	logger.Info().
		Str("content", m.Content).
		Str("app_id", app.ID).
		Str("bot_id", d.botID).
		Str("state_user_id", s.State.User.ID).
		Str("message_author_id", m.Author.ID).
		Str("guild_id", m.GuildID).
		Str("guild_name", guild.Name).
		Msg("received message")

	if !d.isDirectedAtBot(s, m) {
		logger.Info().Msg("message not directed at bot")
		return
	}

	if ch, err := s.State.Channel(m.ChannelID); err != nil || !ch.IsThread() {
		// Creating a new thread
		threadName, err := d.getThreadName(context.Background(), m)
		if err != nil {
			log.Err(err).Msg("failed to get thread name")
			return
		}

		thread, err := s.MessageThreadStartComplex(m.ChannelID, m.ID, &discordgo.ThreadStart{
			Name:                threadName,
			AutoArchiveDuration: 60,
			Invitable:           false,
			RateLimitPerUser:    10,
		})
		if err != nil {
			log.Err(err).Msg("failed to create thread")
			return
		}

		resp, err := d.startChat(context.Background(), app, s, []*discordgo.Message{}, m)
		if err != nil {
			log.Err(err).Msg("failed to get response from inference API")
			_, _ = s.ChannelMessageSend(thread.ID, fmt.Sprintf("Failed to get response: %s", err))
			return
		}

		_, err = s.ChannelMessageSend(thread.ID, resp)
		if err != nil {
			log.Err(err).Msg("failed to send message")
		}

		m.ChannelID = thread.ID

		d.threadsMu.Lock()
		d.threads[thread.ID] = true
		d.threadsMu.Unlock()
		return
	}

	// Existing thread

	history, err := s.ChannelMessages(m.ChannelID, historyLimit, m.ID, "", "")
	if err != nil {
		// TODO: maybe reply directly?
		log.Err(err).Msg("failed to get messages from thread")
		return
	}

	// Trim the last message as it's the current message
	if len(history) > 0 {
		history = history[:len(history)-1]
	}

	resp, err := d.startChat(context.Background(), app, s, history, m)
	if err != nil {
		log.Err(err).Msg("failed to get response from inference API")
		return
	}

	_, err = s.ChannelMessageSendReply(m.ChannelID, resp, m.Reference())
	if err != nil {
		log.Err(err).Msg("failed to send message")
	}

}

func (d *Discord) startChat(ctx context.Context, app *types.App, s *discordgo.Session, history []*discordgo.Message, m *discordgo.MessageCreate) (string, error) {
	system := openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleSystem,
		Content: `You are an AI assistant Discord bot. Be concise with the replies, keep them short but informative.`,
	}

	messages := []openai.ChatCompletionMessage{
		system,
	}

	for _, msg := range history {
		switch {
		case msg.Author.ID == s.State.User.ID:
			messages = append(messages, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleAssistant,
				Content: msg.Content,
			})
		default:
			messages = append(messages, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleUser,
				Content: msg.Content,
			})
		}
	}

	userMessage := openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: m.Content,
	}

	messages = append(messages, userMessage)

	resp, _, err := d.controller.ChatCompletion(
		ctx,
		&types.User{},
		openai.ChatCompletionRequest{
			Stream:   false,
			Model:    discordModel,
			Messages: messages,
		},
		&controller.ChatCompletionOptions{
			OrganizationID: app.OrganizationID,
			AppID:          app.ID,
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

func (d *Discord) getThreadName(ctx context.Context, m *discordgo.MessageCreate) (string, error) {
	req := openai.ChatCompletionRequest{
		Model:     discordModel,
		MaxTokens: int(50),
		Messages: []openai.ChatCompletionMessage{
			{
				Role: "user",
				Content: titleGenPrompt +
					m.Content,
			},
		},
		Stream: false,
	}

	resp, _, err := d.controller.ChatCompletion(
		ctx,
		&types.User{},
		req,
		&controller.ChatCompletionOptions{},
	)
	if err != nil {
		return "", fmt.Errorf("failed to get response from inference API: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	return fmt.Sprintf("Conversation with %s about %s", m.Author.Username, resp.Choices[0].Message.Content), nil
}

const titleGenPrompt = `You are an AI tool that writes concise, 3-5 word titles for the following query, strictly adhering to the 3-5 word limit and avoiding the use of the word "title".
Examples:

**User Input:** tell me about roman empire's early days and how it was formed
**Title:** Roman empire's early days

Another example:

**User Input:** what is the best way to cook a steak
**Title:** Cooking the perfect steak

===END EXAMPLES===

Based on the above, reply with those 3-5 words, nothing other commentary. Here is the user input/questions:`
