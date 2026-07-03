package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

type DetectedProvider struct {
	Name       string   `json:"name"`
	ServerType string   `json:"server_type"`
	BaseURL    string   `json:"base_url"`
	Models     []string `json:"models"`
}

type DetectLocalResponse struct {
	Providers []DetectedProvider `json:"providers"`
}

type probeTarget struct {
	name       string
	serverType string
	port       int
}

var localProbeTargets = []probeTarget{
	{name: "LM Studio", serverType: "lmstudio", port: 1234},
	{name: "Ollama", serverType: "ollama", port: 11434},
	{name: "Local Server", serverType: "generic", port: 8000},
}

var probeHosts = []string{
	"10.0.2.2",
	"host.docker.internal",
	"localhost",
}

func (s *HelixAPIServer) detectLocalProviders(rw http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var mu sync.Mutex
	var wg sync.WaitGroup
	var detected []DetectedProvider

	for _, target := range localProbeTargets {
		wg.Add(1)
		go func(t probeTarget) {
			defer wg.Done()

			for _, host := range probeHosts {
				baseURL := fmt.Sprintf("http://%s:%d/v1", host, t.port)
				models, serverType := probeEndpoint(ctx, baseURL)
				if models != nil {
					if serverType == "" {
						serverType = t.serverType
					}
					name := t.name
					if serverType == "ollama" && t.serverType != "ollama" {
						name = "Ollama"
					} else if serverType == "lmstudio" && t.serverType != "lmstudio" {
						name = "LM Studio"
					}

					mu.Lock()
					detected = append(detected, DetectedProvider{
						Name:       name,
						ServerType: serverType,
						BaseURL:    baseURL,
						Models:     models,
					})
					mu.Unlock()
					return
				}
			}
		}(target)
	}

	wg.Wait()

	resp := DetectLocalResponse{Providers: detected}
	rw.Header().Set("Content-Type", "application/json")
	json.NewEncoder(rw).Encode(resp)
}

func probeEndpoint(ctx context.Context, baseURL string) (models []string, serverType string) {
	client := &http.Client{Timeout: 2 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/models", nil)
	if err != nil {
		return nil, ""
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, ""
	}

	var result struct {
		Data []struct {
			ID      string `json:"id"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, ""
	}

	if len(result.Data) == 0 {
		return nil, ""
	}

	for _, m := range result.Data {
		models = append(models, m.ID)
		if serverType == "" {
			serverType = inferServerType(m.OwnedBy)
		}
	}

	log.Info().Str("base_url", baseURL).Int("model_count", len(models)).Str("server_type", serverType).Msg("detected local provider")
	return models, serverType
}

func (s *HelixAPIServer) ensureLocalModelLoaded(ctx context.Context, providerName, modelID string) {
	endpoints, err := s.Store.ListProviderEndpoints(ctx, &store.ListProviderEndpointsQuery{WithGlobal: true})
	if err != nil {
		return
	}

	var endpoint *types.ProviderEndpoint
	for _, ep := range endpoints {
		if ep.Name == providerName {
			endpoint = ep
			break
		}
	}
	if endpoint == nil {
		return
	}

	if !strings.Contains(endpoint.Name, "lmstudio") && !strings.Contains(endpoint.BaseURL, ":1234") {
		return
	}

	mgmtBase := managementBaseURL(endpoint.BaseURL)
	client := &http.Client{Timeout: 5 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, mgmtBase+"/api/v1/models", nil)
	if err != nil {
		return
	}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	var result struct {
		Models []struct {
			Key             string `json:"key"`
			LoadedInstances []struct {
				ID string `json:"id"`
			} `json:"loaded_instances"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return
	}

	for _, m := range result.Models {
		if m.Key == modelID {
			if len(m.LoadedInstances) > 0 {
				return
			}
			log.Info().Str("model", modelID).Str("provider", providerName).Msg("auto-loading model in LM Studio")
			loadBody, _ := json.Marshal(map[string]interface{}{"model": modelID, "context_length": 32768})
			loadReq, err := http.NewRequestWithContext(ctx, http.MethodPost, mgmtBase+"/api/v1/models/load", bytes.NewReader(loadBody))
			if err != nil {
				return
			}
			loadReq.Header.Set("Content-Type", "application/json")
			loadClient := &http.Client{Timeout: 120 * time.Second}
			loadResp, err := loadClient.Do(loadReq)
			if err != nil {
				log.Err(err).Str("model", modelID).Msg("failed to auto-load model")
				return
			}
			loadResp.Body.Close()
			log.Info().Str("model", modelID).Msg("model auto-loaded successfully")
			return
		}
	}
}

func inferServerType(ownedBy string) string {
	lower := strings.ToLower(ownedBy)
	switch {
	case strings.Contains(lower, "library"):
		return "ollama"
	case strings.Contains(lower, "lmstudio"):
		return "lmstudio"
	default:
		return ""
	}
}

type LocalModel struct {
	Key              string              `json:"key"`
	Type             string              `json:"type"`
	DisplayName      string              `json:"display_name,omitempty"`
	Architecture     string              `json:"architecture,omitempty"`
	Quantization     *LocalModelQuant    `json:"quantization,omitempty"`
	SizeBytes        int64               `json:"size_bytes"`
	ParamsString     string              `json:"params_string,omitempty"`
	MaxContextLength int                 `json:"max_context_length"`
	Format           string              `json:"format,omitempty"`
	LoadedInstances  []LocalModelInstance `json:"loaded_instances"`
}

type LocalModelQuant struct {
	Name          string  `json:"name,omitempty"`
	BitsPerWeight float64 `json:"bits_per_weight,omitempty"`
}

type LocalModelInstance struct {
	ID     string                 `json:"id"`
	Config map[string]interface{} `json:"config,omitempty"`
}

func managementBaseURL(providerBaseURL string) string {
	u := strings.TrimSuffix(providerBaseURL, "/")
	u = strings.TrimSuffix(u, "/v1")
	return u
}

func (s *HelixAPIServer) listLocalModels(rw http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	endpointID := vars["id"]

	endpoint, err := s.Store.GetProviderEndpoint(r.Context(), &store.GetProviderEndpointsQuery{ID: endpointID})
	if err != nil {
		http.Error(rw, "Provider endpoint not found", http.StatusNotFound)
		return
	}

	mgmtURL := managementBaseURL(endpoint.BaseURL) + "/api/v1/models"
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, mgmtURL, nil)
	if err != nil {
		http.Error(rw, "Failed to create request", http.StatusInternalServerError)
		return
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Warn().Err(err).Str("url", mgmtURL).Msg("failed to reach local model server")
		rw.Header().Set("Content-Type", "application/json")
		json.NewEncoder(rw).Encode(map[string]interface{}{"models": []interface{}{}, "error": "Server unreachable"})
		return
	}
	defer resp.Body.Close()

	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(resp.StatusCode)
	io.Copy(rw, resp.Body)
}

func (s *HelixAPIServer) loadLocalModel(rw http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	endpointID := vars["id"]

	endpoint, err := s.Store.GetProviderEndpoint(r.Context(), &store.GetProviderEndpointsQuery{ID: endpointID})
	if err != nil {
		http.Error(rw, "Provider endpoint not found", http.StatusNotFound)
		return
	}

	mgmtURL := managementBaseURL(endpoint.BaseURL) + "/api/v1/models/load"
	client := &http.Client{Timeout: 120 * time.Second}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, mgmtURL, r.Body)
	if err != nil {
		http.Error(rw, "Failed to create request", http.StatusInternalServerError)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		log.Err(err).Str("url", mgmtURL).Msg("failed to load model")
		http.Error(rw, "Failed to load model: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(resp.StatusCode)
	io.Copy(rw, resp.Body)
}

func (s *HelixAPIServer) unloadLocalModel(rw http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	endpointID := vars["id"]

	endpoint, err := s.Store.GetProviderEndpoint(r.Context(), &store.GetProviderEndpointsQuery{ID: endpointID})
	if err != nil {
		http.Error(rw, "Provider endpoint not found", http.StatusNotFound)
		return
	}

	var reqBody struct {
		Model string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		http.Error(rw, "Invalid request body", http.StatusBadRequest)
		return
	}

	lmsBody, _ := json.Marshal(map[string]string{"instance_id": reqBody.Model})
	mgmtURL := managementBaseURL(endpoint.BaseURL) + "/api/v1/models/unload"
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, mgmtURL, bytes.NewReader(lmsBody))
	if err != nil {
		http.Error(rw, "Failed to create request", http.StatusInternalServerError)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		log.Err(err).Str("url", mgmtURL).Msg("failed to unload model")
		http.Error(rw, "Failed to unload model: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(resp.StatusCode)
	io.Copy(rw, resp.Body)
}
