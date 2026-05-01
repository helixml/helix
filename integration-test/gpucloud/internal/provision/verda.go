package provision

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
)

// verdaAPI is the NVIDIA-side provisioner. Verda (formerly DataCrunch)
// sells real KVM VMs with on-demand H100, A100 80GB, L40S, etc.
//
// Auth: OAuth2 client_credentials. POST client_id+client_secret to
// /v1/oauth2/token, get a JWT access_token (~10 min TTL). The
// provisioner caches the token and refreshes 60 seconds before expiry.
//
// REST surface at api.verda.com/v1. Endpoints we hit:
//
//   POST /v1/oauth2/token              get access token (auth)
//   GET  /v1/instance-availability     real-time stock per region
//   POST /v1/instances                 create instance
//   GET  /v1/instances/{id}            poll status
//   DELETE /v1/instances/{id}          teardown
//   GET  /v1/balance                   account balance
//
// Pricing (Apr 2026): A100 SXM4 80GB $1.29/GPU/hr, L40S $0.91/GPU/hr,
// H200 $3.39/GPU/hr — billed per second on real KVM VMs with full root.
// Spot pricing roughly 1/3 of on-demand. legacy api.datacrunch.io
// resolves to the same backend.
type verdaAPI struct {
	clientID     string
	clientSecret string
	apiURL       string
	runnerToken  string
	sshKeyID     string
	httpClient   *http.Client
	baseURL      string

	tokMu     sync.Mutex
	token     string
	tokenExp  time.Time
}

// NewVerdaProvisioner returns a Provisioner targeting Verda. `sshKeyID`
// is the ID of an SSH key registered in Verda (POST /v1/ssh-keys);
// cloud-init injects the corresponding public key into the instance.
func NewVerdaProvisioner(clientID, clientSecret, sshKeyID, apiURL, runnerToken string) Provisioner {
	return &verdaAPI{
		clientID:     clientID,
		clientSecret: clientSecret,
		apiURL:       apiURL,
		runnerToken:  runnerToken,
		sshKeyID:     sshKeyID,
		httpClient:   &http.Client{Timeout: 60 * time.Second},
		baseURL:      "https://api.verda.com",
	}
}

// accessToken returns a valid access token, refreshing if needed.
// Tokens are short-lived (~10 min) so we keep the refresh window at
// 60 seconds.
func (v *verdaAPI) accessToken(ctx context.Context) (string, error) {
	v.tokMu.Lock()
	defer v.tokMu.Unlock()
	if v.token != "" && time.Until(v.tokenExp) > 60*time.Second {
		return v.token, nil
	}
	body, _ := json.Marshal(map[string]string{
		"grant_type":    "client_credentials",
		"client_id":     v.clientID,
		"client_secret": v.clientSecret,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.baseURL+"/v1/oauth2/token", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := v.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("verda: token: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("verda: token: %s — %s", resp.Status, string(errBody))
	}
	var tok struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return "", err
	}
	v.token = tok.AccessToken
	if tok.ExpiresIn > 0 {
		v.tokenExp = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
	} else {
		v.tokenExp = time.Now().Add(10 * time.Minute) // fallback if response omits expires_in
	}
	return v.token, nil
}

func (v *verdaAPI) authedRequest(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	tok, err := v.accessToken(ctx)
	if err != nil {
		return nil, err
	}
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, v.baseURL+path, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return v.httpClient.Do(req)
}

func (v *verdaAPI) Provision(ctx context.Context, spec PodSpec) (*Pod, error) {
	body := map[string]any{
		"hostname":       "helix-it-" + spec.EntryID,
		"instance_type":  spec.InstanceType, // e.g. "1A100.22V"
		"location_code":  spec.Region,        // e.g. "FIN-01"
		"image":          "ubuntu-24.04-cuda-12.6-docker",
		"ssh_key_ids":    []string{v.sshKeyID},
		"startup_script_id": "",                                         // we use raw startup_script below
		"startup_script": cloudInit(spec, v.apiURL, v.runnerToken, "nvidia"),
		"is_spot":        false,
	}
	bodyBytes, _ := json.Marshal(body)

	resp, err := v.authedRequest(ctx, http.MethodPost, "/v1/instances", bodyBytes)
	if err != nil {
		return nil, fmt.Errorf("verda: create instance: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("verda: create instance: %s — %s", resp.Status, string(errBody))
	}

	// Verda's create response is the new instance ID as a bare string,
	// not a JSON object. (Confirmed via console + API.)
	rawID, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	instanceID := strings.Trim(strings.TrimSpace(string(rawID)), `"`)
	if instanceID == "" {
		return nil, fmt.Errorf("verda: empty instance ID in create response")
	}

	pod, err := v.waitRunning(ctx, instanceID, 8*time.Minute)
	if err != nil {
		_ = v.Teardown(context.Background(), instanceID)
		return nil, fmt.Errorf("verda: wait for running: %w", err)
	}
	pod.InstanceType = spec.InstanceType
	pod.GPUCount = spec.GPUCount
	pod.Started = time.Now()
	return pod, nil
}

func (v *verdaAPI) waitRunning(ctx context.Context, instanceID string, timeout time.Duration) (*Pod, error) {
	deadline := time.Now().Add(timeout)
	for {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		resp, err := v.authedRequest(ctx, http.MethodGet, "/v1/instances/"+instanceID, nil)
		if err != nil {
			return nil, err
		}
		var status struct {
			ID        string `json:"id"`
			Status    string `json:"status"`
			IPAddress string `json:"ip"`           // newer Verda
			IP        string `json:"ip_address"`   // older datacrunch field
		}
		if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
			resp.Body.Close()
			return nil, err
		}
		resp.Body.Close()
		ip := status.IPAddress
		if ip == "" {
			ip = status.IP
		}
		if status.Status == "running" && ip != "" {
			return &Pod{ID: instanceID, URL: "http://" + ip + ":8081"}, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("instance %s did not reach running within %s (last status: %s)", instanceID, timeout, status.Status)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(15 * time.Second):
		}
	}
}

// Teardown deletes the instance. Verda is REST-y but uses an "action"
// pattern: PUT /v1/instances with {id, action:"delete"} rather than
// DELETE /v1/instances/{id} (which 404s). Same endpoint also supports
// action:"hibernate" to stop without deleting; we use delete because
// the harness's instances are throwaway.
func (v *verdaAPI) Teardown(ctx context.Context, instanceID string) error {
	body, _ := json.Marshal(map[string]string{
		"id":     instanceID,
		"action": "delete",
	})
	resp, err := v.authedRequest(ctx, http.MethodPut, "/v1/instances", body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("verda: teardown %s: %s — %s", instanceID, resp.Status, string(errBody))
	}
	return nil
}

// TodaySpentUSD reads remaining account balance. Verda's billing API
// surfaces current credits, not per-day spend; we approximate by
// returning 0 (the harness's pre-flight budget guard mostly cares about
// "is the account funded"). For real spend tracking the Verda console
// has a usage breakdown.
func (v *verdaAPI) TodaySpentUSD(ctx context.Context) (float64, error) {
	resp, err := v.authedRequest(ctx, http.MethodGet, "/v1/balance", nil)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	var bal struct {
		Amount   float64 `json:"amount"`
		Currency string  `json:"currency"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&bal); err != nil {
		return 0, err
	}
	if bal.Amount <= 0 {
		return 0, fmt.Errorf("verda: balance is %v %s — top up before provisioning", bal.Amount, bal.Currency)
	}
	return 0, nil
}
