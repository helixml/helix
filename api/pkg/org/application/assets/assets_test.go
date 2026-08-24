package assets

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
	"github.com/stretchr/testify/require"
)

func newTestService(t *testing.T) (*Service, *time.Time, string) {
	t.Helper()
	st := memory.New()
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	svc, err := New(Deps{
		Assets: st.Assets, Links: st.AssetLinks, Nodes: st.Nodes,
		GenerateKey: func() (string, string, error) { return "private", "ssh-ed25519 public", nil },
		Encrypt: func(plaintext []byte) (string, error) {
			require.Equal(t, []byte("private"), plaintext)
			return "encrypted-private", nil
		},
		Now: func() time.Time { return now }, NewID: func() string { return "generated" },
	})
	require.NoError(t, err)
	bot, err := orgchart.NewNode("b-agent", "agent", nil, now, "org-test")
	require.NoError(t, err)
	require.NoError(t, st.Nodes.Create(context.Background(), bot.WithAgentID("app-agent")))
	return svc, &now, "org-test"
}

func TestCreateServerEncryptsPrivateKey(t *testing.T) {
	svc, _, orgID := newTestService(t)
	a, err := svc.CreateServer(context.Background(), orgID, CreateServerParams{
		Name: "production", Address: "10.0.0.8", User: "ubuntu",
	})
	require.NoError(t, err)
	require.Equal(t, "a-generated", a.ID)
	require.Equal(t, uint16(22), a.Config.Server.Port)
	require.Equal(t, "ssh-ed25519 public", a.Config.Server.PublicKey)
	require.Equal(t, "encrypted-private", a.Config.Server.EncryptedPrivateKey)
	require.False(t, a.Disabled)
}

func TestLinkDerivesAndRevokesServerTools(t *testing.T) {
	svc, _, orgID := newTestService(t)
	var notified []string
	svc.onToolsChanged = func(_ context.Context, appID string) { notified = append(notified, appID) }
	a, err := svc.CreateServer(context.Background(), orgID, CreateServerParams{
		Name: "production", Address: "10.0.0.8", User: "ubuntu",
	})
	require.NoError(t, err)
	_, err = svc.Link(context.Background(), orgID, a.ID, "b-agent")
	require.NoError(t, err)
	bot, err := svc.nodes.Get(context.Background(), orgID, "b-agent")
	require.NoError(t, err)
	for _, name := range ServerTools {
		require.Contains(t, bot.Tools, name)
	}
	require.Equal(t, []string{"app-agent"}, notified)

	require.NoError(t, svc.Unlink(context.Background(), orgID, a.ID, "b-agent"))
	bot, err = svc.nodes.Get(context.Background(), orgID, "b-agent")
	require.NoError(t, err)
	for _, name := range ServerTools {
		require.NotContains(t, bot.Tools, name)
	}
	require.Equal(t, []string{"app-agent", "app-agent"}, notified)
}

func TestLinkFiresOnRestartRequiredWhenGrantingNewTools(t *testing.T) {
	svc, _, orgID := newTestService(t)
	var restarted []string
	svc.onRestartRequired = func(_ context.Context, gotOrgID string, id orgchart.NodeID) {
		require.Equal(t, orgID, gotOrgID)
		restarted = append(restarted, string(id))
	}
	a, err := svc.CreateServer(context.Background(), orgID, CreateServerParams{
		Name: "production", Address: "10.0.0.8", User: "ubuntu",
	})
	require.NoError(t, err)

	_, err = svc.Link(context.Background(), orgID, a.ID, "b-agent")
	require.NoError(t, err)
	require.Equal(t, []string{"b-agent"}, restarted, "granting new server tools must fire OnRestartRequired")

	require.NoError(t, svc.Unlink(context.Background(), orgID, a.ID, "b-agent"))
	require.Equal(t, []string{"b-agent", "b-agent"}, restarted, "revoking the tools must also fire OnRestartRequired")
}

func TestLinkDoesNotFireOnRestartRequiredWhenToolsAlreadyGranted(t *testing.T) {
	svc, now, orgID := newTestService(t)
	// b-agent2 already carries every tool a server-kind asset would grant,
	// so linking it to one more server asset changes nothing a running
	// sandbox reads at startup.
	preloaded, err := orgchart.NewNode("b-agent2", "agent2", ServerTools, *now, orgID)
	require.NoError(t, err)
	require.NoError(t, svc.nodes.Create(context.Background(), preloaded.WithAgentID("app-agent2")))

	var restarted []string
	svc.onRestartRequired = func(_ context.Context, _ string, id orgchart.NodeID) { restarted = append(restarted, string(id)) }

	a, err := svc.CreateServer(context.Background(), orgID, CreateServerParams{
		Name: "production", Address: "10.0.0.8", User: "ubuntu",
	})
	require.NoError(t, err)

	_, err = svc.Link(context.Background(), orgID, a.ID, "b-agent2")
	require.NoError(t, err)
	require.Empty(t, restarted, "linking an asset that grants no new tools must not fire OnRestartRequired")
}

func TestAuthorizeRequiresLinkAndEnabledAsset(t *testing.T) {
	svc, _, orgID := newTestService(t)
	ctx := context.Background()
	a, err := svc.CreateServer(ctx, orgID, CreateServerParams{
		Name: "production", Address: "10.0.0.8", User: "ubuntu",
	})
	require.NoError(t, err)

	_, err = svc.Authorize(ctx, orgID, "b-agent", a.ID)
	require.ErrorContains(t, err, "not linked")

	_, err = svc.Link(ctx, orgID, a.ID, "b-agent")
	require.NoError(t, err)
	enabled := false
	a, err = svc.UpdateServer(ctx, orgID, a.ID, UpdateServerParams{Enabled: &enabled})
	require.NoError(t, err)
	require.True(t, a.Disabled)

	linked, err := svc.ListForAgent(ctx, orgID, "b-agent")
	require.NoError(t, err)
	require.Len(t, linked, 1)
	_, err = svc.AuthorizeRef(ctx, orgID, "b-agent", "production")
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "asset \"production\" is disabled"), err.Error())

	enabled = true
	_, err = svc.UpdateServer(ctx, orgID, a.ID, UpdateServerParams{Enabled: &enabled})
	require.NoError(t, err)
	_, err = svc.AuthorizeRef(ctx, orgID, "b-agent", "production")
	require.NoError(t, err)
}
