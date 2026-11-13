package spa

import (
	"bufio"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

type hijackableResponseWriter struct {
	header http.Header
	conn   net.Conn
	rw     *bufio.ReadWriter
}

func (w *hijackableResponseWriter) Header() http.Header {
	return w.header
}

func (w *hijackableResponseWriter) Write(body []byte) (int, error) {
	return len(body), nil
}

func (w *hijackableResponseWriter) WriteHeader(_ int) {}

func (w *hijackableResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return w.conn, w.rw, nil
}

func TestStatusRecorderPreservesHijacker(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	t.Cleanup(func() {
		require.NoError(t, serverConn.Close())
		require.NoError(t, clientConn.Close())
	})

	buffer := bufio.NewReadWriter(bufio.NewReader(serverConn), bufio.NewWriter(serverConn))
	underlying := &hijackableResponseWriter{
		header: make(http.Header),
		conn:   serverConn,
		rw:     buffer,
	}
	recorder := &statusRecorder{ResponseWriter: underlying, status: http.StatusOK}

	conn, rw, err := recorder.Hijack()
	require.NoError(t, err)
	require.Same(t, serverConn, conn)
	require.Same(t, buffer, rw)
}

func newTestFileServer(t *testing.T, policy ShellPolicy) *FileServer {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.html"), []byte("<!doctype html><title>shell</title>"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "assets"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "assets", "index-abc.js"), []byte("console.log(1)"), 0o600))
	return NewSPAFileServer(http.Dir(dir), policy)
}

func serve(t *testing.T, s http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestFileServerFallsBackToShellForClientRoutes(t *testing.T) {
	s := newTestFileServer(t, ShellPolicy{})
	for _, path := range []string{"/", "/login", "/orgs/acme/projects/prj_1/tasks/spt_1"} {
		rec := serve(t, s, path)
		require.Equal(t, http.StatusOK, rec.Code, path)
		require.Contains(t, rec.Body.String(), "<title>shell</title>", path)
		require.Equal(t, "object-src 'none'; base-uri 'self'", rec.Header().Get("Content-Security-Policy"), path)
		require.Contains(t, rec.Header().Get("Permissions-Policy"), "geolocation=()", path)
	}
}

func TestFileServerAnswers404ForMissingFiles(t *testing.T) {
	s := newTestFileServer(t, ShellPolicy{})
	for _, path := range []string{"/.env", "/.git/HEAD", "/foo/.DS_Store", "/api/v2/anything", "/api", "/assets/index-old.js"} {
		rec := serve(t, s, path)
		require.Equal(t, http.StatusNotFound, rec.Code, path)
		require.NotContains(t, rec.Body.String(), "<title>shell</title>", path)
		require.Empty(t, rec.Header().Get("Content-Security-Policy"), path)
	}
}

func TestFileServerServesExistingAssets(t *testing.T) {
	s := newTestFileServer(t, ShellPolicy{})
	rec := serve(t, s, "/assets/index-abc.js")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "public, max-age=31536000, immutable", rec.Header().Get("Cache-Control"))
	require.Empty(t, rec.Header().Get("Content-Security-Policy"))
}

func TestFileServerFrameAncestorsExemptsEmbedRoutes(t *testing.T) {
	s := newTestFileServer(t, ShellPolicy{FrameAncestors: "'self'"})

	rec := serve(t, s, "/orgs/acme/api-keys")
	require.Equal(t, "object-src 'none'; base-uri 'self'; frame-ancestors 'self'", rec.Header().Get("Content-Security-Policy"))

	rec = serve(t, s, "/")
	require.Contains(t, rec.Header().Get("Content-Security-Policy"), "frame-ancestors 'self'")

	for _, path := range []string{"/embed/task/spt_1", "/embed/session/ses_1", "/embed/project/prj_1"} {
		rec = serve(t, s, path)
		require.Equal(t, http.StatusOK, rec.Code, path)
		require.NotContains(t, rec.Header().Get("Content-Security-Policy"), "frame-ancestors", path)
	}
}
