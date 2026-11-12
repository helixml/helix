package services

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRedactURLCredentials(t *testing.T) {
	key := "k" + strings.Repeat("x", 31)
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
	key := "k" + strings.Repeat("x", 31)
	clone := "http://api:" + key + "@api:8080/git/repo_1"

	cause := errors.New("clone " + clone + " failed; token " + key)
	err := redactURLSecretInError(cause, clone)
	require.NotContains(t, err.Error(), key)
	require.Contains(t, err.Error(), "http://api:8080/git/repo_1")
	require.ErrorIs(t, err, cause, "original error stays in the chain")

	plain := errors.New("connection refused")
	require.Same(t, plain, redactURLSecretInError(plain, clone))
	require.NoError(t, redactURLSecretInError(nil, clone))
}

// A token passed as the username (no password) is a credential too.
func TestRedactURLSecretInError_UsernameToken(t *testing.T) {
	token := "t" + strings.Repeat("y", 31)
	remote := "https://" + token + "@github.com/o/r.git"

	err := redactURLSecretInError(errors.New("fetch "+remote+": auth failed for "+token), remote)
	require.NotContains(t, err.Error(), token)
	require.Contains(t, err.Error(), "https://github.com/o/r.git")

	// Conventional non-secret usernames are left alone.
	sshLike := "https://git@github.com/o/r.git"
	err = redactURLSecretInError(errors.New("fetch "+sshLike+": git error"), sshLike)
	require.Equal(t, "fetch https://github.com/o/r.git: git error", err.Error())
}

func TestRedactURLParseError(t *testing.T) {
	key := "k" + strings.Repeat("x", 31)
	_, err := BuildAuthenticatedURL("http://api:"+key+"@host:bad port/", "u", "p")
	require.Error(t, err)
	require.NotContains(t, err.Error(), key)

	_, err = BuildAuthenticatedURL("//api:"+key+"@/repo", "u", "p")
	if err != nil {
		require.NotContains(t, err.Error(), key)
	}
}
