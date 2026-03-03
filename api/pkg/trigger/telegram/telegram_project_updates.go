package telegram

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// startProjectUpdates subscribes to spec task updates for all threads that have updates enabled.
// It runs a subscription per project and sends formatted messages to the relevant Telegram chats.
func (t *TelegramBot) startProjectUpdates(ctx context.Context) {
	// We subscribe once per project to avoid duplicate subscriptions.
	// The reconcile loop in telegram.go periodically calls UpdateApps which
	// could change the thread set, but the subscription is based on store data
	// queried when a task update fires.

	t.appsMu.RLock()
	projectIDs := make(map[string]bool)
	for _, ac := range t.apps {
		for _, assistant := range ac.app.Config.Helix.Assistants {
			if assistant.ProjectManager.Enabled && assistant.ProjectManager.ProjectID != "" {
				projectIDs[assistant.ProjectManager.ProjectID] = true
			}
		}
	}
	t.appsMu.RUnlock()

	for projectID := range projectIDs {
		go t.subscribeProjectUpdates(ctx, projectID)
	}
}

func (t *TelegramBot) subscribeProjectUpdates(ctx context.Context, projectID string) {
	sub, err := t.store.SubscribeForTasks(ctx, &store.SpecTaskSubscriptionFilter{
		ProjectID: projectID,
	}, func(task *types.SpecTask) error {
		return t.handleProjectTaskUpdate(ctx, projectID, task)
	})
	if err != nil {
		log.Error().Err(err).Str("project_id", projectID).Msg("failed to subscribe to project task updates for Telegram")
		return
	}
	defer sub.Unsubscribe()

	<-ctx.Done()
}

func (t *TelegramBot) handleProjectTaskUpdate(ctx context.Context, projectID string, task *types.SpecTask) error {
	if task == nil {
		return nil
	}

	// Get latest task data
	latestTask, err := t.store.GetSpecTask(ctx, task.ID)
	if err != nil {
		return fmt.Errorf("failed to get latest spec task '%s': %w", task.ID, err)
	}
	task = latestTask

	// Find all threads with updates enabled for this project
	threads, err := t.store.ListTelegramThreadsWithUpdates(ctx, projectID)
	if err != nil {
		return fmt.Errorf("failed to list telegram threads with updates: %w", err)
	}

	if len(threads) == 0 {
		return nil
	}

	message := formatTaskUpdate(task)

	for _, thread := range threads {
		_, sendErr := t.botInst.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    thread.TelegramChatID,
			Text:      message,
			ParseMode: "Markdown",
		})
		if sendErr != nil {
			// Retry without markdown
			_, sendErr = t.botInst.SendMessage(ctx, &bot.SendMessageParams{
				ChatID: thread.TelegramChatID,
				Text:   message,
			})
			if sendErr != nil {
				log.Error().Err(sendErr).
					Int64("chat_id", thread.TelegramChatID).
					Str("task_id", task.ID).
					Msg("failed to send project update to Telegram chat")
			}
		}
	}

	return nil
}

func formatTaskUpdate(task *types.SpecTask) string {
	emoji := specTaskStatusEmoji(task.Status)
	status := humanizeSpecTaskStatus(task.Status)

	title := task.Name
	if title == "" {
		title = task.ShortTitle
	}
	if title == "" {
		title = "Untitled task"
	}

	text := fmt.Sprintf("%s *%s*\nStatus: *%s*", emoji, escapeMarkdown(title), status)

	if task.Description != "" {
		desc := task.Description
		if len(desc) > 300 {
			desc = desc[:297] + "..."
		}
		text += "\n\n" + escapeMarkdown(desc)
	}

	return text
}

func specTaskStatusEmoji(status types.SpecTaskStatus) string {
	switch status {
	case types.TaskStatusDone:
		return "✅"
	case types.TaskStatusSpecFailed, types.TaskStatusImplementationFailed:
		return "❌"
	case types.TaskStatusImplementation, types.TaskStatusSpecGeneration:
		return "🚧"
	case types.TaskStatusSpecReview, types.TaskStatusImplementationReview:
		return "👀"
	case types.TaskStatusPullRequest:
		return "🔀"
	default:
		return "📝"
	}
}

func humanizeSpecTaskStatus(status types.SpecTaskStatus) string {
	raw := status.String()
	if raw == "" {
		return "Unknown"
	}

	parts := strings.Split(raw, "_")
	for idx, part := range parts {
		if part == "" {
			continue
		}
		parts[idx] = strings.ToUpper(part[:1]) + part[1:]
	}

	return strings.Join(parts, " ")
}

// escapeMarkdown escapes special Markdown characters for Telegram
func escapeMarkdown(s string) string {
	replacer := strings.NewReplacer(
		"*", "\\*",
		"_", "\\_",
		"`", "\\`",
		"[", "\\[",
	)
	return replacer.Replace(s)
}
