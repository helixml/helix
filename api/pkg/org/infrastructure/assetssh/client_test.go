package assetssh

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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
