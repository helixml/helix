package slack

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/trigger/shared"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	"github.com/slack-go/slack"
)

// postProjectUpdate - subscribes to project updates and posts them to the Slack channel
func (s *SlackBot) postProjectUpdates(ctx context.Context, app *types.App) error {
	if !s.trigger.ProjectUpdates {
		return nil
	}
	if s.trigger.ProjectChannel == "" {
		return fmt.Errorf("project channel is required when project updates are enabled")
	}
	if len(app.Config.Helix.Assistants) == 0 {
		return fmt.Errorf("no assistants found")
	}
	var projectID string

	// Check if Skill is turned on as well
	assistant := app.Config.Helix.Assistants[0]

	if assistant.ProjectManager.Enabled {
		projectID = assistant.ProjectManager.ProjectID
	}

	if projectID == "" {
		return nil
	}

	// Subscribe to project updates
	sub, err := s.store.SubscribeForTasks(ctx, &store.SpecTaskSubscriptionFilter{
		ProjectID: projectID,
	}, func(task *types.SpecTask) error {
		return s.postProjectUpdate(ctx, task)
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to project updates: %w", err)
	}
	defer sub.Unsubscribe()

	<-ctx.Done()
	return nil
}

func (s *SlackBot) postProjectUpdate(ctx context.Context, task *types.SpecTask) error {
	if task == nil {
		return fmt.Errorf("task is required")
	}
	if s.trigger.ProjectChannel == "" {
		return fmt.Errorf("project channel is required")
	}

	if s.postMessage == nil {
		api := slack.New(
			s.trigger.BotToken,
			slack.OptionDebug(false),
		)
		s.postMessage = api.PostMessage
	}

	session := shared.NewTriggerSession(ctx, types.TriggerTypeSlack.String(), s.app).Session
	session.Name = fmt.Sprintf("project update: %s", task.Name)
	session.Metadata.SpecTaskID = task.ID
	session.Metadata.ProjectID = task.ProjectID
	createdSession, err := s.store.CreateSession(ctx, *session)
	if err != nil {
		return fmt.Errorf("failed to create session for project update: %w", err)
	}

	fallback := fmt.Sprintf("Project update: %s (%s)", task.Name, humanizeSpecTaskStatus(task.Status))
	_, threadKey, err := s.postMessage(
		s.trigger.ProjectChannel,
		slack.MsgOptionBlocks(buildProjectUpdateBlocks(task)...),
		slack.MsgOptionText(fallback, false),
	)
	if err != nil {
		return fmt.Errorf("failed to post project update to Slack: %w", err)
	}

	existingThread, err := s.store.GetSlackThread(ctx, s.app.ID, s.trigger.ProjectChannel, threadKey)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("failed to check existing slack thread: %w", err)
	}
	if existingThread == nil {
		_, err = s.createNewThread(ctx, s.trigger.ProjectChannel, threadKey, createdSession.ID)
		if err != nil {
			return fmt.Errorf("failed to create slack thread for project update: %w", err)
		}
	}

	log.Info().
		Str("app_id", s.app.ID).
		Str("project_id", task.ProjectID).
		Str("spec_task_id", task.ID).
		Str("channel", s.trigger.ProjectChannel).
		Str("thread_key", threadKey).
		Msg("posted project update to Slack")
	return nil
}

func buildProjectUpdateBlocks(task *types.SpecTask) []slack.Block {
	statusEmoji := specTaskStatusEmoji(task.Status)
	headerText := slack.NewTextBlockObject(slack.PlainTextType, fmt.Sprintf("%s Project Update", statusEmoji), false, false)

	statusField := slack.NewTextBlockObject(slack.MarkdownType, fmt.Sprintf("*Status*\n`%s`", humanizeSpecTaskStatus(task.Status)), false, false)
	priorityField := slack.NewTextBlockObject(slack.MarkdownType, fmt.Sprintf("*Priority*\n`%s`", strings.ToUpper(string(task.Priority))), false, false)
	typeField := slack.NewTextBlockObject(slack.MarkdownType, fmt.Sprintf("*Type*\n`%s`", task.Type), false, false)
	taskIDField := slack.NewTextBlockObject(slack.MarkdownType, fmt.Sprintf("*Task ID*\n`%s`", task.ID), false, false)

	title := task.Name
	if title == "" {
		title = task.ShortTitle
	}
	if title == "" {
		title = "Untitled task"
	}

	summaryText := slack.NewTextBlockObject(slack.MarkdownType, fmt.Sprintf("*%s*\n%s", title, truncateForSlack(task.Description, 500)), false, false)
	metadataText := slack.NewTextBlockObject(
		slack.MarkdownType,
		fmt.Sprintf("Project `%s` • Updated %s", task.ProjectID, task.UpdatedAt.UTC().Format(time.RFC822)),
		false,
		false,
	)

	blocks := []slack.Block{
		slack.NewHeaderBlock(headerText),
		slack.NewSectionBlock(summaryText, []*slack.TextBlockObject{statusField, priorityField, typeField, taskIDField}, nil),
		slack.NewContextBlock("project_update_metadata", metadataText),
	}

	return blocks
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

func truncateForSlack(value string, maxLen int) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "_No description provided_"
	}
	if len(trimmed) <= maxLen {
		return trimmed
	}

	return trimmed[:maxLen-3] + "..."
}
