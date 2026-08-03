package server

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDesktopUploadURL(t *testing.T) {
	t.Run("preserves chat upload options", func(t *testing.T) {
		require.Equal(
			t,
			"http://localhost:9876/upload?open_file_manager=false",
			desktopUploadURL("open_file_manager=false"),
		)
	})

	t.Run("omits an empty query", func(t *testing.T) {
		require.Equal(t, "http://localhost:9876/upload", desktopUploadURL(""))
	})
}

func TestDesktopFileURL(t *testing.T) {
	require.Equal(
		t,
		"http://localhost:9876/file?name=my+image.png",
		desktopFileURL("my image.png"),
	)
}

func TestValidWorkspaceAttachmentFilename(t *testing.T) {
	require.True(t, validWorkspaceAttachmentFilename("my image.png"))
	require.False(t, validWorkspaceAttachmentFilename(""))
	require.False(t, validWorkspaceAttachmentFilename("../secret"))
	require.False(t, validWorkspaceAttachmentFilename(`folder\\secret`))
}
