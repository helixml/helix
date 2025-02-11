package runner

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
)

func (s *HelixRunnerAPIServer) createFinetuningJob(rw http.ResponseWriter, r *http.Request) {
	slotID := mux.Vars(r)["slot_id"]
	slotUUID, err := uuid.Parse(slotID)
	if err != nil {
		http.Error(rw, fmt.Sprintf("invalid slot id: %s", slotID), http.StatusBadRequest)
		return
	}
	log.Trace().Str("slot_id", slotID).Msg("create finetuning job")

	body, err := io.ReadAll(io.LimitReader(r.Body, 10*MEGABYTE))
	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	var finetuningRequest openai.FineTuningJobRequest
	err = json.Unmarshal(body, &finetuningRequest)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	slot, ok := s.slots.Load(slotUUID)
	if !ok {
		http.Error(rw, fmt.Sprintf("slot %s not found", slotID), http.StatusNotFound)
		return
	}

	addCorsHeaders(rw)
	if r.Method == http.MethodOptions {
		return
	}

	sessionID := r.Header.Get(types.SessionIDHeader)
	// If the session id is provided, download the jsonl files from the control plane
	if sessionID != "" {
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
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}
		// TODO: do we need to clean this temp dir up?

		localPath := path.Join(tempDir, finetuningRequest.TrainingFile)
		err = fileHandler.downloadFile(sessionID, finetuningRequest.TrainingFile, localPath)
		if err != nil {
			log.Error().Err(err).Msg("error downloading training file")
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}

		finetuningRequest.TrainingFile = localPath
	}

	openAIClient, err := CreateOpenaiClient(r.Context(), fmt.Sprintf("%s/v1", slot.Runtime.URL()))
	if err != nil {
		log.Error().Err(err).Msg("error creating openai client")
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	resp, err := openAIClient.CreateFineTuningJob(r.Context(), finetuningRequest)
	if err != nil {
		log.Error().Err(err).Msg("error creating finetuning job")
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	rw.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(rw).Encode(resp)
	if err != nil {
		log.Error().Err(err).Msg("error writing response")
	}
}

func (s *HelixRunnerAPIServer) listFinetuningJobs(rw http.ResponseWriter, r *http.Request) {
	slotID := mux.Vars(r)["slot_id"]
	slotUUID, err := uuid.Parse(slotID)
	if err != nil {
		http.Error(rw, fmt.Sprintf("invalid slot id: %s", slotID), http.StatusBadRequest)
		return
	}
	log.Trace().Str("slot_id", slotID).Msg("list finetuning jobs")

	slot, ok := s.slots.Load(slotUUID)
	if !ok {
		http.Error(rw, fmt.Sprintf("slot %s not found", slotID), http.StatusNotFound)
		return
	}

	addCorsHeaders(rw)
	if r.Method == http.MethodOptions {
		return
	}

	openAIClient, err := CreateOpenaiClient(r.Context(), fmt.Sprintf("%s/v1", slot.Runtime.URL()))
	if err != nil {
		log.Error().Err(err).Msg("error creating openai client")
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	// TODO(Phil): This is warning about depreciation.
	// nolint:staticcheck
	resp, err := openAIClient.ListFineTunes(r.Context())
	if err != nil {
		log.Error().Err(err).Msg("error listing finetuning jobs")
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	rw.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(rw).Encode(resp)
	if err != nil {
		log.Error().Err(err).Msg("error writing response")
	}
}

func (s *HelixRunnerAPIServer) retrieveFinetuningJob(rw http.ResponseWriter, r *http.Request) {
	slotID := mux.Vars(r)["slot_id"]
	jobID := mux.Vars(r)["job_id"]
	slotUUID, err := uuid.Parse(slotID)
	if err != nil {
		http.Error(rw, fmt.Sprintf("invalid slot id: %s", slotID), http.StatusBadRequest)
		return
	}
	log.Trace().Str("slot_id", slotID).Str("job_id", jobID).Msg("retrieve finetuning job")

	slot, ok := s.slots.Load(slotUUID)
	if !ok {
		http.Error(rw, fmt.Sprintf("slot %s not found", slotID), http.StatusNotFound)
		return
	}

	addCorsHeaders(rw)
	if r.Method == http.MethodOptions {
		return
	}

	openAIClient, err := CreateOpenaiClient(r.Context(), fmt.Sprintf("%s/v1", slot.Runtime.URL()))
	if err != nil {
		log.Error().Err(err).Msg("error creating openai client")
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	resp, err := openAIClient.RetrieveFineTuningJob(r.Context(), jobID)
	if err != nil {
		log.Error().Err(err).Msg("error retrieving finetuning job")
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	rw.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(rw).Encode(resp)
	if err != nil {
		log.Error().Err(err).Msg("error writing response")
	}
}

func (s *HelixRunnerAPIServer) listFinetuningJobEvents(rw http.ResponseWriter, r *http.Request) {
	slotID := mux.Vars(r)["slot_id"]
	jobID := mux.Vars(r)["job_id"]
	slotUUID, err := uuid.Parse(slotID)
	if err != nil {
		http.Error(rw, fmt.Sprintf("invalid slot id: %s", slotID), http.StatusBadRequest)
		return
	}
	log.Trace().Str("slot_id", slotID).Str("job_id", jobID).Msg("list finetuning job events")

	slot, ok := s.slots.Load(slotUUID)
	if !ok {
		http.Error(rw, fmt.Sprintf("slot %s not found", slotID), http.StatusNotFound)
		return
	}

	addCorsHeaders(rw)
	if r.Method == http.MethodOptions {
		return
	}

	openAIClient, err := CreateOpenaiClient(r.Context(), fmt.Sprintf("%s/v1", slot.Runtime.URL()))
	if err != nil {
		log.Error().Err(err).Msg("error creating openai client")
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	resp, err := openAIClient.ListFineTuningJobEvents(r.Context(), jobID)
	if err != nil {
		log.Error().Err(err).Msg("error listing finetuning job events")
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	rw.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(rw).Encode(resp)
	if err != nil {
		log.Error().Err(err).Msg("error writing response")
	}
}
