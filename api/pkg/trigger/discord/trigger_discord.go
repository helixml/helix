package discord

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/rs/zerolog/log"

	"github.com/bwmarrin/discordgo"
)

type Discord struct {
	cfg    *config.ServerConfig
	store  store.Store
	client openai.Client

	botID string
}

func New(cfg *config.ServerConfig, store store.Store, client openai.Client) *Discord {
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

func (d *Discord) isDirectedAtBot(m *discordgo.MessageCreate) bool {
	if strings.Contains(m.Content, "<@!"+d.botID+">") {
		return true
	}
	if m.MessageReference != nil && m.MessageReference.MessageID != "" {
		return true
	}

	return false
}

const timeout time.Duration = time.Second * 10

var games map[string]time.Time = make(map[string]time.Time)

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
		Str("bot_id", s.State.User.ID).
		Str("message_author_id", m.Author.ID).
		Msg("received message")

	if !d.isDirectedAtBot(m) {
		logger.Debug().Msg("message not directed at bot")
		return
	}

	if strings.Contains(m.Content, "ping") {
		if ch, err := s.State.Channel(m.ChannelID); err != nil || !ch.IsThread() {
			thread, err := s.MessageThreadStartComplex(m.ChannelID, m.ID, &discordgo.ThreadStart{
				Name:                "Pong game with " + m.Author.Username,
				AutoArchiveDuration: 60,
				Invitable:           false,
				RateLimitPerUser:    10,
			})
			if err != nil {
				log.Err(err).Msg("failed to create thread")
				return
			}
			_, err = s.ChannelMessageSend(thread.ID, "pong")
			if err != nil {
				log.Err(err).Msg("failed to send message")
			}

			m.ChannelID = thread.ID
		} else {
			_, err = s.ChannelMessageSendReply(m.ChannelID, "pong", m.Reference())
			if err != nil {
				log.Err(err).Msg("failed to send message")
			}

		}
		games[m.ChannelID] = time.Now()
		<-time.After(timeout)
		if time.Since(games[m.ChannelID]) >= timeout {
			archived := true
			locked := true
			_, err := s.ChannelEditComplex(m.ChannelID, &discordgo.ChannelEdit{
				Archived: &archived,
				Locked:   &locked,
			})
			if err != nil {
				log.Err(err).Msg("failed to archive channel")
			}
		}
	}
}

func (d *Discord) startSession(m *discordgo.MessageCreate) {
	// TODO: get app configuration from the database
	// to populate rag/tools

}
