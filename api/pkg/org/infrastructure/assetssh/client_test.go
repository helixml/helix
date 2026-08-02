package assetssh

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	helixcrypto "github.com/helixml/helix/api/pkg/crypto"
	"github.com/helixml/helix/api/pkg/org/domain/asset"
	orgaudit "github.com/helixml/helix/api/pkg/org/domain/audit"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

type healthAssets struct{ value asset.Asset }

func (a healthAssets) Resolve(context.Context, string, string) (asset.Asset, error) {
	return a.value, nil
}

func (a healthAssets) AuthorizeRef(context.Context, string, string, string) (asset.Asset, error) {
	return a.value, nil
}

func (healthAssets) PinHostKey(context.Context, string, asset.ID, string) error { return nil }

func TestBuildRemoteCommandQuotesEveryValue(t *testing.T) {
	got, err := buildRemoteCommand(RunRequest{
		Cmd: "/usr/bin/printf", Args: []string{"%s", "a'b"}, Cwd: "/srv/app dir",
		Env: map[string]string{"B": "two words", "A": "one"}, Sudo: true,
	})
	require.NoError(t, err)
	require.Equal(t,
		`cd -- '/srv/app dir' && sudo -- env A='one' B='two words' '/usr/bin/printf' '%s' 'a'"'"'b'`,
		got,
	)
}

func TestBuildRemoteCommandRejectsInvalidEnvironmentName(t *testing.T) {
	_, err := buildRemoteCommand(RunRequest{Cmd: "echo", Env: map[string]string{"BAD-NAME": "value"}})
	require.EqualError(t, err, `invalid environment variable name "BAD-NAME"`)
}

func TestBuildRemoteCommandRejectsRelativeWorkingDirectory(t *testing.T) {
	_, err := buildRemoteCommand(RunRequest{Cmd: "echo", Cwd: "tmp"})
	require.EqualError(t, err, "invalid server command cwd: path must be absolute and contain no NUL")
}

func TestHealthDoesNotConnectToDisabledAsset(t *testing.T) {
	assets := healthAssets{value: asset.Asset{
		ID: "a-disabled", OrganizationID: "org-1", Name: "disabled-server", Disabled: true,
		Kind: asset.KindServer, Config: asset.Config{Server: &asset.Server{
			Address: "192.0.2.1", Port: 22, User: "root", AuthType: asset.AuthPassword,
			EncryptedPassword: "secret",
		}},
	}}
	client, err := New(assets, func(string) ([]byte, error) {
		t.Fatal("disabled asset credentials must not be decrypted")
		return nil, nil
	}, func() string { return "command" })
	require.NoError(t, err)

	health := client.Health(context.Background(), "org-1", "disabled-server")
	require.Equal(t, `asset "disabled-server" is disabled`, health.Error)
	require.False(t, health.TCPReachable)
	require.False(t, health.SSHReachable)
}

func TestRunAuditsSSHConnectionAndCommand(t *testing.T) {
	privateKey, publicKey, err := helixcrypto.GenerateSSHKeyPair("ed25519")
	require.NoError(t, err)
	authorizedKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(publicKey))
	require.NoError(t, err)
	upstreamAddress, commands, stopUpstream := startProxyTestUpstream(t, authorizedKey)
	t.Cleanup(stopUpstream)
	host, rawPort, err := net.SplitHostPort(upstreamAddress)
	require.NoError(t, err)
	port, err := strconv.Atoi(rawPort)
	require.NoError(t, err)

	assets := &proxyAssets{allowed: true, value: asset.Asset{
		ID: "a-prod", OrganizationID: "org-1", Name: "production", Kind: asset.KindServer,
		Config: asset.Config{Server: &asset.Server{
			Address: host, Port: uint16(port), User: "ubuntu", AuthType: asset.AuthSSHKey,
			PublicKey: publicKey, EncryptedPrivateKey: privateKey,
		}},
	}}
	auditLog := newRecordingAudit()
	client, err := New(assets, func(ciphertext string) ([]byte, error) { return []byte(ciphertext), nil }, func() string { return "command-1" })
	require.NoError(t, err)
	client.WithAudit(auditLog, func(context.Context, string, string) (string, error) { return "project-1", nil })

	command, err := client.Run(context.Background(), "org-1", "agent-1", "production", RunRequest{Cmd: "printf", Args: []string{"client-ok"}})
	require.NoError(t, err)
	require.Equal(t, CommandFinished, command.Status)
	select {
	case remote := <-commands:
		require.Equal(t, `'printf' 'client-ok'`, remote)
	case <-time.After(5 * time.Second):
		t.Fatal("upstream did not receive direct command")
	}

	audited := make(map[orgaudit.EventType]orgaudit.Entry)
	for len(audited) < 2 {
		select {
		case entry := <-auditLog.entries:
			audited[entry.EventType] = entry
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for direct SSH audit entries: %+v", audited)
		}
	}
	require.Equal(t, "project-1", audited[orgaudit.EventSSHConnection].ProjectID)
	require.Equal(t, orgaudit.StatusSucceeded, audited[orgaudit.EventSSHConnection].Status)
	require.Equal(t, "command-1", audited[orgaudit.EventSSHCommand].Metadata.CommandID)
	require.Equal(t, `'printf' 'client-ok'`, audited[orgaudit.EventSSHCommand].Metadata.Command)
}
