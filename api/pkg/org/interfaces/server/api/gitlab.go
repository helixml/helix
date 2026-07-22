package api

import "net/http"

type InstallGitLabWebhookResponse struct {
	WebhookID      int64  `json:"webhook_id"`
	WebhookHTMLURL string `json:"webhook_html_url,omitempty"`
	PayloadURL     string `json:"payload_url"`
}

type GitLabWebhookStatusResponse struct {
	State          string `json:"state"`
	WebhookID      int64  `json:"webhook_id,omitempty"`
	WebhookHTMLURL string `json:"webhook_html_url,omitempty"`
	Active         bool   `json:"active,omitempty"`
	PayloadURL     string `json:"payload_url,omitempty"`
	Detail         string `json:"detail,omitempty"`
}

// @Summary Helix-org: auto-install the webhook for a GitLab topic
// @Tags HelixOrg
// @Param id path string true "Topic ID"
// @Produce json
// @Success 200 {object} api.InstallGitLabWebhookResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/topics/{id}/gitlab/install-webhook [post]
func (a *apiHandler) installGitLabWebhook(w http.ResponseWriter, r *http.Request) {
	a.installGitHubWebhook(w, r)
}

// @Summary Helix-org: live webhook status for a GitLab topic
// @Tags HelixOrg
// @Param id path string true "Topic ID"
// @Produce json
// @Success 200 {object} api.GitLabWebhookStatusResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/topics/{id}/gitlab/webhook-status [get]
func (a *apiHandler) getGitLabWebhookStatus(w http.ResponseWriter, r *http.Request) {
	a.getGitHubWebhookStatus(w, r)
}
