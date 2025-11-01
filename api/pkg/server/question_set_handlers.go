package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	"github.com/sourcegraph/conc/pool"
)

func (s *HelixAPIServer) canAccessQuestionSet(ctx context.Context, user *types.User, questionSet *types.QuestionSet) bool {
	if user.ID == questionSet.UserID {
		return true
	}
	if user.Admin {
		return true
	}
	if questionSet.OrganizationID != "" {
		_, err := s.authorizeOrgMember(ctx, user, questionSet.OrganizationID)
		if err == nil {
			return true
		}
	}
	return false
}

// createQuestionSet godoc
// @Summary Create a new question set
// @Description Create a new question set
// @Tags question-sets
// @Accept json
// @Produce json
// @Param questionSet body types.QuestionSet true "Question set to create"
// @Success 201 {object} types.QuestionSet
// @Failure 400 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Router /api/v1/question-sets [post]
// @Security BearerAuth
func (s *HelixAPIServer) createQuestionSet(_ http.ResponseWriter, req *http.Request) (*types.QuestionSet, *system.HTTPError) {
	ctx := req.Context()
	user := getRequestUser(req)

	var questionSet types.QuestionSet
	if err := json.NewDecoder(req.Body).Decode(&questionSet); err != nil {
		return nil, system.NewHTTPError400(fmt.Sprintf("failed to decode request body: %s", err))
	}

	if questionSet.Name == "" {
		return nil, system.NewHTTPError400("name is required")
	}

	questionSet.UserID = user.ID

	orgID := req.URL.Query().Get("org_id")
	if orgID != "" {
		org, err := s.lookupOrg(ctx, orgID)
		if err != nil {
			return nil, system.NewHTTPError404(err.Error())
		}

		_, err = s.authorizeOrgMember(ctx, user, org.ID)
		if err != nil {
			return nil, system.NewHTTPError403(err.Error())
		}

		questionSet.OrganizationID = org.ID
	}

	created, err := s.Store.CreateQuestionSet(ctx, &questionSet)
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to create question set: %s", err))
	}

	return created, nil
}

// getQuestionSet godoc
// @Summary Get a question set by ID
// @Description Get a question set by ID
// @Tags question-sets
// @Produce json
// @Param id path string true "Question set ID"
// @Success 200 {object} types.QuestionSet
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Router /api/v1/question-sets/{id} [get]
// @Security BearerAuth
func (s *HelixAPIServer) getQuestionSet(_ http.ResponseWriter, req *http.Request) (*types.QuestionSet, *system.HTTPError) {
	ctx := req.Context()
	user := getRequestUser(req)
	id := mux.Vars(req)["id"]

	if id == "" {
		return nil, system.NewHTTPError400("question set id is required")
	}

	questionSet, err := s.Store.GetQuestionSet(ctx, id)
	if err != nil {
		if err == store.ErrNotFound {
			return nil, system.NewHTTPError404(fmt.Sprintf("question set not found: %s", id))
		}
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to get question set: %s", err))
	}

	if !s.canAccessQuestionSet(ctx, user, questionSet) {
		return nil, system.NewHTTPError403("you are not allowed to access this question set")
	}

	return questionSet, nil
}

// updateQuestionSet godoc
// @Summary Update a question set
// @Description Update a question set
// @Tags question-sets
// @Accept json
// @Produce json
// @Param id path string true "Question set ID"
// @Param questionSet body types.QuestionSet true "Question set to update"
// @Success 200 {object} types.QuestionSet
// @Failure 400 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Router /api/v1/question-sets/{id} [put]
// @Security BearerAuth
func (s *HelixAPIServer) updateQuestionSet(_ http.ResponseWriter, req *http.Request) (*types.QuestionSet, *system.HTTPError) {
	ctx := req.Context()
	user := getRequestUser(req)
	id := mux.Vars(req)["id"]

	if id == "" {
		return nil, system.NewHTTPError400("question set id is required")
	}

	existing, err := s.Store.GetQuestionSet(ctx, id)
	if err != nil {
		if err == store.ErrNotFound {
			return nil, system.NewHTTPError404(fmt.Sprintf("question set not found: %s", id))
		}
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to get question set: %s", err))
	}

	if !s.canAccessQuestionSet(ctx, user, existing) {
		return nil, system.NewHTTPError403("you are not allowed to update this question set")
	}

	var update types.QuestionSet
	if err := json.NewDecoder(req.Body).Decode(&update); err != nil {
		return nil, system.NewHTTPError400(fmt.Sprintf("failed to decode request body: %s", err))
	}

	update.ID = id
	update.UserID = existing.UserID
	update.OrganizationID = existing.OrganizationID
	update.Created = existing.Created

	updated, err := s.Store.UpdateQuestionSet(ctx, &update)
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to update question set: %s", err))
	}

	return updated, nil
}

// deleteQuestionSet godoc
// @Summary Delete a question set
// @Description Delete a question set
// @Tags question-sets
// @Param id path string true "Question set ID"
// @Success 204 "No Content"
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Router /api/v1/question-sets/{id} [delete]
// @Security BearerAuth
func (s *HelixAPIServer) deleteQuestionSet(_ http.ResponseWriter, req *http.Request) (*struct{}, *system.HTTPError) {
	ctx := req.Context()
	user := getRequestUser(req)
	id := mux.Vars(req)["id"]

	if id == "" {
		return nil, system.NewHTTPError400("question set id is required")
	}

	questionSet, err := s.Store.GetQuestionSet(ctx, id)
	if err != nil {
		if err == store.ErrNotFound {
			return nil, system.NewHTTPError404(fmt.Sprintf("question set not found: %s", id))
		}
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to get question set: %s", err))
	}

	if !s.canAccessQuestionSet(ctx, user, questionSet) {
		return nil, system.NewHTTPError403("you are not allowed to delete this question set")
	}

	err = s.Store.DeleteQuestionSet(ctx, id)
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to delete question set: %s", err))
	}

	log.Info().
		Str("question_set_id", id).
		Str("user_id", user.ID).Msg("question set deleted")

	return &struct{}{}, nil
}

// listQuestionSets godoc
// @Summary List question sets
// @Description List question sets for the current user or organization
// @Tags question-sets
// @Produce json
// @Param org_id query string false "Organization ID or slug"
// @Success 200 {array} types.QuestionSet
// @Failure 400 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Router /api/v1/question-sets [get]
// @Security BearerAuth
func (s *HelixAPIServer) listQuestionSets(_ http.ResponseWriter, req *http.Request) ([]*types.QuestionSet, *system.HTTPError) {
	ctx := req.Context()
	user := getRequestUser(req)

	listReq := &types.ListQuestionSetsRequest{}

	orgID := req.URL.Query().Get("org_id")
	if orgID != "" {
		org, err := s.lookupOrg(ctx, orgID)
		if err != nil {
			return nil, system.NewHTTPError404(err.Error())
		}

		_, err = s.authorizeOrgMember(ctx, user, org.ID)
		if err != nil {
			return nil, system.NewHTTPError403(err.Error())
		}

		listReq.OrganizationID = org.ID
	} else {
		listReq.UserID = user.ID
	}

	questionSets, err := s.Store.ListQuestionSets(ctx, listReq)
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to list question sets: %s", err))
	}

	return questionSets, nil
}

// executeQuestionSet godoc
// @Summary Execute a question set
// @Description Execute a question set, this is a blocking operation and will return a response for each question in the question set
// @Tags question-sets
// @Accept json
// @Produce json
// @Param id path string true "Question set ID"
// @Param executeQuestionSetRequest body types.ExecuteQuestionSetRequest true "Request to execute a question set"
// @Success 200 {object} types.ExecuteQuestionSetResponse
// @Failure 400 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Router /api/v1/question-sets/{id}/execute [post]
// @Security BearerAuth
func (s *HelixAPIServer) executeQuestionSet(_ http.ResponseWriter, req *http.Request) (*types.ExecuteQuestionSetResponse, *system.HTTPError) {
	ctx := req.Context()
	user := getRequestUser(req)
	id := mux.Vars(req)["id"]

	if id == "" {
		return nil, system.NewHTTPError400("question set id is required")
	}

	questionSet, err := s.Store.GetQuestionSet(ctx, id)
	if err != nil {
		if err == store.ErrNotFound {
			return nil, system.NewHTTPError404(fmt.Sprintf("question set not found: %s", id))
		}
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to get question set: %s", err))
	}

	if !s.canAccessQuestionSet(ctx, user, questionSet) {
		return nil, system.NewHTTPError403("you are not allowed to execute this question set")
	}

	var execReq types.ExecuteQuestionSetRequest
	if err := json.NewDecoder(req.Body).Decode(&execReq); err != nil {
		return nil, system.NewHTTPError400(fmt.Sprintf("failed to decode request body: %s", err))
	}

	execReq.QuestionSetID = id

	if execReq.AppID == "" {
		return nil, system.NewHTTPError400("app_id is required")
	}

	app, err := s.Store.GetAppWithTools(ctx, execReq.AppID)
	if err != nil {
		if err == store.ErrNotFound {
			return nil, system.NewHTTPError404(fmt.Sprintf("app not found: %s", execReq.AppID))
		}
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to get app: %s", err))
	}

	if len(questionSet.Questions) == 0 {
		return &types.ExecuteQuestionSetResponse{Questions: []types.QuestionResponse{}}, nil
	}

	responses := make([]types.QuestionResponse, len(questionSet.Questions))
	var mu sync.Mutex

	p := pool.New().WithMaxGoroutines(20)

	for i, question := range questionSet.Questions {
		idx := i
		q := question
		p.Go(func() {
			sessionCtx := context.Background()
			sessionID := system.GenerateSessionID()
			session := &types.Session{
				ID:             sessionID,
				Name:           fmt.Sprintf("Question: %s", q.Question),
				Created:        time.Now(),
				Updated:        time.Now(),
				Mode:           types.SessionModeInference,
				Type:           types.SessionTypeText,
				ParentApp:      execReq.AppID,
				OrganizationID: app.OrganizationID,
				Owner:          user.ID,
				OwnerType:      user.Type,
				QuestionSetID:  questionSet.ID,
				Metadata: types.SessionMetadata{
					Stream:       false,
					SystemPrompt: "",
					HelixVersion: data.GetHelixVersion(),
					AgentType:    "helix",
				},
			}

			if app != nil {
				if len(app.Config.Helix.Assistants) > 0 {
					session.Metadata.SystemPrompt = app.Config.Helix.Assistants[0].SystemPrompt
				}
			}

			sessionCtx = openai.SetContextSessionID(sessionCtx, sessionID)
			if app.OrganizationID != "" {
				sessionCtx = openai.SetContextOrganizationID(sessionCtx, app.OrganizationID)
			}
			if execReq.AppID != "" {
				sessionCtx = openai.SetContextAppID(sessionCtx, execReq.AppID)
			}

			err := s.Controller.WriteSession(sessionCtx, session)
			if err != nil {
				mu.Lock()
				responses[idx] = types.QuestionResponse{
					QuestionID: q.ID,
					Question:   q.Question,
					SessionID:  sessionID,
					Error:      fmt.Sprintf("failed to create session: %s", err),
				}
				mu.Unlock()
				return
			}

			interaction, err := s.Controller.RunBlockingSession(sessionCtx, &controller.RunSessionRequest{
				OrganizationID: app.OrganizationID,
				App:            app,
				Session:        session,
				User:           user,
				PromptMessage:  types.MessageContent{Parts: []any{q.Question}},
				HistoryLimit:   -1,
			})

			mu.Lock()
			if err != nil {
				responses[idx] = types.QuestionResponse{
					QuestionID: q.ID,
					Question:   q.Question,
					SessionID:  sessionID,
					Error:      err.Error(),
				}
			} else {
				responses[idx] = types.QuestionResponse{
					QuestionID:    q.ID,
					Question:      q.Question,
					SessionID:     sessionID,
					InteractionID: interaction.ID,
					Response:      interaction.ResponseMessage,
				}
			}
			mu.Unlock()
		})
	}

	p.Wait()

	return &types.ExecuteQuestionSetResponse{Questions: responses}, nil
}
