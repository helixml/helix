package services

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRedactURLCredentials(t *testing.T) {
	const key = "hl-secretkey123"
	got := RedactURLCredentials("http://api:" + key + "@api:8080/git/repo_1")
	require.Equal(t, "http://api:8080/git/repo_1", got)
	require.NotContains(t, got, key)

	require.Equal(t, "https://github.com/o/r.git", RedactURLCredentials("https://user@github.com/o/r.git"))
	require.Equal(t, "https://github.com/o/r.git", RedactURLCredentials("https://github.com/o/r.git"))
	require.Equal(t, "", RedactURLCredentials(""))

	// Unparseable input must not be echoed back.
	bad := "http://api:" + key + "@host:bad port/%zz"
	require.NotContains(t, RedactURLCredentials(bad), key)
}

func TestRedactURLSecretInError(t *testing.T) {
	const key = "hl-secretkey123"
	clone := "http://api:" + key + "@api:8080/git/repo_1"

	err := redactURLSecretInError(errors.New("clone "+clone+" failed; token "+key), clone)
	require.NotContains(t, err.Error(), key)
	require.Contains(t, err.Error(), "http://api:8080/git/repo_1")

	plain := errors.New("connection refused")
	require.Same(t, plain, redactURLSecretInError(plain, clone))
	require.NoError(t, redactURLSecretInError(nil, clone))
}
