package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	require.Error(t, statErr)
	require.Contains(t, statErr.Error(), root, "precondition: the raw error names the storage root")

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
	handler := system.DefaultWrapper(withFilestoreErrors(func(_ http.ResponseWriter, r *http.Request) (filestore.Item, error) {
		return fsStore.Get(r.Context(), "dev/users/u1/"+r.URL.Query().Get("path"))
	}))

	rec := httptest.NewRecorder()
	handler(rec, httptest.NewRequest(http.MethodGet, "/api/v1/filestore/get?path=nope", nil))
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Equal(t, "file not found\n", rec.Body.String())
}
