package discord

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/rs/zerolog/log"

	"github.com/bwmarrin/discordgo"
)

type Discord struct {
	cfg   *config.ServerConfig
	store store.Store
}

func New(cfg *config.ServerConfig, store store.Store) *Discord {
	return &Discord{
		cfg:   cfg,
		store: store,
	}
}

const timeout time.Duration = time.Second * 10

var games map[string]time.Time = make(map[string]time.Time)

func (d *Discord) Start(ctx context.Context) error {
	s, err := discordgo.New("Bot " + d.cfg.Triggers.Discord.BotToken)
	if err != nil {
		return fmt.Errorf("failed to create discord session: %w", err)
	}

	logger := log.With().Str("trigger", "discord").Logger()

	logger.Info().Msg("starting Discord bot")

	// TODO:
	// 1. Look for bot name in the message
	// 2. Check the trigger database whether bot is configured, ignore non configured bots
	// 3. Start the session with the bot

	s.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if m.Author.ID == s.State.User.ID {
			return
		}

		logger.Info().
			Str("content", m.Content).
			Str("bot_id", s.State.User.ID).
			Str("message_author_id", m.Author.ID).
			Msg("received message")

		fmt.Println("XX msg", m.Content)
		spew.Dump(m)
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
	})
	s.Identify.Intents = discordgo.MakeIntent(discordgo.IntentsAllWithoutPrivileged)

	err = s.Open()
	if err != nil {
		return fmt.Errorf("failed to open discord session: %w", err)
	}
	defer s.Close()

	<-ctx.Done()

	return nil
}
