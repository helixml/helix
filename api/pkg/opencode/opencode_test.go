package opencode

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateVersion(t *testing.T) {
	tests := []struct {
		name    string
		version string
		wantErr string
	}{
		{name: "blank means use the bundled build", version: ""},
		{name: "newer patch", version: "1.18.19"},
		{name: "newer minor", version: "1.19.0"},
		{name: "newer major", version: "2.0.0"},
		{name: "equal to bundled", version: BakedVersion, wantErr: "not newer"},
		{name: "older than bundled", version: "1.18.17", wantErr: "not newer"},
		// The version is interpolated into an outbound release URL, so anything
		// that is not a bare semver is rejected rather than escaped.
		{name: "leading v", version: "v1.19.0", wantErr: "bare semver"},
		{name: "path traversal", version: "../../etc/passwd", wantErr: "bare semver"},
		{name: "url injection", version: "1.19.0/../../evil", wantErr: "bare semver"},
		{name: "prerelease suffix", version: "1.19.0-beta", wantErr: "bare semver"},
		{name: "two components", version: "1.19", wantErr: "bare semver"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateVersion(tt.version)
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// releaseJSON renders a GitHub-shaped release payload.
func releaseJSON(version string, assets map[string]string) string {
	body := fmt.Sprintf(`{"tag_name":"v%s","assets":[`, version)
	first := true
	for name, digest := range assets {
		if !first {
			body += ","
		}
		first = false
		body += fmt.Sprintf(`{"name":%q,"browser_download_url":"https://example.test/%s","digest":%q}`,
			name, name, digest)
	}
	return body + "]}"
}

func TestResolveReturnsBothArchitectures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/tags/v1.19.0", r.URL.Path)
		_, _ = w.Write([]byte(releaseJSON("1.19.0", map[string]string{
			"opencode-linux-x64.tar.gz":   "sha256:aaa",
			"opencode-linux-arm64.tar.gz": "sha256:bbb",
			"opencode-darwin-arm64.zip":   "sha256:ccc",
		})))
	}))
	defer server.Close()

	binary, err := NewResolver(nil, server.URL).Resolve(context.Background(), "1.19.0")
	require.NoError(t, err)
	require.NotNil(t, binary)

	assert.Equal(t, "1.19.0", binary.Version)
	assert.Len(t, binary.Artifacts, 2, "the API cannot know the sandbox host's arch, so both linux builds must ship")
	assert.Equal(t, "aaa", binary.Artifacts["amd64"].SHA256, "the sha256: prefix must be stripped")
	assert.Equal(t, "bbb", binary.Artifacts["arm64"].SHA256)
	assert.Equal(t, "https://example.test/opencode-linux-x64.tar.gz", binary.Artifacts["amd64"].URL)
}

// A release that only builds one architecture would start sessions on one
// sandbox host and break them on another, so it is rejected outright.
func TestResolveRejectsPartialRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(releaseJSON("1.19.0", map[string]string{
			"opencode-linux-x64.tar.gz": "sha256:aaa",
		})))
	}))
	defer server.Close()

	_, err := NewResolver(nil, server.URL).Resolve(context.Background(), "1.19.0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "arm64")
}

// Without a digest the daemon has no way to verify what it downloaded, so we
// refuse to hand it an artifact at all.
func TestResolveRejectsAssetWithoutDigest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(releaseJSON("1.19.0", map[string]string{
			"opencode-linux-x64.tar.gz":   "",
			"opencode-linux-arm64.tar.gz": "sha256:bbb",
		})))
	}))
	defer server.Close()

	_, err := NewResolver(nil, server.URL).Resolve(context.Background(), "1.19.0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unverifiable")
}

func TestResolveReportsMissingVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	_, err := NewResolver(nil, server.URL).Resolve(context.Background(), "9.9.9")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist upstream")
}

// Releases are immutable once published, so repeated session starts must not
// re-hit the release index.
func TestResolveCachesRelease(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(releaseJSON("1.19.0", map[string]string{
			"opencode-linux-x64.tar.gz":   "sha256:aaa",
			"opencode-linux-arm64.tar.gz": "sha256:bbb",
		})))
	}))
	defer server.Close()

	resolver := NewResolver(nil, server.URL)
	for i := 0; i < 3; i++ {
		_, err := resolver.Resolve(context.Background(), "1.19.0")
		require.NoError(t, err)
	}
	assert.Equal(t, int32(1), calls.Load())
}

func TestResolveBlankVersionIsNoop(t *testing.T) {
	resolver := NewResolver(nil, "http://127.0.0.1:1/unreachable")
	binary, err := resolver.Resolve(context.Background(), "")
	require.NoError(t, err)
	assert.Nil(t, binary, "a blank override means the container uses its bundled build with no network access")
}
