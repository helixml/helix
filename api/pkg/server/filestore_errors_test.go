package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/stretchr/testify/require"
)

func TestFilestoreHTTPErrorHidesStorageDetails(t *testing.T) {
	root := t.TempDir()
	fsStore := filestore.NewFileSystemStorage(root, "http://example.test/files", "secret")

	_, statErr := fsStore.Get(context.Background(), "dev/users/u1/%2e%2e%2fmissing")
	require.ErrorIs(t, statErr, filestore.ErrNotFound)
	require.NotContains(t, statErr.Error(), root)

	_, escapeErr := fsStore.Get(context.Background(), "../../etc/passwd")
	require.Error(t, escapeErr)

	cases := []struct {
		name    string
		err     error
		status  int
		message string
	}{
		{"missing local file", statErr, http.StatusNotFound, "file not found"},
		{"missing GCS object", fmt.Errorf("error fetching GCS object attributes: %w", storage.ErrObjectNotExist), http.StatusNotFound, "file not found"},
		{"path outside the store", escapeErr, http.StatusBadRequest, "invalid path"},
		{"scope escape", fmt.Errorf("wrap: %w", filestore.ErrInvalidPath), http.StatusBadRequest, "invalid path"},
		{"storage fault", fmt.Errorf("failed to create directory structure: mkdir %s/x: read-only file system", root), http.StatusInternalServerError, "filestore operation failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var httpErr *system.HTTPError
			require.True(t, errors.As(filestoreHTTPError(tc.err), &httpErr))
			require.Equal(t, tc.status, httpErr.StatusCode)
			require.Equal(t, tc.message, httpErr.Message)
			require.NotContains(t, httpErr.Message, root)
		})
	}

	require.NoError(t, filestoreHTTPError(nil))
	forbidden := system.NewHTTPError403("access denied to app filestore path: apps/app_1")
	require.Same(t, forbidden, filestoreHTTPError(forbidden))
}

func TestWithFilestoreErrorsAnswersThroughTheWrapper(t *testing.T) {
	root := t.TempDir()
	fsStore := filestore.NewFileSystemStorage(root, "http://example.test/files", "secret")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "dev/users/u1/docs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "dev/users/u1/docs/a.txt"), []byte("a"), 0o644))
	locked := filepath.Join(root, "dev/users/u1/locked")
	require.NoError(t, os.MkdirAll(locked, 0o755))
	require.NoError(t, os.Chmod(locked, 0o000))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	get := system.DefaultWrapper(withFilestoreErrors(func(_ http.ResponseWriter, r *http.Request) (filestore.Item, error) {
		return fsStore.Get(r.Context(), "dev/users/u1/"+r.URL.Query().Get("path"))
	}))
	list := system.DefaultWrapper(withFilestoreErrors(func(_ http.ResponseWriter, r *http.Request) ([]filestore.Item, error) {
		return fsStore.List(r.Context(), "dev/users/u1/"+r.URL.Query().Get("path"))
	}))

	tests := []struct {
		name      string
		handler   http.HandlerFunc
		path      string
		status    int
		body      string
		needsPerm bool
	}{
		{name: "get existing file", handler: get, path: "docs/a.txt", status: http.StatusOK},
		{name: "get missing file", handler: get, path: "nope", status: http.StatusNotFound, body: "file not found\n"},
		{name: "get under unreadable folder", handler: get, path: "locked/a.txt", status: http.StatusInternalServerError, body: "filestore operation failed\n", needsPerm: true},
		{name: "list missing folder", handler: list, path: "missing", status: http.StatusOK, body: "[]\n"},
		{name: "list unreadable folder", handler: list, path: "locked", status: http.StatusInternalServerError, body: "filestore operation failed\n", needsPerm: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.needsPerm && os.Geteuid() == 0 {
				t.Skip("running as root: permission bits are not enforced")
			}
			rec := httptest.NewRecorder()
			tc.handler(rec, httptest.NewRequest(http.MethodGet, "/api/v1/filestore/x?path="+tc.path, nil))
			require.Equal(t, tc.status, rec.Code)
			if tc.body != "" {
				require.Equal(t, tc.body, rec.Body.String())
			}
			require.NotContains(t, rec.Body.String(), root)
		})
	}
}
