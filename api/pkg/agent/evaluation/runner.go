package evaluation

import (
	"context"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// ProgressCallback is called after each question completes
type ProgressCallback func(progress types.EvaluationRunProgress)

// RunnerConfig holds the dependencies for running evaluations
type RunnerConfig struct {
	Store      store.Store
	Controller SessionController
	Judge      LLMJudge // optional, needed for llm_judge assertions
}

// RunEvaluation executes all questions in a suite sequentially, calling progressFn after each
func RunEvaluation(
	ctx context.Context,
	cfg *RunnerConfig,
	run *types.EvaluationRun,
	suite *types.EvaluationSuite,
	app *types.App,
	user *types.User,
	progressFn ProgressCallback,
) {
	questions := suite.Questions
	results := make([]types.EvaluationQuestionResult, 0, len(questions))

	summary := types.EvaluationRunSummary{
		TotalQuestions: len(questions),
	}

	skillsUsedSet := map[string]bool{}

	// Update run to running
	run.Status = types.EvaluationRunStatusRunning
	if _, err := cfg.Store.UpdateEvaluationRun(ctx, run); err != nil {
		log.Error().Err(err).Str("run_id", run.ID).Msg("failed to update evaluation run to running")
	}

	if progressFn != nil {
		progressFn(types.EvaluationRunProgress{
			RunID:          run.ID,
			Status:         types.EvaluationRunStatusRunning,
			TotalQuestions: len(questions),
		})
	}

	for i, question := range questions {
		select {
		case <-ctx.Done():
			run.Status = types.EvaluationRunStatusCancelled
			run.Error = "cancelled"
			run.Summary = summary
			run.Results = results
			if _, err := cfg.Store.UpdateEvaluationRun(ctx, run); err != nil {
				log.Error().Err(err).Msg("failed to update cancelled evaluation run")
			}
			return
		default:
		}

		result := runSingleQuestion(ctx, cfg, question, app, user)

		// Aggregate
		if result.Passed {
			summary.Passed++
		} else {
			summary.Failed++
		}
		summary.TotalDurationMs += result.DurationMs
		summary.TotalTokens += result.TokensUsed.TotalTokens
		summary.TotalCost += result.Cost
		for _, skill := range result.SkillsUsed {
			if !skillsUsedSet[skill] {
				skillsUsedSet[skill] = true
				summary.SkillsUsed = append(summary.SkillsUsed, skill)
			}
		}

		results = append(results, result)

		// Persist progress
		run.Results = results
		run.Summary = summary
		if _, err := cfg.Store.UpdateEvaluationRun(ctx, run); err != nil {
			log.Error().Err(err).Msg("failed to update evaluation run progress")
		}

		if progressFn != nil {
			progressFn(types.EvaluationRunProgress{
				RunID:           run.ID,
				Status:          types.EvaluationRunStatusRunning,
				CurrentQuestion: i + 1,
				TotalQuestions:  len(questions),
				LatestResult:    &result,
				Summary:         &summary,
			})
		}
	}

	// Finalize
	run.Status = types.EvaluationRunStatusCompleted
	run.Summary = summary
	run.Results = results
	if summary.Failed > 0 {
		run.Error = fmt.Sprintf("%d/%d questions failed", summary.Failed, summary.TotalQuestions)
	}
	if _, err := cfg.Store.UpdateEvaluationRun(ctx, run); err != nil {
		log.Error().Err(err).Msg("failed to update completed evaluation run")
	}

	if progressFn != nil {
		progressFn(types.EvaluationRunProgress{
			RunID:           run.ID,
			Status:          types.EvaluationRunStatusCompleted,
			CurrentQuestion: len(questions),
			TotalQuestions:  len(questions),
			Summary:         &summary,
		})
	}
}

func runSingleQuestion(
	ctx context.Context,
	cfg *RunnerConfig,
	question types.EvaluationQuestion,
	app *types.App,
	user *types.User,
) types.EvaluationQuestionResult {
	start := time.Now()

	sessionID := system.GenerateSessionID()
	session := &types.Session{
		ID:             sessionID,
		Name:           fmt.Sprintf("Evaluation: %s", truncate(question.Question, 80)),
		Created:        time.Now(),
		Updated:        time.Now(),
		Mode:           types.SessionModeInference,
		Type:           types.SessionTypeText,
		ParentApp:      app.ID,
		OrganizationID: app.OrganizationID,
		Owner:          user.ID,
		OwnerType:      user.Type,
		Metadata: types.SessionMetadata{
			Stream:       false,
			HelixVersion: data.GetHelixVersion(),
			AgentType:    "helix",
		},
	}

	if len(app.Config.Helix.Assistants) > 0 {
		session.Metadata.SystemPrompt = app.Config.Helix.Assistants[0].SystemPrompt
	}

	sessionCtx := openai.SetContextSessionID(ctx, sessionID)
	if app.OrganizationID != "" {
		sessionCtx = openai.SetContextOrganizationID(sessionCtx, app.OrganizationID)
	}
	sessionCtx = openai.SetContextAppID(sessionCtx, app.ID)

	if err := cfg.Controller.WriteSession(sessionCtx, session); err != nil {
		return types.EvaluationQuestionResult{
			QuestionID: question.ID,
			Question:   question.Question,
			SessionID:  sessionID,
			DurationMs: time.Since(start).Milliseconds(),
			Error:      fmt.Sprintf("failed to create session: %s", err),
		}
	}

	interaction, err := cfg.Controller.RunBlockingSession(sessionCtx, &controller.RunSessionRequest{
		OrganizationID: app.OrganizationID,
		App:            app,
		Session:        session,
		User:           user,
		PromptMessage:  types.MessageContent{Parts: []any{question.Question}},
		HistoryLimit:   -1,
	})

	durationMs := time.Since(start).Milliseconds()

	if err != nil {
		return types.EvaluationQuestionResult{
			QuestionID: question.ID,
			Question:   question.Question,
			SessionID:  sessionID,
			DurationMs: durationMs,
			Error:      err.Error(),
		}
	}

	response := types.TextFromInteraction(interaction)
	skillsUsed := extractSkillNames(interaction)
	usage := interaction.Usage

	// Run assertions
	assertionResults, passed := CheckAllAssertions(ctx, question.Assertions, response, skillsUsed, cfg.Judge)

	return types.EvaluationQuestionResult{
		QuestionID:       question.ID,
		Question:         question.Question,
		Response:         response,
		SessionID:        sessionID,
		InteractionID:    interaction.ID,
		DurationMs:       durationMs,
		TokensUsed:       usage,
		Cost:             estimateCost(usage),
		SkillsUsed:       skillsUsed,
		AssertionResults: assertionResults,
		Passed:           passed,
	}
}

// extractSkillNames gets the tool/skill names from an interaction's tool calls
func extractSkillNames(interaction *types.Interaction) []string {
	if len(interaction.ToolCalls) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var names []string
	for _, tc := range interaction.ToolCalls {
		name := tc.Function.Name
		if name != "" && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names
}

// estimateCost provides a rough cost estimate based on token usage
// Using approximate rates for a mid-tier model ($3/1M input, $15/1M output)
func estimateCost(usage types.Usage) float64 {
	inputCost := float64(usage.PromptTokens) * 3.0 / 1_000_000
	outputCost := float64(usage.CompletionTokens) * 15.0 / 1_000_000
	return inputCost + outputCost
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
