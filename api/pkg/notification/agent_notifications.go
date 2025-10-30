package notification

import (
	"context"
	"fmt"

	"github.com/rs/zerolog/log"
)

// HelpRequestNotificationService handles notifications for help requests
type HelpRequestNotificationService struct {
	// Add notification channels here (Slack, email, etc.)
	// For now, we'll just log
}

// NewHelpRequestNotificationService creates a new help request notification service
func NewHelpRequestNotificationService() *HelpRequestNotificationService {
	return &HelpRequestNotificationService{}
}

// SendHelpNotification sends a notification when an agent requests help
func (s *HelpRequestNotificationService) SendHelpNotification(ctx context.Context, request *HelpRequest) error {
	// For now, just log the notification
	// TODO: Implement actual notification channels (Slack, email, Teams, etc.)

	log.Info().
		Str("request_id", request.ID).
		Str("session_id", request.SessionID).
		Str("help_type", request.HelpType).
		Str("urgency", request.Urgency).
		Str("context", request.Context).
		Str("specific_need", request.SpecificNeed).
		Msg("🚨 AGENT HELP REQUEST - Human assistance needed")

	// Format notification message
	message := fmt.Sprintf(`🚨 **Agent Help Request**

**Session**: %s
**Type**: %s
**Urgency**: %s

**Context**: %s

**Specific Need**: %s

**Attempted Solutions**: %s

**Suggested Approaches**: %s

**Dashboard**: Check the agent dashboard to respond to this request.
`, request.SessionID, request.HelpType, request.Urgency,
		request.Context, request.SpecificNeed,
		request.AttemptedSolutions, request.SuggestedApproaches)

	// TODO: Send to configured notification channels
	// - Slack channels
	// - Email alerts
	// - Microsoft Teams
	// - Discord
	// - Push notifications

	log.Info().Str("message", message).Msg("Help request notification prepared")

	return nil
}

// JobCompletionNotificationService handles notifications for job completions
type JobCompletionNotificationService struct {
	// Add notification channels here
}

// NewJobCompletionNotificationService creates a new job completion notification service
func NewJobCompletionNotificationService() *JobCompletionNotificationService {
	return &JobCompletionNotificationService{}
}

// SendCompletionNotification sends a notification when an agent completes work
func (s *JobCompletionNotificationService) SendCompletionNotification(ctx context.Context, completion *JobCompletion) error {
	// For now, just log the notification
	// TODO: Implement actual notification channels

	var icon string
	switch completion.CompletionStatus {
	case "fully_completed":
		icon = "🎉"
	case "milestone_reached":
		icon = "🏁"
	case "partial_completion":
		icon = "⏸️"
	case "ready_for_review":
		icon = "👀"
	case "blocked":
		icon = "🚫"
	default:
		icon = "✅"
	}

	log.Info().
		Str("completion_id", completion.ID).
		Str("session_id", completion.SessionID).
		Str("completion_status", completion.CompletionStatus).
		Str("confidence", completion.Confidence).
		Bool("review_needed", completion.ReviewNeeded).
		Str("summary", completion.Summary).
		Msg("📝 AGENT WORK COMPLETED")

	// Format notification message
	message := fmt.Sprintf(`%s **Agent Work Completed**

**Session**: %s
**Status**: %s
**Confidence**: %s

**Summary**: %s

**Deliverables**: %s

**Review Needed**: %t
`, icon, completion.SessionID, completion.CompletionStatus,
		completion.Confidence, completion.Summary, completion.Deliverables,
		completion.ReviewNeeded)

	if completion.ReviewNeeded {
		message += fmt.Sprintf("\n**Review Type**: %s", completion.ReviewType)
	}

	if completion.NextSteps != "" {
		message += fmt.Sprintf("\n\n**Next Steps**: %s", completion.NextSteps)
	}

	if completion.Limitations != "" {
		message += fmt.Sprintf("\n\n**Limitations**: %s", completion.Limitations)
	}

	message += "\n\n**Dashboard**: Check the agent dashboard to review this completion."

	log.Info().Str("message", message).Msg("Job completion notification prepared")

	return nil
}

// HelpRequest represents a request for human assistance (duplicated here for compilation)
// TODO: This should be imported from the skill package once dependencies are resolved
type HelpRequest struct {
	ID                  string            `json:"id"`
	SessionID           string            `json:"session_id"`
	InteractionID       string            `json:"interaction_id"`
	UserID              string            `json:"user_id"`
	AppID               string            `json:"app_id"`
	HelpType            string            `json:"help_type"`
	Context             string            `json:"context"`
	SpecificNeed        string            `json:"specific_need"`
	AttemptedSolutions  string            `json:"attempted_solutions"`
	Urgency             string            `json:"urgency"`
	SuggestedApproaches string            `json:"suggested_approaches"`
	Status              string            `json:"status"`
	Metadata            map[string]string `json:"metadata,omitempty"`
}

// JobCompletion represents a completed job/task (duplicated here for compilation)
// TODO: This should be imported from the skill package once dependencies are resolved
type JobCompletion struct {
	ID               string            `json:"id"`
	SessionID        string            `json:"session_id"`
	InteractionID    string            `json:"interaction_id"`
	UserID           string            `json:"user_id"`
	AppID            string            `json:"app_id"`
	WorkItemID       string            `json:"work_item_id,omitempty"`
	CompletionStatus string            `json:"completion_status"`
	Summary          string            `json:"summary"`
	Deliverables     string            `json:"deliverables"`
	ReviewNeeded     bool              `json:"review_needed"`
	ReviewType       string            `json:"review_type"`
	NextSteps        string            `json:"next_steps"`
	Limitations      string            `json:"limitations"`
	FilesCreated     string            `json:"files_created"`
	TimeSpent        string            `json:"time_spent"`
	Confidence       string            `json:"confidence"`
	Status           string            `json:"status"`
	Metadata         map[string]string `json:"metadata,omitempty"`
}

// NotificationConfig represents configuration for various notification channels
type NotificationConfig struct {
	Slack struct {
		Enabled    bool   `json:"enabled"`
		WebhookURL string `json:"webhook_url"`
		Channel    string `json:"channel"`
		Username   string `json:"username"`
		IconEmoji  string `json:"icon_emoji"`
	} `json:"slack"`

	Email struct {
		Enabled    bool     `json:"enabled"`
		SMTPServer string   `json:"smtp_server"`
		SMTPPort   int      `json:"smtp_port"`
		Username   string   `json:"username"`
		Password   string   `json:"password"`
		From       string   `json:"from"`
		To         []string `json:"to"`
	} `json:"email"`

	Teams struct {
		Enabled    bool   `json:"enabled"`
		WebhookURL string `json:"webhook_url"`
	} `json:"teams"`

	Discord struct {
		Enabled    bool   `json:"enabled"`
		WebhookURL string `json:"webhook_url"`
		Username   string `json:"username"`
		AvatarURL  string `json:"avatar_url"`
	} `json:"discord"`
}

// ConfigurableNotificationService provides a notification service with multiple channels
type ConfigurableNotificationService struct {
	config NotificationConfig
}

// NewConfigurableNotificationService creates a notification service with configuration
func NewConfigurableNotificationService(config NotificationConfig) *ConfigurableNotificationService {
	return &ConfigurableNotificationService{
		config: config,
	}
}

// SendHelpNotification implements the notification interface with multiple channels
func (s *ConfigurableNotificationService) SendHelpNotification(ctx context.Context, request *HelpRequest) error {
	// TODO: Implement actual notification sending to configured channels
	// For now, delegate to the simple service
	simple := NewHelpRequestNotificationService()
	return simple.SendHelpNotification(ctx, request)
}

// SendCompletionNotification implements the notification interface with multiple channels
func (s *ConfigurableNotificationService) SendCompletionNotification(ctx context.Context, completion *JobCompletion) error {
	// TODO: Implement actual notification sending to configured channels
	// For now, delegate to the simple service
	simple := NewJobCompletionNotificationService()
	return simple.SendCompletionNotification(ctx, completion)
}
