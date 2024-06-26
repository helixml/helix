package discord

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/helixml/helix/api/pkg/config"
	helixopenai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/bwmarrin/discordgo"
	openai "github.com/lukemarsden/go-openai2"
	"github.com/rs/zerolog/log"
)

const (
	// TODO: take from
	discordModel = string(types.Model_Ollama_Llama3_8b)
)

type Discord struct {
	cfg    *config.ServerConfig
	store  store.Store
	client helixopenai.Client

	botID string

	threadsMu sync.Mutex
	threads   map[string]bool
}

func New(cfg *config.ServerConfig, store store.Store, client helixopenai.Client) *Discord {
	return &Discord{
		cfg:     cfg,
		store:   store,
		client:  client,
		threads: make(map[string]bool), // TODO: store this in the database
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

func (d *Discord) isDirectedAtBot(s *discordgo.Session, m *discordgo.MessageCreate) bool {
	if strings.Contains(m.Content, "<@"+s.State.User.ID+">") {
		fmt.Println("XX bot id")
		return true
	}

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

		resp, err := d.starChat(context.Background(), s, []*discordgo.Message{}, m)
		if err != nil {
			log.Err(err).Msg("failed to get response from inference API")
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

	history, err := s.ChannelMessages(m.ChannelID, 10, m.ID, "", "")
	if err != nil {
		// TODO: maybe reply directly?
		log.Err(err).Msg("failed to get messages from thread")
		return
	}

	// Trim the last message as it's the current message
	if len(history) > 0 {
		history = history[:len(history)-1]
	}

	resp, err := d.starChat(context.Background(), s, history, m)
	if err != nil {
		log.Err(err).Msg("failed to get response from inference API")
		return
	}

	_, err = s.ChannelMessageSendReply(m.ChannelID, resp, m.Reference())
	if err != nil {
		log.Err(err).Msg("failed to send message")
	}

}

func (d *Discord) starChat(ctx context.Context, s *discordgo.Session, history []*discordgo.Message, m *discordgo.MessageCreate) (string, error) {
	// TODO: get app configuration from the database
	// to populate rag/tools

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

	resp, err := d.client.CreateChatCompletion(
		ctx,
		openai.ChatCompletionRequest{
			Stream:   false,
			Model:    discordModel,
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

	resp, err := d.client.CreateChatCompletion(
		ctx,
		req,
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
