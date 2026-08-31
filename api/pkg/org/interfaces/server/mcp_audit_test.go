package server

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/org/domain/asset"
	orgaudit "github.com/helixml/helix/api/pkg/org/domain/audit"
	orgmemory "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
)

func TestNewMCPAuditEntryResolvesAssetAndRedactsSecrets(t *testing.T) {
	st := orgmemory.New()
	value, err := asset.New(
		"asset-1",
		"org-1",
		"production",
		"",
		"",
		asset.KindServer,
		asset.Config{Server: &asset.Server{
			Address:             "10.0.0.8",
			Port:                22,
			User:                "ubuntu",
			AuthType:            asset.AuthSSHKey,
			PublicKey:           "ssh-ed25519 AAAA",
			EncryptedPrivateKey: "encrypted",
		}},
		time.Now().UTC(),
	)
	require.NoError(t, err)
	require.NoError(t, st.Assets.Create(context.Background(), value))

	server := &Server{assets: st.Assets}
	entry := server.newMCPAuditEntry(
		context.Background(),
		botCaller{id: "bot-1", orgID: "org-1"},
		"update_server_asset",
		json.RawMessage(`{"asset":"production","project_id":"project-1","password":"secret-value"}`),
	)

	require.Equal(t, "asset-1", entry.AssetID)
	require.Equal(t, "production", entry.Metadata.AssetRef)
	require.Equal(t, "project-1", entry.ProjectID)
	require.JSONEq(t, `{"asset":"production","project_id":"project-1","password":"[REDACTED]"}`, string(entry.Metadata.Arguments))
}

type wrappedDelegatedCaller struct{}

func (wrappedDelegatedCaller) ID() string             { return "b-owner" }
func (wrappedDelegatedCaller) OrganizationID() string { return "org-1" }
func (wrappedDelegatedCaller) AuditActorType() orgaudit.ActorType {
	return orgaudit.ActorSpecTask
}
func (wrappedDelegatedCaller) AuditActorID() string { return "spt-1" }

func TestNewMCPAuditEntryAuditsDelegatedCallerUnderTaskID(t *testing.T) {
	server := &Server{}
	entry := server.newMCPAuditEntry(
		context.Background(),
		wrappedDelegatedCaller{},
		"get_secret",
		json.RawMessage(`{}`),
	)
	// The acting identity is the bound Agent; the audit identity stays
	// the task that pulled the trigger.
	require.Equal(t, "spt-1", entry.ActorID)
	require.Equal(t, orgaudit.ActorSpecTask, entry.ActorType)
}
