package tools

import (
	"net/http"
	"testing"

	"github.com/helixml/helix/api/pkg/agent/skill/mcp"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
)

func apiTool(url string) *types.Tool {
	return &types.Tool{
		ToolType: types.ToolTypeAPI,
		Config:   types.ToolConfig{API: &types.ToolAPIConfig{Schema: alphaVantageAPI, URL: url}},
	}
}

// Saving an API tool whose URL the outbound policy would block at call time
// is refused with a 400 when the agent is saved.
func TestValidateTool_RejectsReservedDestinations(t *testing.T) {
	planner := &ChainStrategy{cfg: &config.ServerConfig{}}
	for _, url := range []string{
		"http://127.0.0.1:8080",
		"http://localhost/api",
		"http://10.0.0.5",
		"http://192.168.1.10:9000",
		"http://169.254.169.254/latest/meta-data",
		"http://[::1]:8080",
		"http://[fd00:ec2::254]",
		"file:///etc/passwd",
	} {
		err := ValidateTool("u", &types.AssistantConfig{AgentType: types.AgentTypeHelixBasic}, apiTool(url), nil, planner, &mcp.DefaultClientGetter{}, false)
		require.Error(t, err, url)
		httpErr, ok := err.(*system.HTTPError)
		require.True(t, ok, url)
		require.Equal(t, http.StatusBadRequest, httpErr.StatusCode, url)
	}
}

// Self-hosted deployments that opt in to private networks keep working;
// public hosts are accepted either way.
func TestValidateTool_AllowsPermittedDestinations(t *testing.T) {
	private := &ChainStrategy{cfg: &config.ServerConfig{}}
	private.cfg.Tools.AllowPrivateNetworks = true
	require.NoError(t, ValidateTool("u", &types.AssistantConfig{AgentType: types.AgentTypeHelixBasic}, apiTool("http://10.0.0.5"), nil, private, &mcp.DefaultClientGetter{}, false))

	public := &ChainStrategy{cfg: &config.ServerConfig{}}
	require.NoError(t, ValidateTool("u", &types.AssistantConfig{AgentType: types.AgentTypeHelixBasic}, apiTool("https://93.184.215.14"), nil, public, &mcp.DefaultClientGetter{}, false))
	// Link-local / metadata stays blocked even with private networks allowed.
	require.Error(t, ValidateTool("u", &types.AssistantConfig{AgentType: types.AgentTypeHelixBasic}, apiTool("http://169.254.169.254"), nil, private, &mcp.DefaultClientGetter{}, false))
}
