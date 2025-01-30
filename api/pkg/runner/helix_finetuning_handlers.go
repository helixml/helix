package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
)

type TrainingStatusReport struct {
	Type         string  `json:"type"`
	Loss         float64 `json:"loss"`
	GradNorm     float64 `json:"grad_norm"`
	LearningRate float64 `json:"learning_rate"`
	Epoch        float64 `json:"epoch"`
	Progress     int     `json:"progress"`
}

func (s *HelixRunnerAPIServer) createHelixFinetuningJob(w http.ResponseWriter, r *http.Request) {
	addCorsHeaders(w)
	if r.Method == http.MethodOptions {
		return
	}

	slot_id := mux.Vars(r)["slot_id"]
	slot_uuid, err := uuid.Parse(slot_id)
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid slot id: %s", slot_id), http.StatusBadRequest)
		return
	}

	slot, ok := s.slots[slot_uuid]
	if !ok {
		http.Error(w, fmt.Sprintf("slot %s not found", slot_id), http.StatusNotFound)
		return
	}
	log.Trace().Str("slot_id", slot_id).Msg("create helix finetuning job")

	body, err := io.ReadAll(io.LimitReader(r.Body, 10*MEGABYTE))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	log.Trace().Str("body", string(body)).Msg("parsing nats reply request")

	var finetuningRequest openai.FineTuningJobRequest
	err = json.Unmarshal(body, &finetuningRequest)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if finetuningRequest.Model == "" {
		finetuningRequest.Model = slot.Model
	}
	if finetuningRequest.Model != slot.Model {
		http.Error(w, fmt.Sprintf("model mismatch, expecting %s", slot.Model), http.StatusBadRequest)
		return
	}

	// Parse Session ID header from request
	sessionID := r.Header.Get(types.SessionIDHeader)
	if sessionID == "" {
		http.Error(w, "session id header is required", http.StatusBadRequest)
		return
	}

	// Download the jsonl files from the control plane
	log.Info().Str("session_id", sessionID).Msg("processing fine-tuning interaction")
	// accumulate all JSONL files across all interactions
	// and append them to one large JSONL file
	clientOptions := system.ClientOptions{
		Host:  s.runnerOptions.APIHost,
		Token: s.runnerOptions.APIToken,
	}
	fileHandler := NewFileHandler(s.runnerOptions.ID, clientOptions, func(response *types.RunnerTaskResponse) {
		log.Debug().Interface("response", response).Msg("File handler event")
	})
	tempDir, err := os.MkdirTemp("", "finetuning")
	if err != nil {
		log.Error().Err(err).Msg("error creating temp directory")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// TODO: do we need to clean this temp dir up?

	localPath := path.Join(tempDir, finetuningRequest.TrainingFile)
	err = fileHandler.downloadFile(sessionID, finetuningRequest.TrainingFile, localPath)
	if err != nil {
		log.Error().Err(err).Msg("error downloading training file")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	finetuningRequest.TrainingFile = localPath

	openAIClient, err := CreateOpenaiClient(r.Context(), fmt.Sprintf("%s/v1", slot.Runtime.URL()))
	if err != nil {
		log.Error().Err(err).Msg("error creating openai client")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	job, err := openAIClient.CreateFineTuningJob(r.Context(), finetuningRequest)
	if err != nil {
		log.Error().Err(err).Msg("error creating finetuning job")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	timeoutCtx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()

	// Now keep track of that job id and stream the events back to the control plane
	ch := make(chan struct{})
	go func() {
		defer close(ch)
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-timeoutCtx.Done():
				return
			case <-ticker.C:
				events, err := openAIClient.ListFineTuningJobEvents(timeoutCtx, job.ID)
				if err != nil {
					if strings.Contains(err.Error(), "connection refused") {
						continue
					}
					log.Error().Err(err).Msg("error listing fine-tuning job events")
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}

				status, err := openAIClient.RetrieveFineTuningJob(timeoutCtx, job.ID)
				if err != nil {
					if strings.Contains(err.Error(), "connection refused") {
						continue
					}
					log.Error().Err(err).Msg("error retrieving fine-tuning job")
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}

				// Get latest training report
				var report TrainingStatusReport
				for _, event := range events.Data {
					// ignore errors, just capture latest whatever we can
					var newReport TrainingStatusReport
					err := json.Unmarshal([]byte(event.Message), &newReport)
					if err == nil {
						if newReport.Type == "training_progress_report" {
							report = newReport
						}
					}
				}

				switch status.Status {
				case "running":
					finalResponse := types.HelixFineTuningUpdate{
						Created:   status.CreatedAt,
						Error:     "",
						Progress:  report.Progress,
						Completed: false,
					}
					bts, err := json.Marshal(finalResponse)
					if err != nil {
						http.Error(w, err.Error(), http.StatusInternalServerError)
					}

					if err := writeChunk(w, bts); err != nil {
						log.Error().Msgf("failed to write completion chunk: %v", err)
					}
				case "succeeded":
					if len(status.ResultFiles) < 1 {
						log.Error().Msg("fine-tuning succeeded but no result files")
						http.Error(w, "fine-tuning succeeded but no result files", http.StatusInternalServerError)
						return
					}

					// TODO(Phil): Probably need to upload the files to the control plane
					finalResponse := types.HelixFineTuningUpdate{
						Created:   status.CreatedAt,
						Error:     "",
						Progress:  100,
						Completed: true,
						LoraDir:   status.ResultFiles[0],
					}
					bts, err := json.Marshal(finalResponse)
					if err != nil {
						http.Error(w, err.Error(), http.StatusInternalServerError)
					}

					if err := writeChunk(w, bts); err != nil {
						log.Error().Msgf("failed to write completion chunk: %v", err)
					}
					return
				case string(openai.RunStatusFailed):
					if len(events.Data) > 0 {
						log.Error().Msgf("fine-tuning failed: %s", events.Data[len(events.Data)-1].Message)
						http.Error(w, events.Data[len(events.Data)-1].Message, http.StatusInternalServerError)
						return
					}
					log.Error().Msg("fine-tuning failed with no events")
					http.Error(w, "fine-tuning failed with no events", http.StatusInternalServerError)
					return
				default:
					log.Error().Msgf("unknown fine-tuning status: %s", status.Status)
					http.Error(w, fmt.Sprintf("unknown fine-tuning status: %s", status.Status), http.StatusInternalServerError)
					return
				}
			}
		}
	}()

	// Wait for the goroutine to finish
	<-ch
}
