package api

import "net/http"

// @Summary Helix-org: list agents
// @Description List the canonical Agents in an organization, including their instructions, tools, runtime, model configuration, and reporting lines.
// @Tags HelixOrg
// @Produce json
// @Param org path string true "Organization slug or ID"
// @Success 200 {array} api.BotDTO
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/agents [get]
func (a *apiHandler) listAgents(w http.ResponseWriter, r *http.Request) {
	a.listBots(w, r)
}

// @Summary Helix-org: create an agent
// @Description Create a canonical Agent with its org-chart position, communication topics, tools, and Agent App configuration.
// @Tags HelixOrg
// @Accept json
// @Produce json
// @Param org path string true "Organization slug or ID"
// @Param payload body api.CreateBotRequest true "Agent specification"
// @Success 201 {object} api.CreateBotResponse
// @Failure 400 {object} api.ErrorResponse
// @Failure 501 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/agents [post]
func (a *apiHandler) createAgent(w http.ResponseWriter, r *http.Request) {
	a.createBot(w, r)
}

// @Summary Helix-org: update an agent
// @Description Update the canonical Agent instructions, tools, project access, runtime, provider, model, or reasoning configuration.
// @Tags HelixOrg
// @Accept json
// @Param org path string true "Organization slug or ID"
// @Param id path string true "Agent ID"
// @Param payload body api.UpdateBotRequest true "Agent fields to update"
// @Success 200 {object} api.BotDTO
// @Failure 404 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/agents/{id} [patch]
func (a *apiHandler) updateAgent(w http.ResponseWriter, r *http.Request) {
	a.updateBot(w, r)
}

// @Summary Helix-org: delete an agent
// @Description Delete an Agent after archiving its runtime-owned project and deleting its Agent App, knowledge, runtime state, subscriptions, reporting lines, and org-chart row. Repositories are preserved.
// @Tags HelixOrg
// @Param org path string true "Organization slug or ID"
// @Param id path string true "Agent ID"
// @Success 204
// @Failure 404 {object} api.ErrorResponse
// @Failure 409 {object} api.ErrorResponse
// @Failure 501 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/agents/{id} [delete]
func (a *apiHandler) deleteAgent(w http.ResponseWriter, r *http.Request) {
	a.deleteBot(w, r)
}

// @Summary Helix-org: add an agent manager
// @Tags HelixOrg
// @Accept json
// @Param org path string true "Organization slug or ID"
// @Param id path string true "Agent ID of the direct report"
// @Param payload body api.AddBotParentRequest true "Manager Agent ID"
// @Success 204
// @Failure 400 {object} api.ErrorResponse
// @Failure 404 {object} api.ErrorResponse
// @Failure 409 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/agents/{id}/parents [post]
func (a *apiHandler) addAgentParent(w http.ResponseWriter, r *http.Request) {
	a.addBotParent(w, r)
}

// @Summary Helix-org: remove an agent manager
// @Tags HelixOrg
// @Param org path string true "Organization slug or ID"
// @Param id path string true "Agent ID of the direct report"
// @Param parent_id path string true "Manager Agent ID"
// @Success 204
// @Failure 400 {object} api.ErrorResponse
// @Failure 404 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/agents/{id}/parents/{parent_id} [delete]
func (a *apiHandler) removeAgentParent(w http.ResponseWriter, r *http.Request) {
	a.removeBotParent(w, r)
}

// @Summary Helix-org: provision an agent chat
// @Tags HelixOrg
// @Param org path string true "Organization slug or ID"
// @Param id path string true "Agent ID"
// @Success 200 {object} api.BotChatDTO
// @Failure 404 {object} api.ErrorResponse
// @Failure 501 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/agents/{id}/chat [post]
func (a *apiHandler) ensureAgentChat(w http.ResponseWriter, r *http.Request) {
	a.ensureBotChat(w, r)
}

// @Summary Helix-org: activate an agent
// @Tags HelixOrg
// @Param org path string true "Organization slug or ID"
// @Param id path string true "Agent ID"
// @Success 202 {object} api.BotActivateDTO
// @Failure 404 {object} api.ErrorResponse
// @Failure 501 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/agents/{id}/activate [post]
func (a *apiHandler) activateAgent(w http.ResponseWriter, r *http.Request) {
	a.activateBot(w, r)
}

// @Summary Helix-org: stop an agent desktop
// @Tags HelixOrg
// @Param org path string true "Organization slug or ID"
// @Param id path string true "Agent ID"
// @Success 204
// @Failure 404 {object} api.ErrorResponse
// @Failure 501 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/agents/{id}/stop-agent [post]
func (a *apiHandler) stopAgent(w http.ResponseWriter, r *http.Request) {
	a.stopBotAgent(w, r)
}

// @Summary Helix-org: restart an agent session
// @Tags HelixOrg
// @Param org path string true "Organization slug or ID"
// @Param id path string true "Agent ID"
// @Success 202 {object} api.BotActivateDTO
// @Failure 404 {object} api.ErrorResponse
// @Failure 500 {object} api.ErrorResponse
// @Failure 501 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/agents/{id}/restart-agent [post]
func (a *apiHandler) restartAgent(w http.ResponseWriter, r *http.Request) {
	a.restartBotAgent(w, r)
}

// @Summary Helix-org: list an agent's subscriptions
// @Tags HelixOrg
// @Param org path string true "Organization slug or ID"
// @Param id path string true "Agent ID"
// @Success 200 {object} api.BotSubscriptionsResponse
// @Failure 404 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/agents/{id}/subscriptions [get]
func (a *apiHandler) listAgentSubscriptions(w http.ResponseWriter, r *http.Request) {
	a.listBotSubscriptions(w, r)
}

// @Summary Helix-org: subscribe an agent to a topic
// @Tags HelixOrg
// @Param org path string true "Organization slug or ID"
// @Param id path string true "Agent ID"
// @Param payload body api.SubscribeBotRequest true "Topic to subscribe the Agent to"
// @Success 200 {object} api.BotSubscriptionDTO
// @Success 201 {object} api.BotSubscriptionDTO
// @Failure 404 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/agents/{id}/subscriptions [post]
func (a *apiHandler) subscribeAgent(w http.ResponseWriter, r *http.Request) {
	a.subscribeBot(w, r)
}

// @Summary Helix-org: unsubscribe an agent from a topic
// @Tags HelixOrg
// @Param org path string true "Organization slug or ID"
// @Param id path string true "Agent ID"
// @Param topic_id path string true "Topic ID"
// @Success 204
// @Failure 404 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/agents/{id}/subscriptions/{topic_id} [delete]
func (a *apiHandler) unsubscribeAgent(w http.ResponseWriter, r *http.Request) {
	a.unsubscribeBot(w, r)
}
