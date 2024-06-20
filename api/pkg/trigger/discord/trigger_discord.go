package discord

import (
	"context"
	"fmt"
	"strings"

	"github.com/helixml/helix/api/pkg/config"
	helixopenai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/bwmarrin/discordgo"
	openai "github.com/lukemarsden/go-openai2"
	"github.com/rs/zerolog/log"
)

type Discord struct {
	cfg    *config.ServerConfig
	store  store.Store
	client helixopenai.Client

	botID string
}

func New(cfg *config.ServerConfig, store store.Store, client helixopenai.Client) *Discord {
	return &Discord{
		cfg:    cfg,
		store:  store,
		client: client,
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

	err = s.Open()
	if err != nil {
		return fmt.Errorf("failed to open discord session: %w", err)
	}
	defer s.Close()

	<-ctx.Done()

	return nil
}

// 2024-06-20T22:59:44Z INF pkg/trigger/discord/trigger_discord.go:98 >
//received message bot_id=1251942355980779531
//content="<@1251942355980779531> how about now" message_author_id=341274053312643073 state_user_id=1251942355980779531 trigger=discord

func (d *Discord) isDirectedAtBot(s *discordgo.Session, m *discordgo.MessageCreate) bool {
	if strings.Contains(m.Content, "<@"+s.State.User.ID+">") {
		fmt.Println("XX bot id")
		return true
	}
	if m.MessageReference != nil && m.MessageReference.MessageID != "" {
		return true
	}

	return false
}

// TODO:
// 1. Look for bot name in the message
// 2. Check the trigger database whether bot is configured, ignore non configured bots
// 3. Start the session with the bot
func (d *Discord) messageHandler(s *discordgo.Session, m *discordgo.MessageCreate) {
	logger := log.With().Str("trigger", "discord").Logger()

	if m.Author.ID == s.State.User.ID {
		return
	}

	logger.Info().
		Str("content", m.Content).
		Str("bot_id", d.botID).
		Str("state_user_id", s.State.User.ID).
		Str("message_author_id", m.Author.ID).
		Msg("received message")

	if !d.isDirectedAtBot(s, m) {
		logger.Debug().Msg("message not directed at bot")
		return
	}

	if ch, err := s.State.Channel(m.ChannelID); err != nil || !ch.IsThread() {
		// Creating a new thread
		thread, err := s.MessageThreadStartComplex(m.ChannelID, m.ID, &discordgo.ThreadStart{
			Name:                "Conversation with " + m.Author.Username,
			AutoArchiveDuration: 60,
			Invitable:           false,
			RateLimitPerUser:    10,
		})
		if err != nil {
			log.Err(err).Msg("failed to create thread")
			return
		}

		logger.Info().Msg("calling llm")

		resp, err := d.starChat(context.Background(), m)
		if err != nil {
			log.Err(err).Msg("failed to get response from inference API")
			return
		}

		_, err = s.ChannelMessageSend(thread.ID, resp)
		if err != nil {
			log.Err(err).Msg("failed to send message")
		}

		m.ChannelID = thread.ID
	} else {
		// Get existing messages from the thread
		fmt.Println("XX thread messages")
		fmt.Println(ch.Messages)

		_, err = s.ChannelMessageSendReply(m.ChannelID, "pong", m.Reference())
		if err != nil {
			log.Err(err).Msg("failed to send message")
		}

	}

}

func (d *Discord) starChat(ctx context.Context, m *discordgo.MessageCreate) (string, error) {
	// TODO: get app configuration from the database
	// to populate rag/tools

	userMessage := openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: m.Content,
	}

	messages := []openai.ChatCompletionMessage{userMessage}

	resp, err := d.client.CreateChatCompletion(
		ctx,
		openai.ChatCompletionRequest{
			Stream:   false,
			Model:    string(types.Model_Ollama_Llama3_8b),
			Messages: messages,
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
