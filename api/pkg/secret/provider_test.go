package secret

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadFileUnderRoot(t *testing.T) {
	root := t.TempDir()
	secretPath := filepath.Join(root, "provider-key")
	require.NoError(t, os.WriteFile(secretPath, []byte("secret"), 0o600))

	got, err := readFileUnderRoot(secretPath, root)
	require.NoError(t, err)
	require.Equal(t, []byte("secret"), got)
}

func TestReadFileUnderRootRejectsEscapes(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "stolen")
	require.NoError(t, os.WriteFile(outside, []byte("secret"), 0o600))

	symlink := filepath.Join(root, "provider-key")
	require.NoError(t, os.Symlink(outside, symlink))

	for _, path := range []string{outside, symlink} {
		_, err := readFileUnderRoot(path, root)
		require.Error(t, err)
	}
}
