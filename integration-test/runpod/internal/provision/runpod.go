package provision

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// runpodAPI is the real provisioner. Talks to RunPod's v2 REST API.
//
// Auth: Bearer <api-key>. Endpoints:
//   POST /v2/pod                              create
//   GET  /v2/pod/{id}                         poll until "RUNNING"
//   POST /v2/pod/{id}/stop                    teardown
//   GET  /v2/billing/usage?start=YYYY-MM-DD   daily spend
//
// The cloud-init script (templated in cloudInit) installs the right GPU
// runtime, pulls the helix-sandbox image, and boots it.
type runpodAPI struct {
	apiKey      string
	apiURL      string // helix API URL the sandbox should connect to
	runnerToken string
	httpClient  *http.Client
	baseURL     string // RunPod API base — overridable for tests
}

// NewRunPodProvisioner returns a real provisioner. apiURL + runnerToken
// are passed into each pod's environment so the sandbox can register.
func NewRunPodProvisioner(apiKey, apiURL, runnerToken string) Provisioner {
	return &runpodAPI{
		apiKey:      apiKey,
		apiURL:      apiURL,
		runnerToken: runnerToken,
		httpClient:  &http.Client{Timeout: 60 * time.Second},
		baseURL:     "https://api.runpod.io",
	}
}

func (r *runpodAPI) Provision(ctx context.Context, spec PodSpec) (*Pod, error) {
	body := map[string]any{
		"name":            "helix-it-" + spec.EntryID,
		"imageName":       spec.ImageRef,
		"gpuTypeId":       spec.GPUType,
		"gpuCount":        spec.GPUCount,
		"containerDiskInGb": 100,
		"volumeMountPath": "/models",
		// RunPod auto-terminates the pod at this many minutes — belt &
		// braces against the harness leaking GPU spend.
		"terminationMinutes": 35,
		// docker run -e ... — the sandbox's entrypoint reads these.
		"env": []map[string]string{
			{"key": "HELIX_API_URL", "value": r.apiURL},
			{"key": "RUNNER_TOKEN", "value": r.runnerToken},
			{"key": "SANDBOX_INSTANCE_ID", "value": "it-" + spec.EntryID},
			{"key": "GPU_VENDOR", "value": vendorOf(spec.GPUType)},
			// NVIDIA pods auto-mount /dev/nvidia*; AMD pods auto-mount
			// /dev/kfd + /dev/dri.
		},
		"region": spec.Region,
	}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.baseURL+"/v2/pod", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+r.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("create pod: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create pod: %s — %s", resp.Status, string(errBody))
	}

	var created struct {
		ID    string `json:"id"`
		PodIP string `json:"podIp"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return nil, fmt.Errorf("decode create response: %w", err)
	}

	// Poll for RUNNING.
	pod, err := r.waitRunning(ctx, created.ID, 8*time.Minute)
	if err != nil {
		_ = r.Teardown(context.Background(), created.ID)
		return nil, fmt.Errorf("wait for running: %w", err)
	}
	pod.GPUType = spec.GPUType
	pod.GPUCount = spec.GPUCount
	pod.Started = time.Now()
	return pod, nil
}

func (r *runpodAPI) waitRunning(ctx context.Context, podID string, timeout time.Duration) (*Pod, error) {
	deadline := time.Now().Add(timeout)
	for {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.baseURL+"/v2/pod/"+podID, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+r.apiKey)
		resp, err := r.httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		var status struct {
			ID            string `json:"id"`
			DesiredStatus string `json:"desiredStatus"`
			RuntimeStatus string `json:"runtimeStatus"`
			PodIP         string `json:"podIp"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
			resp.Body.Close()
			return nil, err
		}
		resp.Body.Close()
		if status.RuntimeStatus == "RUNNING" && status.PodIP != "" {
			return &Pod{ID: podID, URL: "http://" + status.PodIP + ":8081"}, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("pod did not reach RUNNING within %s (last status: %s)", timeout, status.RuntimeStatus)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(15 * time.Second):
		}
	}
}

func (r *runpodAPI) Teardown(ctx context.Context, podID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.baseURL+"/v2/pod/"+podID+"/stop", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+r.apiKey)
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("teardown %s: %s — %s", podID, resp.Status, string(body))
	}
	return nil
}

func (r *runpodAPI) TodaySpentUSD(ctx context.Context) (float64, error) {
	today := time.Now().UTC().Format("2006-01-02")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.baseURL+"/v2/billing/usage?start="+today, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+r.apiKey)
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	var usage struct {
		USD float64 `json:"usd"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&usage); err != nil {
		return 0, err
	}
	return usage.USD, nil
}

// vendorOf picks the GPU_VENDOR env var value from the RunPod GPU type
// string. RunPod's strings start with "NVIDIA " or "AMD ".
func vendorOf(runpodGPUType string) string {
	if len(runpodGPUType) >= 6 && runpodGPUType[:6] == "NVIDIA" {
		return "nvidia"
	}
	if len(runpodGPUType) >= 3 && runpodGPUType[:3] == "AMD" {
		return "amd"
	}
	return ""
}
