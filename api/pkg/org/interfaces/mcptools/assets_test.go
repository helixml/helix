package mcptools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	assetapp "github.com/helixml/helix/api/pkg/org/application/assets"
	"github.com/helixml/helix/api/pkg/org/domain/asset"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	orgmemory "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
)

type assetCallerIdentity struct{ agentID, orgID string }

func (c assetCallerIdentity) ID() string             { return c.agentID }
func (c assetCallerIdentity) OrganizationID() string { return c.orgID }

func TestAssetDiscoveryOnlyReturnsLinkedAssetsAndAgentNotes(t *testing.T) {
	ctx := context.Background()
	st := orgmemory.New()
	now := time.Now().UTC()
	a, err := asset.New("a-prod", "org-1", "production", "Primary API", "Deploy only after checking the runbook.", asset.KindServer, asset.Config{
		Server: &asset.Server{Address: "10.0.0.8", Port: 22, User: "ubuntu", AuthType: asset.AuthSSHKey, PublicKey: "ssh-ed25519 public", EncryptedPrivateKey: "encrypted"},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Assets.Create(ctx, a); err != nil {
		t.Fatal(err)
	}
	link, err := asset.NewLink("org-1", a.ID, "agent-1", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AssetLinks.Create(ctx, link); err != nil {
		t.Fatal(err)
	}
	svc, err := assetapp.New(assetapp.Deps{
		Assets: st.Assets, Links: st.AssetLinks, Nodes: st.Nodes,
		GenerateKey: func() (string, string, error) { return "private", "public", nil },
		Encrypt:     func(value []byte) (string, error) { return string(value), nil },
		NewID:       func() string { return "new" }, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	deps := Deps{Assets: svc}
	caller := assetCallerIdentity{agentID: "agent-1", orgID: "org-1"}

	raw, err := (&ListAssets{deps: deps}).Invoke(ctx, tool.Invocation{Caller: caller, Args: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"notes_for_agents":"Deploy only after checking the runbook."`) {
		t.Fatalf("agent notes missing from list_assets: %s", raw)
	}
	var listed struct {
		Assets []assetView `json:"assets"`
	}
	if err := json.Unmarshal(raw, &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Assets) != 1 || listed.Assets[0].Server == nil {
		t.Fatalf("server view missing from list_assets: %s", raw)
	}
	server := listed.Assets[0].Server
	if len(server.Capabilities) != 4 || server.Capabilities[3] != "ssh_via_helix_proxy" {
		t.Fatalf("server capabilities = %v", server.Capabilities)
	}
	if !server.SSHAccess.Available || server.SSHAccess.Tool != "server_ssh_access" || server.SSHAccess.Target != "production@<helix-ssh-proxy>" {
		t.Fatalf("SSH proxy guidance = %+v", server.SSHAccess)
	}
	if strings.Contains(string(raw), "encrypted") {
		t.Fatalf("encrypted credential leaked through list_assets: %s", raw)
	}
	if !listed.Assets[0].Enabled {
		t.Fatalf("new asset unexpectedly disabled: %s", raw)
	}

	raw, err = (&GetAsset{deps: deps}).Invoke(ctx, tool.Invocation{Caller: caller, Args: json.RawMessage(`{"asset":"production"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"name":"production"`) {
		t.Fatalf("get_asset did not resolve by name: %s", raw)
	}

	enabled := false
	if _, err := svc.UpdateServer(ctx, "org-1", a.ID, assetapp.UpdateServerParams{Enabled: &enabled}); err != nil {
		t.Fatal(err)
	}
	raw, err = (&ListAssets{deps: deps}).Invoke(ctx, tool.Invocation{Caller: caller, Args: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &listed); err != nil {
		t.Fatal(err)
	}
	if listed.Assets[0].Enabled || listed.Assets[0].Server.SSHAccess.Available || len(listed.Assets[0].Server.Capabilities) != 0 {
		t.Fatalf("disabled asset advertised as usable: %s", raw)
	}
	_, err = (&GetAsset{deps: deps}).Invoke(ctx, tool.Invocation{Caller: caller, Args: json.RawMessage(`{"asset":"production"}`)})
	if err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("disabled asset remained callable: %v", err)
	}

	_, err = (&GetAsset{deps: deps}).Invoke(ctx, tool.Invocation{
		Caller: assetCallerIdentity{agentID: "agent-2", orgID: "org-1"},
		Args:   json.RawMessage(`{"asset":"production"}`),
	})
	if err == nil || !strings.Contains(err.Error(), "not linked") {
		t.Fatalf("unlinked agent error = %v", err)
	}
}
