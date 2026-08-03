package desktop

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveFileWithinRoot(t *testing.T) {
	root := t.TempDir()
	attachmentPath := filepath.Join(root, "image.png")
	require.NoError(t, os.WriteFile(attachmentPath, []byte("image"), 0o600))

	resolved, err := resolveFileWithinRoot(root, "image.png")
	require.NoError(t, err)
	require.Equal(t, attachmentPath, resolved)

	_, err = resolveFileWithinRoot(root, "../secret.txt")
	require.Error(t, err)
	_, err = resolveFileWithinRoot(root, `folder\\secret.txt`)
	require.Error(t, err)

	outside := filepath.Join(t.TempDir(), "outside.png")
	require.NoError(t, os.WriteFile(outside, []byte("outside"), 0o600))
	symlinkPath := filepath.Join(root, "linked.png")
	require.NoError(t, os.Symlink(outside, symlinkPath))
	_, err = resolveFileWithinRoot(root, "linked.png")
	require.Error(t, err)
}
