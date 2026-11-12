package server

import (
	"context"
	"net/http"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
)

func apiToolAssistant(url string) *types.AssistantConfig {
	return &types.AssistantConfig{Tools: []*types.Tool{{
		Name: "t", ToolType: types.ToolTypeAPI, Config: types.ToolConfig{API: &types.ToolAPIConfig{URL: url}},
	}}}
}

// Saving an agent refuses an API tool URL the outbound policy blocks, but
// only when the URL is new: an agent saved before the check existed stays
// editable as long as its tool URL is unchanged.
func TestCheckNewAPIToolDestinations(t *testing.T) {
	srv := &HelixAPIServer{Cfg: &config.ServerConfig{}}
	ctx := context.Background()

	httpErr := srv.checkNewAPIToolDestinations(ctx, apiToolAssistant("http://127.0.0.1:8080"), nil)
	require.NotNil(t, httpErr)
	require.Equal(t, http.StatusBadRequest, httpErr.StatusCode)

	existing := &types.App{Config: types.AppConfig{Helix: types.AppHelixConfig{Assistants: []types.AssistantConfig{{
		APIs: []types.AssistantAPI{{Name: "t", URL: "http://127.0.0.1:8080"}},
	}}}}}
	require.Nil(t, srv.checkNewAPIToolDestinations(ctx, apiToolAssistant("http://127.0.0.1:8080"), existing),
		"an unchanged URL is not re-checked")

	httpErr = srv.checkNewAPIToolDestinations(ctx, apiToolAssistant("http://169.254.169.254"), existing)
	require.NotNil(t, httpErr, "a changed URL is checked")

	require.Nil(t, srv.checkNewAPIToolDestinations(ctx, apiToolAssistant("/api"), nil), "relative server URLs stay allowed")
}
