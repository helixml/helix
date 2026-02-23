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

// postProjectUpdates - subscribes to project updates and posts them to the Slack channel
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
	for _, assistant := range app.Config.Helix.Assistants {
		if assistant.ProjectManager.Enabled && assistant.ProjectManager.ProjectID != "" {
			projectID = assistant.ProjectManager.ProjectID
			break
		}
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
	if task.ID == "" {
		return fmt.Errorf("task ID is required")
	}

	latestTask, err := s.store.GetSpecTask(ctx, task.ID)
	if err != nil {
		return fmt.Errorf("failed to get latest spec task '%s': %w", task.ID, err)
	}
	task = latestTask

	if s.postMessage == nil || s.updateMessage == nil {
		api := slack.New(
			s.trigger.BotToken,
			slack.OptionDebug(false),
		)
		s.postMessage = api.PostMessage
		s.updateMessage = api.UpdateMessage
		s.getConversationReplies = api.GetConversationReplies
	}

	existingThread, err := s.store.GetSlackThreadBySpecTaskID(ctx, s.app.ID, task.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("failed to look up existing thread for spec task: %w", err)
	}

	if existingThread != nil {
		// Post update as a thread reply
		return s.postProjectUpdateReply(ctx, existingThread, task)
	}

	// First update for this task: post as top-level message
	return s.postProjectUpdateNew(ctx, task)
}

// postProjectUpdateNew posts the first message for a spec task and creates a thread record
func (s *SlackBot) postProjectUpdateNew(ctx context.Context, task *types.SpecTask) error {
	session := shared.NewTriggerSession(ctx, types.TriggerTypeSlack.String(), s.app).Session
	session.Name = fmt.Sprintf("project update: %s", task.Name)
	session.Metadata.SpecTaskID = task.ID
	session.Metadata.ProjectID = task.ProjectID
	createdSession, err := s.store.CreateSession(ctx, *session)
	if err != nil {
		return fmt.Errorf("failed to create session for project update: %w", err)
	}

	attachment := s.buildProjectUpdateAttachment(ctx, task, s.cfg.Notifications.AppURL)
	fallback := fmt.Sprintf("Project update: %s (%s)", task.Name, humanizeSpecTaskStatus(task.Status))

	channelID, threadKey, err := s.postMessage(
		s.trigger.ProjectChannel,
		slack.MsgOptionAttachments(attachment),
		slack.MsgOptionText(fallback, false),
	)
	if err != nil {
		return fmt.Errorf("failed to post project update to Slack: %w", err)
	}

	_, err = s.store.CreateSlackThread(ctx, &types.SlackThread{
		ThreadKey:  threadKey,
		AppID:      s.app.ID,
		Channel:    channelID,
		SessionID:  createdSession.ID,
		SpecTaskID: task.ID,
	})
	if err != nil {
		return fmt.Errorf("failed to create slack thread for project update: %w", err)
	}

	log.Info().
		Str("app_id", s.app.ID).
		Str("project_id", task.ProjectID).
		Str("spec_task_id", task.ID).
		Str("channel", s.trigger.ProjectChannel).
		Str("thread_key", threadKey).
		Msg("posted new project update to Slack")
	return nil
}

// postProjectUpdateReply posts a status update as a thread reply to an existing project update message
func (s *SlackBot) postProjectUpdateReply(ctx context.Context, thread *types.SlackThread, task *types.SpecTask) error {
	attachment := s.buildProjectUpdateReplyAttachment(ctx, task, s.cfg.Notifications.AppURL)
	fallback := fmt.Sprintf("Status update: %s → %s", task.Name, humanizeSpecTaskStatus(task.Status))

	// postMessage accepts both channel names and IDs and returns the resolved
	// channel ID. We use that resolved ID for updateMessage which requires a
	// real channel ID (older thread records may store the channel name).
	resolvedChannelID := thread.Channel

	alreadyPosted, err := s.hasProjectUpdateReply(ctx, thread, fallback)
	if err != nil {
		log.Warn().
			Err(err).
			Str("app_id", s.app.ID).
			Str("spec_task_id", task.ID).
			Str("channel", thread.Channel).
			Str("thread_key", thread.ThreadKey).
			Msg("failed to check existing project update replies in Slack, continuing without duplicate guard")
		alreadyPosted = false
	}

	if !alreadyPosted {
		channelID, _, postErr := s.postMessage(
			thread.Channel,
			slack.MsgOptionAttachments(attachment),
			slack.MsgOptionText(fallback, false),
			slack.MsgOptionTS(thread.ThreadKey),
		)
		if postErr != nil {
			return fmt.Errorf("failed to post project update reply to Slack: %w", postErr)
		}
		if channelID != "" {
			resolvedChannelID = channelID
		}
	} else {
		log.Info().
			Str("app_id", s.app.ID).
			Str("spec_task_id", task.ID).
			Str("channel", thread.Channel).
			Str("thread_key", thread.ThreadKey).
			Str("status", string(task.Status)).
			Msg("skipping duplicate project update reply in Slack thread")
	}

	if err := s.updateProjectUpdateFirstMessage(ctx, resolvedChannelID, thread.ThreadKey, task); err != nil {
		log.Error().
			Err(err).
			Str("app_id", s.app.ID).
			Str("spec_task_id", task.ID).
			Str("channel", resolvedChannelID).
			Str("thread_key", thread.ThreadKey).
			Msg("failed to update first project update message in Slack")
	}

	log.Info().
		Str("app_id", s.app.ID).
		Str("project_id", task.ProjectID).
		Str("spec_task_id", task.ID).
		Str("channel", resolvedChannelID).
		Str("thread_key", thread.ThreadKey).
		Str("status", string(task.Status)).
		Msg("posted project update reply to Slack thread")
	return nil
}

func (s *SlackBot) hasProjectUpdateReply(ctx context.Context, thread *types.SlackThread, fallback string) (bool, error) {
	if s.getConversationReplies == nil {
		return false, nil
	}

	replies, err := s.getSlackThreadMessages(thread.Channel, thread.ThreadKey)
	if err != nil {
		return false, err
	}

	for _, reply := range replies {
		if reply.Timestamp == thread.ThreadKey {
			continue
		}
		if reply.Text == fallback {
			return true, nil
		}
	}

	return false, nil
}

func (s *SlackBot) updateProjectUpdateFirstMessage(ctx context.Context, channelID, threadKey string, task *types.SpecTask) error {
	attachment := s.buildProjectUpdateAttachment(ctx, task, s.cfg.Notifications.AppURL)
	fallback := fmt.Sprintf("Project update: %s (%s)", task.Name, humanizeSpecTaskStatus(task.Status))

	_, _, _, err := s.updateMessage(
		channelID,
		threadKey,
		slack.MsgOptionAttachments(attachment),
		slack.MsgOptionText(fallback, false),
	)
	if err != nil {
		return fmt.Errorf("failed to update project update message in Slack: %w", err)
	}

	return nil
}

// buildProjectUpdateAttachment creates a colored Slack attachment for the initial project update
func (s *SlackBot) buildProjectUpdateAttachment(ctx context.Context, task *types.SpecTask, appURL string) slack.Attachment {
	statusEmoji := specTaskStatusEmoji(task.Status)
	color := specTaskStatusColor(task.Status)

	title := task.Name
	if title == "" {
		title = task.ShortTitle
	}
	if title == "" {
		title = "Untitled task"
	}

	baseURL := strings.TrimRight(appURL, "/")
	taskLink := fmt.Sprintf("<%s/projects/%s/tasks/%s?view=details|%s>", baseURL, task.ProjectID, task.ID, task.ID)
	projectLink := s.buildProjectLink(ctx, task, baseURL)

	createdByUserName := ""

	if task.CreatedBy != "" {
		createdByUser, err := s.store.GetUser(ctx, &store.GetUserQuery{
			ID: task.CreatedBy,
		})
		if err != nil {
			log.Error().Err(err).Str("user_id", task.CreatedBy).Msg("failed to get created by user")
		} else {
			createdByUserName = createdByUser.FullName
		}
	}

	fields := []slack.AttachmentField{
		{Title: "Status", Value: fmt.Sprintf("`%s`", humanizeSpecTaskStatus(task.Status)), Short: true},
		{Title: "Priority", Value: fmt.Sprintf("`%s`", strings.ToUpper(string(task.Priority))), Short: true},
		{Title: "Task ID", Value: taskLink, Short: true},
		{Title: "User", Value: createdByUserName, Short: true},
	}

	if projectLink != "" {
		fields = append(fields, slack.AttachmentField{Title: "Project", Value: projectLink, Short: true})
	}

	return slack.Attachment{
		Color:      color,
		Title:      fmt.Sprintf("%s Project Update", statusEmoji),
		Text:       truncateForSlack(task.Description, 500),
		Fields:     fields,
		Footer:     fmt.Sprintf("Project %s • Updated %s", task.ProjectID, task.UpdatedAt.UTC().Format(time.RFC822)),
		MarkdownIn: []string{"text", "fields"},
	}
}

// buildProjectUpdateReplyAttachment creates a compact colored attachment for thread replies
func (s *SlackBot) buildProjectUpdateReplyAttachment(ctx context.Context, task *types.SpecTask, appURL string) slack.Attachment {
	statusEmoji := specTaskStatusEmoji(task.Status)
	color := specTaskStatusColor(task.Status)

	title := task.Name
	if title == "" {
		title = task.ShortTitle
	}
	if title == "" {
		title = "Untitled task"
	}

	baseURL := strings.TrimRight(appURL, "/")
	taskLink := fmt.Sprintf("<%s/projects/%s/tasks/%s?view=details|View task>", baseURL, task.ProjectID, task.ID)
	links := taskLink
	if projectLink := s.buildProjectLink(ctx, task, baseURL); projectLink != "" {
		links += " | " + projectLink
	}
	text := fmt.Sprintf("%s *%s* → *%s*\n%s", statusEmoji, title, humanizeSpecTaskStatus(task.Status), links)

	if task.Description != "" {
		text += "\n" + truncateForSlack(task.Description, 300)
	}

	return slack.Attachment{
		Color:      color,
		Text:       text,
		MarkdownIn: []string{"text"},
	}
}

func (s *SlackBot) buildProjectLink(ctx context.Context, task *types.SpecTask, baseURL string) string {
	if task.OrganizationID == "" || task.ProjectID == "" {
		return ""
	}
	org, err := s.store.GetOrganization(ctx, &store.GetOrganizationQuery{ID: task.OrganizationID})
	if err != nil {
		log.Error().Err(err).Str("organization_id", task.OrganizationID).Msg("failed to get organization for project link")
		return ""
	}
	return fmt.Sprintf("<%s/org/%s/projects/%s/specs|View project>", baseURL, org.Name, task.ProjectID)
}

// specTaskStatusColor returns the hex color for the colored sidebar based on status
func specTaskStatusColor(status types.SpecTaskStatus) string {
	switch status {
	case types.TaskStatusBacklog:
		return "#808080" // Grey
	case types.TaskStatusSpecGeneration, types.TaskStatusSpecRevision,
		types.TaskStatusQueuedSpecGeneration, types.TaskStatusSpecApproved:
		return "#FF8C00" // Orange - planning phase
	case types.TaskStatusImplementation, types.TaskStatusImplementationQueued,
		types.TaskStatusQueuedImplementation:
		return "#36a64f" // Green - implementation
	case types.TaskStatusSpecReview, types.TaskStatusImplementationReview:
		return "#2196F3" // Blue - review
	case types.TaskStatusPullRequest:
		return "#9C27B0" // Purple - pull request
	case types.TaskStatusDone:
		return "#36a64f" // Green - done
	case types.TaskStatusSpecFailed, types.TaskStatusImplementationFailed:
		return "#E53935" // Red - failed
	default:
		return "#808080" // Grey - default
	}
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
