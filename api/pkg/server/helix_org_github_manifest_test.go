package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGitHubManifestState_RoundTrip(t *testing.T) {
	in := githubManifestState{OrgID: "org-1", GitHubOrg: "acme", ExpiresAt: time.Now().Add(time.Hour).Unix()}
	enc, err := encodeGitHubManifestState(in, testEncKey)
	require.NoError(t, err)
	require.NotEmpty(t, enc)

	out, err := decodeGitHubManifestState(enc, testEncKey)
	require.NoError(t, err)
	require.Equal(t, in.OrgID, out.OrgID)
	require.Equal(t, in.GitHubOrg, out.GitHubOrg)
}

func TestGitHubManifestState_Expired(t *testing.T) {
	in := githubManifestState{OrgID: "org-1", GitHubOrg: "acme", ExpiresAt: time.Now().Add(-time.Minute).Unix()}
	enc, err := encodeGitHubManifestState(in, testEncKey)
	require.NoError(t, err)
	_, err = decodeGitHubManifestState(enc, testEncKey)
	require.Error(t, err, "expired state must be rejected")
}

func TestGitHubManifestState_WrongKeyRejected(t *testing.T) {
	in := githubManifestState{OrgID: "org-1", GitHubOrg: "acme", ExpiresAt: time.Now().Add(time.Hour).Unix()}
	enc, err := encodeGitHubManifestState(in, testEncKey)
	require.NoError(t, err)
	otherKey := []byte("ffffffffffffffffffffffffffffffff")
	_, err = decodeGitHubManifestState(enc, otherKey)
	require.Error(t, err, "state encrypted with a different key must not decode (CSRF)")
}

func TestNormalizeOrigin(t *testing.T) {
	ok, err := normalizeOrigin("http://localhost:8080/orgs/test/streams")
	require.NoError(t, err)
	require.Equal(t, "http://localhost:8080", ok, "must strip path, keep scheme+host")

	ok, err = normalizeOrigin("https://helix.example.com")
	require.NoError(t, err)
	require.Equal(t, "https://helix.example.com", ok)

	for _, bad := range []string{"", "ftp://x", "notaurl", "https://"} {
		_, err := normalizeOrigin(bad)
		require.Error(t, err, "origin %q must be rejected", bad)
	}
}

func TestGitHubManifestStart_BuildsManifestAndPostURL(t *testing.T) {
	start := newGitHubManifestStart(testKeyGetter)
	resp, err := start(context.Background(), "org-1", "acme", "http://localhost:8080")
	require.NoError(t, err)

	// POST target is the org-owned app-creation URL with our state.
	require.True(t, strings.HasPrefix(resp.PostURL, "https://github.com/organizations/acme/settings/apps/new?state="),
		"post_url = %s", resp.PostURL)
	require.NotEmpty(t, resp.State)

	var m githubManifest
	require.NoError(t, json.Unmarshal([]byte(resp.Manifest), &m))
	require.Equal(t, "Helix acme", m.Name)
	require.False(t, m.Public)
	// Least-privilege bot permissions.
	require.Equal(t, "write", m.DefaultPermissions["contents"])
	require.Equal(t, "write", m.DefaultPermissions["pull_requests"])
	require.Equal(t, "write", m.DefaultPermissions["issues"])
	require.Equal(t, "read", m.DefaultPermissions["metadata"])
	// Callback + setup URLs carry the helix org and the caller's origin.
	require.Equal(t, "http://localhost:8080/api/v1/orgs/org-1/github/app-manifest/callback", m.RedirectURL)
	require.Equal(t, "http://localhost:8080/api/v1/orgs/org-1/github/app-setup", m.SetupURL)

	// The state must decode back to the same org.
	decoded, err := decodeGitHubManifestState(resp.State, testEncKey)
	require.NoError(t, err)
	require.Equal(t, "org-1", decoded.OrgID)
	require.Equal(t, "acme", decoded.GitHubOrg)
}

func TestGitHubManifestStart_RejectsBadInput(t *testing.T) {
	start := newGitHubManifestStart(testKeyGetter)
	_, err := start(context.Background(), "org-1", "", "http://localhost:8080")
	require.Error(t, err, "empty github org rejected")
	_, err = start(context.Background(), "org-1", "acme", "not-a-url")
	require.Error(t, err, "bad origin rejected")
}
