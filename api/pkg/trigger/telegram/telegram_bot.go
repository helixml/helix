package telegram

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/trigger/shared"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/rs/zerolog/log"
)

// appContext groups an app with its Telegram trigger config.
type appContext struct {
	app     *types.App
	trigger *types.TelegramTrigger
}

// TelegramBot manages a single long-polling connection for one bot token.
// It may serve multiple apps that share the same token.
type TelegramBot struct {
	cfg        *config.ServerConfig
	store      store.Store
	controller *controller.Controller
	token      string

	appsMu sync.RWMutex
	apps   map[string]*appContext // appID -> appContext

	ctx       context.Context
	ctxCancel context.CancelFunc
	botUser   *models.User
	botInst   *bot.Bot
}

func NewTelegramBot(cfg *config.ServerConfig, store store.Store, controller *controller.Controller, token string, apps map[string]*appContext) *TelegramBot {
	return &TelegramBot{
		cfg:        cfg,
		store:      store,
		controller: controller,
		token:      token,
		apps:       apps,
	}
}

// UpdateApps replaces the app list without restarting the bot.
func (t *TelegramBot) UpdateApps(apps map[string]*appContext) {
	t.appsMu.Lock()
	defer t.appsMu.Unlock()
	t.apps = apps
}

func (t *TelegramBot) setStatusForAll(ok bool, message string) {
	t.appsMu.RLock()
	defer t.appsMu.RUnlock()
	for _, ac := range t.apps {
		t.controller.SetTriggerStatus(ac.app.ID, types.TriggerTypeTelegram, types.TriggerStatus{
			Type:    types.TriggerTypeTelegram,
			OK:      ok,
			Message: message,
		})
	}
}

func (t *TelegramBot) Stop() {
	if t.ctxCancel != nil {
		log.Info().Msg("stopping Telegram bot")
		t.ctxCancel()
	}
	t.setStatusForAll(false, "Telegram bot stopped")
}

func (t *TelegramBot) RunBot(ctx context.Context) error {
	defer func() {
		if r := recover(); r != nil {
			log.Error().Msgf("Telegram bot panic: %v", r)
		}
	}()

	t.ctx, t.ctxCancel = context.WithCancel(ctx)

	t.setStatusForAll(false, "Telegram bot connecting")

	opts := []bot.Option{
		bot.WithDefaultHandler(t.handleUpdate),
	}

	b, err := bot.New(t.token, opts...)
	if err != nil {
		t.setStatusForAll(false, fmt.Sprintf("Telegram bot failed to initialize: %v", err))
		return fmt.Errorf("failed to create telegram bot: %w", err)
	}

	t.botInst = b

	botUser, err := b.GetMe(t.ctx)
	if err != nil {
		t.setStatusForAll(false, fmt.Sprintf("Telegram bot failed to get info: %v", err))
		return fmt.Errorf("failed to get bot info: %w", err)
	}

	t.botUser = botUser
	log.Info().
		Str("bot_username", botUser.Username).
		Msg("Telegram bot connected")

	t.setStatusForAll(true, fmt.Sprintf("Connected as @%s", botUser.Username))

	// Start project update subscriptions
	t.startProjectUpdates(t.ctx)

	// Start blocks until context is cancelled
	b.Start(t.ctx)

	return nil
}

// findMatchingApps returns all apps where the given Telegram user ID is allowed.
// If an app has an empty AllowedUsers list, all users are allowed.
func (t *TelegramBot) findMatchingApps(telegramUserID int64) []*appContext {
	t.appsMu.RLock()
	defer t.appsMu.RUnlock()

	var matched []*appContext
	for _, ac := range t.apps {
		if len(ac.trigger.AllowedUsers) == 0 {
			matched = append(matched, ac)
			continue
		}
		for _, uid := range ac.trigger.AllowedUsers {
			if uid == telegramUserID {
				matched = append(matched, ac)
				break
			}
		}
	}
	return matched
}

func (t *TelegramBot) handleUpdate(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	msg := update.Message
	if msg.Text == "" {
		return
	}

	// In group chats, only respond when the bot is mentioned or replied to
	if msg.Chat.Type == "group" || msg.Chat.Type == "supergroup" {
		if !t.isBotMentioned(msg) && !t.isReplyToBot(msg) {
			return
		}
	}

	content := t.cleanMessageText(msg.Text)
	if content == "" {
		return
	}

	userID := int64(0)
	username := ""
	if msg.From != nil {
		userID = msg.From.ID
		username = msg.From.Username
	}

	log.Debug().
		Int64("chat_id", msg.Chat.ID).
		Int64("user_id", userID).
		Str("from", username).
		Msg("handling Telegram message")

	var err error
	switch {
	case content == "/start":
		err = t.handleStartCommand(ctx, b, msg.Chat.ID, userID)
	case content == "/project" || content == "/projects":
		err = t.handleProjectCommand(ctx, b, msg.Chat.ID, userID, "")
	case strings.HasPrefix(content, "/project "):
		projectName := strings.TrimPrefix(content, "/project ")
		err = t.handleProjectCommand(ctx, b, msg.Chat.ID, userID, strings.TrimSpace(projectName))
	case content == "/updates":
		err = t.handleUpdatesCommand(ctx, b, msg.Chat.ID, userID)
	default:
		err = t.handleTextMessage(ctx, b, msg.Chat.ID, userID, content)
	}

	if err != nil {
		log.Error().Err(err).
			Int64("chat_id", msg.Chat.ID).
			Msg("failed to handle Telegram message")
	}
}

func (t *TelegramBot) handleStartCommand(ctx context.Context, b *bot.Bot, chatID int64, telegramUserID int64) error {
	matchedApps := t.findMatchingApps(telegramUserID)
	if len(matchedApps) == 0 {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "You don't have access to any apps through this bot.",
		})
		return err
	}

	text := "Welcome! I'm your Helix AI assistant.\n\n"
	text += "Commands:\n"
	text += "/project - List available projects\n"
	text += "/project <name> - Select a project for this chat\n"
	text += "/updates - Toggle spec task notifications\n\n"
	text += "You can also just send me a message and I'll respond."

	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   text,
	})
	return err
}

func (t *TelegramBot) handleProjectCommand(ctx context.Context, b *bot.Bot, chatID int64, telegramUserID int64, projectName string) error {
	matchedApps := t.findMatchingApps(telegramUserID)
	if len(matchedApps) == 0 {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "You don't have access to any apps through this bot.",
		})
		return err
	}

	// Collect all projects from all matched apps' owners
	type projectInfo struct {
		project *types.Project
		app     *types.App
	}
	var allProjects []projectInfo
	seenProjects := make(map[string]bool)

	for _, ac := range matchedApps {
		projects, err := t.store.ListProjects(ctx, &store.ListProjectsQuery{
			UserID: ac.app.Owner,
		})
		if err != nil {
			log.Error().Err(err).Str("app_id", ac.app.ID).Msg("failed to list projects for app owner")
			continue
		}
		for _, p := range projects {
			if !seenProjects[p.ID] {
				seenProjects[p.ID] = true
				allProjects = append(allProjects, projectInfo{project: p, app: ac.app})
			}
		}
	}

	if projectName == "" {
		// List projects
		if len(allProjects) == 0 {
			_, err := b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: chatID,
				Text:   "No projects available.",
			})
			return err
		}

		// Check current project
		thread, _ := t.store.GetTelegramThreadByChatID(ctx, chatID)
		currentProjectID := ""
		if thread != nil {
			currentProjectID = thread.ProjectID
		}

		text := "Available projects:\n\n"
		for _, pi := range allProjects {
			marker := ""
			if pi.project.ID == currentProjectID {
				marker = " (current)"
			}
			text += fmt.Sprintf("- %s%s\n", pi.project.Name, marker)
		}
		text += "\nUse /project <name> to select one."

		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   text,
		})
		return err
	}

	// Select project by name (case-insensitive prefix match)
	var matched *projectInfo
	projectNameLower := strings.ToLower(projectName)
	for i := range allProjects {
		if strings.ToLower(allProjects[i].project.Name) == projectNameLower {
			matched = &allProjects[i]
			break
		}
	}
	if matched == nil {
		// Try prefix match
		for i := range allProjects {
			if strings.HasPrefix(strings.ToLower(allProjects[i].project.Name), projectNameLower) {
				matched = &allProjects[i]
				break
			}
		}
	}

	if matched == nil {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   fmt.Sprintf("Project %q not found. Use /project to list available projects.", projectName),
		})
		return err
	}

	// Get or create thread for this chat + app
	thread, err := t.store.GetTelegramThread(ctx, matched.app.ID, chatID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("failed to get telegram thread: %w", err)
	}

	if thread != nil {
		thread.ProjectID = matched.project.ID
		_, err = t.store.UpdateTelegramThread(ctx, thread)
		if err != nil {
			return fmt.Errorf("failed to update telegram thread: %w", err)
		}
	} else {
		// Create a new session and thread linked to this project
		newSession := shared.NewTriggerSession(ctx, types.TriggerTypeTelegram.String(), matched.app)
		session := newSession.Session
		session.Name = fmt.Sprintf("telegram: %s", matched.project.Name)

		err = t.controller.WriteSession(ctx, session)
		if err != nil {
			return fmt.Errorf("failed to create session: %w", err)
		}

		_, err = t.store.CreateTelegramThread(ctx, &types.TelegramThread{
			TelegramChatID: chatID,
			AppID:          matched.app.ID,
			SessionID:      session.ID,
			ProjectID:      matched.project.ID,
		})
		if err != nil {
			return fmt.Errorf("failed to create telegram thread: %w", err)
		}
	}

	text := fmt.Sprintf("Project set to: %s\n\nUse /updates to toggle spec task notifications.", matched.project.Name)
	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   text,
	})
	return err
}

func (t *TelegramBot) handleUpdatesCommand(ctx context.Context, b *bot.Bot, chatID int64, telegramUserID int64) error {
	// Find existing thread for this chat
	thread, err := t.store.GetTelegramThreadByChatID(ctx, chatID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			_, err := b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: chatID,
				Text:   "No project linked to this chat. Use /project <name> to select one first.",
			})
			return err
		}
		return fmt.Errorf("failed to get telegram thread: %w", err)
	}

	if thread.ProjectID == "" {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "No project linked to this chat. Use /project <name> to select one first.",
		})
		return err
	}

	// Toggle updates
	thread.Updates = !thread.Updates
	_, err = t.store.UpdateTelegramThread(ctx, thread)
	if err != nil {
		return fmt.Errorf("failed to update telegram thread: %w", err)
	}

	status := "disabled"
	if thread.Updates {
		status = "enabled"
	}

	project, err := t.store.GetProject(ctx, thread.ProjectID)
	projectName := thread.ProjectID
	if err == nil {
		projectName = project.Name
	}

	text := fmt.Sprintf("Spec task notifications %s for project: %s", status, projectName)
	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   text,
	})
	return err
}

func (t *TelegramBot) handleTextMessage(ctx context.Context, b *bot.Bot, chatID int64, telegramUserID int64, content string) error {
	matchedApps := t.findMatchingApps(telegramUserID)
	if len(matchedApps) == 0 {
		_, err := b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   "You don't have access to any apps through this bot.",
		})
		return err
	}

	// Try to find an existing thread for this chat
	thread, err := t.store.GetTelegramThreadByChatID(ctx, chatID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("failed to get telegram thread: %w", err)
	}

	var targetApp *appContext

	if thread != nil {
		// Route to the app the thread belongs to
		t.appsMu.RLock()
		targetApp = t.apps[thread.AppID]
		t.appsMu.RUnlock()
	}

	if targetApp == nil {
		if len(matchedApps) == 1 {
			targetApp = matchedApps[0]
		} else {
			// Multiple apps match and no thread — prompt user
			_, err := b.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: chatID,
				Text:   "Multiple apps are available. Use /project <name> to select one first.",
			})
			return err
		}
	}

	return t.runSession(ctx, b, chatID, targetApp, thread, content)
}

func (t *TelegramBot) runSession(ctx context.Context, b *bot.Bot, chatID int64, ac *appContext, thread *types.TelegramThread, content string) error {
	var session *types.Session
	var err error

	if thread != nil {
		session, err = t.store.GetSession(ctx, thread.SessionID)
		if err != nil {
			return fmt.Errorf("failed to get helix session: %w", err)
		}

		session.GenerationID++
		err = t.controller.WriteSession(ctx, session)
		if err != nil {
			return fmt.Errorf("failed to update session: %w", err)
		}
	} else {
		newSession := shared.NewTriggerSession(ctx, types.TriggerTypeTelegram.String(), ac.app)
		session = newSession.Session

		err = t.controller.WriteSession(ctx, session)
		if err != nil {
			return fmt.Errorf("failed to create session: %w", err)
		}

		_, err = t.store.CreateTelegramThread(ctx, &types.TelegramThread{
			TelegramChatID: chatID,
			AppID:          ac.app.ID,
			SessionID:      session.ID,
		})
		if err != nil {
			return fmt.Errorf("failed to create telegram thread: %w", err)
		}
	}

	user, err := t.store.GetUser(ctx, &store.GetUserQuery{
		ID: ac.app.Owner,
	})
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	interactionID := system.GenerateInteractionID()

	resp, err := t.controller.RunBlockingSession(ctx, &controller.RunSessionRequest{
		OrganizationID: ac.app.OrganizationID,
		App:            ac.app,
		Session:        session,
		User:           user,
		InteractionID:  interactionID,
		PromptMessage:  types.MessageContent{Parts: []any{content}},
		HistoryLimit:   -1,
	})
	if err != nil {
		return fmt.Errorf("failed to get response from inference API: %w", err)
	}

	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    chatID,
		Text:      resp.ResponseMessage,
		ParseMode: models.ParseModeMarkdown,
	})
	if err != nil {
		// Retry without markdown if parsing fails
		_, err = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: chatID,
			Text:   resp.ResponseMessage,
		})
		if err != nil {
			return fmt.Errorf("failed to send telegram message: %w", err)
		}
	}

	return nil
}

func (t *TelegramBot) isBotMentioned(msg *models.Message) bool {
	if t.botUser == nil {
		return false
	}
	mention := "@" + t.botUser.Username
	return strings.Contains(msg.Text, mention)
}

func (t *TelegramBot) isReplyToBot(msg *models.Message) bool {
	if msg.ReplyToMessage == nil || msg.ReplyToMessage.From == nil || t.botUser == nil {
		return false
	}
	return msg.ReplyToMessage.From.ID == t.botUser.ID
}

func (t *TelegramBot) cleanMessageText(text string) string {
	if t.botUser == nil {
		return text
	}
	mention := "@" + t.botUser.Username
	cleaned := strings.ReplaceAll(text, mention, "")
	return strings.TrimSpace(cleaned)
}
