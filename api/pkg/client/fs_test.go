package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFilestoreUploadSendsDirectoryAndMultipartFilename(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/filestore/upload", r.URL.Path)
		require.Equal(t, "engagements/prj_1/retests", r.URL.Query().Get("path"))
		require.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		require.NoError(t, r.ParseMultipartForm(1<<20))
		files := r.MultipartForm.File["files"]
		require.Len(t, files, 1)
		require.Equal(t, "retest_1.json", files[0].Filename)
		file, err := files[0].Open()
		require.NoError(t, err)
		defer file.Close()
		body, err := io.ReadAll(file)
		require.NoError(t, err)
		require.Equal(t, []byte(`{"status":"passed"}`), body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key", false)
	require.NoError(t, err)
	require.NoError(t, client.FilestoreUpload(
		context.Background(),
		"engagements/prj_1/retests/retest_1.json",
		strings.NewReader(`{"status":"passed"}`),
	))
}

func TestNormalizeFilestoreUploadPath(t *testing.T) {
	got, err := normalizeFilestoreUploadPath(`engagements\prj_1\retests\retest_1.json`, "windows")
	require.NoError(t, err)
	require.Equal(t, "engagements/prj_1/retests/retest_1.json", got)

	_, err = normalizeFilestoreUploadPath(`engagements\prj_1\retests\retest_1.json`, "linux")
	require.EqualError(t, err, "path contains backslashes; use forward slashes on linux")
}

func TestFilestoreUploadRequiresDestinationFilename(t *testing.T) {
	client, err := NewClient("http://example.test", "test-key", false)
	require.NoError(t, err)

	for _, path := range []string{"engagements/prj_1/retests/", ".", ".."} {
		err = client.FilestoreUpload(context.Background(), path, strings.NewReader("receipt"))
		require.EqualError(t, err, "path must include a filename")
	}
}
