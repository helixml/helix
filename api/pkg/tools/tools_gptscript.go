package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gptscript-ai/gptscript/pkg/cache"
	"github.com/gptscript-ai/gptscript/pkg/gptscript"
	"github.com/gptscript-ai/gptscript/pkg/loader"
	"github.com/gptscript-ai/gptscript/pkg/monitor"
	"github.com/gptscript-ai/gptscript/pkg/openai"
	gptscript_types "github.com/gptscript-ai/gptscript/pkg/types"
	"github.com/rs/zerolog/log"

	api "github.com/helixml/helix/api/pkg/testfaster_client"
	"github.com/helixml/helix/api/pkg/types"
)

type GptScriptRunRequest struct {
	Tool           *types.Tool          `json:"tool"`
	History        []*types.Interaction `json:"history"`
	CurrentMessage string               `json:"current_message"`
	Action         string               `json:"action"`
}

func (c *ChainStrategy) RunRemoteGPTScriptAction(ctx context.Context, tool *types.Tool, history []*types.Interaction, currentMessage, action string) (*RunActionResponse, error) {

	req := GptScriptRunRequest{
		Tool:           tool,
		History:        history,
		CurrentMessage: currentMessage,
		Action:         action,
	}
	helixGptscriptUrl, err := getTestfasterCluster()
	if err != nil {
		return nil, fmt.Errorf("failed to get testfaster cluster: %w", err)
	}

	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := http.Post(helixGptscriptUrl+"/api/v1/run", "application/json", bytes.NewBuffer(reqBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to send HTTP request: %w", err)
	}
	defer resp.Body.Close()

	s, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return &RunActionResponse{
		Message:    string(s),
		RawMessage: string(s),
	}, nil

}

func (c *ChainStrategy) RunGPTScriptAction(ctx context.Context, tool *types.Tool, history []*types.Interaction, currentMessage, action string) (*RunActionResponse, error) {

	// Validate whether action is valid
	if action == "" {
		return nil, fmt.Errorf("action is required")
	}
	started := time.Now()

	gptOpt := gptscript.Options{
		Cache:   cache.Options{},
		OpenAI:  openai.Options{},
		Monitor: monitor.Options{},
		// Quiet: false,
		Env: os.Environ(),
	}

	gptScript, err := gptscript.New(&gptOpt)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize gptscript: %w", err)
	}
	defer gptScript.Close()

	var (
		prg gptscript_types.Program
	)

	switch {
	case tool.Config.GPTScript.Script != "":
		prg, err = loader.ProgramFromSource(ctx, tool.Config.GPTScript.Script, "")
		if err != nil {
			return nil, fmt.Errorf("failed to load program from source: %w", err)
		}
	case tool.Config.GPTScript.ScriptURL != "":
		resp, err := c.httpClient.Get(tool.Config.GPTScript.ScriptURL)
		if err != nil {
			return nil, fmt.Errorf("failed to get script from url: %w", err)
		}
		defer resp.Body.Close()

		bts, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read response body: %w", err)
		}

		prg, err = loader.ProgramFromSource(ctx, string(bts), "")
		if err != nil {
			return nil, fmt.Errorf("failed to load program from source: %w", err)
		}
	default:
		return nil, fmt.Errorf("no script or script url provided")
	}

	s, err := gptScript.Run(ctx, prg, os.Environ(), currentMessage)
	if err != nil {
		return nil, fmt.Errorf("failed to run script: %w", err)
	}

	log.Info().
		Str("tool", tool.Name).
		Str("action", action).
		Dur("time_taken", time.Since(started)).
		Msg("GPTScript done")

	return &RunActionResponse{
		Message:    s,
		RawMessage: s,
	}, nil
}

// testfaster microVM integraton

const TestfasterPoolTimeoutHours = 1

const osDockerfile = `# This dockerfile defines the base disk image for your VMs
FROM quay.io/testfaster/kube-ubuntu
# poor man's versioning
ENV cache 2024-03-28c
# Some common dependencies for gptscript stuff
RUN apt-get update && apt install -y unzip wget sqlite
RUN wget https://storage.googleapis.com/helixml/helix && chmod +x helix && mv helix /usr/local/bin
`

const bootstrapScript = `
# This gets run after each individual VM starts up, so
# start services you need in your tests here and they'll be
# already running when you testctl get
#!/bin/bash
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

func getTestfasterCluster() (string, error) {
	if os.Getenv("HELIX_TESTFASTER_URL") == "" {
		return "", fmt.Errorf("Please set HELIX_TESTFASTER_URL to use remote gptscript execution - join the helix.ml discord for more info")
	}
	if os.Getenv("HELIX_TESTFASTER_TOKEN") == "" {
		return "", fmt.Errorf("Please set HELIX_TESTFASTER_TOKEN to use remote gptscript execution - join the helix.ml discord for more info")
	}

	apiHandler := api.NewHttpApiHandler(
		os.Getenv("HELIX_TESTFASTER_URL"),
		os.Getenv("HELIX_TESTFASTER_TOKEN"),
	)

	lease, err := apiHandler.Get(&api.PoolRequest{
		Config: api.PoolConfig{
			Name: "Helix GPTScript",
			Base: api.BaseConfig{
				OsDockerfile:        osDockerfile,
				KernelImage:         "quay.io/testfaster/ignite-kernel:latest",
				DockerBakeScript:    "",
				PreloadDockerImages: []string{},
				PrewarmScript:       bootstrapScript,
				KubernetesVersion:   "",
			},
			Runtime: api.RuntimeConfig{
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
		return "", err
	}
	var externalIP string
	var port string
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
		return "", fmt.Errorf("no external IP found in testfaster returned config")
	}
	if port == "" {
		return "", fmt.Errorf("no port found in testfaster returned config")
	}

	// TODO: drop VM when we're done with it

	return fmt.Sprintf("http://%s:%s", externalIP, port), nil
}
