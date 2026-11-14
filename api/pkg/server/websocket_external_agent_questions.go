package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/types"
)

const (
	maxAgentQuestions       = 16
	maxAgentQuestionOptions = 64
	maxAgentQuestionText    = 16 * 1024
)

type questionResolvedEvent struct {
	ThreadID      string            `json:"thread_id"`
	RequestID     string            `json:"request_id"`
	TurnRequestID string            `json:"turn_request_id"`
	Outcome       string            `json:"outcome"`
	Answers       map[string]string `json:"answers"`
	// SubmissionID echoes the nonce Helix sent with an API answer or cancel.
	// The Zed build pinned in sandbox-versions.txt does not echo it yet, so
	// every resolution it reports is attributed to external_agent.
	SubmissionID string `json:"submission_id"`
}

func decodeSyncData(data map[string]interface{}, target interface{}) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal sync data: %w", err)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("decode sync data: %w", err)
	}
	return nil
}

func validatePendingQuestion(question *types.PendingQuestion) error {
	if question.RequestID == "" || question.ThreadID == "" || question.TurnRequestID == "" {
		return errors.New("request_id, thread_id, and turn_request_id are required")
	}
	if question.Source != "elicitation" && question.Source != "permission" {
		return fmt.Errorf("unsupported question source %q", question.Source)
	}
	if len(question.Questions) == 0 || len(question.Questions) > maxAgentQuestions {
		return fmt.Errorf("questions must contain between 1 and %d items", maxAgentQuestions)
	}
	seen := make(map[string]struct{}, len(question.Questions))
	for i := range question.Questions {
		item := &question.Questions[i]
		item.ID = strings.TrimSpace(item.ID)
		item.Header = strings.TrimSpace(item.Header)
		item.Question = strings.TrimSpace(item.Question)
		if item.ID == "" || item.Question == "" {
			return errors.New("every question requires an id and question")
		}
		if strings.ContainsRune(item.ID, '\x00') || strings.ContainsRune(item.Header, '\x00') || strings.ContainsRune(item.Question, '\x00') {
			return errors.New("question text contains an invalid null character")
		}
		if len(item.Header) > maxAgentQuestionText || len(item.Question) > maxAgentQuestionText {
			return errors.New("question text is too long")
		}
		if _, ok := seen[item.ID]; ok {
			return fmt.Errorf("duplicate question id %q", item.ID)
		}
		seen[item.ID] = struct{}{}
		if len(item.Options) > maxAgentQuestionOptions {
			return fmt.Errorf("question %q has too many options", item.ID)
		}
		for optionIndex := range item.Options {
			option := &item.Options[optionIndex]
			option.Label = strings.TrimSpace(option.Label)
			option.Description = strings.TrimSpace(option.Description)
			if option.Label == "" {
				return fmt.Errorf("question %q has an option without a label", item.ID)
			}
			if strings.ContainsAny(option.Label, "\r\n") {
				return fmt.Errorf("question %q has an option label containing a newline", item.ID)
			}
			if strings.ContainsRune(option.Label, '\x00') || strings.ContainsRune(option.Description, '\x00') {
				return fmt.Errorf("question %q has option text containing an invalid null character", item.ID)
			}
			if len(option.Label) > maxAgentQuestionText || len(option.Description) > maxAgentQuestionText {
				return fmt.Errorf("question %q has option text that is too long", item.ID)
			}
		}
	}
	return nil
}

// settledByHelix attributes a question Helix cancels itself, ignoring any
// API submission still on it: nobody's answer reached the agent.
func settledByHelix(*types.QuestionSubmission) types.TurnProvenance {
	return types.SystemTurn(types.SystemTurnQuestionSettled)
}

func (apiServer *HelixAPIServer) settlePendingQuestionAsCancelled(
	ctx context.Context,
	interaction *types.Interaction,
) (*types.Interaction, bool, error) {
	if interaction == nil || interaction.PendingQuestion == nil {
		return interaction, false, nil
	}
	requestID := interaction.PendingQuestion.RequestID
	updated, changed, err := apiServer.Store.ResolveInteractionPendingQuestion(
		ctx,
		interaction.ID,
		interaction.GenerationID,
		requestID,
		"cancelled",
		nil,
		settledByHelix,
	)
	if err != nil {
		return interaction, false, fmt.Errorf("settle pending question as cancelled: %w", err)
	}
	if updated == nil {
		return interaction, false, errors.New("settle pending question returned no interaction")
	}
	apiServer.finishQuestionAction(interaction.ID, requestID)
	return updated, changed, nil
}

func (apiServer *HelixAPIServer) handleQuestionRequested(sessionID string, syncMsg *types.SyncMessage) error {
	var question types.PendingQuestion
	if err := decodeSyncData(syncMsg.Data, &question); err != nil {
		return err
	}
	if err := validatePendingQuestion(&question); err != nil {
		return fmt.Errorf("invalid question_requested event: %w", err)
	}
	if question.AskedAt.IsZero() {
		question.AskedAt = syncMsg.Timestamp
		if question.AskedAt.IsZero() {
			question.AskedAt = time.Now()
		}
	}

	ctx := context.Background()
	interaction := apiServer.interactionForRequest(ctx, question.TurnRequestID)
	if interaction == nil {
		return fmt.Errorf("question turn request %q does not map to an interaction", question.TurnRequestID)
	}
	// Turns on agent-created threads belong to child sessions carried by
	// this (the parent's) connection; another lane's turn is refused.
	if err := apiServer.interactionOnConnection(ctx, sessionID, interaction); err != nil {
		return fmt.Errorf("question for interaction %q: %w", interaction.ID, err)
	}

	updated, changed, err := apiServer.Store.SetInteractionPendingQuestion(
		ctx, interaction.ID, interaction.GenerationID, &question,
	)
	if err != nil {
		return fmt.Errorf("set pending question: %w", err)
	}
	if updated == nil {
		return errors.New("set pending question returned no interaction")
	}
	if !changed {
		if updated.PendingQuestion != nil && updated.PendingQuestion.RequestID == question.RequestID {
			// A replay proves the agent is still waiting. An API action that
			// is recorded for this question is resent, nonce and all, rather
			// than dropped: the replay may have been emitted before that
			// action reached the agent, and dropping it would let a second,
			// different action through. Resending the same command is at
			// worst a duplicate of the one answer.
			if err := apiServer.resendQuestionSubmission(ctx, updated, question.RequestID); err != nil {
				return err
			}
			return apiServer.publishQuestionInteractionUpdate(updated)
		}
		for _, resolved := range updated.QuestionHistory {
			if resolved.RequestID == question.RequestID {
				return nil
			}
		}
		return fmt.Errorf("interaction %q cannot accept question %q", interaction.ID, question.RequestID)
	}
	return apiServer.publishQuestionInteractionUpdate(updated)
}

// resendQuestionSubmission redelivers the API submission recorded for the
// replayed question requestID. With none recorded, any in-memory claim on the
// question is released: whatever held it was lost with the old connection.
// If the resend cannot be delivered the submission is withdrawn, exactly as
// when the first send failed.
func (apiServer *HelixAPIServer) resendQuestionSubmission(ctx context.Context, interaction *types.Interaction, requestID string) error {
	submission := interaction.SubmissionFor(requestID)
	if submission == nil {
		apiServer.finishQuestionAction(interaction.ID, requestID)
		return nil
	}
	if err := apiServer.sendCommandToExternalAgent(interaction.SessionID, questionCommand(submission)); err != nil {
		clearErr := apiServer.Store.ClearInteractionQuestionSubmission(ctx, interaction.ID, interaction.GenerationID, requestID, submission.ID)
		apiServer.finishQuestionAction(interaction.ID, requestID)
		if clearErr != nil {
			return fmt.Errorf("withdraw undeliverable question submission: %w", clearErr)
		}
	}
	return nil
}

func (apiServer *HelixAPIServer) handleQuestionResolved(sessionID string, syncMsg *types.SyncMessage) error {
	var event questionResolvedEvent
	if err := decodeSyncData(syncMsg.Data, &event); err != nil {
		return err
	}
	if event.RequestID == "" || event.TurnRequestID == "" {
		return errors.New("question_resolved requires request_id and turn_request_id")
	}
	if event.Outcome != "answered" && event.Outcome != "cancelled" {
		return fmt.Errorf("invalid question outcome %q", event.Outcome)
	}
	ctx := context.Background()
	interaction := apiServer.interactionForRequest(ctx, event.TurnRequestID)
	if interaction == nil {
		return fmt.Errorf("question turn request %q does not map to an interaction", event.TurnRequestID)
	}
	// Turns on agent-created threads belong to child sessions carried by
	// this (the parent's) connection; another lane's turn is refused.
	if err := apiServer.interactionOnConnection(ctx, sessionID, interaction); err != nil {
		return fmt.Errorf("question for interaction %q: %w", interaction.ID, err)
	}
	updated, changed, err := apiServer.Store.ResolveInteractionPendingQuestion(
		ctx, interaction.ID, interaction.GenerationID, event.RequestID, event.Outcome, event.Answers,
		func(submission *types.QuestionSubmission) types.TurnProvenance {
			return submission.Answerer(event.RequestID, event.SubmissionID, event.Outcome)
		},
	)
	if err != nil {
		return fmt.Errorf("resolve pending question: %w", err)
	}
	apiServer.finishQuestionAction(interaction.ID, event.RequestID)
	if !changed {
		return nil
	}
	return apiServer.publishQuestionInteractionUpdate(updated)
}

func (apiServer *HelixAPIServer) publishQuestionInteractionUpdate(interaction *types.Interaction) error {
	session, err := apiServer.Store.GetSession(context.Background(), interaction.SessionID)
	if err != nil {
		return fmt.Errorf("get session for question update: %w", err)
	}
	return apiServer.publishInteractionUpdateToFrontend(interaction.SessionID, session.Owner, interaction)
}
