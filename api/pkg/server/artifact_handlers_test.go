package server

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
)

func TestReadArtifactZIP(t *testing.T) {
	archive := artifactTestZIP(t, map[string]string{
		"index.html":    "<h1>Hello</h1>",
		"assets/app.js": "document.body.dataset.ready = 'yes'",
	})
	files, err := readArtifactZIP(archive)
	require.NoError(t, err)
	require.Len(t, files, 2)
	require.Equal(t, "assets/app.js", files[0].Metadata.Path)
	require.Equal(t, "text/html; charset=utf-8", files[1].Metadata.ContentType)
}

func TestReadArtifactZIPRejectsTraversal(t *testing.T) {
	archive := artifactTestZIP(t, map[string]string{"../index.html": "nope"})
	_, err := readArtifactZIP(archive)
	require.ErrorContains(t, err, "invalid artifact path")
}

func TestResolveArtifactFile(t *testing.T) {
	artifact := &types.Artifact{
		ID: "art_test", Kind: types.ArtifactKindSPA, Entrypoint: "index.html",
		ActiveVersion: &types.ArtifactVersion{ID: "artv_test", Files: []types.ArtifactFile{
			{Path: "index.html", SHA256: "one"},
			{Path: "assets/app.js", SHA256: "two"},
		}},
	}

	filename, _, ok := resolveArtifactFile(artifact, "")
	require.True(t, ok)
	require.Equal(t, "index.html", filename)
	filename, _, ok = resolveArtifactFile(artifact, "assets/app.js")
	require.True(t, ok)
	require.Equal(t, "assets/app.js", filename)
	filename, _, ok = resolveArtifactFile(artifact, "settings/profile")
	require.True(t, ok)
	require.Equal(t, "index.html", filename)
	filename, _, ok = resolveArtifactFile(artifact, "settings/")
	require.True(t, ok)
	require.Equal(t, "index.html", filename)
	_, _, ok = resolveArtifactFile(artifact, "../secret")
	require.False(t, ok)
}

func TestArtifactSecurityHeaders(t *testing.T) {
	canonical := http.Header{}
	setArtifactSecurityHeaders(canonical, true)
	require.Contains(t, canonical.Get("Content-Security-Policy"), "sandbox allow-scripts")
	require.Contains(t, canonical.Get("Content-Security-Policy"), "connect-src 'none'")
	require.NotContains(t, canonical.Get("Content-Security-Policy"), "allow-same-origin")

	subdomain := http.Header{}
	setArtifactSecurityHeaders(subdomain, false)
	require.NotContains(t, subdomain.Get("Content-Security-Policy"), "sandbox")
	require.Contains(t, subdomain.Get("Content-Security-Policy"), "connect-src 'self' https: wss:")
}

func TestArtifactRequestOrigin(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "http://api:8080/artifacts/art_123", nil)
	require.Equal(t, "https://helix.example", artifactRequestOrigin(r, "https://helix.example/"))
	require.Equal(t, "http://api:8080", artifactRequestOrigin(r, ""))

	r.Header.Set("X-Forwarded-Proto", "https")
	require.Equal(t, "https://api:8080", artifactRequestOrigin(r, ""))
}

func TestDefaultAndValidateArtifactEntrypoint(t *testing.T) {
	files := []types.ArtifactFile{{Path: "replacement.html"}}

	entrypoint, err := defaultAndValidateArtifactEntrypoint(files, "index.html", true)
	require.NoError(t, err)
	require.Equal(t, "replacement.html", entrypoint)

	_, err = defaultAndValidateArtifactEntrypoint(files, "index.html", false)
	require.ErrorContains(t, err, "entrypoint does not exist")
}

func TestArtifactAccessRedirectPath(t *testing.T) {
	redirectPath, err := artifactAccessRedirectPath("settings/", "tab=billing")
	require.NoError(t, err)
	require.Equal(t, "/settings/?tab=billing", redirectPath)
	redirectPath, err = artifactAccessRedirectPath("", "preview=true")
	require.NoError(t, err)
	require.Equal(t, "/?preview=true", redirectPath)
	_, err = artifactAccessRedirectPath("../secrets", "")
	require.Error(t, err)
}

func TestArtifactAccessToken(t *testing.T) {
	server := &HelixAPIServer{Cfg: &config.ServerConfig{}}
	server.Cfg.Auth.Regular.JWTSecret = "artifact-test-secret"
	raw, err := server.signArtifactAccessToken("usr_test", "art_test", "/settings", artifactBootstrapAud, time.Minute)
	require.NoError(t, err)
	claims, err := server.parseArtifactAccessToken(raw, "art_test", artifactBootstrapAud)
	require.NoError(t, err)
	require.Equal(t, "usr_test", claims.Subject)
	require.Equal(t, "/settings", claims.ArtifactPath)
	_, err = server.parseArtifactAccessToken(raw, "art_other", artifactBootstrapAud)
	require.Error(t, err)
	_, err = server.parseArtifactAccessToken(raw, "art_test", artifactSessionAud)
	require.Error(t, err)
}

func artifactTestZIP(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for filename, body := range files {
		part, err := writer.Create(filename)
		require.NoError(t, err)
		_, err = part.Write([]byte(body))
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	return buffer.Bytes()
}
