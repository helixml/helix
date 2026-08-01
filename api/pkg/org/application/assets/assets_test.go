package assets

import (
	"context"
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
	require.NoError(t, st.Nodes.Create(context.Background(), bot))
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
}

func TestLinkDerivesAndRevokesServerTools(t *testing.T) {
	svc, _, orgID := newTestService(t)
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

	require.NoError(t, svc.Unlink(context.Background(), orgID, a.ID, "b-agent"))
	bot, err = svc.nodes.Get(context.Background(), orgID, "b-agent")
	require.NoError(t, err)
	for _, name := range ServerTools {
		require.NotContains(t, bot.Tools, name)
	}
}
