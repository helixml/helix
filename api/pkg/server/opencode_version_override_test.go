package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/helixml/helix/api/pkg/opencode"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

const openCodeTestRelease = `{"tag_name":"v1.19.0","assets":[` +
	`{"name":"opencode-linux-x64.tar.gz","browser_download_url":"https://example.test/x64","digest":"sha256:aaa"},` +
	`{"name":"opencode-linux-arm64.tar.gz","browser_download_url":"https://example.test/arm64","digest":"sha256:bbb"}]}`

func openCodeOverrideServer(t *testing.T, pinned string) (*HelixAPIServer, *atomic.Int32) {
	t.Helper()
	var lookups atomic.Int32
	releases := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		lookups.Add(1)
		_, _ = w.Write([]byte(openCodeTestRelease))
	}))
	t.Cleanup(releases.Close)

	mockStore := store.NewMockStore(gomock.NewController(t))
	mockStore.EXPECT().GetSystemSettings(gomock.Any()).Return(&types.SystemSettings{OpenCodeVersion: pinned}, nil)
	return &HelixAPIServer{
		Store:            mockStore,
		openCodeResolver: opencode.NewResolver(nil, releases.URL),
	}, &lookups
}

func TestApplyOpenCodeVersionOverrideAttachesValidPin(t *testing.T) {
	server, lookups := openCodeOverrideServer(t, "1.19.0")
	cfg := &types.CodeAgentConfig{}

	server.applyOpenCodeVersionOverride(context.Background(), cfg)

	require.NotNil(t, cfg.OpenCodeBinary)
	assert.Equal(t, "1.19.0", cfg.OpenCodeBinary.Version)
	assert.Equal(t, int32(1), lookups.Load())
}

// A pin older than the bundled build can only reach the store outside the
// settings API, or by being overtaken by an image bump. Sessions must not
// receive it: opencode 1.2.3 sent a literal "{env:HELIX_API_KEY}" bearer.
func TestApplyOpenCodeVersionOverrideIgnoresPinOlderThanBundled(t *testing.T) {
	server, lookups := openCodeOverrideServer(t, "1.2.3")
	cfg := &types.CodeAgentConfig{}

	server.applyOpenCodeVersionOverride(context.Background(), cfg)

	assert.Nil(t, cfg.OpenCodeBinary)
	assert.Zero(t, lookups.Load(), "an invalid pin must not be resolved")
}

func TestSystemSettingsResponseFlagsInvalidOpenCodePin(t *testing.T) {
	resp := systemSettingsResponse(&types.SystemSettings{OpenCodeVersion: "1.2.3"}, "")
	assert.Contains(t, resp.OpenCodeVersionError, "not newer than the bundled version")

	resp = systemSettingsResponse(&types.SystemSettings{OpenCodeVersion: "1.19.0"}, "")
	assert.Empty(t, resp.OpenCodeVersionError)

	resp = systemSettingsResponse(&types.SystemSettings{}, "")
	assert.Empty(t, resp.OpenCodeVersionError)
}
