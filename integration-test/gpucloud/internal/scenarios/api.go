package scenarios

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// API helpers used by every scenario. Auth: the harness owns a separate
// admin token for orchestration calls (creating / assigning profiles)
// and uses the runner token for sandbox-side calls.
//
// All paths assume the harness runs against an API server reachable at
// HELIX_TEST_API_URL with a HELIX_TEST_ADMIN_TOKEN env var set.

func httpClient() *http.Client { return &http.Client{Timeout: 60 * time.Second} }

func adminAPIBase() string {
	if v := os.Getenv("HELIX_TEST_API_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "http://localhost:8080"
}

func adminToken() string { return os.Getenv("HELIX_TEST_ADMIN_TOKEN") }

func (r *Runner) sandboxOnlineWithGPUs(ctx context.Context) (bool, error) {
	type gpu struct {
		Vendor string `json:"vendor"`
	}
	type sandbox struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		GPUs   []gpu  `json:"gpus"`
	}
	body, err := r.adminGET(ctx, "/api/v1/sandboxes")
	if err != nil {
		return false, err
	}
	var sbxs []sandbox
	if err := json.Unmarshal(body, &sbxs); err != nil {
		return false, err
	}
	for _, s := range sbxs {
		if strings.HasPrefix(s.ID, "it-") && s.Status == "online" && len(s.GPUs) > 0 {
			return true, nil
		}
	}
	return false, nil
}

type compatProfile struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (r *Runner) listCompatibleProfiles(ctx context.Context) ([]compatProfile, error) {
	sandboxID := r.podSandboxID()
	body, err := r.adminGET(ctx, "/api/v1/runners/"+sandboxID+"/compatible-profiles")
	if err != nil {
		return nil, err
	}
	var ps []compatProfile
	if err := json.Unmarshal(body, &ps); err != nil {
		return nil, err
	}
	return ps, nil
}

func (r *Runner) assignProfile(ctx context.Context, profilePath string) error {
	status, body, err := r.assignProfileRaw(ctx, profilePath)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("assign-profile: %d — %s", status, body)
	}
	return nil
}

func (r *Runner) assignProfileRaw(ctx context.Context, profilePath string) (int, string, error) {
	// Look up the profile by name (the harness creates each matrix
	// profile in the API once at startup; here we just need the ID).
	pid, err := r.profileIDByPath(ctx, profilePath)
	if err != nil {
		return 0, "", err
	}
	sandboxID := r.podSandboxID()
	bodyBytes, _ := json.Marshal(map[string]string{"profile_id": pid})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		adminAPIBase()+"/api/v1/runners/"+sandboxID+"/assign-profile",
		bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer "+adminToken())
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient().Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body), nil
}

func (r *Runner) clearProfileAPI(ctx context.Context) error {
	sandboxID := r.podSandboxID()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		adminAPIBase()+"/api/v1/runners/"+sandboxID+"/clear-profile", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken())
	resp, err := httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("clear-profile: %d — %s", resp.StatusCode, string(body))
	}
	return nil
}

func (r *Runner) waitForRunning(ctx context.Context, timeout time.Duration) error {
	type sandbox struct {
		ID            string            `json:"id"`
		ProfileStatus string            `json:"profile_status"`
		ServiceHealth map[string]string `json:"service_health"`
	}
	deadline := time.Now().Add(timeout)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		body, err := r.adminGET(ctx, "/api/v1/sandboxes")
		if err == nil {
			var sbxs []sandbox
			if err := json.Unmarshal(body, &sbxs); err == nil {
				for _, s := range sbxs {
					if s.ID != r.podSandboxID() {
						continue
					}
					if s.ProfileStatus == "running" {
						all := true
						for _, h := range s.ServiceHealth {
							if h != "healthy" {
								all = false
								break
							}
						}
						if all {
							return nil
						}
					}
					if s.ProfileStatus == "failed" {
						return fmt.Errorf("profile failed during apply")
					}
				}
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("waitForRunning: timeout after %s", timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(15 * time.Second):
		}
	}
}

func (r *Runner) chatCompletion(ctx context.Context, model, prompt string) error {
	bodyBytes, _ := json.Marshal(map[string]any{
		"model":      model,
		"provider":   "helix",
		"messages":   []map[string]string{{"role": "user", "content": prompt}},
		"max_tokens": 16,
	})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		adminAPIBase()+"/v1/chat/completions",
		bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer "+adminToken())
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("chat completion: %d — %s", resp.StatusCode, string(body))
	}
	return nil
}

// modelAvailable hits /v1/models and returns true if the given model is in
// the list. Used by profile_switch to check the prior profile's models
// have stopped being served.
func (r *Runner) modelAvailable(ctx context.Context, model string) bool {
	body, err := r.adminGET(ctx, "/api/v1/v1/models")
	if err != nil {
		return false
	}
	var resp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return false
	}
	for _, m := range resp.Data {
		if strings.EqualFold(m.ID, model) {
			return true
		}
	}
	return false
}

func (r *Runner) anyModelAvailable(ctx context.Context) bool {
	body, err := r.adminGET(ctx, "/api/v1/v1/models")
	if err != nil {
		return false
	}
	var resp struct {
		Data []any `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return false
	}
	return len(resp.Data) > 0
}

func (r *Runner) adminGET(ctx context.Context, path string) ([]byte, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, adminAPIBase()+path, nil)
	req.Header.Set("Authorization", "Bearer "+adminToken())
	resp, err := httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("GET %s: %d — %s", path, resp.StatusCode, string(body))
	}
	return body, nil
}

// profileIDByPath looks up an existing profile by name (derived from the
// path basename) and returns its rprof_ ID. The harness creates one
// profile per matrix entry at startup; here we just look it up.
func (r *Runner) profileIDByPath(ctx context.Context, path string) (string, error) {
	want := profileNameFromPath(path)
	body, err := r.adminGET(ctx, "/api/v1/runner-profiles")
	if err != nil {
		return "", err
	}
	var ps []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &ps); err != nil {
		return "", err
	}
	for _, p := range ps {
		if p.Name == want {
			return p.ID, nil
		}
	}
	return "", fmt.Errorf("profile %q not found via API (did you forget to create it?)", want)
}

// profileModels reads the compose YAML at the path and returns the list of
// model names. We re-parse rather than ask the API to keep this scenario
// independent of the API state.
func (r *Runner) profileModels(profilePath string) ([]string, error) {
	data, err := os.ReadFile(profilePath)
	if err != nil {
		return nil, err
	}
	var compose struct {
		Services map[string]struct {
			Command yaml.Node `yaml:"command"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(data, &compose); err != nil {
		return nil, err
	}
	var models []string
	for _, svc := range compose.Services {
		toks := commandTokens(svc.Command)
		if name := flagValue(toks, "--served-model-name"); name != "" {
			models = append(models, name)
			continue
		}
		if m := flagValue(toks, "--model"); m != "" {
			models = append(models, strings.ToLower(filepath.Base(m)))
		}
	}
	return models, nil
}

func (r *Runner) podSandboxID() string {
	// Matches what RunPod cloud-init sets via SANDBOX_INSTANCE_ID env var.
	return "it-" + strings.TrimPrefix(r.pod.ID, "dryrun-")
}

func profileNameFromPath(p string) string {
	base := filepath.Base(p)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func commandTokens(n yaml.Node) []string {
	switch n.Kind {
	case yaml.ScalarNode:
		return strings.Fields(n.Value)
	case yaml.SequenceNode:
		out := make([]string, 0, len(n.Content))
		for _, c := range n.Content {
			out = append(out, c.Value)
		}
		return out
	}
	return nil
}

func flagValue(tokens []string, flag string) string {
	for i, t := range tokens {
		if t == flag && i+1 < len(tokens) {
			return tokens[i+1]
		}
		if strings.HasPrefix(t, flag+"=") {
			return strings.TrimPrefix(t, flag+"=")
		}
	}
	return ""
}
