package helix

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_getSafePath(t *testing.T) {
	repoDir := "/Users/jason/"
	path := "some_dir/some_file.txt"

	safePath, err := getSafePath(repoDir, path)
	require.NoError(t, err)

	require.Equal(t, "/Users/jason/some_dir/some_file.txt", safePath)
}
