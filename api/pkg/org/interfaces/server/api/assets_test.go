package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	assetapp "github.com/helixml/helix/api/pkg/org/application/assets"
	"github.com/helixml/helix/api/pkg/org/domain/asset"
	orgapi "github.com/helixml/helix/api/pkg/org/interfaces/server/api"
	"github.com/stretchr/testify/require"
)

func TestAssetsAPIKeyAuthCreateUpdateAndSecretRedaction(t *testing.T) {
	deps, _, _ := newDeps(t)
	h := orgapi.Handler(deps)

	created := do(t, h, http.MethodPost, "/assets", orgapi.CreateAssetRequest{
		Name: "production", Description: "Primary API", NotesForAgents: "Do not restart during deploys.",
		Kind: asset.KindServer,
		Server: &orgapi.ServerAssetWriteRequest{
			Address: "10.0.0.8", Port: 22, User: "ubuntu", AuthType: asset.AuthSSHKey,
		},
	})
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	require.NotContains(t, created.Body.String(), "private-key")
	require.NotContains(t, created.Body.String(), "encrypted")
	var dto orgapi.AssetDTO
	decode(t, created, &dto)
	require.Equal(t, "a-test-id", dto.ID)
	require.Equal(t, "ssh-ed25519 public-key", dto.Server.PublicKey)
	require.Equal(t, "Do not restart during deploys.", dto.NotesForAgents)
	require.True(t, dto.Enabled)

	address := "prod.internal"
	port := uint16(2222)
	notes := "Run migrations before deploy."
	updated := do(t, h, http.MethodPatch, "/assets/"+dto.ID, orgapi.UpdateAssetRequest{
		NotesForAgents: &notes,
		Server:         &orgapi.UpdateServerAssetRequest{Address: &address, Port: &port},
	})
	require.Equal(t, http.StatusOK, updated.Code, updated.Body.String())
	decode(t, updated, &dto)
	require.Equal(t, "prod.internal", dto.Server.Address)
	require.Equal(t, uint16(2222), dto.Server.Port)
	require.Equal(t, notes, dto.NotesForAgents)

	enabled := false
	disabled := do(t, h, http.MethodPatch, "/assets/"+dto.ID, orgapi.UpdateAssetRequest{Enabled: &enabled})
	require.Equal(t, http.StatusOK, disabled.Code, disabled.Body.String())
	decode(t, disabled, &dto)
	require.False(t, dto.Enabled)

	listed := do(t, h, http.MethodGet, "/assets", nil)
	require.Equal(t, http.StatusOK, listed.Code, listed.Body.String())
	var response orgapi.AssetsResponse
	decode(t, listed, &response)
	require.Len(t, response.Assets, 1)
	require.Equal(t, dto.ID, response.Assets[0].ID)
}

func TestAssetsAPIPasswordAuthNeverReturnsPassword(t *testing.T) {
	deps, _, _ := newDeps(t)
	h := orgapi.Handler(deps)
	const password = "correct horse battery staple"
	created := do(t, h, http.MethodPost, "/assets", orgapi.CreateAssetRequest{
		Name: "legacy", Kind: asset.KindServer,
		Server: &orgapi.ServerAssetWriteRequest{
			Address: "legacy.internal", User: "root", AuthType: asset.AuthPassword, Password: password,
		},
	})
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	require.NotContains(t, created.Body.String(), password)
	require.NotContains(t, created.Body.String(), "encrypted:")
	var dto orgapi.AssetDTO
	decode(t, created, &dto)
	require.True(t, dto.Server.PasswordConfigured)
	require.Empty(t, dto.Server.PublicKey)

	persisted, err := deps.Assets.Get(context.Background(), "org-test", dto.ID)
	require.NoError(t, err)
	require.Equal(t, "encrypted:"+password, persisted.Config.Server.EncryptedPassword)
}

func TestAssetsAPILinkDerivesAgentToolsAndDeleteRevokesThem(t *testing.T) {
	deps, st, _ := newDeps(t)
	h := orgapi.Handler(deps)
	seedBot(t, st, context.Background(), "b-operator", "operator")

	created := do(t, h, http.MethodPost, "/assets", orgapi.CreateAssetRequest{
		Name: "production", Kind: asset.KindServer,
		Server: &orgapi.ServerAssetWriteRequest{Address: "10.0.0.8", User: "ubuntu", AuthType: asset.AuthSSHKey},
	})
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	var dto orgapi.AssetDTO
	decode(t, created, &dto)

	linked := do(t, h, http.MethodPost, "/assets/"+dto.ID+"/links", orgapi.AssetLinkRequest{AgentID: "b-operator"})
	require.Equal(t, http.StatusCreated, linked.Code, linked.Body.String())
	agent, err := st.Nodes.Get(context.Background(), "org-test", "b-operator")
	require.NoError(t, err)
	for _, name := range assetapp.ServerTools {
		require.Contains(t, agent.Tools, name)
	}

	got := do(t, h, http.MethodGet, "/assets/"+dto.ID, nil)
	require.Equal(t, http.StatusOK, got.Code, got.Body.String())
	decode(t, got, &dto)
	require.Equal(t, []string{"b-operator"}, dto.AgentIDs)

	deleted := do(t, h, http.MethodDelete, "/assets/"+dto.ID, nil)
	require.Equal(t, http.StatusNoContent, deleted.Code, deleted.Body.String())
	agent, err = st.Nodes.Get(context.Background(), "org-test", "b-operator")
	require.NoError(t, err)
	for _, name := range assetapp.ServerTools {
		require.NotContains(t, agent.Tools, name)
	}
}

func TestAssetsAPIRejectsUnknownFields(t *testing.T) {
	deps, _, _ := newDeps(t)
	h := orgapi.Handler(deps)
	rec := do(t, h, http.MethodPost, "/assets", json.RawMessage(`{
		"name":"production","kind":"server","server":{"address":"10.0.0.8","user":"ubuntu","auth_type":"ssh_key"},"private_key":"leak"
	}`))
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	require.True(t, strings.Contains(rec.Body.String(), "unknown field"), rec.Body.String())
}
