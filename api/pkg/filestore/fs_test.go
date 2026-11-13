package filestore

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func newTestFileSystemStorage(t *testing.T) (*FileSystemStorage, string) {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "users/u1/docs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "users/u1/docs/a.txt"), []byte("a"), 0o644))
	return NewFileSystemStorage(root, "http://example.test/files", "secret"), root
}

// makeUnreadable creates a directory the process cannot read. Root ignores
// permission bits, so the case is skipped there.
func makeUnreadable(t *testing.T, root, rel string) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("running as root: permission bits are not enforced")
	}
	dir := filepath.Join(root, rel)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.Chmod(dir, 0o000))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
}

func TestFileSystemStorageList(t *testing.T) {
	tests := []struct {
		name       string
		prefix     string
		setup      func(t *testing.T, root string)
		wantNames  []string
		wantErr    bool
		wantErrMsg string
	}{
		{name: "existing directory", prefix: "users/u1/docs", wantNames: []string{"a.txt"}},
		{name: "missing directory is empty", prefix: "users/u1/missing", wantNames: []string{}},
		{
			name:       "unreadable directory is an error",
			prefix:     "users/u1/locked",
			setup:      func(t *testing.T, root string) { makeUnreadable(t, root, "users/u1/locked") },
			wantErr:    true,
			wantErrMsg: "list users/u1/locked: permission denied",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store, root := newTestFileSystemStorage(t)
			if tc.setup != nil {
				tc.setup(t, root)
			}
			items, err := store.List(context.Background(), tc.prefix)
			if tc.wantErr {
				require.Error(t, err)
				require.Nil(t, items)
				require.Equal(t, tc.wantErrMsg, err.Error())
				require.NotContains(t, err.Error(), root)
				require.True(t, errors.Is(err, fs.ErrPermission))
				require.False(t, errors.Is(err, ErrNotFound))
				return
			}
			require.NoError(t, err)
			names := []string{}
			for _, item := range items {
				names = append(names, item.Name)
			}
			require.Equal(t, tc.wantNames, names)
		})
	}
}

func TestFileSystemStorageGetAndOpenMissing(t *testing.T) {
	store, root := newTestFileSystemStorage(t)
	ctx := context.Background()

	item, err := store.Get(ctx, "users/u1/docs/a.txt")
	require.NoError(t, err)
	require.Equal(t, "a.txt", item.Name)

	_, err = store.Get(ctx, "users/u1/docs/missing.txt")
	require.ErrorIs(t, err, ErrNotFound)
	require.Equal(t, "stat users/u1/docs/missing.txt: not found", err.Error())
	require.NotContains(t, err.Error(), root)

	_, err = store.OpenFile(ctx, "users/u1/docs/missing.txt")
	require.ErrorIs(t, err, ErrNotFound)
	require.NotContains(t, err.Error(), root)
}

func TestFileSystemStorageGetUnreadableIsNotNotFound(t *testing.T) {
	store, root := newTestFileSystemStorage(t)
	makeUnreadable(t, root, "users/u1/locked")

	_, err := store.Get(context.Background(), "users/u1/locked/a.txt")
	require.Error(t, err)
	require.False(t, errors.Is(err, ErrNotFound))
	require.True(t, errors.Is(err, fs.ErrPermission))
	require.NotContains(t, err.Error(), root)
}
