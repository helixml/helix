//go:build livetest

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestOpenCodeConfigIsAcceptedByRealBinary feeds the exact config the daemon
// generates to a real `opencode acp` process and checks the agent comes up
// with only the Helix model selectable.
//
// Not part of the normal suite: it needs an opencode binary and a reachable
// model endpoint. Run with:
//
//	OPENCODE_BIN=/path/to/opencode \
//	OPENCODE_TEST_BASE_URL=https://api.openai.com/v1 \
//	OPENCODE_TEST_MODEL=gpt-4.1-mini \
//	OPENCODE_TEST_API_KEY=sk-... \
//	go test -tags livetest -run TestOpenCodeConfigIsAcceptedByRealBinary ./cmd/settings-sync-daemon/
func TestOpenCodeConfigIsAcceptedByRealBinary(t *testing.T) {
	bin := os.Getenv("OPENCODE_BIN")
	baseURL := os.Getenv("OPENCODE_TEST_BASE_URL")
	model := os.Getenv("OPENCODE_TEST_MODEL")
	apiKey := os.Getenv("OPENCODE_TEST_API_KEY")
	if bin == "" || baseURL == "" || model == "" || apiKey == "" {
		t.Skip("set OPENCODE_BIN, OPENCODE_TEST_BASE_URL, OPENCODE_TEST_MODEL and OPENCODE_TEST_API_KEY to run")
	}

	d := &SettingsDaemon{
		codeAgentConfig: &CodeAgentConfig{
			Runtime:   "opencode",
			BaseURL:   baseURL,
			Model:     model,
			APIType:   "openai",
			MaxTokens: 128000,
		},
		userAPIKey: apiKey,
	}
	content, err := marshalOpenCodeConfig(d.buildOpenCodeConfig(baseURL))
	require.NoError(t, err)

	workdir := t.TempDir()
	cmd := exec.Command(bin, "acp", "--cwd", workdir)
	cmd.Dir = workdir
	cmd.Env = append(os.Environ(),
		"OPENCODE_CONFIG_CONTENT="+content,
		"HELIX_API_KEY="+apiKey,
		"XDG_CONFIG_HOME="+t.TempDir(),
		"XDG_DATA_HOME="+t.TempDir(),
	)

	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	send := func(id int, method string, params any) {
		payload, err := json.Marshal(map[string]any{
			"jsonrpc": "2.0", "id": id, "method": method, "params": params,
		})
		require.NoError(t, err)
		_, err = stdin.Write(append(payload, '\n'))
		require.NoError(t, err)
	}

	// readResult drains notifications until the response for id arrives.
	dec := json.NewDecoder(stdout)
	readResult := func(id int) map[string]any {
		for {
			var msg map[string]any
			require.NoError(t, dec.Decode(&msg))
			if got, ok := msg["id"].(float64); ok && int(got) == id {
				require.NotContains(t, msg, "error", "opencode rejected the daemon's config: %v", msg["error"])
				result, ok := msg["result"].(map[string]any)
				require.True(t, ok)
				return result
			}
		}
	}

	send(1, "initialize", map[string]any{
		"protocolVersion":    1,
		"clientCapabilities": map[string]any{"fs": map[string]any{"readTextFile": true, "writeTextFile": true}},
	})
	require.EqualValues(t, 1, readResult(1)["protocolVersion"])

	send(2, "session/new", map[string]any{"cwd": workdir, "mcpServers": []any{}})
	session := readResult(2)

	options, ok := session["configOptions"].([]any)
	require.True(t, ok, "session/new must report config options")

	var models []string
	for _, raw := range options {
		option, ok := raw.(map[string]any)
		if !ok || option["id"] != "model" {
			continue
		}
		for _, rawModel := range option["options"].([]any) {
			models = append(models, rawModel.(map[string]any)["value"].(string))
		}
	}

	// The whole point of enabled_providers: opencode's own free models and any
	// provider it could infer from ambient env vars must not be offered, or a
	// user could route around the Helix proxy.
	require.Equal(t, []string{openCodeProviderID + "/" + model}, models)
}
