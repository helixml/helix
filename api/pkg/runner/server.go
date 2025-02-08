package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/server"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"

	_ "net/http/pprof" // enable profiling
)

const APIPrefix = "/api/v1"

var (
	startTime = time.Now()
)

type HelixRunnerAPIServer struct {
	runnerOptions *Options
	cliContext    context.Context
	slots         map[uuid.UUID]*Slot
	slotsMtx      sync.RWMutex
	gpuManager    *GPUManager
}

func NewHelixRunnerAPIServer(
	ctx context.Context,
	runnerOptions *Options,
) (*HelixRunnerAPIServer, error) {
	if ctx == nil {
		return nil, fmt.Errorf("cli context is required")
	}
	if runnerOptions.WebServer.Host == "" {
		runnerOptions.WebServer.Host = "127.0.0.1"
	}

	if runnerOptions.WebServer.Port == 0 {
		runnerOptions.WebServer.Port = 8080
	}

	return &HelixRunnerAPIServer{
		runnerOptions: runnerOptions,
		slots:         make(map[uuid.UUID]*Slot),
		slotsMtx:      sync.RWMutex{},
		cliContext:    ctx,
		gpuManager:    NewGPUManager(runnerOptions),
	}, nil
}

func (apiServer *HelixRunnerAPIServer) ListenAndServe(ctx context.Context, _ *system.CleanupManager) error {
	apiRouter, err := apiServer.registerRoutes(ctx)
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", apiServer.runnerOptions.WebServer.Host, apiServer.runnerOptions.WebServer.Port),
		WriteTimeout:      time.Minute * 15,
		ReadTimeout:       time.Minute * 15,
		ReadHeaderTimeout: time.Minute * 15,
		IdleTimeout:       time.Minute * 60,
		Handler:           apiRouter,
	}
	return srv.ListenAndServe()
}

func (apiServer *HelixRunnerAPIServer) registerRoutes(_ context.Context) (*mux.Router, error) {
	router := mux.NewRouter()

	// we do token extraction for all routes
	// if there is a token we will assign the user if not then oh well no user it's all gravy
	router.Use(server.ErrorLoggingMiddleware)

	// any route that lives under /api/v1
	subRouter := router.PathPrefix(APIPrefix).Subrouter()
	subRouter.HandleFunc("/healthz", apiServer.healthz).Methods(http.MethodGet)
	subRouter.HandleFunc("/status", apiServer.status).Methods(http.MethodGet)
	subRouter.HandleFunc("/slots", apiServer.createSlot).Methods(http.MethodPost)
	subRouter.HandleFunc("/slots", apiServer.listSlots).Methods(http.MethodGet)
	subRouter.HandleFunc("/slots/{slot_id}", apiServer.deleteSlot).Methods(http.MethodDelete)
	subRouter.HandleFunc("/slots/{slot_id}/v1/chat/completions", apiServer.createChatCompletion).Methods(http.MethodPost, http.MethodOptions)
	subRouter.HandleFunc("/slots/{slot_id}/v1/models", apiServer.listModels).Methods(http.MethodGet, http.MethodOptions)
	subRouter.HandleFunc("/slots/{slot_id}/v1/embedding", apiServer.createEmbedding).Methods(http.MethodPost, http.MethodOptions)
	subRouter.HandleFunc("/slots/{slot_id}/v1/images/generations", apiServer.createImageGeneration).Methods(http.MethodPost, http.MethodOptions)
	subRouter.HandleFunc("/slots/{slot_id}/v1/helix/images/generations", apiServer.createHelixImageGeneration).Methods(http.MethodPost, http.MethodOptions)
	subRouter.HandleFunc("/slots/{slot_id}/v1/fine_tuning/jobs", apiServer.listFinetuningJobs).Methods(http.MethodGet, http.MethodOptions)
	subRouter.HandleFunc("/slots/{slot_id}/v1/fine_tuning/jobs/{job_id}", apiServer.retrieveFinetuningJob).Methods(http.MethodGet, http.MethodOptions)
	subRouter.HandleFunc("/slots/{slot_id}/v1/fine_tuning/jobs", apiServer.createFinetuningJob).Methods(http.MethodPost, http.MethodOptions)
	subRouter.HandleFunc("/slots/{slot_id}/v1/fine_tuning/jobs/{job_id}/events", apiServer.listFinetuningJobEvents).Methods(http.MethodGet, http.MethodOptions)
	subRouter.HandleFunc("/slots/{slot_id}/v1/helix/fine_tuning/jobs", apiServer.createHelixFinetuningJob).Methods(http.MethodPost, http.MethodOptions)

	// register pprof routes
	router.PathPrefix("/debug/pprof/").Handler(http.DefaultServeMux)

	return subRouter, nil
}

func (apiServer *HelixRunnerAPIServer) healthz(w http.ResponseWriter, _ *http.Request) {
	_, err := w.Write([]byte("ok"))
	if err != nil {
		log.Error().Err(err).Msg("error writing healthz response")
	}
}

func (apiServer *HelixRunnerAPIServer) status(w http.ResponseWriter, _ *http.Request) {
	status := &types.RunnerStatus{
		ID:          apiServer.runnerOptions.ID,
		Created:     startTime,
		Updated:     time.Now(),
		Version:     data.GetHelixVersion(),
		TotalMemory: apiServer.gpuManager.GetTotalMemory(),
		FreeMemory:  apiServer.gpuManager.GetFreeMemory(),
		Labels:      apiServer.runnerOptions.Labels,
	}
	err := json.NewEncoder(w).Encode(status)
	if err != nil {
		log.Error().Err(err).Msg("error encoding status response")
	}
}

func (apiServer *HelixRunnerAPIServer) createSlot(w http.ResponseWriter, r *http.Request) {
	slot := &types.CreateRunnerSlotRequest{}
	err := json.NewDecoder(r.Body).Decode(slot)
	if err != nil {
		log.Error().Err(err).Msg("error decoding create slot request")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate the request
	if slot.ID == uuid.Nil {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	if slot.Attributes.Runtime == "" {
		http.Error(w, "runtime is required", http.StatusBadRequest)
		return
	}
	if slot.Attributes.Model == "" {
		http.Error(w, "model is required", http.StatusBadRequest)
		return
	}

	log.Debug().Str("slot_id", slot.ID.String()).Msg("creating slot")

	// Must pass the context from the cli to ensure that the underlying runtime continues to run so
	// long as the cli is running
	s, err := CreateSlot(apiServer.cliContext, CreateSlotParams{
		RunnerOptions: apiServer.runnerOptions,
		ID:            slot.ID,
		Runtime:       slot.Attributes.Runtime,
		Model:         slot.Attributes.Model,
	})
	if err != nil {
		if strings.Contains(err.Error(), "pull model manifest: file does not exist") {
			http.Error(w, err.Error(), http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	apiServer.slotsMtx.Lock()
	defer apiServer.slotsMtx.Unlock()
	apiServer.slots[slot.ID] = s

	// TODO(Phil): Return some representation of the slot
	w.WriteHeader(http.StatusCreated)
}

func (apiServer *HelixRunnerAPIServer) listSlots(w http.ResponseWriter, _ *http.Request) {
	apiServer.slotsMtx.RLock()
	defer apiServer.slotsMtx.RUnlock()

	slotList := make([]*types.RunnerSlot, 0, len(apiServer.slots))
	for id, slot := range apiServer.slots {
		slotList = append(slotList, &types.RunnerSlot{
			ID:      id,
			Runtime: slot.Runtime.Runtime(),
			Version: slot.Runtime.Version(),
			Model:   slot.Model,
		})
	}
	response := &types.ListRunnerSlotsResponse{
		Slots: slotList,
	}
	err := json.NewEncoder(w).Encode(response)
	if err != nil {
		log.Error().Err(err).Msg("error encoding list slots response")
	}
}

func (apiServer *HelixRunnerAPIServer) deleteSlot(w http.ResponseWriter, r *http.Request) {
	apiServer.slotsMtx.Lock()
	defer apiServer.slotsMtx.Unlock()

	slotID := mux.Vars(r)["slot_id"]
	slotUUID, err := uuid.Parse(slotID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	slot, ok := apiServer.slots[slotUUID]
	if !ok {
		http.Error(w, "slot not found", http.StatusNotFound)
		return
	}
	err = slot.Runtime.Stop()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	delete(apiServer.slots, slotUUID)
	w.WriteHeader(http.StatusNoContent)
}
