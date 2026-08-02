package assetssh

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/org/domain/asset"
	"github.com/stretchr/testify/require"
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
