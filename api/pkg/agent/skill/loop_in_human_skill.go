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

const loopInHumanMainPrompt = `You are an AI agent that can request human assistance when you encounter complex situations that require human judgment, expertise, or decision-making.

Key responsibilities:
1. Problem Assessment:
   - Recognize when a task requires human input or expertise
   - Identify situations beyond your current capabilities
   - Assess when human judgment would be valuable

2. Help Request Formation:
   - Clearly articulate what kind of help you need
   - Provide relevant context about the current task
   - Specify the type of human expertise required
   - Include any relevant information that would help a human understand the situation

3. Communication:
   - Be clear and concise in your help requests
   - Provide actionable information for humans
   - Explain what you've tried and why human input is needed
   - Be respectful of human time and attention

When to use this skill:
- Complex decision-making requiring human judgment
- Tasks requiring domain expertise you don't possess
- Ambiguous requirements that need clarification
- Ethical considerations that require human oversight
- Creative decisions that benefit from human input
- Situations where you're stuck or uncertain about next steps

Best Practices:
- Use this skill judiciously - don't ask for help on every minor issue
- Provide clear context and explain what you need
- Be specific about the type of help required
- Include relevant background information
- Suggest potential approaches if you have any ideas

Remember: This skill will alert humans and pause your workflow until assistance is provided. Use it when human input would genuinely improve the outcome or when you're genuinely stuck.`

var loopInHumanSkillParameters = jsonschema.Definition{
	Type: jsonschema.Object,
	Properties: map[string]jsonschema.Definition{
		"help_type": {
			Type:        jsonschema.String,
			Description: "The type of help needed",
			Enum:        []string{"decision", "expertise", "clarification", "review", "guidance", "stuck", "other"},
		},
		"context": {
			Type:        jsonschema.String,
			Description: "Brief context about the current task or situation",
		},
		"specific_need": {
			Type:        jsonschema.String,
			Description: "Specific description of what help is needed",
		},
		"attempted_solutions": {
			Type:        jsonschema.String,
			Description: "What you've already tried or considered (optional)",
		},
		"urgency": {
			Type:        jsonschema.String,
			Description: "Urgency level of the request",
			Enum:        []string{"low", "medium", "high", "critical"},
		},
		"suggested_approaches": {
			Type:        jsonschema.String,
			Description: "Any potential approaches or solutions you can think of (optional)",
		},
	},
	Required: []string{"help_type", "context", "specific_need", "urgency"},
}

// NewLoopInHumanSkill creates a new skill for requesting human assistance
func NewLoopInHumanSkill(helpRequestStore HelpRequestStore, notificationService NotificationService) agent.Skill {
	return agent.Skill{
		Name:         "LoopInHuman",
		Description:  "Request human assistance when you need help, guidance, or expertise beyond your current capabilities",
		SystemPrompt: loopInHumanMainPrompt,
		Parameters:   loopInHumanSkillParameters,
		Direct:       true,
		Tools: []agent.Tool{
			&LoopInHumanTool{
				helpRequestStore:    helpRequestStore,
				notificationService: notificationService,
			},
		},
	}
}

// HelpRequestStore interface for storing help requests
type HelpRequestStore interface {
	CreateHelpRequest(ctx context.Context, request *HelpRequest) error
	GetHelpRequest(ctx context.Context, sessionID, interactionID string) (*HelpRequest, error)
	UpdateHelpRequest(ctx context.Context, request *HelpRequest) error
	ListActiveHelpRequests(ctx context.Context) ([]*HelpRequest, error)
}

// NotificationService interface for sending notifications
type NotificationService interface {
	SendHelpNotification(ctx context.Context, request *HelpRequest) error
}

// HelpRequest represents a request for human assistance
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
	Status              string            `json:"status"` // "pending", "in_progress", "resolved", "cancelled"
	CreatedAt           time.Time         `json:"created_at"`
	UpdatedAt           time.Time         `json:"updated_at"`
	ResolvedAt          *time.Time        `json:"resolved_at,omitempty"`
	ResolvedBy          string            `json:"resolved_by,omitempty"`
	Resolution          string            `json:"resolution,omitempty"`
	Metadata            map[string]string `json:"metadata,omitempty"`
}

type LoopInHumanTool struct {
	helpRequestStore    HelpRequestStore
	notificationService NotificationService
}

func (t *LoopInHumanTool) Name() string {
	return "LoopInHuman"
}

func (t *LoopInHumanTool) Description() string {
	return "Request human assistance when you need help, guidance, or expertise beyond your current capabilities"
}

func (t *LoopInHumanTool) String() string {
	return "Loop In Human"
}

func (t *LoopInHumanTool) StatusMessage() string {
	return "Requesting human assistance"
}

func (t *LoopInHumanTool) Icon() string {
	return "👋" // Hand wave emoji
}

func (t *LoopInHumanTool) OpenAI() []openai.Tool {
	return []openai.Tool{
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "LoopInHuman",
				Description: "Request human assistance when you need help, guidance, or expertise beyond your current capabilities",
				Parameters:  loopInHumanSkillParameters,
			},
		},
	}
}

func (t *LoopInHumanTool) Execute(ctx context.Context, meta agent.Meta, args map[string]interface{}) (string, error) {
	// Extract parameters
	helpType, ok := args["help_type"].(string)
	if !ok {
		return "", fmt.Errorf("help_type is required")
	}

	context_str, ok := args["context"].(string)
	if !ok {
		return "", fmt.Errorf("context is required")
	}

	specificNeed, ok := args["specific_need"].(string)
	if !ok {
		return "", fmt.Errorf("specific_need is required")
	}

	urgency, ok := args["urgency"].(string)
	if !ok {
		return "", fmt.Errorf("urgency is required")
	}

	// Optional parameters
	attemptedSolutions, _ := args["attempted_solutions"].(string)
	suggestedApproaches, _ := args["suggested_approaches"].(string)

	log.Info().
		Str("help_type", helpType).
		Str("context", context_str).
		Str("specific_need", specificNeed).
		Str("urgency", urgency).
		Str("user_id", meta.UserID).
		Str("session_id", meta.SessionID).
		Str("interaction_id", meta.InteractionID).
		Str("app_id", meta.AppID).
		Msg("Creating human help request")

	// Create help request
	helpRequest := &HelpRequest{
		ID:                  fmt.Sprintf("help-%s-%d", meta.SessionID, time.Now().Unix()),
		SessionID:           meta.SessionID,
		InteractionID:       meta.InteractionID,
		UserID:              meta.UserID,
		AppID:               meta.AppID,
		HelpType:            helpType,
		Context:             context_str,
		SpecificNeed:        specificNeed,
		AttemptedSolutions:  attemptedSolutions,
		Urgency:             urgency,
		SuggestedApproaches: suggestedApproaches,
		Status:              "pending",
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
		Metadata: map[string]string{
			"agent_type": "helix",
			"source":     "loop_in_human_skill",
		},
	}

	// Store the help request
	if err := t.helpRequestStore.CreateHelpRequest(ctx, helpRequest); err != nil {
		log.Error().Err(err).Msg("Failed to store help request")
		return "", fmt.Errorf("failed to store help request: %w", err)
	}

	// Send notifications
	if err := t.notificationService.SendHelpNotification(ctx, helpRequest); err != nil {
		log.Warn().Err(err).Msg("Failed to send help notification (continuing anyway)")
	}

	// Format response
	response := fmt.Sprintf(`🚨 **Human assistance requested**

**Type**: %s
**Urgency**: %s

**Context**: %s

**Specific Need**: %s`, helpType, urgency, context_str, specificNeed)

	if attemptedSolutions != "" {
		response += fmt.Sprintf("\n\n**What I've tried**: %s", attemptedSolutions)
	}

	if suggestedApproaches != "" {
		response += fmt.Sprintf("\n\n**Potential approaches**: %s", suggestedApproaches)
	}

	response += fmt.Sprintf(`

**Status**: Help request submitted (ID: %s)
**Action**: Waiting for human assistance. You will be notified when help is available.

*This agent session will pause until human assistance is provided.*`, helpRequest.ID)

	return response, nil
}
