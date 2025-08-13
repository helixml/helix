package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
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
	logManager    *system.LogManager

	modelStatusMu sync.Mutex
	modelStatus   map[string]*types.RunnerModelStatus

	modelsMu sync.Mutex
	models   []*types.Model

	// System configuration received from control plane
	systemConfigMu   sync.RWMutex
	huggingFaceToken string
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
		logManager:    system.NewLogManager(1000, 14*24*time.Hour), // 1000 lines, 14 days error retention
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
	subRouter.HandleFunc("/system/config", apiServer.updateSystemConfig).Methods(http.MethodPut)
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

	// Log endpoints
	subRouter.HandleFunc("/logs", apiServer.getLogsSummary).Methods(http.MethodGet)
	subRouter.HandleFunc("/logs/{slot_id}", apiServer.getSlotLogs).Methods(http.MethodGet)

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

// updateSystemConfig handles system configuration updates from the control plane
func (apiServer *HelixRunnerAPIServer) updateSystemConfig(w http.ResponseWriter, r *http.Request) {
	var req types.RunnerSystemConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error().Err(err).Msg("error decoding system config request")
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	apiServer.systemConfigMu.Lock()
	defer apiServer.systemConfigMu.Unlock()

	// Update Hugging Face token if provided
	if req.HuggingFaceToken != nil {
		apiServer.huggingFaceToken = *req.HuggingFaceToken
		log.Info().
			Str("runner_id", apiServer.runnerOptions.ID).
			Bool("token_provided", *req.HuggingFaceToken != "").
			Msg("updated hugging face token from control plane")
	}

	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte(`{"status": "ok"}`))
	if err != nil {
		log.Error().Err(err).Msg("error writing system config response")
	}
}

// GetEffectiveHuggingFaceToken returns the effective HF token, preferring control plane config over environment
func (apiServer *HelixRunnerAPIServer) GetEffectiveHuggingFaceToken() string {
	apiServer.systemConfigMu.RLock()
	defer apiServer.systemConfigMu.RUnlock()

	// Prefer token from control plane if set
	if apiServer.huggingFaceToken != "" {
		return apiServer.huggingFaceToken
	}

	// Fall back to environment variable for backward compatibility
	return os.Getenv("HF_TOKEN")
}

func (apiServer *HelixRunnerAPIServer) status(w http.ResponseWriter, _ *http.Request) {
	// Calculate allocated memory by summing the memory requirements of all slots
	var allocatedMemory uint64

	// Log runner options to see what filter model name is being used
	log.Debug().
		Str("runner_id", apiServer.runnerOptions.ID).
		Str("filter_model_name", apiServer.runnerOptions.FilterModelName).
		Msg("Runner filter model information")

	// Get GPU memory info before doing anything else
	totalMem := apiServer.gpuManager.GetTotalMemory()
	freeMem := apiServer.gpuManager.GetFreeMemory()
	usedMem := apiServer.gpuManager.GetUsedMemory()

	log.Debug().
		Str("runner_id", apiServer.runnerOptions.ID).
		Uint64("total_memory_bytes", totalMem).
		Uint64("free_memory_bytes", freeMem).
		Uint64("used_memory_bytes", usedMem).
		Msg("Raw GPU memory values")

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

	// Convert per-GPU info to status format
	var gpuStatuses []*types.GPUStatus
	gpuInfo := apiServer.gpuManager.GetGPUInfo()
	for _, gpu := range gpuInfo {
		gpuStatuses = append(gpuStatuses, &types.GPUStatus{
			Index:         gpu.Index,
			TotalMemory:   gpu.TotalMemory,
			FreeMemory:    gpu.FreeMemory,
			UsedMemory:    gpu.UsedMemory,
			ModelName:     gpu.ModelName,
			DriverVersion: gpu.DriverVersion,
			CUDAVersion:   gpu.CUDAVersion,
		})
	}

	status := &types.RunnerStatus{
		ID:              apiServer.runnerOptions.ID,
		Created:         startTime,
		Updated:         time.Now(),
		Version:         data.GetHelixVersion(),
		TotalMemory:     apiServer.gpuManager.GetTotalMemory(),
		FreeMemory:      apiServer.gpuManager.GetFreeMemory(),
		UsedMemory:      apiServer.gpuManager.GetUsedMemory(),
		AllocatedMemory: allocatedMemory,
		GPUCount:        apiServer.gpuManager.GetGPUCount(),
		GPUs:            gpuStatuses,
		Labels:          apiServer.runnerOptions.Labels,
		Models:          apiServer.listHelixModelsStatus(),
	}

	// Add debug logging to see memory values
	log.Debug().
		Str("runner_id", apiServer.runnerOptions.ID).
		Uint64("total_memory", status.TotalMemory).
		Uint64("free_memory", status.FreeMemory).
		Uint64("used_memory", status.UsedMemory).
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

	// Log GPU allocation received from scheduler
	log.Debug().
		Str("slot_id", slotRequest.ID.String()).
		Str("model", slotRequest.Attributes.Model).
		Interface("gpu_index", slotRequest.Attributes.GPUIndex).
		Ints("gpu_indices", slotRequest.Attributes.GPUIndices).
		Int("tensor_parallel_size", slotRequest.Attributes.TensorParallelSize).
		Msg("Using GPU allocation from scheduler")

	s := NewEmptySlot(CreateSlotParams{
		RunnerOptions:          apiServer.runnerOptions,
		ID:                     slotRequest.ID,
		Runtime:                slotRequest.Attributes.Runtime,
		Model:                  slotRequest.Attributes.Model,
		ModelMemoryRequirement: slotRequest.Attributes.ModelMemoryRequirement,
		ContextLength:          slotRequest.Attributes.ContextLength,
		RuntimeArgs:            slotRequest.Attributes.RuntimeArgs,
		APIServer:              apiServer,

		// GPU allocation from scheduler - authoritative allocation decision
		GPUIndex:           slotRequest.Attributes.GPUIndex,
		GPUIndices:         slotRequest.Attributes.GPUIndices,
		TensorParallelSize: slotRequest.Attributes.TensorParallelSize,
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
			ID:                 id,
			Runtime:            slot.Runtime(),
			Version:            slot.Version(),
			Model:              slot.Model,
			ContextLength:      slot.ContextLength,
			RuntimeArgs:        slot.RuntimeArgs, // Include runtime args for frontend display
			Active:             slot.Active,
			Ready:              slot.Ready,
			Status:             slot.Status(r.Context()),
			GPUIndex:           slot.GPUIndex,
			GPUIndices:         slot.GPUIndices,
			TensorParallelSize: slot.TensorParallelSize,
			CommandLine:        slot.CommandLine,
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
		ID:                 slotUUID,
		Runtime:            slot.Runtime(),
		Version:            slot.Version(),
		Model:              slot.Model,
		ContextLength:      slot.ContextLength,
		RuntimeArgs:        slot.RuntimeArgs, // Include runtime args for frontend display
		Active:             slot.Active,
		Ready:              slot.Ready,
		Status:             slot.Status(r.Context()),
		GPUIndex:           slot.GPUIndex,
		GPUIndices:         slot.GPUIndices,
		TensorParallelSize: slot.TensorParallelSize,
		CommandLine:        slot.CommandLine,
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
func (apiServer *HelixRunnerAPIServer) listHelixModelsHandler(w http.ResponseWriter, _ *http.Request) {
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

	log.Trace().Int("models_count", len(models)).Msg("setting helix models")

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

// getLogsSummary returns a summary of all log buffers
func (apiServer *HelixRunnerAPIServer) getLogsSummary(w http.ResponseWriter, _ *http.Request) {
	summary := apiServer.logManager.GetLogsSummary()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(summary); err != nil {
		log.Error().Err(err).Msg("Error encoding logs summary response")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// getSlotLogs returns logs for a specific slot
func (apiServer *HelixRunnerAPIServer) getSlotLogs(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	slotID := vars["slot_id"]

	if slotID == "" {
		http.Error(w, "slot_id is required", http.StatusBadRequest)
		return
	}

	// Parse query parameters
	query := r.URL.Query()
	maxLines := 1000 // default
	if lines := query.Get("lines"); lines != "" {
		if parsed, err := fmt.Sscanf(lines, "%d", &maxLines); err != nil || parsed != 1 {
			http.Error(w, "Invalid lines parameter", http.StatusBadRequest)
			return
		}
	}

	var since *time.Time
	if sinceStr := query.Get("since"); sinceStr != "" {
		if parsedTime, err := time.Parse(time.RFC3339, sinceStr); err == nil {
			since = &parsedTime
		}
	}

	level := query.Get("level") // ERROR, WARN, INFO, DEBUG

	// Get the log buffer (check both active and recent errors)
	var buffer *system.ModelInstanceLogBuffer
	if buffer = apiServer.logManager.GetBuffer(slotID); buffer == nil {
		// Check recent errors
		recentErrors := apiServer.logManager.GetRecentErrors()
		if recentBuffer, exists := recentErrors[slotID]; exists {
			buffer = recentBuffer
		}
	}

	if buffer == nil {
		http.Error(w, "Slot not found or no logs available", http.StatusNotFound)
		return
	}

	// Get logs and metadata
	logs := buffer.GetLogs(maxLines, since, level)
	metadata := buffer.GetMetadata()

	response := map[string]interface{}{
		"slot_id":  slotID,
		"metadata": metadata,
		"logs":     logs,
		"count":    len(logs),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Error().Err(err).Msg("Error encoding slot logs response")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}
