package server

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/connman"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/revdial"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// serveFakeDesktop registers handler as the desktop behind connman key,
// reachable over a real revdial tunnel: the API side dials through the
// connection manager exactly as the handlers under test do.
func serveFakeDesktop(t *testing.T, cm *connman.ConnectionManager, key string, handler http.Handler) {
	t.Helper()
	pickup := httptest.NewServer(revdial.ConnHandler(websocket.Upgrader{}))
	apiSide, desktopSide := net.Pipe()
	cm.Set(key, apiSide, "test")
	listener := revdial.NewListener(desktopSide, func(ctx context.Context, path string) (*websocket.Conn, *http.Response, error) {
		return websocket.DefaultDialer.DialContext(ctx, "ws"+strings.TrimPrefix(pickup.URL, "http")+path, nil)
	})
	go func() { _ = http.Serve(listener, handler) }()
	t.Cleanup(func() {
		_ = listener.Close()
		cm.Remove(key)
		pickup.Close()
	})
}

// desktopFileResponder answers every request with the given headers and body,
// standing in for a desktop whose file responses must not be trusted to
// decide rendering on the API origin.
func desktopFileResponder(contentType, disposition, body string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		if disposition != "" {
			w.Header().Set("Content-Disposition", disposition)
		}
		_, _ = w.Write([]byte(body))
	})
}

func newDesktopProxyTestServer(t *testing.T) (*HelixAPIServer, *store.MockStore) {
	t.Helper()
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	cm := connman.New()
	t.Cleanup(cm.Stop)
	server := &HelixAPIServer{
		Store:   mockStore,
		connman: cm,
		Cfg:     &config.ServerConfig{},
		Controller: &controller.Controller{
			Options: controller.Options{Store: mockStore, PubSub: pubsub.NewNoop()},
		},
	}
	return server, mockStore
}

func TestGetExternalAgentFileDeliversHTMLAttachmentAsDownload(t *testing.T) {
	server, mockStore := newDesktopProxyTestServer(t)
	const sessionID = "ses_shared"
	mockStore.EXPECT().GetSession(gomock.Any(), sessionID).Return(&types.Session{ID: sessionID, Owner: "usr_viewer"}, nil).AnyTimes()

	const body = "<script>fetch('/api/v1/users')</script>"
	serveFakeDesktop(t, server.connman, "desktop-"+sessionID,
		desktopFileResponder("text/html; charset=utf-8", `inline; filename="evil.html"`, body))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/external-agents/"+sessionID+"/file?name=evil.html", nil)
	req = mux.SetURLVars(req, map[string]string{"sessionID": sessionID})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: "usr_viewer"}))
	rec := httptest.NewRecorder()
	server.getExternalAgentFile(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, body, rec.Body.String())
	require.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	require.Contains(t, rec.Header().Get("Content-Disposition"), "attachment")
	require.Contains(t, rec.Header().Get("Content-Disposition"), "evil.html")
	require.Contains(t, rec.Header().Get("Content-Security-Policy"), "sandbox")
}

func TestGetExternalAgentFileKeepsImagesInline(t *testing.T) {
	server, mockStore := newDesktopProxyTestServer(t)
	const sessionID = "ses_images"
	mockStore.EXPECT().GetSession(gomock.Any(), sessionID).Return(&types.Session{ID: sessionID, Owner: "usr_viewer"}, nil).AnyTimes()

	serveFakeDesktop(t, server.connman, "desktop-"+sessionID,
		desktopFileResponder("image/png", `inline; filename="photo.png"`, "\x89PNG"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/external-agents/"+sessionID+"/file?name=photo.png", nil)
	req = mux.SetURLVars(req, map[string]string{"sessionID": sessionID})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: "usr_viewer"}))
	rec := httptest.NewRecorder()
	server.getExternalAgentFile(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "image/png", rec.Header().Get("Content-Type"))
	require.Equal(t, `inline; filename="photo.png"`, rec.Header().Get("Content-Disposition"))
	require.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	require.Empty(t, rec.Header().Get("Content-Security-Policy"))
}

func TestDownloadWorkspaceFileDoesNotTrustDesktopDisposition(t *testing.T) {
	server, mockStore := newDesktopProxyTestServer(t)
	const sessionID = "ses_workspace"
	mockStore.EXPECT().GetSession(gomock.Any(), sessionID).Return(&types.Session{ID: sessionID, Owner: "usr_viewer"}, nil).AnyTimes()

	const body = "<html><script>1</script></html>"
	serveFakeDesktop(t, server.connman, "desktop-"+sessionID,
		desktopFileResponder("text/html", `inline; filename="index.html"`, body))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/external-agents/"+sessionID+"/workspace-file/download?path=docs/index.html", nil)
	req = mux.SetURLVars(req, map[string]string{"sessionID": sessionID})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: "usr_viewer"}))
	rec := httptest.NewRecorder()
	server.downloadWorkspaceFile(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, body, rec.Body.String())
	require.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	require.Contains(t, rec.Header().Get("Content-Disposition"), "attachment")
	require.Contains(t, rec.Header().Get("Content-Disposition"), "index.html")
	require.Contains(t, rec.Header().Get("Content-Security-Policy"), "sandbox")
}
