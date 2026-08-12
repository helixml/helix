package org

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/org/domain/asset"
	orgapi "github.com/helixml/helix/api/pkg/org/interfaces/server/api"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
)

func newAssetCLITestServer(t *testing.T, assetHandler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/organizations/org_test" {
			writeCLIJSON(t, w, types.Organization{ID: "org_test", Name: "test"})
			return
		}
		assetHandler(w, r)
	}))
}

func writeCLIJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	require.NoError(t, json.NewEncoder(w).Encode(value))
}

func TestAssetsCLICreateServerSSHKey(t *testing.T) {
	server := newAssetCLITestServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/api/v1/orgs/org_test/assets", r.URL.Path)
		var request orgapi.CreateAssetRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		require.Equal(t, asset.KindServer, request.Kind)
		require.Equal(t, "production", request.Name)
		require.Equal(t, "10.0.0.8", request.Server.Address)
		require.Equal(t, "Deploy only after draining traffic.", request.NotesForAgents)
		require.Equal(t, asset.AuthSSHKey, request.Server.AuthType)
		require.Empty(t, request.Server.Password)
		w.WriteHeader(http.StatusCreated)
		writeCLIJSON(t, w, orgapi.AssetDTO{
			ID: "a-server", Name: request.Name, Kind: request.Kind,
			Server: &orgapi.ServerAssetDTO{Address: request.Server.Address, Port: 22, User: request.Server.User, AuthType: asset.AuthSSHKey, PublicKey: "ssh-ed25519 public-key"},
		})
	})
	defer server.Close()
	t.Setenv("HELIX_URL", server.URL)
	t.Setenv("HELIX_API_KEY", "test-key")

	cmd := newAssetsCmd()
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"create", "server", "production", "--org", "org_test", "--address", "10.0.0.8", "--user", "ubuntu", "--notes-for-agents", "Deploy only after draining traffic."})
	require.NoError(t, cmd.Execute())
	require.Contains(t, output.String(), "Created asset production (a-server)")
	require.Contains(t, output.String(), "ssh-ed25519 public-key")
}

func TestAssetsCLIPasswordComesOnlyFromStdin(t *testing.T) {
	const password = "correct horse battery staple"
	server := newAssetCLITestServer(t, func(w http.ResponseWriter, r *http.Request) {
		var request orgapi.CreateAssetRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		require.Equal(t, asset.AuthPassword, request.Server.AuthType)
		require.Equal(t, password, request.Server.Password)
		w.WriteHeader(http.StatusCreated)
		writeCLIJSON(t, w, orgapi.AssetDTO{
			ID: "a-password", Name: request.Name, Kind: request.Kind,
			Server: &orgapi.ServerAssetDTO{Address: request.Server.Address, Port: 22, User: request.Server.User, AuthType: asset.AuthPassword, PasswordConfigured: true},
		})
	})
	defer server.Close()
	t.Setenv("HELIX_URL", server.URL)
	t.Setenv("HELIX_API_KEY", "test-key")

	cmd := newAssetsCmd()
	var output bytes.Buffer
	cmd.SetIn(strings.NewReader(password + "\n"))
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"create", "server", "legacy", "--org", "org_test", "--address", "legacy.internal", "--user", "root", "--auth", "password", "--password-stdin"})
	require.NoError(t, cmd.Execute())
	require.NotContains(t, output.String(), password)
}

func TestAssetsCLILinkAndUnlinkUseAgentPaths(t *testing.T) {
	var requests []string
	server := newAssetCLITestServer(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		if r.Method == http.MethodPost {
			var request orgapi.AssetLinkRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
			require.Equal(t, "agent-one", request.AgentID)
			w.WriteHeader(http.StatusCreated)
			writeCLIJSON(t, w, asset.Link{OrganizationID: "org_test", AssetID: "a-server", AgentID: request.AgentID})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	defer server.Close()
	t.Setenv("HELIX_URL", server.URL)
	t.Setenv("HELIX_API_KEY", "test-key")

	for _, args := range [][]string{
		{"link", "a-server", "agent-one", "--org", "org_test"},
		{"unlink", "a-server", "agent-one", "--org", "org_test"},
	} {
		cmd := newAssetsCmd()
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetArgs(args)
		require.NoError(t, cmd.Execute())
	}
	require.Equal(t, []string{
		"POST /api/v1/orgs/org_test/assets/a-server/links",
		"DELETE /api/v1/orgs/org_test/assets/a-server/links/agent-one",
	}, requests)
}

func TestReadAssetPasswordValidation(t *testing.T) {
	_, err := readAssetPassword(strings.NewReader("secret\n"), asset.AuthPassword, false)
	require.EqualError(t, err, "password authentication requires --password-stdin")
	_, err = readAssetPassword(strings.NewReader("\n"), asset.AuthPassword, true)
	require.EqualError(t, err, "password from stdin is empty")
	_, err = readAssetPassword(strings.NewReader("secret\n"), asset.AuthSSHKey, true)
	require.EqualError(t, err, "--password-stdin requires --auth password")
}
