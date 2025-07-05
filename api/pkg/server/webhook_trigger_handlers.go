package server

import "net/http"

// webhookTriggerHandler godoc
// @Summary Webhook trigger handler
// @Description Handles webhook trigger requests from Azure DevOps, GitHub, etc.
// @Tags    webhook-triggers
// @Accept  json
// @Produce json
// @Param   id path string true "Trigger configuration ID"
func (s *HelixAPIServer) webhookTriggerHandler(w http.ResponseWriter, r *http.Request) {

}
