package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/puzpuzpuz/xsync/v3"
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
	slots         *xsync.MapOf[uuid.UUID, *Slot]
	gpuManager    *GPUManager

	modelStatusMu sync.Mutex
	modelStatus   map[string]*types.RunnerModelStatus

	modelsMu sync.Mutex
	models   []*types.Model
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
		slots:         xsync.NewMapOf[uuid.UUID, *Slot](),
		cliContext:    ctx,
		gpuManager:    NewGPUManager(ctx, runnerOptions),
		modelStatusMu: sync.Mutex{},
		modelStatus:   make(map[string]*types.RunnerModelStatus),
		modelsMu:      sync.Mutex{},
		models:        []*types.Model{},
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
	subRouter.Use(server.ErrorLoggingMiddleware)
	subRouter.HandleFunc("/healthz", apiServer.healthz).Methods(http.MethodGet)
	subRouter.HandleFunc("/status", apiServer.status).Methods(http.MethodGet)
	subRouter.HandleFunc("/helix-models", apiServer.listHelixModelsHandler).Methods(http.MethodGet) // List current models
	subRouter.HandleFunc("/helix-models", apiServer.setHelixModelsHandler).Methods(http.MethodPost) // Which models to pull
	subRouter.HandleFunc("/slots", apiServer.createSlot).Methods(http.MethodPost)
	subRouter.HandleFunc("/slots", apiServer.listSlots).Methods(http.MethodGet)
	subRouter.HandleFunc("/slots/{slot_id}", apiServer.getSlot).Methods(http.MethodGet)
	subRouter.HandleFunc("/slots/{slot_id}", apiServer.deleteSlot).Methods(http.MethodDelete)
	subRouter.HandleFunc("/slots/{slot_id}/v1/chat/completions", apiServer.slotActivationMiddleware(apiServer.createChatCompletion)).Methods(http.MethodPost, http.MethodOptions)
	subRouter.HandleFunc("/slots/{slot_id}/v1/models", apiServer.listModels).Methods(http.MethodGet, http.MethodOptions)
	subRouter.HandleFunc("/slots/{slot_id}/v1/embedding", apiServer.createEmbedding).Methods(http.MethodPost, http.MethodOptions)
	subRouter.HandleFunc("/slots/{slot_id}/v1/images/generations", apiServer.slotActivationMiddleware(apiServer.createImageGeneration)).Methods(http.MethodPost, http.MethodOptions)
	subRouter.HandleFunc("/slots/{slot_id}/v1/helix/images/generations", apiServer.slotActivationMiddleware(apiServer.createHelixImageGeneration)).Methods(http.MethodPost, http.MethodOptions)
	subRouter.HandleFunc("/slots/{slot_id}/v1/fine_tuning/jobs", apiServer.listFinetuningJobs).Methods(http.MethodGet, http.MethodOptions)
	subRouter.HandleFunc("/slots/{slot_id}/v1/fine_tuning/jobs/{job_id}", apiServer.retrieveFinetuningJob).Methods(http.MethodGet, http.MethodOptions)
	subRouter.HandleFunc("/slots/{slot_id}/v1/fine_tuning/jobs", apiServer.slotActivationMiddleware(apiServer.createFinetuningJob)).Methods(http.MethodPost, http.MethodOptions)
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
	// Calculate allocated memory by summing the memory requirements of all slots
	var allocatedMemory uint64

	// Log runner options to see what filter model name is being used
	log.Debug().
		Str("runner_id", apiServer.runnerOptions.ID).
		Str("filter_model_name", apiServer.runnerOptions.FilterModelName).
		Msg("Runner filter model information")

	// Count slots for debugging
	slotCount := 0
	apiServer.slots.Range(func(id uuid.UUID, slot *Slot) bool {
		slotCount++
		log.Debug().
			Str("slot_id", id.String()).
			Str("model", slot.Model).
			Bool("ready", slot.Ready).
			Bool("active", slot.Active).
			Msg("Processing slot for memory calculation")

		// We need to get the memory requirements for each slot
		// If we have a model for this slot, we can calculate memory requirements
		if slot.Model != "" {
			// Try to get the model
			// modelObj, err := model.GetModel(slot.Model)
			if slot.ModelMemoryRequirement > 0 {
				// Add the memory requirements to our running total
				allocatedMemory += slot.ModelMemoryRequirement
				log.Debug().
					Str("slot_id", id.String()).
					Str("model", slot.Model).
					Uint64("memory", slot.ModelMemoryRequirement).
					Msg("Found memory requirements for model")
			} else {
				log.Warn().
					Str("slot_id", id.String()).
					Str("model", slot.Model).
					Msg("Could not get memory requirements for model")
			}
		} else {
			log.Debug().
				Str("slot_id", id.String()).
				Msg("Slot has no model assigned")
		}
		return true
	})

	status := &types.RunnerStatus{
		ID:              apiServer.runnerOptions.ID,
		Created:         startTime,
		Updated:         time.Now(),
		Version:         data.GetHelixVersion(),
		TotalMemory:     apiServer.gpuManager.GetTotalMemory(),
		FreeMemory:      apiServer.gpuManager.GetFreeMemory(),
		AllocatedMemory: allocatedMemory,
		Labels:          apiServer.runnerOptions.Labels,
		Models:          apiServer.listHelixModelsStatus(),
	}

	// Add debug logging to see memory values
	log.Debug().
		Str("runner_id", apiServer.runnerOptions.ID).
		Uint64("total_memory", status.TotalMemory).
		Uint64("free_memory", status.FreeMemory).
		Uint64("allocated_memory", status.AllocatedMemory).
		Int("slot_count", slotCount).
		Int("models", len(apiServer.models)).
		Any("models_status", apiServer.modelStatus).
		Msg("Runner status memory values")

	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(status)
	if err != nil {
		log.Error().Err(err).Msg("error encoding status response")
	}
}

func (apiServer *HelixRunnerAPIServer) createSlot(w http.ResponseWriter, r *http.Request) {
	slotRequest := &types.CreateRunnerSlotRequest{}
	err := json.NewDecoder(r.Body).Decode(slotRequest)
	if err != nil {
		log.Error().Err(err).Msg("error decoding create slot request")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate the request
	if slotRequest.ID == uuid.Nil {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	if slotRequest.Attributes.Runtime == "" {
		http.Error(w, "runtime is required", http.StatusBadRequest)
		return
	}
	if slotRequest.Attributes.Model == "" {
		http.Error(w, "model is required", http.StatusBadRequest)
		return
	}

	// Log request details with RuntimeArgs for debugging
	log.Debug().
		Str("slot_id", slotRequest.ID.String()).
		Str("model", slotRequest.Attributes.Model).
		Str("runtime", string(slotRequest.Attributes.Runtime)).
		Interface("runtime_args", slotRequest.Attributes.RuntimeArgs).
		Msg("Runner received createSlot request with RuntimeArgs")

	// For VLLM, check the type of args in RuntimeArgs
	if slotRequest.Attributes.Runtime == types.RuntimeVLLM && slotRequest.Attributes.RuntimeArgs != nil {
		if args, ok := slotRequest.Attributes.RuntimeArgs["args"]; ok {
			log.Debug().
				Str("slot_id", slotRequest.ID.String()).
				Str("model", slotRequest.Attributes.Model).
				Interface("args_value", args).
				Str("args_type", fmt.Sprintf("%T", args)).
				Msg("Args value and type in RuntimeArgs")
		}
	}

	s := NewEmptySlot(CreateSlotParams{
		RunnerOptions:          apiServer.runnerOptions,
		ID:                     slotRequest.ID,
		Runtime:                slotRequest.Attributes.Runtime,
		Model:                  slotRequest.Attributes.Model,
		ModelMemoryRequirement: slotRequest.Attributes.ModelMemoryRequirement,
		ContextLength:          slotRequest.Attributes.ContextLength,
		RuntimeArgs:            slotRequest.Attributes.RuntimeArgs,
	})
	apiServer.slots.Store(slotRequest.ID, s)

	// Must pass the context from the cli to ensure that the underlying runtime continues to run so
	// long as the cli is running
	err = s.Create(apiServer.cliContext)
	if err != nil {
		log.Error().Err(err).Msg("error creating slot, deleting...")
		apiServer.slots.Delete(slotRequest.ID)
		if strings.Contains(err.Error(), "pull model manifest: file does not exist") {
			http.Error(w, err.Error(), http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	// TODO(Phil): Return some representation of the slot
	w.WriteHeader(http.StatusCreated)
}

func (apiServer *HelixRunnerAPIServer) listSlots(w http.ResponseWriter, r *http.Request) {
	slotList := make([]*types.RunnerSlot, 0, apiServer.slots.Size())
	apiServer.slots.Range(func(id uuid.UUID, slot *Slot) bool {
		slotList = append(slotList, &types.RunnerSlot{
			ID:            id,
			Runtime:       slot.Runtime(),
			Version:       slot.Version(),
			Model:         slot.Model,
			ContextLength: slot.ContextLength,
			Active:        slot.Active,
			Ready:         slot.Ready,
			Status:        slot.Status(r.Context()),
		})
		return true
	})
	response := &types.ListRunnerSlotsResponse{
		Slots: slotList,
	}
	err := json.NewEncoder(w).Encode(response)
	if err != nil {
		log.Error().Err(err).Msg("error encoding list slots response")
	}
}

func (apiServer *HelixRunnerAPIServer) getSlot(w http.ResponseWriter, r *http.Request) {
	slotID := mux.Vars(r)["slot_id"]
	slotUUID, err := uuid.Parse(slotID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	slot, ok := apiServer.slots.Load(slotUUID)
	if !ok {
		http.Error(w, "slot not found", http.StatusNotFound)
		return
	}

	response := &types.RunnerSlot{
		ID:            slotUUID,
		Runtime:       slot.Runtime(),
		Version:       slot.Version(),
		Model:         slot.Model,
		ContextLength: slot.ContextLength,
		Active:        slot.Active,
		Ready:         slot.Ready,
		Status:        slot.Status(r.Context()),
	}

	err = json.NewEncoder(w).Encode(response)
	if err != nil {
		log.Error().Err(err).Msg("error encoding get slot response")
	}
}

func (apiServer *HelixRunnerAPIServer) deleteSlot(w http.ResponseWriter, r *http.Request) {
	slotID := mux.Vars(r)["slot_id"]
	slotUUID, err := uuid.Parse(slotID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	slot, ok := apiServer.slots.Load(slotUUID)
	if !ok {
		http.Error(w, "slot not found", http.StatusNotFound)
		return
	}

	// We must always delete the slot if we are told to, even if we think the slot is active,
	// because some runtimes might be in a bad state and we need to clean up after them.

	// Delete slot first to ensure it is not used while we are stopping it
	apiServer.slots.Delete(slotUUID)

	// Then delete the slot (to stop the runtime)
	err = slot.Delete()
	if err != nil {
		log.Error().Err(err).Msg("error stopping slot, potential gpu memory leak")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// slotActivationMiddleware is a http middleware that parses the slot ID from the request and sets
// it as active.
// It doesn't mark it as complete, you must do that yourself in each handler. This is because some
// handlers are async, like fine tuning
func (apiServer *HelixRunnerAPIServer) slotActivationMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slotID := mux.Vars(r)["slot_id"]
		slotUUID, err := uuid.Parse(slotID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		apiServer.slots.Compute(slotUUID, func(oldValue *Slot, loaded bool) (*Slot, bool) {
			if !loaded {
				http.Error(w, "slot not found", http.StatusNotFound)
				return nil, true
			}
			oldValue.Active = true
			return oldValue, false
		})
		next.ServeHTTP(w, r)
	})
}

func (apiServer *HelixRunnerAPIServer) markSlotAsComplete(slotUUID uuid.UUID) {
	apiServer.slots.Compute(slotUUID, func(oldValue *Slot, loaded bool) (*Slot, bool) {
		if !loaded {
			return nil, true
		}
		oldValue.Active = false
		return oldValue, false
	})
}

// listHelixModels - returns current model status
func (apiServer *HelixRunnerAPIServer) listHelixModelsHandler(w http.ResponseWriter, r *http.Request) {
	log.Info().Msg("listing helix models")

	apiServer.modelsMu.Lock()
	defer apiServer.modelsMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(apiServer.modelStatus)
}

// setHelixModels - sets the helix models, used to sync from controller to runner currently enabled
// Helix models
func (apiServer *HelixRunnerAPIServer) setHelixModelsHandler(w http.ResponseWriter, r *http.Request) {
	var models []*types.Model

	err := json.NewDecoder(r.Body).Decode(&models)
	if err != nil {
		log.Error().Err(err).Msg("error decoding helix models")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	log.Debug().Int("models_count", len(models)).Msg("setting helix models")

	apiServer.setHelixModels(models)

	w.WriteHeader(http.StatusOK)
}

func (apiServer *HelixRunnerAPIServer) listHelixModels() []*types.Model {
	apiServer.modelsMu.Lock()
	defer apiServer.modelsMu.Unlock()

	return apiServer.models
}

func (apiServer *HelixRunnerAPIServer) setHelixModels(models []*types.Model) {
	apiServer.modelsMu.Lock()
	defer apiServer.modelsMu.Unlock()
	apiServer.models = models
}

func (apiServer *HelixRunnerAPIServer) listHelixModelsStatus() []*types.RunnerModelStatus {
	apiServer.modelStatusMu.Lock()
	defer apiServer.modelStatusMu.Unlock()

	var resp []*types.RunnerModelStatus
	for _, status := range apiServer.modelStatus {
		resp = append(resp, status)
	}

	// Sort by model_id
	sort.Slice(resp, func(i, j int) bool {
		return resp[i].ModelID < resp[j].ModelID
	})

	return resp
}

func (apiServer *HelixRunnerAPIServer) setHelixModelsStatus(status *types.RunnerModelStatus) {
	apiServer.modelStatusMu.Lock()
	defer apiServer.modelStatusMu.Unlock()

	apiServer.modelStatus[status.ModelID] = status
}
