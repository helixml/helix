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
