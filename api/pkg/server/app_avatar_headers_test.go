package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gocloud.dev/blob/memblob"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

func newAvatarTestServer(t *testing.T, app *types.App, avatar []byte) *HelixAPIServer {
	t.Helper()
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().GetApp(gomock.Any(), app.ID).Return(app, nil).AnyTimes()
	bucket := memblob.OpenBucket(nil)
	t.Cleanup(func() { _ = bucket.Close() })
	require.NoError(t, bucket.WriteAll(context.Background(), getAvatarKey(app.ID), avatar, nil))
	return &HelixAPIServer{Cfg: &config.ServerConfig{}, Store: mockStore, avatarsBucket: bucket}
}

func getAvatar(server *HelixAPIServer, appID string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/"+appID+"/avatar", nil)
	req = mux.SetURLVars(req, map[string]string{"id": appID})
	rec := httptest.NewRecorder()
	server.getAppAvatar(rec, req)
	return rec
}

func TestGetAppAvatarDeliversSVGAsSandboxedDownload(t *testing.T) {
	const svg = `<svg xmlns="http://www.w3.org/2000/svg"><script>fetch('/api/v1/users')</script></svg>`
	app := &types.App{ID: "app_svg", Owner: "usr_a"}
	app.Config.Helix.Avatar = "/api/v1/agents/app_svg/avatar"
	app.Config.Helix.AvatarContentType = "image/svg+xml"
	server := newAvatarTestServer(t, app, []byte(svg))

	rec := getAvatar(server, "app_svg")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, svg, rec.Body.String())
	require.Equal(t, "image/svg+xml", rec.Header().Get("Content-Type"))
	require.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	require.Contains(t, rec.Header().Get("Content-Disposition"), "attachment")
	require.Contains(t, rec.Header().Get("Content-Security-Policy"), "sandbox")
}

// A content type that reached the config through the agent API rather than
// the upload route is not served: the bytes decide.
func TestGetAppAvatarIgnoresNonImageStoredContentType(t *testing.T) {
	app := &types.App{ID: "app_html", Owner: "usr_a"}
	app.Config.Helix.Avatar = "/api/v1/agents/app_html/avatar"
	app.Config.Helix.AvatarContentType = "text/html; charset=utf-8"
	server := newAvatarTestServer(t, app, []byte("<html><script>1</script></html>"))

	rec := getAvatar(server, "app_html")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "application/octet-stream", rec.Header().Get("Content-Type"))
	require.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	require.Contains(t, rec.Header().Get("Content-Disposition"), "attachment")
	require.Contains(t, rec.Header().Get("Content-Security-Policy"), "sandbox")
}

func TestGetAppAvatarKeepsRasterImagesInline(t *testing.T) {
	app := &types.App{ID: "app_png", Owner: "usr_a"}
	app.Config.Helix.Avatar = "/api/v1/agents/app_png/avatar"
	app.Config.Helix.AvatarContentType = "image/png"
	server := newAvatarTestServer(t, app, []byte("\x89PNG\r\n\x1a\n"))

	rec := getAvatar(server, "app_png")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "image/png", rec.Header().Get("Content-Type"))
	require.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	require.Empty(t, rec.Header().Get("Content-Disposition"))
	require.Empty(t, rec.Header().Get("Content-Security-Policy"))
}

func TestUpdateAgentCannotRelabelAvatarContentType(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	existing := &types.App{ID: "app_1", Owner: "usr_bob"}
	existing.Config.Helix.AvatarContentType = "image/png"
	mockStore.EXPECT().GetApp(gomock.Any(), "app_1").Return(existing, nil)
	var saved *types.App
	mockStore.EXPECT().UpdateApp(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, app *types.App) (*types.App, error) { saved = app; return app, nil })
	mockStore.EXPECT().ListTriggerConfigurations(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockStore.EXPECT().ListKnowledge(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	server := &HelixAPIServer{Cfg: &config.ServerConfig{}, Store: mockStore}
	req := httptest.NewRequest(http.MethodPut, "/api/v1/agents/app_1",
		strings.NewReader(`{"config":{"helix":{"name":"mine","avatar_content_type":"text/html","assistants":[{"name":"a"}]}}}`))
	req = req.WithContext(setRequestUser(context.Background(), types.User{ID: "usr_bob"}))
	req = mux.SetURLVars(req, map[string]string{"id": "app_1"})

	_, httpErr := server.updateAgent(httptest.NewRecorder(), req)
	require.Nil(t, httpErr)
	require.NotNil(t, saved)
	require.Equal(t, "image/png", saved.Config.Helix.AvatarContentType)
}
