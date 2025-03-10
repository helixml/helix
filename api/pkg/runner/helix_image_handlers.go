package runner

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
)

func (s *HelixRunnerAPIServer) createHelixImageGeneration(w http.ResponseWriter, r *http.Request) {
	addCorsHeaders(w)
	if r.Method == http.MethodOptions {
		return
	}

	slotID := mux.Vars(r)["slot_id"]
	slotUUID, err := uuid.Parse(slotID)
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid slot id: %s", slotID), http.StatusBadRequest)
		return
	}

	slot, ok := s.slots.Load(slotUUID)
	if !ok {
		http.Error(w, fmt.Sprintf("slot %s not found", slotUUID.String()), http.StatusNotFound)
		return
	}
	log.Trace().Str("slot_id", slotUUID.String()).Msg("create helix image generation")

	body, err := io.ReadAll(io.LimitReader(r.Body, 10*MEGABYTE))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	log.Trace().Str("body", string(body)).Msg("parsing nats reply request")

	var imageRequest openai.ImageRequest
	err = json.Unmarshal(body, &imageRequest)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if imageRequest.Model == "" {
		imageRequest.Model = slot.Model
	}
	if imageRequest.Model != slot.Model {
		http.Error(w, fmt.Sprintf("model mismatch, expecting %s", slot.Model), http.StatusBadRequest)
		return
	}

	// Parse Session ID header from request
	sessionID := r.Header.Get(types.SessionIDHeader)
	if sessionID == "" {
		http.Error(w, "session id header is required", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	diffusersClient, err := NewDiffusersClient(r.Context(), slot.URL())
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to create diffusers client: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	err = diffusersClient.GenerateStreaming(r.Context(), imageRequest.Prompt, func(update types.HelixImageGenerationUpdate) error {
		log.Trace().Interface("update", update).Msg("Image generation update")
		if update.Error != "" {
			return fmt.Errorf("python diffusers error: %s", update.Error)
		}
		if update.Completed {
			// When everything has finished, mark the slot as complete
			defer s.markSlotAsComplete(slotUUID)

			// Intercept the result and upload the files to the control plane
			clientOptions := system.ClientOptions{
				Host:  s.runnerOptions.APIHost,
				Token: s.runnerOptions.APIToken,
			}
			fileHandler := NewFileHandler(s.runnerOptions.ID, clientOptions, func(response *types.RunnerTaskResponse) {
				log.Debug().Interface("response", response).Msg("File handler event")
			})
			localFiles := []string{}
			for _, image := range update.Data {
				localFiles = append(localFiles, image.URL)
			}
			resFiles, err := fileHandler.uploadFiles(sessionID, localFiles, types.FilestoreResultsDir)
			if err != nil {
				return fmt.Errorf("failed to upload files: %w", err)
			}
			// Overwrite the original urls with the new ones
			inner := []openai.ImageResponseDataInner{}
			for i := range update.Data {
				inner = append(inner, openai.ImageResponseDataInner{
					URL: resFiles[i],
				})
			}
			finalResponse := types.HelixImageGenerationUpdate{
				Created:   update.Created,
				Step:      update.Step,
				Timestep:  update.Timestep,
				Error:     update.Error,
				Completed: update.Completed,
				Data:      inner,
			}
			bts, err := json.Marshal(finalResponse)
			if err != nil {
				return fmt.Errorf("failed to marshal update: %w", err)
			}

			if err := writeChunk(w, bts); err != nil {
				return fmt.Errorf("failed to write completion chunk: %w", err)
			}
			return nil
		}

		bts, err := json.Marshal(update)
		if err != nil {
			return fmt.Errorf("failed to marshal update: %w", err)
		}

		if err := writeChunk(w, bts); err != nil {
			return fmt.Errorf("failed to write completion chunk: %w", err)
		}
		return nil
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to create image: %s", err.Error()), http.StatusInternalServerError)
		return
	}

}
