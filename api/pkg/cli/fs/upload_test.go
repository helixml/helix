package fs

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUploadShortcutDoesNotRemoveFilesystemWriteCommand(t *testing.T) {
	filesystemCmd := New()
	nestedBefore, _, err := filesystemCmd.Find([]string{"write"})
	require.NoError(t, err)
	require.Equal(t, "write", nestedBefore.Name())
	require.Same(t, filesystemCmd, nestedBefore.Parent())

	shortcut := NewUploadCmd()
	require.Equal(t, "upload", shortcut.Name())
	require.NotSame(t, nestedBefore, shortcut)

	nestedAfter, _, err := filesystemCmd.Find([]string{"upload"})
	require.NoError(t, err)
	require.Same(t, nestedBefore, nestedAfter)
	require.Same(t, filesystemCmd, nestedAfter.Parent())
}
