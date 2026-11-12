package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/types"
)

// newFilestoreViewerTestServer serves a temporary directory through the real
// viewer handler (authorization + http.FileServer), the way registerRoutes
// mounts it.
func newFilestoreViewerTestServer(t *testing.T, root string) http.Handler {
	t.Helper()
	cfg := &config.ServerConfig{}
	cfg.Controller.FilePrefixGlobal = "dev"
	server := &HelixAPIServer{
		Cfg:               cfg,
		Controller:        &controller.Controller{Options: controller.Options{Config: cfg}},
		fileServerHandler: http.FileServer(neuteredFileSystem{http.Dir(root)}),
	}
	return http.StripPrefix(APIPrefix+"/filestore/viewer/", http.HandlerFunc(server.filestoreViewerHandler))
}

func getFilestoreViewerFile(t *testing.T, handler http.Handler, user types.User, relPath string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, APIPrefix+"/filestore/viewer/"+relPath, nil)
	req = req.WithContext(setRequestUser(req.Context(), user))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestFilestoreViewerDeliversActiveContentAsDownload(t *testing.T) {
	root := t.TempDir()
	userDir := filepath.Join(root, "dev", "users", "usr_owner")
	require.NoError(t, os.MkdirAll(userDir, 0o755))
	files := map[string]string{
		"page.html":  "<script>document.title=document.cookie</script>",
		"vector.svg": `<svg xmlns="http://www.w3.org/2000/svg"><script>1</script></svg>`,
		"photo.png":  "\x89PNG\r\n\x1a\n",
		"report.pdf": "%PDF-1.4",
		"notes.txt":  "plain words",
	}
	for name, body := range files {
		require.NoError(t, os.WriteFile(filepath.Join(userDir, name), []byte(body), 0o644))
	}
	handler := newFilestoreViewerTestServer(t, root)
	owner := types.User{ID: "usr_owner"}

	for _, name := range []string{"page.html", "vector.svg"} {
		rec := getFilestoreViewerFile(t, handler, owner, "dev/users/usr_owner/"+name)
		require.Equal(t, http.StatusOK, rec.Code, name)
		require.Equal(t, files[name], rec.Body.String(), name)
		require.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"), name)
		require.Contains(t, rec.Header().Get("Content-Disposition"), "attachment", name)
		require.Contains(t, rec.Header().Get("Content-Disposition"), name, name)
		require.Contains(t, rec.Header().Get("Content-Security-Policy"), "sandbox", name)
	}

	for name, contentType := range map[string]string{"photo.png": "image/png", "report.pdf": "application/pdf", "notes.txt": "text/plain"} {
		rec := getFilestoreViewerFile(t, handler, owner, "dev/users/usr_owner/"+name)
		require.Equal(t, http.StatusOK, rec.Code, name)
		require.Contains(t, rec.Header().Get("Content-Type"), contentType, name)
		require.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"), name)
		require.Empty(t, rec.Header().Get("Content-Disposition"), name)
	}
}

func TestFilestoreViewerUserPrefixIsASegmentBoundary(t *testing.T) {
	root := t.TempDir()
	for _, user := range []string{"usr_a", "usr_ab"} {
		dir := filepath.Join(root, "dev", "users", user)
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "secret.txt"), []byte(user), 0o644))
	}
	handler := newFilestoreViewerTestServer(t, root)

	rec := getFilestoreViewerFile(t, handler, types.User{ID: "usr_a"}, "dev/users/usr_a/secret.txt")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "usr_a", rec.Body.String())

	rec = getFilestoreViewerFile(t, handler, types.User{ID: "usr_a"}, "dev/users/usr_ab/secret.txt")
	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestApplyServedFilePolicy(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		inline      bool
	}{
		{name: "html", contentType: "text/html; charset=utf-8"},
		{name: "xhtml", contentType: "application/xhtml+xml"},
		{name: "svg", contentType: "image/svg+xml"},
		{name: "xml", contentType: "text/xml"},
		{name: "javascript", contentType: "text/javascript"},
		{name: "unknown", contentType: ""},
		{name: "png", contentType: "image/png", inline: true},
		{name: "pdf", contentType: "application/pdf", inline: true},
		{name: "mp4", contentType: "video/mp4", inline: true},
		{name: "plain", contentType: "text/plain; charset=utf-8", inline: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header := http.Header{}
			if tt.contentType != "" {
				header.Set("Content-Type", tt.contentType)
			}
			header.Set("Content-Disposition", `inline; filename="f"`)
			applyServedFilePolicy(header, "f")
			require.Equal(t, "nosniff", header.Get("X-Content-Type-Options"))
			require.NotEmpty(t, header.Get("Content-Type"))
			if tt.inline {
				require.Equal(t, `inline; filename="f"`, header.Get("Content-Disposition"))
				require.Empty(t, header.Get("Content-Security-Policy"))
				return
			}
			require.Equal(t, `attachment; filename=f`, header.Get("Content-Disposition"))
			require.Contains(t, header.Get("Content-Security-Policy"), "sandbox")
		})
	}
}
