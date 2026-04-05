package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// AttentionService emits attention events when human action is needed on a
// spectask. Events are persisted to the database (for the frontend queue) and
// optionally forwarded to per-project Slack threads.
type AttentionService struct {
	store store.Store
	cfg   *config.ServerConfig
}

// NewAttentionService creates a new AttentionService.
func NewAttentionService(store store.Store, cfg *config.ServerConfig) *AttentionService {
	return &AttentionService{
		store: store,
		cfg:   cfg,
	}
}

// EmitEvent creates an attention event for the owner of the given spectask.
// It is idempotent: if an event with the same idempotency key already exists,
// the call succeeds without creating a duplicate.
//
// The qualifier is appended to the idempotency key to distinguish events of the
// same type on the same task (e.g., different commit hashes for specs_pushed).
func (s *AttentionService) EmitEvent(
	ctx context.Context,
	eventType types.AttentionEventType,
	task *types.SpecTask,
	qualifier string,
	metadata map[string]interface{},
) (*types.AttentionEvent, error) {
	if task == nil {
		return nil, fmt.Errorf("task is required")
	}
	if task.ID == "" {
		return nil, fmt.Errorf("task ID is required")
	}

	// Look up the project to get the owner and name.
	project, err := s.store.GetProject(ctx, task.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get project %s: %w", task.ProjectID, err)
	}

	// Determine the user who should be notified. Use the project owner.
	userID := project.UserID

	title := buildTitle(eventType, task)
	description := buildDescription(eventType, task)

	var metadataJSON []byte
	if metadata != nil {
		metadataJSON, err = json.Marshal(metadata)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	// Resolve assignee name for display in notification UI.
	var assigneeName string
	if task.AssigneeID != "" {
		if assignee, err := s.store.GetUser(ctx, &store.GetUserQuery{ID: task.AssigneeID}); err == nil {
			assigneeName = assignee.FullName
			if assigneeName == "" {
				assigneeName = assignee.Username
			}
		}
	}

	event := &types.AttentionEvent{
		ID:             system.GenerateAttentionEventID(),
		UserID:         userID,
		OrganizationID: project.OrganizationID,
		ProjectID:      task.ProjectID,
		SpecTaskID:     task.ID,
		EventType:      eventType,
		Title:          title,
		Description:    description,
		CreatedAt:      time.Now(),
		IdempotencyKey: types.BuildAttentionEventIdempotencyKey(task.ID, eventType, qualifier),
		ProjectName:    project.Name,
		SpecTaskName:   task.Name,
		AssigneeName:   assigneeName,
	}
	if metadataJSON != nil {
		event.Metadata = metadataJSON
	}

	created, err := s.store.CreateAttentionEvent(ctx, event)
	if err != nil {
		return nil, fmt.Errorf("failed to create attention event: %w", err)
	}

	// If the event was deduplicated (existing row returned), skip Slack.
	if created.ID != event.ID {
		log.Debug().
			Str("event_type", string(eventType)).
			Str("spec_task_id", task.ID).
			Str("idempotency_key", event.IdempotencyKey).
			Msg("Attention event already exists (idempotent skip)")
		return created, nil
	}

	log.Info().
		Str("event_id", created.ID).
		Str("event_type", string(eventType)).
		Str("spec_task_id", task.ID).
		Str("project_id", task.ProjectID).
		Str("user_id", userID).
		Msg("Attention event emitted")

	// Fire-and-forget Slack notification via the per-project Slack trigger.
	go s.notifySlack(context.Background(), created, task, project)

	return created, nil
}

// notifySlack posts an attention event as a threaded reply in the project's
// Slack channel. It looks up the project's app with ProjectUpdates enabled,
// finds the existing SlackThread for this spectask, and posts a reply.
//
// If no Slack bot is configured or the thread doesn't exist yet, it silently
// returns. The regular SubscribeForTasks flow will create the thread on the
// next task status change.
func (s *AttentionService) notifySlack(ctx context.Context, event *types.AttentionEvent, task *types.SpecTask, project *types.Project) {
	// Find apps in this project's org that have Slack project updates enabled.
	apps, err := s.store.ListApps(ctx, &store.ListAppsQuery{
		OrganizationID: project.OrganizationID,
	})
	if err != nil {
		log.Warn().Err(err).
			Str("project_id", project.ID).
			Msg("Failed to list apps for Slack notification")
		return
	}

	for _, app := range apps {
		for _, trigger := range app.Config.Helix.Triggers {
			if trigger.Slack == nil || !trigger.Slack.Enabled || !trigger.Slack.ProjectUpdates {
				continue
			}
			if trigger.Slack.BotToken == "" || trigger.Slack.ProjectChannel == "" {
				continue
			}

			// Check that this app is actually linked to the project
			// (by checking if any assistant has ProjectManager enabled for this project).
			isProjectApp := false
			for _, assistant := range app.Config.Helix.Assistants {
				if assistant.ProjectManager.Enabled && assistant.ProjectManager.ProjectID == project.ID {
					isProjectApp = true
					break
				}
			}
			if !isProjectApp {
				continue
			}

			// Look up an existing Slack thread for this spectask.
			thread, err := s.store.GetSlackThreadBySpecTaskID(ctx, app.ID, task.ID)
			if err != nil {
				// No thread yet — the regular SubscribeForTasks flow will create
				// one on the next task status change. Skip silently.
				log.Debug().
					Str("app_id", app.ID).
					Str("spec_task_id", task.ID).
					Msg("No Slack thread for spectask, skipping attention notification")
				continue
			}

			s.postSlackThreadReply(ctx, trigger.Slack, thread, event)
			return // Only post once per event
		}
	}
}

// postSlackThreadReply posts an attention event message as a reply to an
// existing Slack thread using the Slack API.
func (s *AttentionService) postSlackThreadReply(
	ctx context.Context,
	slackCfg *types.SlackTrigger,
	thread *types.SlackThread,
	event *types.AttentionEvent,
) {
	emoji := eventEmoji(event.EventType)
	text := fmt.Sprintf("%s *%s*\n_%s_ — %s", emoji, event.Title, event.SpecTaskName, event.ProjectName)

	if s.cfg != nil && s.cfg.Notifications.AppURL != "" {
		link := fmt.Sprintf("%s/projects/%s/tasks/%s?view=details",
			s.cfg.Notifications.AppURL, event.ProjectID, event.SpecTaskID)
		text += fmt.Sprintf("\n<%s|View task>", link)
	}

	payload := map[string]interface{}{
		"channel":   thread.Channel,
		"thread_ts": thread.ThreadKey,
		"text":      text,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal Slack payload")
		return
	}

	req, err := newSlackAPIRequest(ctx, "https://slack.com/api/chat.postMessage", slackCfg.BotToken, payloadBytes)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create Slack API request")
		return
	}

	resp, err := defaultHTTPClient.Do(req)
	if err != nil {
		log.Error().Err(err).Msg("Failed to send Slack attention notification")
		return
	}
	defer resp.Body.Close()

	log.Info().
		Str("event_id", event.ID).
		Str("channel", thread.Channel).
		Str("thread_key", thread.ThreadKey).
		Msg("Posted attention event reply to Slack thread")
}

func buildTitle(eventType types.AttentionEventType, task *types.SpecTask) string {
	switch eventType {
	case types.AttentionEventSpecsPushed:
		return "Specs ready for review"
	case types.AttentionEventAgentInteractionCompleted:
		return "Agent finished working"
	case types.AttentionEventSpecFailed:
		return "Spec generation failed"
	case types.AttentionEventImplementationFailed:
		return "Implementation failed"
	case types.AttentionEventPRReady:
		return "Pull request ready"
	default:
		return "Attention needed"
	}
}

func buildDescription(eventType types.AttentionEventType, task *types.SpecTask) string {
	name := task.Name
	if name == "" {
		name = task.ShortTitle
	}
	if name == "" {
		name = task.ID
	}

	switch eventType {
	case types.AttentionEventSpecsPushed:
		return fmt.Sprintf("Design docs pushed for \"%s\" — ready for your review", name)
	case types.AttentionEventAgentInteractionCompleted:
		return fmt.Sprintf("Agent finished its current work on \"%s\"", name)
	case types.AttentionEventSpecFailed:
		return fmt.Sprintf("Spec generation failed for \"%s\" — needs triage", name)
	case types.AttentionEventImplementationFailed:
		return fmt.Sprintf("Implementation failed for \"%s\" — needs triage", name)
	case types.AttentionEventPRReady:
		return fmt.Sprintf("Pull request opened for \"%s\" — awaiting merge", name)
	default:
		return fmt.Sprintf("Task \"%s\" needs your attention", name)
	}
}

func eventEmoji(eventType types.AttentionEventType) string {
	switch eventType {
	case types.AttentionEventSpecsPushed:
		return "📋"
	case types.AttentionEventAgentInteractionCompleted:
		return "🛑"
	case types.AttentionEventSpecFailed, types.AttentionEventImplementationFailed:
		return "❌"
	case types.AttentionEventPRReady:
		return "🔀"
	default:
		return "🔔"
	}
}

// --- helpers ---

var defaultHTTPClient = &defaultHTTPClientType{}

type defaultHTTPClientType struct{}

func (c *defaultHTTPClientType) Do(req *http.Request) (*http.Response, error) {
	return http.DefaultClient.Do(req)
}

func newSlackAPIRequest(ctx context.Context, url, token string, body []byte) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+token)
	return req, nil
}
