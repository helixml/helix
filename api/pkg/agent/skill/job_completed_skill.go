package skill

import (
	"context"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/agent"
	"github.com/helixml/helix/api/pkg/util/jsonschema"
	"github.com/rs/zerolog/log"
	"github.com/sashabaranov/go-openai"
)

const jobCompletedMainPrompt = `You are an AI agent that can signal the completion of tasks and request human review when appropriate.

Key responsibilities:
1. Task Completion Assessment:
   - Recognize when you have completed a task or reached a logical stopping point
   - Assess the quality and completeness of your work
   - Determine if human review is needed

2. Completion Reporting:
   - Clearly summarize what was accomplished
   - Highlight key outcomes and deliverables
   - Identify any limitations or areas requiring attention
   - Provide recommendations for next steps

3. Review Request:
   - Specify what type of review is needed (if any)
   - Indicate the urgency of review
   - Provide context for reviewers

When to use this skill:
- You have successfully completed a task or significant milestone
- You have reached a natural stopping point in your work
- You need human review or approval before proceeding
- You want to update stakeholders on your progress
- You have completed deliverables that are ready for handoff

Best Practices:
- Be clear about what was accomplished vs. what remains
- Provide specific deliverables or outcomes
- Be honest about limitations or uncertainties
- Include relevant links, files, or references
- Suggest appropriate next steps

Remember: This skill will update your status to "completed" and notify relevant stakeholders. Use it when you have genuinely finished your work or reached a meaningful checkpoint.`

var jobCompletedSkillParameters = jsonschema.Definition{
	Type: jsonschema.Object,
	Properties: map[string]jsonschema.Definition{
		"completion_status": {
			Type:        jsonschema.String,
			Description: "The completion status",
			Enum:        []string{"fully_completed", "milestone_reached", "partial_completion", "ready_for_review", "blocked"},
		},
		"summary": {
			Type:        jsonschema.String,
			Description: "Brief summary of what was accomplished",
		},
		"deliverables": {
			Type:        jsonschema.String,
			Description: "Specific deliverables, outcomes, or artifacts created",
		},
		"review_needed": {
			Type:        jsonschema.Boolean,
			Description: "Whether human review is needed",
		},
		"review_type": {
			Type:        jsonschema.String,
			Description: "Type of review needed (if any)",
			Enum:        []string{"approval", "feedback", "validation", "testing", "deployment", "none"},
		},
		"next_steps": {
			Type:        jsonschema.String,
			Description: "Recommended next steps or follow-up actions",
		},
		"limitations": {
			Type:        jsonschema.String,
			Description: "Any limitations, assumptions, or areas needing attention (optional)",
		},
		"files_created": {
			Type:        jsonschema.String,
			Description: "List of files or resources created/modified (optional)",
		},
		"time_spent": {
			Type:        jsonschema.String,
			Description: "Approximate time spent on this task (optional)",
		},
		"confidence": {
			Type:        jsonschema.String,
			Description: "Confidence level in the completion",
			Enum:        []string{"high", "medium", "low"},
		},
	},
	Required: []string{"completion_status", "summary", "deliverables", "review_needed", "confidence"},
}

// NewJobCompletedSkill creates a new skill for signaling job completion
func NewJobCompletedSkill(completionStore JobCompletionStore, notificationService CompletionNotificationService) agent.Skill {
	return agent.Skill{
		Name:         "JobCompleted",
		Description:  "Signal that a task or job has been completed and update your status accordingly",
		SystemPrompt: jobCompletedMainPrompt,
		Parameters:   jobCompletedSkillParameters,
		Direct:       true,
		Tools: []agent.Tool{
			&JobCompletedTool{
				completionStore:     completionStore,
				notificationService: notificationService,
			},
		},
	}
}

// JobCompletionStore interface for storing job completion records
type JobCompletionStore interface {
	CreateJobCompletion(ctx context.Context, completion *JobCompletion) error
	GetJobCompletion(ctx context.Context, sessionID, interactionID string) (*JobCompletion, error)
	UpdateJobCompletion(ctx context.Context, completion *JobCompletion) error
	ListJobCompletions(ctx context.Context, query *ListJobCompletionsQuery) ([]*JobCompletion, error)
}

// CompletionNotificationService interface for sending completion notifications
type CompletionNotificationService interface {
	SendCompletionNotification(ctx context.Context, completion *JobCompletion) error
}

// JobCompletion represents a completed job/task
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
	Status           string            `json:"status"` // "pending_review", "approved", "needs_changes", "archived"
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
	ReviewedAt       *time.Time        `json:"reviewed_at,omitempty"`
	ReviewedBy       string            `json:"reviewed_by,omitempty"`
	ReviewNotes      string            `json:"review_notes,omitempty"`
	Metadata         map[string]string `json:"metadata,omitempty"`
}

// ListJobCompletionsQuery for querying job completions
type ListJobCompletionsQuery struct {
	Page             int
	PageSize         int
	UserID           string
	SessionID        string
	CompletionStatus string
	ReviewNeeded     *bool
	Status           string
}

type JobCompletedTool struct {
	completionStore     JobCompletionStore
	notificationService CompletionNotificationService
}

func (t *JobCompletedTool) Name() string {
	return "JobCompleted"
}

func (t *JobCompletedTool) Description() string {
	return "Signal that a task or job has been completed and update your status accordingly"
}

func (t *JobCompletedTool) String() string {
	return "Job Completed"
}

func (t *JobCompletedTool) StatusMessage() string {
	return "Marking job as completed"
}

func (t *JobCompletedTool) Icon() string {
	return "✅" // Check mark emoji
}

func (t *JobCompletedTool) OpenAI() []openai.Tool {
	return []openai.Tool{
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "JobCompleted",
				Description: "Signal that a task or job has been completed and update your status accordingly",
				Parameters:  jobCompletedSkillParameters,
			},
		},
	}
}

func (t *JobCompletedTool) Execute(ctx context.Context, meta agent.Meta, args map[string]interface{}) (string, error) {
	// Extract required parameters
	completionStatus, ok := args["completion_status"].(string)
	if !ok {
		return "", fmt.Errorf("completion_status is required")
	}

	summary, ok := args["summary"].(string)
	if !ok {
		return "", fmt.Errorf("summary is required")
	}

	deliverables, ok := args["deliverables"].(string)
	if !ok {
		return "", fmt.Errorf("deliverables is required")
	}

	reviewNeeded, ok := args["review_needed"].(bool)
	if !ok {
		return "", fmt.Errorf("review_needed is required")
	}

	confidence, ok := args["confidence"].(string)
	if !ok {
		return "", fmt.Errorf("confidence is required")
	}

	// Extract optional parameters
	reviewType, _ := args["review_type"].(string)
	if reviewType == "" && reviewNeeded {
		reviewType = "approval" // Default review type
	}
	nextSteps, _ := args["next_steps"].(string)
	limitations, _ := args["limitations"].(string)
	filesCreated, _ := args["files_created"].(string)
	timeSpent, _ := args["time_spent"].(string)

	log.Info().
		Str("completion_status", completionStatus).
		Str("summary", summary).
		Bool("review_needed", reviewNeeded).
		Str("confidence", confidence).
		Str("user_id", meta.UserID).
		Str("session_id", meta.SessionID).
		Str("interaction_id", meta.InteractionID).
		Str("app_id", meta.AppID).
		Msg("Creating job completion record")

	// Determine initial status
	status := "completed"
	if reviewNeeded {
		status = "pending_review"
	}

	// Create job completion record
	completion := &JobCompletion{
		ID:               fmt.Sprintf("completion-%s-%d", meta.SessionID, time.Now().Unix()),
		SessionID:        meta.SessionID,
		InteractionID:    meta.InteractionID,
		UserID:           meta.UserID,
		AppID:            meta.AppID,
		CompletionStatus: completionStatus,
		Summary:          summary,
		Deliverables:     deliverables,
		ReviewNeeded:     reviewNeeded,
		ReviewType:       reviewType,
		NextSteps:        nextSteps,
		Limitations:      limitations,
		FilesCreated:     filesCreated,
		TimeSpent:        timeSpent,
		Confidence:       confidence,
		Status:           status,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
		Metadata: map[string]string{
			"agent_type": "helix",
			"source":     "job_completed_skill",
		},
	}

	// Store the completion record
	if err := t.completionStore.CreateJobCompletion(ctx, completion); err != nil {
		log.Error().Err(err).Msg("Failed to store job completion")
		return "", fmt.Errorf("failed to store job completion: %w", err)
	}

	// Send notifications
	if err := t.notificationService.SendCompletionNotification(ctx, completion); err != nil {
		log.Warn().Err(err).Msg("Failed to send completion notification (continuing anyway)")
	}

	// Format response based on completion status and review needs
	var statusIcon string
	var statusText string

	switch completionStatus {
	case "fully_completed":
		statusIcon = "🎉"
		statusText = "Task fully completed"
	case "milestone_reached":
		statusIcon = "🏁"
		statusText = "Milestone reached"
	case "partial_completion":
		statusIcon = "⏸️"
		statusText = "Partial completion"
	case "ready_for_review":
		statusIcon = "👀"
		statusText = "Ready for review"
	case "blocked":
		statusIcon = "🚫"
		statusText = "Blocked - cannot proceed"
	default:
		statusIcon = "✅"
		statusText = "Task completed"
	}

	response := fmt.Sprintf(`%s **%s**

**Summary**: %s

**Deliverables**: %s

**Confidence**: %s`, statusIcon, statusText, summary, deliverables, confidence)

	if limitations != "" {
		response += fmt.Sprintf("\n\n**Limitations/Notes**: %s", limitations)
	}

	if filesCreated != "" {
		response += fmt.Sprintf("\n\n**Files Created/Modified**: %s", filesCreated)
	}

	if timeSpent != "" {
		response += fmt.Sprintf("\n\n**Time Spent**: %s", timeSpent)
	}

	if reviewNeeded {
		response += fmt.Sprintf(`

🔍 **Review Required**
**Review Type**: %s
**Status**: Pending review`, reviewType)
	}

	if nextSteps != "" {
		response += fmt.Sprintf("\n\n**Next Steps**: %s", nextSteps)
	}

	response += fmt.Sprintf(`

**Completion ID**: %s
**Session Status**: %s

*This agent session is now marked as completed and ready for review.*`, completion.ID, status)

	return response, nil
}
