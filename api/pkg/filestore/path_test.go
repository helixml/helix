package filestore

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestJoinScopedPath(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		want      string
		wantError bool
	}{
		{name: "nested path", path: "engagements/prj_1/receipt.json", want: filepath.Join("dev/users/user-1", "engagements/prj_1/receipt.json")},
		{name: "scope root", path: "", want: filepath.Join("dev/users/user-1")},
		{name: "parent traversal", path: "../../users/user-2/secret.json", wantError: true},
		{name: "parent directory", path: "..", wantError: true},
		{name: "absolute path", path: filepath.Join(string(filepath.Separator), "dev", "users", "user-2"), wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := JoinScopedPath("dev/users/user-1", test.path)
			if test.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.want, got)
		})
	}
}

func TestFileSystemStorageSafePathRejectsScopeEscape(t *testing.T) {
	basePath := t.TempDir()
	storage := NewFileSystemStorage(basePath, "", "")

	inside, err := storage.getSafePath(filepath.Join(basePath, "users", "user-1"))
	require.NoError(t, err)
	require.Equal(t, filepath.Join(basePath, "users", "user-1"), inside)

	_, err = storage.getSafePath(filepath.Join(basePath, "..", filepath.Base(basePath)+"-sibling"))
	require.Error(t, err)

	_, err = storage.getSafePath(filepath.Join(basePath, "..", "other"))
	require.Error(t, err)
}
