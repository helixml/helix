package gptscript

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/helixml/helix/api/pkg/config"
	testfaster "github.com/helixml/helix/api/pkg/testfaster_client"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// TestFasterExecutor runs GPTScript scripts on the TestFaster cluster
type TestFasterExecutor struct {
	cfg *config.ServerConfig
}

var _ Executor = &TestFasterExecutor{}

func NewTestFasterExecutor(cfg *config.ServerConfig) *TestFasterExecutor {
	return &TestFasterExecutor{
		cfg: cfg,
	}
}

func (e *TestFasterExecutor) ExecuteApp(ctx context.Context, app *types.GptScriptGithubApp) (*types.GptScriptResponse, error) {
	return e.runGPTAppTestfaster(ctx, app)
}

func (e *TestFasterExecutor) ExecuteScript(ctx context.Context, script *types.GptScript) (*types.GptScriptResponse, error) {
	return e.runGPTScriptTestfaster(ctx, script)
}

// TODO: delete
type TestFasterCluster struct {
	PoolID  string
	LeaseID string
	URL     string
}

const TestfasterPoolTimeoutHours = 1

const osDockerfile = `# This dockerfile defines the base disk image for your VMs
FROM quay.io/testfaster/kube-ubuntu
# poor man's versioning
ENV cache 2024-04-16h
# Some common dependencies for gptscript stuff
RUN apt-get update && apt install -y unzip wget sqlite
RUN wget https://storage.googleapis.com/helixml/helix && chmod +x helix && mv helix /usr/local/bin
`

const bootstrapScript = `
# This gets run after each individual VM starts up, so
# start services you need in your tests here and they'll be
# already running when you testctl get the lease
#!/bin/bash
echo "cache 2024-04-16h"
set -euo pipefail
sed -i 's/^export //' /root/secrets
mkdir -p /gptscript
cat > /etc/systemd/system/gptscript.service <<EOF
[Unit]
Description=Run gptscript

[Service]
EnvironmentFile=-/root/secrets
ExecStart=/usr/local/bin/helix gptscript
Restart=always
User=root
WorkingDirectory=/gptscript

[Install]
WantedBy=multi-user.target
EOF
systemctl daemon-reload
systemctl enable gptscript.service
systemctl start gptscript.service
`

// TODO: delete
func getTestfasterAPIHandler(cfg *config.ServerConfig) (*testfaster.HTTPAPIHandler, error) {
	if cfg.GPTScript.TestFaster.URL == "" {
		return nil, fmt.Errorf("Please set HELIX_TESTFASTER_URL to use remote gptscript execution - join the helix.ml discord for more info")
	}
	if cfg.GPTScript.TestFaster.Token == "" {
		return nil, fmt.Errorf("Please set HELIX_TESTFASTER_TOKEN to use remote gptscript execution - join the helix.ml discord for more info")
	}

	apiHandler := testfaster.NewHTTPAPIHandler(
		cfg.GPTScript.TestFaster.URL,
		cfg.GPTScript.TestFaster.Token,
	)

	return apiHandler, nil
}

// TODO: delete
func getTestfasterCluster(cfg *config.ServerConfig) (*TestFasterCluster, error) {
	// used for iterating on the gptscript server code without
	// having to keep pushing new testfaster configs
	// shold be of the form http://localhost:8080
	// api paths are appended to this
	if os.Getenv("HELIX_TESTFASTER_MOCK_VM_URL") != "" {
		return &TestFasterCluster{
			URL: os.Getenv("HELIX_TESTFASTER_MOCK_VM_URL"),
		}, nil
	}

	apiHandler, err := getTestfasterAPIHandler(cfg)
	if err != nil {
		return nil, err
	}
	lease, err := apiHandler.Get(&testfaster.PoolRequest{
		Config: testfaster.PoolConfig{
			Name: "Helix GPTScript",
			Base: testfaster.BaseConfig{
				OsDockerfile:        osDockerfile,
				KernelImage:         "quay.io/testfaster/ignite-kernel:latest",
				DockerBakeScript:    "",
				PreloadDockerImages: []string{},
				PrewarmScript:       bootstrapScript,
				KubernetesVersion:   "",
			},
			Runtime: testfaster.RuntimeConfig{
				Cpus:   4,
				Memory: "1G",
				Disk:   "2G",
			},
			PrewarmPoolSize:               10,
			MaxPoolSize:                   200,
			DefaultLeaseTimeout:           fmt.Sprintf("%dh", TestfasterPoolTimeoutHours),
			DefaultLeaseAllocationTimeout: "1h",
			PoolSleepTimeout:              "never",
			Shared:                        true,
		},
		Meta: map[string]string{},
	})
	if err != nil {
		return nil, err
	}
	var externalIP string
	var port string

	fmt.Printf("lease --------------------------------------\n")
	spew.Dump(lease)
	config := lease.Kubeconfig // not really a kubeconfig, don't be alarmed
	lines := strings.Split(config, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "##EXTERNAL_IP=") {
			externalIP = strings.TrimPrefix(line, "##EXTERNAL_IP=")
		}
		if strings.HasPrefix(line, "##ISTIO_FORWARDED_PORT=") {
			port = strings.TrimPrefix(line, "##ISTIO_FORWARDED_PORT=")
		}
	}
	if externalIP == "" {
		return nil, fmt.Errorf("no external IP found in testfaster returned config")
	}
	if port == "" {
		return nil, fmt.Errorf("no port found in testfaster returned config")
	}

	return &TestFasterCluster{
		PoolID:  lease.Pool,
		LeaseID: lease.ID,
		URL:     fmt.Sprintf("http://%s:%s", externalIP, port),
	}, nil
}

func (e *TestFasterExecutor) runGPTScriptTestfaster(ctx context.Context, script *types.GptScript) (*types.GptScriptResponse, error) {
	cluster, err := getTestfasterCluster(e.cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to get testfaster cluster: %w", err)
	}

	defer func() {
		if cluster.PoolID != "" && cluster.LeaseID != "" {
			apiHandler, err := getTestfasterAPIHandler(e.cfg)
			if err != nil {
				log.Error().Err(err).Msg("failed to release testfaster lease")
			}
			err = apiHandler.DeleteLease(cluster.PoolID, cluster.LeaseID)
			if err != nil {
				log.Error().Err(err).Msg("failed to release testfaster lease")
			}
		}
	}()

	reqBytes, err := json.Marshal(script)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/api/v1/run/script", cluster.URL), bytes.NewBuffer(reqBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send HTTP request: %w", err)
	}
	defer resp.Body.Close()

	var result types.GptScriptResponse
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	err = json.Unmarshal(body, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to decode response body: %w %s", err, string(body))
	}

	return &result, nil
}

func (e *TestFasterExecutor) runGPTAppTestfaster(ctx context.Context, app *types.GptScriptGithubApp) (*types.GptScriptResponse, error) {
	cluster, err := getTestfasterCluster(e.cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to get testfaster cluster: %w", err)
	}

	defer func() {
		if cluster.PoolID != "" && cluster.LeaseID != "" {
			apiHandler, err := getTestfasterAPIHandler(e.cfg)
			if err != nil {
				log.Error().Err(err).Msg("failed to release testfaster lease")
			}
			err = apiHandler.DeleteLease(cluster.PoolID, cluster.LeaseID)
			if err != nil {
				log.Error().Err(err).Msg("failed to release testfaster lease")
			}
		}
	}()

	reqBytes, err := json.Marshal(app)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/api/v1/run/app", cluster.URL), bytes.NewBuffer(reqBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send HTTP request: %w", err)
	}
	defer resp.Body.Close()

	var result types.GptScriptResponse
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	err = json.Unmarshal(body, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to decode response body: %w %s", err, string(body))
	}

	return &result, nil
}
