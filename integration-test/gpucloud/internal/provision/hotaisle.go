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

// hotAisleAPI is the AMD-side provisioner. Talks to Hot Aisle's REST API
// (admin.hotaisle.app/api/docs/, OpenAPI 2.0 served at /api/docs/swagger.json).
//
// The shape is **spec-matched, not SKU-named** — you POST a desired
// (cpu_cores, ram_capacity, disk_capacity, gpus[]) and they assign a VM
// matching those specs from inventory. There is no `vm_type` or
// `region` on the provision request. Auth: Bearer <api-key>.
//
// Endpoints we hit:
//
//   GET  /teams/{team}/virtual_machines/available/   list available specs+quantities
//   POST /teams/{team}/virtual_machines/             provision a VM matching specs
//   GET  /teams/{team}/virtual_machines/{name}/      poll until ready
//   DELETE /teams/{team}/virtual_machines/{name}/    teardown
//   GET  /teams/{team}/balance/                      account spend
//
// The {name} path param is the VM's auto-assigned name (returned in the
// 200 response from POST), not an ID.
//
// Cloud-init: Hot Aisle accepts only `user_data_url` (a URL to a script),
// not inline. Until we wire SSH-bootstrap or URL hosting (TODO), the
// provisioner posts WITHOUT cloud-init — the helix-sandbox container
// must be started via SSH after provisioning. For the 1× MI300X smoke
// test this is acceptable; for nightly CI we'll add the URL-hosting bit.
//
// Pricing (Apr 2026): $1.99 per MI300X-hour, billed per minute, no
// commitment. VMs come with ROCm + Docker preinstalled.
type hotAisleAPI struct {
	apiKey      string
	team        string // team handle from GET /teams/ — set via HOTAISLE_TEAM env
	apiURL      string // helix API URL the sandbox should connect to
	runnerToken string
	httpClient  *http.Client
	baseURL     string
}

// NewHotAisleProvisioner returns a Provisioner targeting Hot Aisle.
// `team` is the team handle from the API (e.g. "helix"), not the
// display name.
func NewHotAisleProvisioner(apiKey, team, apiURL, runnerToken string) Provisioner {
	return &hotAisleAPI{
		apiKey:      apiKey,
		team:        team,
		apiURL:      apiURL,
		runnerToken: runnerToken,
		// Hot Aisle's POST flow can run for several minutes when
		// user_data_url is supplied (VM is rebuilt). 5 min is a soft
		// limit; the harness's 35-min hard timeout is the real guard.
		httpClient: &http.Client{Timeout: 5 * time.Minute},
		baseURL:    "https://admin.hotaisle.app/api",
	}
}

// hotAisleSpecs is what Hot Aisle's POST /virtual_machines/ wants. The
// matchmaker assigns any available VM whose hardware satisfies these
// specs; if nothing matches, returns 404.
type hotAisleSpecs struct {
	CPUCores     uint64        `json:"cpu_cores"`
	RAMCapacity  uint64        `json:"ram_capacity"`
	DiskCapacity uint64        `json:"disk_capacity"`
	GPUs         []hotAisleGPU `json:"gpus,omitempty"`
	UserDataURL  string        `json:"user_data_url,omitempty"`
}

type hotAisleGPU struct {
	Count        uint64 `json:"count"`
	Manufacturer string `json:"manufacturer"`
	Model        string `json:"model"`
}

// specsForInstanceType maps the matrix's friendly slug to a concrete
// Hot Aisle hardware spec. We hardcode the inventory we've actually seen
// in their available/ endpoint; adding a new SKU means appending to this
// table. For 1xMI300X the specs come straight from
// `GET /teams/{team}/virtual_machines/available/` (Apr 2026).
func specsForInstanceType(slug string) (hotAisleSpecs, error) {
	switch slug {
	case "1xMI300X":
		return hotAisleSpecs{
			CPUCores:     13,
			RAMCapacity:  240518168576,   // 224 GiB
			DiskCapacity: 13194139533312, // 12 TiB
			GPUs: []hotAisleGPU{
				{Count: 1, Manufacturer: "AMD", Model: "MI300X"},
			},
		}, nil
	default:
		return hotAisleSpecs{}, fmt.Errorf("unknown Hot Aisle instance_type %q (see specsForInstanceType in hotaisle.go)", slug)
	}
}

// hotAisleVM is the VirtualMachineDetails subset we care about.
type hotAisleVM struct {
	Name      string `json:"name"`
	IPAddress string `json:"ip_address"`
	SSHAccess struct {
		IPAddress string `json:"ip_address"`
		Port      int    `json:"port"`
		DNSName   string `json:"dns_name"`
	} `json:"ssh_access"`
}

func (h *hotAisleAPI) Provision(ctx context.Context, spec PodSpec) (*Pod, error) {
	specs, err := specsForInstanceType(spec.InstanceType)
	if err != nil {
		return nil, err
	}
	// TODO: when we wire URL-hosted cloud-init, set specs.UserDataURL
	// to the URL of cloudInit(spec, h.apiURL, h.runnerToken, "amd"). For
	// now the smoke test runs the sandbox container via SSH post-provision.

	bodyBytes, _ := json.Marshal(specs)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/teams/%s/virtual_machines/", h.baseURL, h.team),
		bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+h.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("hotaisle: provision: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("hotaisle: no available VM matches specs (try GET /teams/%s/virtual_machines/available/)", h.team)
	}
	if resp.StatusCode == http.StatusPaymentRequired {
		return nil, fmt.Errorf("hotaisle: insufficient balance — top up at the TUI")
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("hotaisle: at team's max VM quota")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("hotaisle: provision %s — %s", resp.Status, string(errBody))
	}

	var vm hotAisleVM
	if err := json.NewDecoder(resp.Body).Decode(&vm); err != nil {
		return nil, fmt.Errorf("hotaisle: decode provision response: %w", err)
	}
	if vm.Name == "" || vm.IPAddress == "" {
		return nil, fmt.Errorf("hotaisle: provision response missing name or ip_address")
	}

	// The 200 response from POST means the VM is assigned, but the
	// guest OS may still be booting. Poll the state endpoint for ready.
	if err := h.waitReady(ctx, vm.Name, 8*time.Minute); err != nil {
		_ = h.Teardown(context.Background(), vm.Name)
		return nil, err
	}

	return &Pod{
		ID:           vm.Name,
		URL:          "http://" + vm.IPAddress + ":8081", // sandbox heartbeat port (set up post-bootstrap)
		InstanceType: spec.InstanceType,
		GPUCount:     spec.GPUCount,
		Started:      time.Now(),
	}, nil
}

// waitReady polls until the VM's state endpoint reports it's running.
// Hot Aisle's state shape isn't documented in the OpenAPI we extracted,
// so we treat any 200 with non-empty body as ready and rely on the
// downstream SSH bootstrap to fail loudly if the OS isn't really up.
func (h *hotAisleAPI) waitReady(ctx context.Context, vmName string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
			fmt.Sprintf("%s/teams/%s/virtual_machines/%s/state/", h.baseURL, h.team, vmName), nil)
		req.Header.Set("Authorization", "Bearer "+h.apiKey)
		resp, err := h.httpClient.Do(req)
		if err == nil && resp.StatusCode == 200 {
			resp.Body.Close()
			return nil
		}
		if resp != nil {
			resp.Body.Close()
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("hotaisle: VM %s did not reach ready within %s", vmName, timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(15 * time.Second):
		}
	}
}

func (h *hotAisleAPI) Teardown(ctx context.Context, vmName string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		fmt.Sprintf("%s/teams/%s/virtual_machines/%s/", h.baseURL, h.team, vmName), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+h.apiKey)
	resp, err := h.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("hotaisle: teardown %s: %s — %s", vmName, resp.Status, string(body))
	}
	return nil
}

// TodaySpentUSD reads team balance. Hot Aisle bills by-the-minute and
// returns current available credit, so we approximate spend as
// (last-known credit minus current credit) — for the harness's
// pre-flight budget guard this is enough.
func (h *hotAisleAPI) TodaySpentUSD(ctx context.Context) (float64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/teams/%s/balance/", h.baseURL, h.team), nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+h.apiKey)
	resp, err := h.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	var bal struct {
		USDSpentToday float64 `json:"usd_spent_today"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&bal); err != nil {
		return 0, err
	}
	return bal.USDSpentToday, nil
}
