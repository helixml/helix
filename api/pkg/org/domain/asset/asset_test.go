package asset

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func validServerConfig() Config {
	return Config{Server: &Server{
		Address: "server.example.com", Port: 22, User: "ubuntu",
		AuthType:  AuthSSHKey,
		PublicKey: "ssh-ed25519 public", EncryptedPrivateKey: "encrypted",
	}}
}

func TestNewServer(t *testing.T) {
	now := time.Now().UTC()
	a, err := New("a-1", "org-1", "production-api", "Primary API", "deploy carefully", KindServer, validServerConfig(), now)
	require.NoError(t, err)
	require.Equal(t, "production-api", a.Name)
	require.Equal(t, uint16(22), a.Config.Server.Port)
}

func TestNewRejectsUnsafeProxyName(t *testing.T) {
	_, err := New("a-1", "org-1", "Production API", "", "", KindServer, validServerConfig(), time.Now().UTC())
	require.EqualError(t, err, "asset name must be 1-63 lowercase letters, numbers, dots, underscores, or hyphens")
}

func TestNewRejectsAddressWithEmbeddedPort(t *testing.T) {
	cfg := validServerConfig()
	cfg.Server.Address = "server.example.com:2222"
	_, err := New("a-1", "org-1", "server", "", "", KindServer, cfg, time.Now().UTC())
	require.EqualError(t, err, "server address contains an invalid IP address")
}
