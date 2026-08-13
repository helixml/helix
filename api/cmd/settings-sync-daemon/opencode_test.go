package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openCodeDaemon() *SettingsDaemon {
	return &SettingsDaemon{
		codeAgentConfig: &CodeAgentConfig{
			Runtime:         "opencode",
			BaseURL:         "http://outer-api:8080/v1",
			Model:           "anthropic/claude-opus-4-8",
			APIType:         "openai",
			MaxTokens:       200000,
			MaxOutputTokens: 32000,
		},
		userAPIKey: "hl-test-key",
	}
}

// decodeOpenCodeConfig pulls the config back out of the generated agent_servers
// entry, which is how the agent actually receives it.
func decodeOpenCodeConfig(t *testing.T, cfg map[string]interface{}) map[string]interface{} {
	t.Helper()
	entry, ok := cfg["opencode"].(map[string]interface{})
	require.True(t, ok, "agent_servers must contain an opencode entry for the opencode runtime")
	env, ok := entry["env"].(map[string]interface{})
	require.True(t, ok, "opencode entry must carry an env map")
	raw, ok := env["OPENCODE_CONFIG_CONTENT"].(string)
	require.True(t, ok, "opencode env must carry OPENCODE_CONFIG_CONTENT")

	var decoded map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(raw), &decoded))
	return decoded
}

func TestOpenCodeAgentServerUsesBakedBinaryByDefault(t *testing.T) {
	d := openCodeDaemon()

	cfg := d.generateAgentServerConfig()
	entry, ok := cfg["opencode"].(map[string]interface{})
	require.True(t, ok)

	assert.Equal(t, OpenCodeBakedBinary, entry["command"],
		"with no admin override the session must run the binary baked into the image")
	assert.Equal(t, []string{"acp"}, entry["args"])
	assert.Equal(t, "custom", entry["type"],
		"Zed deserializes agent_servers with a tagged enum; the entry must declare type=custom")
}

// The container also exports OPENAI_API_KEY / ANTHROPIC_API_KEY for other
// runtimes. Without enabled_providers, opencode auto-registers those as direct
// providers (plus its own free "Zen" models) and the user can pick a model that
// bypasses the Helix proxy entirely — no routing, no billing.
func TestOpenCodeConfigRestrictsProvidersToHelix(t *testing.T) {
	d := openCodeDaemon()

	decoded := decodeOpenCodeConfig(t, d.generateAgentServerConfig())

	enabled, ok := decoded["enabled_providers"].([]interface{})
	require.True(t, ok, "config must gate providers so ambient API keys cannot be used directly")
	assert.Equal(t, []interface{}{"helix"}, enabled)

	providers, ok := decoded["provider"].(map[string]interface{})
	require.True(t, ok)
	assert.Len(t, providers, 1, "only the Helix-proxied provider may be declared")

	helix, ok := providers["helix"].(map[string]interface{})
	require.True(t, ok)
	options, ok := helix["options"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "http://outer-api:8080/v1", options["baseURL"])
	assert.Equal(t, "{env:HELIX_API_KEY}", options["apiKey"],
		"the key must be referenced from env, not inlined into Zed's settings.json")
}

// A headless sandbox has nobody to click "Allow". Without a blanket allow the
// agent stalls on the first edit waiting for a session/request_permission
// nobody will answer — the same failure --yolo prevents for qwen.
func TestOpenCodeConfigAutoApprovesToolCalls(t *testing.T) {
	d := openCodeDaemon()

	decoded := decodeOpenCodeConfig(t, d.generateAgentServerConfig())

	permissions, ok := decoded["permission"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "allow", permissions["*"])
	assert.Equal(t, "allow", permissions["external_directory"])
	assert.Equal(t, "deny", permissions["doom_loop"],
		"headless mode must reject OpenCode's third identical tool call instead of approving an infinite loop")
	assert.Equal(t, false, decoded["autoupdate"],
		"opencode must never swap its own binary mid-session; the version is pinned by the image or the admin override")
}

func TestOpenCodeConfigBoundsDeepSeekV4FlashTurns(t *testing.T) {
	d := openCodeDaemon()
	d.codeAgentConfig.Model = "ds4-flash-node06/deepseek-v4-flash"

	decoded := decodeOpenCodeConfig(t, d.generateAgentServerConfig())
	agents, ok := decoded["agent"].(map[string]interface{})
	require.True(t, ok)
	build, ok := agents["build"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, float64(openCodeDeepSeekV4FlashSteps), build["steps"])
}

func TestOpenCodeConfigDoesNotCapOtherModels(t *testing.T) {
	decoded := decodeOpenCodeConfig(t, openCodeDaemon().generateAgentServerConfig())
	assert.NotContains(t, decoded, "agent")
}

func TestOpenCodeConfigModelIsProviderQualified(t *testing.T) {
	d := openCodeDaemon()

	decoded := decodeOpenCodeConfig(t, d.generateAgentServerConfig())

	assert.Equal(t, "helix/anthropic/claude-opus-4-8", decoded["model"])
	assert.Equal(t, "helix/anthropic/claude-opus-4-8", decoded["small_model"])

	providers := decoded["provider"].(map[string]interface{})
	models := providers["helix"].(map[string]interface{})["models"].(map[string]interface{})
	model, ok := models["anthropic/claude-opus-4-8"].(map[string]interface{})
	require.True(t, ok, "the model id declared to opencode must be the provider-prefixed id the Helix proxy routes on")

	limit, ok := model["limit"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, float64(200000), limit["context"])
	assert.Equal(t, float64(32000), limit["output"])
}

// Zero limits would tell opencode the context window is empty, so it would
// compact after every turn. Unknown limits must be omitted instead.
func TestOpenCodeConfigOmitsUnknownLimits(t *testing.T) {
	d := openCodeDaemon()
	d.codeAgentConfig.MaxTokens = 0
	d.codeAgentConfig.MaxOutputTokens = 0

	decoded := decodeOpenCodeConfig(t, d.generateAgentServerConfig())

	providers := decoded["provider"].(map[string]interface{})
	models := providers["helix"].(map[string]interface{})["models"].(map[string]interface{})
	model := models["anthropic/claude-opus-4-8"].(map[string]interface{})
	assert.NotContains(t, model, "limit")
}

func TestOpenCodeConfigPassesReasoningEffort(t *testing.T) {
	d := openCodeDaemon()
	d.codeAgentConfig.ReasoningEffort = "high"

	decoded := decodeOpenCodeConfig(t, d.generateAgentServerConfig())

	providers := decoded["provider"].(map[string]interface{})
	models := providers["helix"].(map[string]interface{})["models"].(map[string]interface{})
	model := models["anthropic/claude-opus-4-8"].(map[string]interface{})
	options, ok := model["options"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "high", options["reasoningEffort"])
}

// buildOpenCodeArchive builds a tar.gz containing a single "opencode" file,
// matching the layout of an upstream release, and returns it with its digest.
func buildOpenCodeArchive(t *testing.T, contents string) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name:     "opencode",
		Mode:     0755,
		Size:     int64(len(contents)),
		Typeflag: tar.TypeReg,
	}))
	_, err := tw.Write([]byte(contents))
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())

	sum := sha256.Sum256(buf.Bytes())
	return buf.Bytes(), hex.EncodeToString(sum[:])
}

func TestExtractOpenCodeBinaryRejectsUnexpectedRegularFile(t *testing.T) {
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	contents := []byte("release notes")
	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name:     "README.md",
		Mode:     0644,
		Size:     int64(len(contents)),
		Typeflag: tar.TypeReg,
	}))
	_, err := tw.Write(contents)
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())

	var dest bytes.Buffer
	err = extractOpenCodeBinary(bytes.NewReader(archive.Bytes()), &dest)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected regular file")
}

func TestOpenCodeInstallsPinnedVersion(t *testing.T) {
	archive, digest := buildOpenCodeArchive(t, "#!/bin/sh\necho pinned\n")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer server.Close()

	cacheDir := t.TempDir()
	originalCache, originalVersionFile := OpenCodeCacheDir, OpenCodeBakedVersionFile
	OpenCodeCacheDir = cacheDir
	OpenCodeBakedVersionFile = filepath.Join(t.TempDir(), "opencode.version")
	require.NoError(t, os.WriteFile(OpenCodeBakedVersionFile, []byte("1.18.18\n"), 0644))
	defer func() {
		OpenCodeCacheDir, OpenCodeBakedVersionFile = originalCache, originalVersionFile
	}()

	d := openCodeDaemon()
	d.codeAgentConfig.OpenCodeBinary = &CodeAgentBinary{
		Version: "1.19.0",
		Artifacts: map[string]CodeAgentBinaryArtifact{
			runtime.GOARCH: {URL: server.URL, SHA256: digest},
		},
	}

	command, err := d.resolveOpenCodeCommand()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(cacheDir, "1.19.0", "opencode"), command)

	info, err := os.Stat(command)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&0111, "installed binary must be executable")
	body, err := os.ReadFile(command)
	require.NoError(t, err)
	assert.Equal(t, "#!/bin/sh\necho pinned\n", string(body))
}

// A tampered or truncated download must never be executed, and must not leave
// a partial file behind for the next session to pick up.
func TestOpenCodeRejectsDigestMismatch(t *testing.T) {
	archive, _ := buildOpenCodeArchive(t, "malicious")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer server.Close()

	cacheDir := t.TempDir()
	originalCache, originalVersionFile := OpenCodeCacheDir, OpenCodeBakedVersionFile
	OpenCodeCacheDir = cacheDir
	OpenCodeBakedVersionFile = filepath.Join(t.TempDir(), "opencode.version")
	require.NoError(t, os.WriteFile(OpenCodeBakedVersionFile, []byte("1.18.18"), 0644))
	defer func() {
		OpenCodeCacheDir, OpenCodeBakedVersionFile = originalCache, originalVersionFile
	}()

	d := openCodeDaemon()
	d.codeAgentConfig.OpenCodeBinary = &CodeAgentBinary{
		Version: "1.19.0",
		Artifacts: map[string]CodeAgentBinaryArtifact{
			runtime.GOARCH: {URL: server.URL, SHA256: "00000000000000000000000000000000000000000000000000000000deadbeef"},
		},
	}

	_, err := d.resolveOpenCodeCommand()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "digest mismatch")

	_, statErr := os.Stat(filepath.Join(cacheDir, "1.19.0", "opencode"))
	assert.True(t, os.IsNotExist(statErr), "a failed install must not leave a binary behind")
}

// The critical safety property: when a pinned release cannot be installed the
// daemon emits no agent_servers entry at all. Falling back to the bundled
// binary would leave an admin believing their rollout had landed.
func TestOpenCodeFailedInstallEmitsNoAgentServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	originalCache, originalVersionFile := OpenCodeCacheDir, OpenCodeBakedVersionFile
	OpenCodeCacheDir = t.TempDir()
	OpenCodeBakedVersionFile = filepath.Join(t.TempDir(), "opencode.version")
	require.NoError(t, os.WriteFile(OpenCodeBakedVersionFile, []byte("1.18.18"), 0644))
	defer func() {
		OpenCodeCacheDir, OpenCodeBakedVersionFile = originalCache, originalVersionFile
	}()

	d := openCodeDaemon()
	d.codeAgentConfig.OpenCodeBinary = &CodeAgentBinary{
		Version: "1.19.0",
		Artifacts: map[string]CodeAgentBinaryArtifact{
			runtime.GOARCH: {URL: server.URL, SHA256: "abc"},
		},
	}

	assert.Nil(t, d.generateAgentServerConfig(),
		"a failed install must block the agent rather than silently downgrade to the bundled build")
}

// generateAgentServerConfig runs on every settings merge, including ones a
// file-watcher event triggers. A broken pin must not turn each of those into
// another download attempt.
func TestOpenCodeThrottlesRetriesAfterFailure(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	originalCache, originalVersionFile := OpenCodeCacheDir, OpenCodeBakedVersionFile
	OpenCodeCacheDir = t.TempDir()
	OpenCodeBakedVersionFile = filepath.Join(t.TempDir(), "opencode.version")
	require.NoError(t, os.WriteFile(OpenCodeBakedVersionFile, []byte("1.18.18"), 0644))
	defer func() {
		OpenCodeCacheDir, OpenCodeBakedVersionFile = originalCache, originalVersionFile
	}()

	d := openCodeDaemon()
	d.codeAgentConfig.OpenCodeBinary = &CodeAgentBinary{
		Version: "1.19.0",
		Artifacts: map[string]CodeAgentBinaryArtifact{
			runtime.GOARCH: {URL: server.URL, SHA256: "abc"},
		},
	}

	for i := 0; i < 5; i++ {
		assert.Nil(t, d.generateAgentServerConfig())
	}
	assert.Equal(t, int32(1), attempts.Load(),
		"repeated merges within the retry interval must not re-download")

	// Once the interval lapses the daemon tries again, so a transient outage
	// still self-heals without an operator restarting anything.
	d.openCodeLastAttempt = time.Now().Add(-2 * openCodeRetryInterval)
	assert.Nil(t, d.generateAgentServerConfig())
	assert.Equal(t, int32(2), attempts.Load())
}

// A pin equal to what the image already ships must not trigger a download.
func TestOpenCodePinMatchingBakedVersionSkipsDownload(t *testing.T) {
	originalCache, originalVersionFile := OpenCodeCacheDir, OpenCodeBakedVersionFile
	OpenCodeCacheDir = t.TempDir()
	OpenCodeBakedVersionFile = filepath.Join(t.TempDir(), "opencode.version")
	require.NoError(t, os.WriteFile(OpenCodeBakedVersionFile, []byte("1.19.0\n"), 0644))
	defer func() {
		OpenCodeCacheDir, OpenCodeBakedVersionFile = originalCache, originalVersionFile
	}()

	d := openCodeDaemon()
	d.codeAgentConfig.OpenCodeBinary = &CodeAgentBinary{
		Version: "1.19.0",
		Artifacts: map[string]CodeAgentBinaryArtifact{
			runtime.GOARCH: {URL: "http://127.0.0.1:1/unreachable", SHA256: "abc"},
		},
	}

	command, err := d.resolveOpenCodeCommand()
	require.NoError(t, err)
	assert.Equal(t, OpenCodeBakedBinary, command)
}

// A cached copy from an earlier session (or another container sharing the host
// mount) must be reused rather than re-downloaded.
func TestOpenCodeReusesCachedBinary(t *testing.T) {
	cacheDir := t.TempDir()
	originalCache, originalVersionFile := OpenCodeCacheDir, OpenCodeBakedVersionFile
	OpenCodeCacheDir = cacheDir
	OpenCodeBakedVersionFile = filepath.Join(t.TempDir(), "opencode.version")
	require.NoError(t, os.WriteFile(OpenCodeBakedVersionFile, []byte("1.18.18"), 0644))
	defer func() {
		OpenCodeCacheDir, OpenCodeBakedVersionFile = originalCache, originalVersionFile
	}()

	cached := filepath.Join(cacheDir, "1.19.0", "opencode")
	require.NoError(t, os.MkdirAll(filepath.Dir(cached), 0755))
	require.NoError(t, os.WriteFile(cached, []byte("cached"), 0755))

	d := openCodeDaemon()
	d.codeAgentConfig.OpenCodeBinary = &CodeAgentBinary{
		Version: "1.19.0",
		Artifacts: map[string]CodeAgentBinaryArtifact{
			// Unreachable: reaching for it at all would fail the test.
			runtime.GOARCH: {URL: "http://127.0.0.1:1/unreachable", SHA256: "abc"},
		},
	}

	command, err := d.resolveOpenCodeCommand()
	require.NoError(t, err)
	assert.Equal(t, cached, command)
}
