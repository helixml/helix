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
	if interaction.SessionID != sessionID {
		return fmt.Errorf("question session %q does not match interaction session %q", sessionID, interaction.SessionID)
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
	if interaction.SessionID != sessionID {
		return fmt.Errorf("question session %q does not match interaction session %q", sessionID, interaction.SessionID)
	}
	updated, changed, err := apiServer.Store.ResolveInteractionPendingQuestion(
		ctx, interaction.ID, interaction.GenerationID, event.RequestID, event.Outcome, event.Answers,
	)
	if err != nil {
		return fmt.Errorf("resolve pending question: %w", err)
	}
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
