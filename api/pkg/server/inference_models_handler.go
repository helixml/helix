package server

import (
	"net/http"
	"time"
)

// openaiModelsResponse mimics the OpenAI /v1/models response shape so
// existing OpenAI-compatible clients (curl, the OpenAI SDK, etc.) work
// against Helix without code changes.
type openaiModelsResponse struct {
	Object string             `json:"object"`
	Data   []openaiModelEntry `json:"data"`
}

type openaiModelEntry struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// listInferenceModels godoc
// @Summary List models currently available for inference
// @Description OpenAI-compatible /v1/models endpoint. Returns the union of model
// @Description names exposed by every connected runner whose active profile is in the
// @Description "running" state.
// @Tags    inference
// @Success 200 {object} openaiModelsResponse
// @Router /v1/models [get]
func (apiServer *HelixAPIServer) listInferenceModels(rw http.ResponseWriter, _ *http.Request) {
	var names []string
	if apiServer.inferenceRouter != nil {
		names = apiServer.inferenceRouter.AvailableModels()
	}
	now := time.Now().Unix()
	resp := openaiModelsResponse{
		Object: "list",
		Data:   make([]openaiModelEntry, 0, len(names)),
	}
	for _, n := range names {
		resp.Data = append(resp.Data, openaiModelEntry{
			ID:      n,
			Object:  "model",
			Created: now,
			OwnedBy: "helix",
		})
	}
	writeResponse(rw, resp, http.StatusOK)
}
