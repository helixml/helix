package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/agent/evaluation"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// --- Authorization helper ---

func (s *HelixAPIServer) canAccessEvaluationSuite(ctx context.Context, user *types.User, suite *types.EvaluationSuite) bool {
	if user.ID == suite.UserID {
		return true
	}
	if user.Admin {
		return true
	}
	if suite.OrganizationID != "" {
		_, err := s.authorizeOrgMember(ctx, user, suite.OrganizationID)
		if err == nil {
			return true
		}
	}
	return false
}

// --- Evaluation Suite CRUD ---

// createEvaluationSuite godoc
// @Summary Create an evaluation suite
// @Description Create a new evaluation suite for an agent
// @Tags evaluations
// @Accept json
// @Produce json
// @Param app_id path string true "App ID"
// @Param suite body types.EvaluationSuite true "Evaluation suite to create"
// @Success 201 {object} types.EvaluationSuite
// @Failure 400 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Router /api/v1/apps/{app_id}/evaluation-suites [post]
// @Security BearerAuth
func (s *HelixAPIServer) createEvaluationSuite(_ http.ResponseWriter, req *http.Request) (*types.EvaluationSuite, *system.HTTPError) {
	ctx := req.Context()
	user := getRequestUser(req)
	appID := mux.Vars(req)["app_id"]

	app, httpErr := s.getAppForEvaluation(ctx, user, appID)
	if httpErr != nil {
		return nil, httpErr
	}

	var suite types.EvaluationSuite
	if err := json.NewDecoder(req.Body).Decode(&suite); err != nil {
		return nil, system.NewHTTPError400(fmt.Sprintf("failed to decode request body: %s", err))
	}

	if suite.Name == "" {
		return nil, system.NewHTTPError400("name is required")
	}

	suite.UserID = user.ID
	suite.AppID = appID
	suite.OrganizationID = app.OrganizationID

	// Ensure question IDs are set
	for i := range suite.Questions {
		if suite.Questions[i].ID == "" {
			suite.Questions[i].ID = system.GenerateID()
		}
	}

	created, err := s.Store.CreateEvaluationSuite(ctx, &suite)
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to create evaluation suite: %s", err))
	}

	return created, nil
}

// getEvaluationSuite godoc
// @Summary Get an evaluation suite
// @Description Get an evaluation suite by ID
// @Tags evaluations
// @Produce json
// @Param app_id path string true "App ID"
// @Param id path string true "Suite ID"
// @Success 200 {object} types.EvaluationSuite
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Router /api/v1/apps/{app_id}/evaluation-suites/{id} [get]
// @Security BearerAuth
func (s *HelixAPIServer) getEvaluationSuite(_ http.ResponseWriter, req *http.Request) (*types.EvaluationSuite, *system.HTTPError) {
	ctx := req.Context()
	user := getRequestUser(req)
	id := mux.Vars(req)["id"]

	suite, err := s.Store.GetEvaluationSuite(ctx, id)
	if err != nil {
		if err == store.ErrNotFound {
			return nil, system.NewHTTPError404("evaluation suite not found")
		}
		return nil, system.NewHTTPError500(err.Error())
	}

	if !s.canAccessEvaluationSuite(ctx, user, suite) {
		return nil, system.NewHTTPError403("access denied")
	}

	return suite, nil
}

// updateEvaluationSuite godoc
// @Summary Update an evaluation suite
// @Description Update an evaluation suite
// @Tags evaluations
// @Accept json
// @Produce json
// @Param app_id path string true "App ID"
// @Param id path string true "Suite ID"
// @Param suite body types.EvaluationSuite true "Updated suite"
// @Success 200 {object} types.EvaluationSuite
// @Failure 400 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Router /api/v1/apps/{app_id}/evaluation-suites/{id} [put]
// @Security BearerAuth
func (s *HelixAPIServer) updateEvaluationSuite(_ http.ResponseWriter, req *http.Request) (*types.EvaluationSuite, *system.HTTPError) {
	ctx := req.Context()
	user := getRequestUser(req)
	id := mux.Vars(req)["id"]

	existing, err := s.Store.GetEvaluationSuite(ctx, id)
	if err != nil {
		if err == store.ErrNotFound {
			return nil, system.NewHTTPError404("evaluation suite not found")
		}
		return nil, system.NewHTTPError500(err.Error())
	}

	if !s.canAccessEvaluationSuite(ctx, user, existing) {
		return nil, system.NewHTTPError403("access denied")
	}

	var suite types.EvaluationSuite
	if err := json.NewDecoder(req.Body).Decode(&suite); err != nil {
		return nil, system.NewHTTPError400(fmt.Sprintf("failed to decode request body: %s", err))
	}

	suite.ID = id
	suite.UserID = existing.UserID
	suite.AppID = existing.AppID
	suite.OrganizationID = existing.OrganizationID

	// Ensure question IDs are set
	for i := range suite.Questions {
		if suite.Questions[i].ID == "" {
			suite.Questions[i].ID = system.GenerateID()
		}
	}

	updated, err := s.Store.UpdateEvaluationSuite(ctx, &suite)
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to update evaluation suite: %s", err))
	}

	return updated, nil
}

// deleteEvaluationSuite godoc
// @Summary Delete an evaluation suite
// @Description Delete an evaluation suite
// @Tags evaluations
// @Param app_id path string true "App ID"
// @Param id path string true "Suite ID"
// @Success 200 {object} map[string]string
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Router /api/v1/apps/{app_id}/evaluation-suites/{id} [delete]
// @Security BearerAuth
func (s *HelixAPIServer) deleteEvaluationSuite(_ http.ResponseWriter, req *http.Request) (map[string]string, *system.HTTPError) {
	ctx := req.Context()
	user := getRequestUser(req)
	id := mux.Vars(req)["id"]

	suite, err := s.Store.GetEvaluationSuite(ctx, id)
	if err != nil {
		if err == store.ErrNotFound {
			return nil, system.NewHTTPError404("evaluation suite not found")
		}
		return nil, system.NewHTTPError500(err.Error())
	}

	if !s.canAccessEvaluationSuite(ctx, user, suite) {
		return nil, system.NewHTTPError403("access denied")
	}

	if err := s.Store.DeleteEvaluationSuite(ctx, id); err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to delete evaluation suite: %s", err))
	}

	return map[string]string{"status": "deleted"}, nil
}

// listEvaluationSuites godoc
// @Summary List evaluation suites for an app
// @Description List all evaluation suites for an app
// @Tags evaluations
// @Produce json
// @Param app_id path string true "App ID"
// @Success 200 {array} types.EvaluationSuite
// @Failure 403 {object} system.HTTPError
// @Router /api/v1/apps/{app_id}/evaluation-suites [get]
// @Security BearerAuth
func (s *HelixAPIServer) listEvaluationSuites(_ http.ResponseWriter, req *http.Request) ([]*types.EvaluationSuite, *system.HTTPError) {
	ctx := req.Context()
	user := getRequestUser(req)
	appID := mux.Vars(req)["app_id"]

	_, httpErr := s.getAppForEvaluation(ctx, user, appID)
	if httpErr != nil {
		return nil, httpErr
	}

	suites, err := s.Store.ListEvaluationSuites(ctx, &types.ListEvaluationSuitesRequest{
		AppID: appID,
	})
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	return suites, nil
}

// --- Evaluation Runs ---

// activeEvaluationRuns tracks in-progress runs for SSE streaming
var (
	activeRunsMu       sync.RWMutex
	activeRunListeners = map[string][]chan types.EvaluationRunProgress{}
)

func broadcastRunProgress(runID string, progress types.EvaluationRunProgress) {
	activeRunsMu.RLock()
	listeners := activeRunListeners[runID]
	activeRunsMu.RUnlock()

	for _, ch := range listeners {
		select {
		case ch <- progress:
		default:
			// listener too slow, skip
		}
	}
}

func addRunListener(runID string) chan types.EvaluationRunProgress {
	ch := make(chan types.EvaluationRunProgress, 32)
	activeRunsMu.Lock()
	activeRunListeners[runID] = append(activeRunListeners[runID], ch)
	activeRunsMu.Unlock()
	return ch
}

func removeRunListener(runID string, ch chan types.EvaluationRunProgress) {
	activeRunsMu.Lock()
	defer activeRunsMu.Unlock()
	listeners := activeRunListeners[runID]
	for i, l := range listeners {
		if l == ch {
			activeRunListeners[runID] = append(listeners[:i], listeners[i+1:]...)
			break
		}
	}
	if len(activeRunListeners[runID]) == 0 {
		delete(activeRunListeners, runID)
	}
	close(ch)
}

// startEvaluationRun godoc
// @Summary Start an evaluation run
// @Description Start running an evaluation suite against an agent
// @Tags evaluations
// @Accept json
// @Produce json
// @Param app_id path string true "App ID"
// @Param id path string true "Suite ID"
// @Success 200 {object} types.EvaluationRun
// @Failure 400 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Router /api/v1/apps/{app_id}/evaluation-suites/{id}/runs [post]
// @Security BearerAuth
func (s *HelixAPIServer) startEvaluationRun(_ http.ResponseWriter, req *http.Request) (*types.EvaluationRun, *system.HTTPError) {
	ctx := req.Context()
	user := getRequestUser(req)
	appID := mux.Vars(req)["app_id"]
	suiteID := mux.Vars(req)["id"]

	app, httpErr := s.getAppForEvaluation(ctx, user, appID)
	if httpErr != nil {
		return nil, httpErr
	}

	suite, err := s.Store.GetEvaluationSuite(ctx, suiteID)
	if err != nil {
		if err == store.ErrNotFound {
			return nil, system.NewHTTPError404("evaluation suite not found")
		}
		return nil, system.NewHTTPError500(err.Error())
	}

	if !s.canAccessEvaluationSuite(ctx, user, suite) {
		return nil, system.NewHTTPError403("access denied")
	}

	if len(suite.Questions) == 0 {
		return nil, system.NewHTTPError400("suite has no questions")
	}

	run := &types.EvaluationRun{
		SuiteID:           suiteID,
		AppID:             appID,
		UserID:            user.ID,
		OrganizationID:    app.OrganizationID,
		Status:            types.EvaluationRunStatusPending,
		AppConfigSnapshot: app.Config,
	}

	createdRun, err := s.Store.CreateEvaluationRun(ctx, run)
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to create evaluation run: %s", err))
	}

	// Run evaluation in background
	go func() {
		runCtx := context.Background()
		runnerCfg := &evaluation.RunnerConfig{
			Store:      s.Store,
			Controller: s.Controller,
		}

		evaluation.RunEvaluation(runCtx, runnerCfg, createdRun, suite, app, user, func(progress types.EvaluationRunProgress) {
			broadcastRunProgress(createdRun.ID, progress)
		})
	}()

	return createdRun, nil
}

// listEvaluationRuns godoc
// @Summary List evaluation runs
// @Description List evaluation runs for a suite
// @Tags evaluations
// @Produce json
// @Param app_id path string true "App ID"
// @Param id path string true "Suite ID"
// @Success 200 {array} types.EvaluationRun
// @Failure 403 {object} system.HTTPError
// @Router /api/v1/apps/{app_id}/evaluation-suites/{id}/runs [get]
// @Security BearerAuth
func (s *HelixAPIServer) listEvaluationRuns(_ http.ResponseWriter, req *http.Request) ([]*types.EvaluationRun, *system.HTTPError) {
	ctx := req.Context()
	user := getRequestUser(req)
	appID := mux.Vars(req)["app_id"]
	suiteID := mux.Vars(req)["id"]

	_, httpErr := s.getAppForEvaluation(ctx, user, appID)
	if httpErr != nil {
		return nil, httpErr
	}

	runs, err := s.Store.ListEvaluationRuns(ctx, &types.ListEvaluationRunsRequest{
		SuiteID: suiteID,
		AppID:   appID,
		Limit:   50,
	})
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	_ = user // used for auth via getAppForEvaluation
	return runs, nil
}

// getEvaluationRun godoc
// @Summary Get an evaluation run
// @Description Get evaluation run details
// @Tags evaluations
// @Produce json
// @Param app_id path string true "App ID"
// @Param run_id path string true "Run ID"
// @Success 200 {object} types.EvaluationRun
// @Failure 404 {object} system.HTTPError
// @Router /api/v1/apps/{app_id}/evaluation-runs/{run_id} [get]
// @Security BearerAuth
func (s *HelixAPIServer) getEvaluationRun(_ http.ResponseWriter, req *http.Request) (*types.EvaluationRun, *system.HTTPError) {
	ctx := req.Context()
	user := getRequestUser(req)
	runID := mux.Vars(req)["run_id"]
	appID := mux.Vars(req)["app_id"]

	_, httpErr := s.getAppForEvaluation(ctx, user, appID)
	if httpErr != nil {
		return nil, httpErr
	}

	run, err := s.Store.GetEvaluationRun(ctx, runID)
	if err != nil {
		if err == store.ErrNotFound {
			return nil, system.NewHTTPError404("evaluation run not found")
		}
		return nil, system.NewHTTPError500(err.Error())
	}

	return run, nil
}

// deleteEvaluationRun godoc
// @Summary Delete an evaluation run
// @Description Delete an evaluation run
// @Tags evaluations
// @Param app_id path string true "App ID"
// @Param run_id path string true "Run ID"
// @Success 200 {object} map[string]string
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Router /api/v1/apps/{app_id}/evaluation-runs/{run_id} [delete]
// @Security BearerAuth
func (s *HelixAPIServer) deleteEvaluationRun(_ http.ResponseWriter, req *http.Request) (map[string]string, *system.HTTPError) {
	ctx := req.Context()
	user := getRequestUser(req)
	runID := mux.Vars(req)["run_id"]
	appID := mux.Vars(req)["app_id"]

	_, httpErr := s.getAppForEvaluation(ctx, user, appID)
	if httpErr != nil {
		return nil, httpErr
	}

	if err := s.Store.DeleteEvaluationRun(ctx, runID); err != nil {
		if err == store.ErrNotFound {
			return nil, system.NewHTTPError404("evaluation run not found")
		}
		return nil, system.NewHTTPError500(err.Error())
	}

	return map[string]string{"status": "deleted"}, nil
}

// streamEvaluationRun streams SSE progress for an in-progress evaluation run
func (s *HelixAPIServer) streamEvaluationRun(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	user := getRequestUser(req)
	runID := mux.Vars(req)["run_id"]
	appID := mux.Vars(req)["app_id"]

	_, httpErr := s.getAppForEvaluation(ctx, user, appID)
	if httpErr != nil {
		http.Error(w, httpErr.Message, httpErr.StatusCode)
		return
	}

	// Check run exists
	run, err := s.Store.GetEvaluationRun(ctx, runID)
	if err != nil {
		if err == store.ErrNotFound {
			http.Error(w, "run not found", http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	// If already completed, send the final state and close
	if run.Status == types.EvaluationRunStatusCompleted || run.Status == types.EvaluationRunStatusFailed || run.Status == types.EvaluationRunStatusCancelled {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		progress := types.EvaluationRunProgress{
			RunID:          run.ID,
			Status:         run.Status,
			TotalQuestions: run.Summary.TotalQuestions,
			CurrentQuestion: run.Summary.TotalQuestions,
			Summary:        &run.Summary,
			Error:          run.Error,
		}
		data, _ := json.Marshal(progress)
		fmt.Fprintf(w, "data: %s\n\n", data)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := addRunListener(runID)
	defer removeRunListener(runID, ch)

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case progress, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(progress)
			if err != nil {
				log.Error().Err(err).Msg("failed to marshal evaluation progress")
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()

			// Close stream when run finishes
			if progress.Status == types.EvaluationRunStatusCompleted ||
				progress.Status == types.EvaluationRunStatusFailed ||
				progress.Status == types.EvaluationRunStatusCancelled {
				return
			}
		}
	}
}

// --- Helper ---

func (s *HelixAPIServer) getAppForEvaluation(ctx context.Context, user *types.User, appID string) (*types.App, *system.HTTPError) {
	if appID == "" {
		return nil, system.NewHTTPError400("app_id is required")
	}

	app, err := s.Store.GetAppWithTools(ctx, appID)
	if err != nil {
		if err == store.ErrNotFound {
			return nil, system.NewHTTPError404("app not found")
		}
		return nil, system.NewHTTPError500(err.Error())
	}

	// Check user can access the app
	if app.Owner != user.ID && !user.Admin {
		if app.OrganizationID != "" {
			_, err := s.authorizeOrgMember(ctx, user, app.OrganizationID)
			if err != nil {
				return nil, system.NewHTTPError403("access denied")
			}
		} else {
			return nil, system.NewHTTPError403("access denied")
		}
	}

	return app, nil
}
