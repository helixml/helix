package mcptools_test

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	assetapp "github.com/helixml/helix/api/pkg/org/application/assets"
	orgmemory "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
	"github.com/helixml/helix/api/pkg/org/interfaces/mcptools"
	"github.com/helixml/helix/api/pkg/org/interfaces/server"
)

func TestChiefOfStaffCreatesAssetAndReceivesOperationalToolsOverMCP(t *testing.T) {
	t.Parallel()
	st := orgmemory.New()
	seedActingBot(t, st, "org-test", "chief-of-staff", mcptools.OwnerBotTools())

	cfg := mcptools.DefaultDeps(st)
	injectTestPublishing(&cfg)
	assets, err := assetapp.New(assetapp.Deps{
		Assets: st.Assets, Links: st.AssetLinks, Nodes: st.Nodes,
		GenerateKey: func() (string, string, error) {
			return "private", "ssh-ed25519 AAAA-public\n", nil
		},
		Encrypt: func(value []byte) (string, error) { return "encrypted", nil },
		Now:     func() time.Time { return time.Now().UTC() },
		NewID:   func() string { return "mcp-server" },
	})
	require.NoError(t, err)
	cfg.Assets = assets

	reg := mcptools.NewRegistry()
	require.NoError(t, mcptools.RegisterBuiltins(reg, cfg.Build()))
	srv := httptest.NewServer(server.NewFromStore(st, reg, nil, nil, nil).Handler())
	t.Cleanup(srv.Close)
	session := connectMCP(t, srv.URL, "chief-of-staff")

	before, err := session.ListTools(context.Background(), nil)
	require.NoError(t, err)
	beforeNames := listedToolNames(before.Tools)
	for _, name := range mcptools.AssetManagementTools {
		assert.True(t, beforeNames[string(name)], "Chief of Staff missing %q", name)
	}
	assert.False(t, beforeNames[string(mcptools.ServerRunCommandName)])

	_, err = invokeTool(t, session, mcptools.CreateServerAssetName, map[string]any{
		"name": "production", "address": "10.0.0.8", "user": "ubuntu",
	})
	require.NoError(t, err)

	after, err := session.ListTools(context.Background(), nil)
	require.NoError(t, err)
	afterNames := listedToolNames(after.Tools)
	for _, name := range assetapp.ServerTools {
		assert.True(t, afterNames[string(name)], "linked Chief of Staff missing %q", name)
	}
}

func listedToolNames(tools []*mcp.Tool) map[string]bool {
	names := make(map[string]bool, len(tools))
	for _, value := range tools {
		names[value.Name] = true
	}
	return names
}
