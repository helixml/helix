package mcptools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	assetapp "github.com/helixml/helix/api/pkg/org/application/assets"
	"github.com/helixml/helix/api/pkg/org/application/nodes"
	"github.com/helixml/helix/api/pkg/org/domain/asset"
	orgstore "github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/infrastructure/assetssh"
	orgmemory "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
)

func managedAssetTestDeps(t *testing.T) (Deps, *assetapp.Service, *orgstore.Store) {
	t.Helper()
	st := orgmemory.New()
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	nodeSvc := nodes.New(nodes.Deps{Nodes: st.Nodes})
	_, err := nodeSvc.Create(context.Background(), "org-1", nodes.CreateParams{
		ID: "chief-of-staff", Content: "# Chief of Staff", Tools: OwnerBotTools(),
	})
	require.NoError(t, err)
	_, err = nodeSvc.Create(context.Background(), "org-1", nodes.CreateParams{
		ID: "b-operator", Content: "# Operator",
	})
	require.NoError(t, err)

	service, err := assetapp.New(assetapp.Deps{
		Assets: st.Assets, Links: st.AssetLinks, Nodes: st.Nodes,
		GenerateKey: func() (string, string, error) {
			return "PRIVATE KEY MATERIAL", "ssh-ed25519 AAAA-helix-public\n", nil
		},
		Encrypt: func(value []byte) (string, error) {
			return "encrypted:" + string(value), nil
		},
		Now: func() time.Time { return now }, NewID: func() string { return "server-1" },
	})
	require.NoError(t, err)
	deps := Deps{
		Assets: service,
		AssetHealth: func(_ context.Context, _, _ string) assetssh.Health {
			return assetssh.Health{TCPReachable: true, SSHReachable: true, LatencyMS: 4, CheckedAt: now}
		},
	}
	return deps, service, st
}

func invokeManagedAssetTool(t *testing.T, target tool.Tool, caller assetCallerIdentity, args string) json.RawMessage {
	t.Helper()
	raw, err := target.Invoke(context.Background(), tool.Invocation{Caller: caller, Args: json.RawMessage(args)})
	require.NoError(t, err)
	return raw
}

func TestChiefOfStaffManagesServerAssetLifecycle(t *testing.T) {
	t.Parallel()
	deps, service, st := managedAssetTestDeps(t)
	ctx := context.Background()
	caller := assetCallerIdentity{agentID: "chief-of-staff", orgID: "org-1"}

	raw := invokeManagedAssetTool(t, &CreateServerAsset{deps: deps}, caller,
		`{"name":"production","description":"Primary API","notes_for_agents":"Check the runbook","address":"10.0.0.8","user":"ubuntu"}`)
	var created managedAssetResult
	require.NoError(t, json.Unmarshal(raw, &created))
	assert.Equal(t, "production", created.Asset.Name)
	assert.Equal(t, []string{"chief-of-staff"}, created.Asset.AgentIDs)
	require.NotNil(t, created.Setup)
	assert.Equal(t, "ssh-ed25519 AAAA-helix-public", created.Setup.PublicKey)
	assert.Contains(t, created.Setup.InstallCommand, "authorized_keys")
	assert.Equal(t, "get_asset_health", created.Setup.VerificationToolName)
	assert.NotContains(t, string(raw), "PRIVATE KEY MATERIAL")
	assert.NotContains(t, string(raw), "encrypted:")

	assetID := created.Asset.ID
	chief, err := st.Nodes.Get(ctx, "org-1", "chief-of-staff")
	require.NoError(t, err)
	for _, name := range assetapp.ServerTools {
		assert.Contains(t, chief.Tools, name)
	}

	invokeManagedAssetTool(t, &LinkAsset{deps: deps}, caller,
		`{"asset":"production","agent_id":"b-operator"}`)
	raw = invokeManagedAssetTool(t, &ListAssetLinks{deps: deps}, caller,
		`{"asset":"production"}`)
	var links assetLinksResult
	require.NoError(t, json.Unmarshal(raw, &links))
	assert.ElementsMatch(t, []string{"chief-of-staff", "b-operator"}, links.AgentIDs)

	raw = invokeManagedAssetTool(t, &UpdateServerAsset{deps: deps}, caller,
		`{"asset":"production","description":"Primary production API","address":"10.0.0.9"}`)
	var updated managedAssetResult
	require.NoError(t, json.Unmarshal(raw, &updated))
	assert.Equal(t, "Primary production API", updated.Asset.Description)
	require.NotNil(t, updated.Asset.Server)
	assert.Equal(t, "10.0.0.9", updated.Asset.Server.Address)
	assert.True(t, updated.Asset.Enabled)

	raw = invokeManagedAssetTool(t, &UpdateServerAsset{deps: deps}, caller,
		`{"asset":"production","enabled":false}`)
	require.NoError(t, json.Unmarshal(raw, &updated))
	assert.False(t, updated.Asset.Enabled)
	linkedAssets, err := service.ListForAgent(ctx, "org-1", "chief-of-staff")
	require.NoError(t, err)
	require.Len(t, linkedAssets, 1)
	_, err = service.AuthorizeRef(ctx, "org-1", "chief-of-staff", "production")
	require.ErrorContains(t, err, "disabled")

	// Exercise the immediately following normal operation: re-enable the asset
	// and verify that the existing link becomes usable again.
	raw = invokeManagedAssetTool(t, &UpdateServerAsset{deps: deps}, caller,
		`{"asset":"production","enabled":true}`)
	require.NoError(t, json.Unmarshal(raw, &updated))
	_, err = service.AuthorizeRef(ctx, "org-1", "chief-of-staff", "production")
	require.NoError(t, err)

	raw = invokeManagedAssetTool(t, &GetAssetHealth{deps: deps}, caller,
		`{"asset":"production"}`)
	var health assetssh.Health
	require.NoError(t, json.Unmarshal(raw, &health))
	assert.True(t, health.TCPReachable)
	assert.True(t, health.SSHReachable)

	raw = invokeManagedAssetTool(t, &ListOrgAssets{deps: deps}, caller, `{}`)
	var listed orgAssetsResult
	require.NoError(t, json.Unmarshal(raw, &listed))
	require.Len(t, listed.Assets, 1)
	assert.Equal(t, assetID, listed.Assets[0].ID)

	invokeManagedAssetTool(t, &UnlinkAsset{deps: deps}, caller,
		`{"asset":"production","agent_id":"b-operator"}`)
	invokeManagedAssetTool(t, &DeleteAsset{deps: deps}, caller,
		`{"asset":"production"}`)
	_, err = service.Get(ctx, "org-1", assetID)
	require.Error(t, err)

	// Exercise the immediately following normal operation: the same name can
	// be recreated after deletion and receives a fresh working link.
	raw = invokeManagedAssetTool(t, &CreateServerAsset{deps: deps}, caller,
		`{"name":"production","address":"10.0.0.10","user":"ubuntu"}`)
	require.NoError(t, json.Unmarshal(raw, &created))
	assert.Equal(t, "production", created.Asset.Name)
	assert.Equal(t, []string{"chief-of-staff"}, created.Asset.AgentIDs)
}

func TestCreateServerAssetRollsBackWhenLinkFails(t *testing.T) {
	t.Parallel()
	deps, service, _ := managedAssetTestDeps(t)
	caller := assetCallerIdentity{agentID: "chief-of-staff", orgID: "org-1"}
	_, err := (&CreateServerAsset{deps: deps}).Invoke(context.Background(), tool.Invocation{
		Caller: caller,
		Args:   json.RawMessage(`{"name":"broken","address":"10.0.0.20","user":"ubuntu","agent_ids":["missing-agent"]}`),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing-agent")
	values, listErr := service.List(context.Background(), "org-1")
	require.NoError(t, listErr)
	assert.Empty(t, values)
}

func TestListOrgAssetsIncludesUnlinkedInventory(t *testing.T) {
	t.Parallel()
	deps, service, _ := managedAssetTestDeps(t)
	_, err := service.CreateServer(context.Background(), "org-1", assetapp.CreateServerParams{
		Name: "unlinked", Address: "10.0.0.30", User: "root", AuthType: asset.AuthSSHKey,
	})
	require.NoError(t, err)
	caller := assetCallerIdentity{agentID: "chief-of-staff", orgID: "org-1"}
	raw := invokeManagedAssetTool(t, &ListOrgAssets{deps: deps}, caller, `{}`)
	assert.Contains(t, string(raw), `"name":"unlinked"`)

	linkedRaw := invokeManagedAssetTool(t, &ListAssets{deps: deps}, caller, `{}`)
	assert.False(t, strings.Contains(string(linkedRaw), `"name":"unlinked"`))
}
