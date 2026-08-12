package desktop

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenFileWithinRoot(t *testing.T) {
	root := t.TempDir()
	attachmentPath := filepath.Join(root, "image.png")
	require.NoError(t, os.WriteFile(attachmentPath, []byte("image"), 0o600))

	file, info, err := openFileWithinRoot(root, "image.png")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, file.Close()) })
	require.Equal(t, "image.png", info.Name())
	contents, err := io.ReadAll(file)
	require.NoError(t, err)
	require.Equal(t, []byte("image"), contents)

	_, _, err = openFileWithinRoot(root, "../secret.txt")
	require.Error(t, err)
	_, _, err = openFileWithinRoot(root, `folder\\secret.txt`)
	require.Error(t, err)

	outside := filepath.Join(t.TempDir(), "outside.png")
	require.NoError(t, os.WriteFile(outside, []byte("outside"), 0o600))
	symlinkPath := filepath.Join(root, "linked.png")
	require.NoError(t, os.Symlink(outside, symlinkPath))
	_, _, err = openFileWithinRoot(root, "linked.png")
	require.Error(t, err)
}
