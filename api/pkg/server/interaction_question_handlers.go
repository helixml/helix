package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

const maxQuestionResponseBody = 64 * 1024

// respondToInteractionQuestion godoc
// @Summary Respond to an agent question
// @Description Sends answers to the agent question currently pending on an interaction
// @Tags interactions
// @Accept json
// @Produce json
// @Param interaction_id path string true "Interaction ID"
// @Param request_id path string true "Question request ID"
// @Param request body types.QuestionRespondRequest true "Question answers"
// @Success 200 {object} types.QuestionActionResponse
// @Router /api/v1/interactions/{interaction_id}/questions/{request_id}/respond [post]
// @Security BearerAuth
func (s *HelixAPIServer) respondToInteractionQuestion(w http.ResponseWriter, req *http.Request) (*types.QuestionActionResponse, *system.HTTPError) {
	interaction, question, response, httpErr := s.loadAuthorizedPendingQuestion(req)
	if httpErr != nil || response != nil {
		return response, httpErr
	}

	req.Body = http.MaxBytesReader(w, req.Body, maxQuestionResponseBody)
	var body types.QuestionRespondRequest
	decoder := json.NewDecoder(req.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		return nil, system.NewHTTPError400("invalid question response")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, system.NewHTTPError400("invalid question response")
	}
	if err := validateQuestionAnswers(question, body.Answers); err != nil {
		return nil, system.NewHTTPError400(err.Error())
	}
	if err := s.sendCommandToExternalAgent(interaction.SessionID, types.ExternalAgentCommand{
		Type: "respond_question",
		Data: map[string]interface{}{
			"request_id": question.RequestID,
			"answers":    body.Answers,
		},
	}); err != nil {
		return nil, system.NewHTTPError409("agent is not available to receive the answer")
	}
	return &types.QuestionActionResponse{Status: "accepted"}, nil
}

// cancelInteractionQuestion godoc
// @Summary Cancel an agent question
// @Description Cancels the agent question currently pending on an interaction
// @Tags interactions
// @Produce json
// @Param interaction_id path string true "Interaction ID"
// @Param request_id path string true "Question request ID"
// @Success 200 {object} types.QuestionActionResponse
// @Router /api/v1/interactions/{interaction_id}/questions/{request_id}/cancel [post]
// @Security BearerAuth
func (s *HelixAPIServer) cancelInteractionQuestion(_ http.ResponseWriter, req *http.Request) (*types.QuestionActionResponse, *system.HTTPError) {
	interaction, question, response, httpErr := s.loadAuthorizedPendingQuestion(req)
	if httpErr != nil || response != nil {
		return response, httpErr
	}
	if err := s.sendCommandToExternalAgent(interaction.SessionID, types.ExternalAgentCommand{
		Type: "cancel_question",
		Data: map[string]interface{}{
			"request_id": question.RequestID,
		},
	}); err != nil {
		return nil, system.NewHTTPError409("agent is not available to cancel the question")
	}
	return &types.QuestionActionResponse{Status: "accepted"}, nil
}

func (s *HelixAPIServer) loadAuthorizedPendingQuestion(req *http.Request) (*types.Interaction, *types.PendingQuestion, *types.QuestionActionResponse, *system.HTTPError) {
	ctx := req.Context()
	interactionID := mux.Vars(req)["interaction_id"]
	requestID := mux.Vars(req)["request_id"]
	interaction, err := s.Store.GetInteraction(ctx, interactionID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil, nil, system.NewHTTPError404("interaction not found")
		}
		return nil, nil, nil, system.NewHTTPError500("failed to load interaction")
	}
	session, err := s.Store.GetSession(ctx, interaction.SessionID)
	if err != nil {
		return nil, nil, nil, system.NewHTTPError500("failed to load session")
	}
	if err := s.authorizeUserToSession(ctx, getRequestUser(req), session, types.ActionUpdate); err != nil {
		return nil, nil, nil, system.NewHTTPError403("you are not allowed to answer this question")
	}
	if interaction.PendingQuestion != nil && interaction.PendingQuestion.RequestID == requestID {
		return interaction, interaction.PendingQuestion, nil, nil
	}
	for _, resolved := range interaction.QuestionHistory {
		if resolved.RequestID == requestID {
			return nil, nil, &types.QuestionActionResponse{Status: "resolved"}, nil
		}
	}
	return nil, nil, nil, system.NewHTTPError409("question is no longer pending")
}

func validateQuestionAnswers(question *types.PendingQuestion, answers map[string]string) error {
	if len(answers) != len(question.Questions) {
		return errors.New("an answer is required for every question")
	}
	questions := make(map[string]types.UserQuestion, len(question.Questions))
	for _, item := range question.Questions {
		questions[item.ID] = item
	}
	for id, answer := range answers {
		item, ok := questions[id]
		if !ok {
			return fmt.Errorf("unknown question id %q", id)
		}
		if strings.TrimSpace(answer) == "" {
			return fmt.Errorf("answer for question %q is required", id)
		}
		if strings.ContainsRune(answer, '\x00') {
			return fmt.Errorf("answer for question %q contains an invalid null character", id)
		}
		if len(answer) > maxAgentQuestionText {
			return fmt.Errorf("answer for question %q is too long", id)
		}
		if !item.AllowCustomAnswer && !answerMatchesQuestionOptions(item, answer) {
			return fmt.Errorf("answer for question %q is not an available option", id)
		}
	}
	return nil
}

func answerMatchesQuestionOptions(question types.UserQuestion, answer string) bool {
	allowed := make(map[string]struct{}, len(question.Options))
	for _, option := range question.Options {
		allowed[option.Label] = struct{}{}
	}
	answers := []string{answer}
	if question.MultiSelect {
		answers = strings.Split(answer, "\n")
	}
	for _, value := range answers {
		if _, ok := allowed[value]; !ok {
			return false
		}
	}
	return true
}
