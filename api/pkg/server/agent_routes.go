package server

import (
	"net/http"

	"github.com/gorilla/mux"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

func (apiServer *HelixAPIServer) registerAgentRoutes(authRouter, insecureRouter *mux.Router) {
	for _, resource := range []string{"agents", "apps"} {
		base := "/" + resource
		authRouter.HandleFunc(base, wrapWithETag[[]*types.Agent](apiServer.listAgents)).Methods(http.MethodGet)
		authRouter.HandleFunc(base, system.Wrapper(apiServer.createAgent)).Methods(http.MethodPost)
		authRouter.HandleFunc(base+"/{id}", system.Wrapper(apiServer.getAgent)).Methods(http.MethodGet)
		authRouter.HandleFunc(base+"/{id}", system.Wrapper(apiServer.updateAgent)).Methods(http.MethodPut)
		authRouter.HandleFunc(base+"/{id}", system.Wrapper(apiServer.deleteAgent)).Methods(http.MethodDelete)
		authRouter.HandleFunc(base+"/{id}/claude-subscription-status", system.Wrapper(apiServer.getAppClaudeSubscriptionStatus)).Methods(http.MethodGet)
		authRouter.HandleFunc(base+"/{id}/daily-usage", system.Wrapper(apiServer.getAppDailyUsage)).Methods(http.MethodGet)
		authRouter.HandleFunc(base+"/{id}/users-daily-usage", system.Wrapper(apiServer.getAppUsersDailyUsage)).Methods(http.MethodGet)
		authRouter.HandleFunc(base+"/{id}/llm-calls", system.Wrapper(apiServer.listAppLLMCalls)).Methods(http.MethodGet)
		authRouter.HandleFunc(base+"/{id}/interactions", system.Wrapper(apiServer.listAppInteractions)).Methods(http.MethodGet)
		authRouter.HandleFunc(base+"/{id}/step-info", system.Wrapper(apiServer.listAppStepInfo)).Methods(http.MethodGet)
		authRouter.HandleFunc(base+"/{id}/api-actions", system.Wrapper(apiServer.appRunAPIAction)).Methods(http.MethodPost)
		authRouter.HandleFunc(base+"/{id}/user-access", system.Wrapper(apiServer.getAppUserAccess)).Methods(http.MethodGet)
		authRouter.HandleFunc(base+"/{id}/access-grants", apiServer.listAppAccessGrants).Methods(http.MethodGet)
		authRouter.HandleFunc(base+"/{id}/access-grants", apiServer.createAppAccessGrant).Methods(http.MethodPost)
		authRouter.HandleFunc(base+"/{id}/access-grants/{grant_id}", apiServer.deleteAppAccessGrant).Methods(http.MethodDelete)
		authRouter.HandleFunc(base+"/{id}/duplicate", system.Wrapper(apiServer.duplicateApp)).Methods(http.MethodPost)
		authRouter.HandleFunc(base+"/{id}/memories", system.Wrapper(apiServer.listAppMemories)).Methods(http.MethodGet)
		authRouter.HandleFunc(base+"/{id}/memories/{memory_id}", system.Wrapper(apiServer.deleteAppMemory)).Methods(http.MethodDelete)
		authRouter.HandleFunc(base+"/{agent_id}/triggers", system.Wrapper(apiServer.listAppTriggers)).Methods(http.MethodGet)
		authRouter.HandleFunc(base+"/{id}/avatar", apiServer.uploadAppAvatar).Methods(http.MethodPost)
		authRouter.HandleFunc(base+"/{id}/avatar", apiServer.deleteAppAvatar).Methods(http.MethodDelete)
		insecureRouter.HandleFunc(base+"/{id}/avatar", apiServer.getAppAvatar).Methods(http.MethodGet)
		authRouter.HandleFunc(base+"/{id}/trigger-status", apiServer.getAppTriggerStatus).Methods(http.MethodGet)
		authRouter.HandleFunc(base+"/{id}/skills/{skill}/enable", system.Wrapper(apiServer.handleEnableSkill)).Methods(http.MethodPost)

		authRouter.HandleFunc(base+"/{agent_id}/evaluation-suites", system.Wrapper(apiServer.listEvaluationSuites)).Methods(http.MethodGet)
		authRouter.HandleFunc(base+"/{agent_id}/evaluation-suites", system.Wrapper(apiServer.createEvaluationSuite)).Methods(http.MethodPost)
		authRouter.HandleFunc(base+"/{agent_id}/evaluation-suites/{id}", system.Wrapper(apiServer.getEvaluationSuite)).Methods(http.MethodGet)
		authRouter.HandleFunc(base+"/{agent_id}/evaluation-suites/{id}", system.Wrapper(apiServer.updateEvaluationSuite)).Methods(http.MethodPut)
		authRouter.HandleFunc(base+"/{agent_id}/evaluation-suites/{id}", system.Wrapper(apiServer.deleteEvaluationSuite)).Methods(http.MethodDelete)
		authRouter.HandleFunc(base+"/{agent_id}/evaluation-suites/{id}/runs", system.Wrapper(apiServer.startEvaluationRun)).Methods(http.MethodPost)
		authRouter.HandleFunc(base+"/{agent_id}/evaluation-suites/{id}/runs", system.Wrapper(apiServer.listEvaluationRuns)).Methods(http.MethodGet)
		authRouter.HandleFunc(base+"/{agent_id}/evaluation-runs/{run_id}", system.Wrapper(apiServer.getEvaluationRun)).Methods(http.MethodGet)
		authRouter.HandleFunc(base+"/{agent_id}/evaluation-runs/{run_id}", system.Wrapper(apiServer.deleteEvaluationRun)).Methods(http.MethodDelete)
		authRouter.HandleFunc(base+"/{agent_id}/evaluation-runs/{run_id}/stream", apiServer.streamEvaluationRun).Methods(http.MethodGet)
	}
}
