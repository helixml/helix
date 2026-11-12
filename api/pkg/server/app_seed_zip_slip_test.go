package server

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/filestore"
)

// Seed zip entry names are user-controlled; "../" entries must not be written
// into other apps' or users' filestore folders.
func TestExtractZipFileRejectsTraversal(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range []string{"../../other-app/knowledge/evil.txt", "docs/../../escape.txt", "docs/ok.txt"} {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = w.Write([]byte("x"))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	require.NoError(t, err)

	ctrl := gomock.NewController(t)
	fs := filestore.NewMockFileStore(ctrl)
	base := "dev/apps/app_1/knowledge"
	fs.EXPECT().CreateFolder(gomock.Any(), base+"/docs").Return(filestore.Item{}, nil)
	fs.EXPECT().WriteFile(gomock.Any(), base+"/docs/ok.txt", gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, r io.Reader) (filestore.Item, error) {
			_, _ = io.Copy(io.Discard, r)
			return filestore.Item{}, nil
		})
	srv := &HelixAPIServer{Controller: &controller.Controller{Options: controller.Options{Filestore: fs}}}

	require.Error(t, srv.extractZipFile(context.Background(), zr.File[0], base))
	require.Error(t, srv.extractZipFile(context.Background(), zr.File[1], base))
	require.NoError(t, srv.extractZipFile(context.Background(), zr.File[2], base))
}
