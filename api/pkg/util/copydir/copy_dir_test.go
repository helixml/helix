package copydir

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCopyDir_symlinks(t *testing.T) {
	tmpdir := t.TempDir()

	moduleDir := filepath.Join(tmpdir, "modules")
	err := os.Mkdir(moduleDir, os.ModePerm)
	require.NoError(t, err)

	subModuleDir := filepath.Join(moduleDir, "test-module")
	err = os.Mkdir(subModuleDir, os.ModePerm)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(subModuleDir, "main.tf"), []byte("hello"), 0644)
	require.NoError(t, err)

	err = os.Symlink("test-module", filepath.Join(moduleDir, "symlink-module"))
	require.NoError(t, err)

	targetDir := filepath.Join(tmpdir, "target")
	_ = os.Mkdir(targetDir, os.ModePerm)

	err = CopyDir(targetDir, moduleDir)
	require.NoError(t, err)
	if _, err = os.Lstat(filepath.Join(targetDir, "test-module", "main.tf")); os.IsNotExist(err) {
		t.Fatal("target test-module/main.tf was not created")
	}

	if _, err = os.Lstat(filepath.Join(targetDir, "symlink-module", "main.tf")); os.IsNotExist(err) {
		t.Fatal("target symlink-module/main.tf was not created")
	}
}

func TestCopyDir_symlink_file(t *testing.T) {
	tmpdir := t.TempDir()

	moduleDir := filepath.Join(tmpdir, "modules")
	err := os.Mkdir(moduleDir, os.ModePerm)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(moduleDir, "main.tf"), []byte("hello"), 0644)
	require.NoError(t, err)

	err = os.Symlink("main.tf", filepath.Join(moduleDir, "symlink.tf"))
	require.NoError(t, err)

	targetDir := filepath.Join(tmpdir, "target")
	_ = os.Mkdir(targetDir, os.ModePerm)

	err = CopyDir(targetDir, moduleDir)
	require.NoError(t, err)

	if _, err = os.Lstat(filepath.Join(targetDir, "main.tf")); os.IsNotExist(err) {
		t.Fatal("target/main.tf was not created")
	}

	if _, err = os.Lstat(filepath.Join(targetDir, "symlink.tf")); os.IsNotExist(err) {
		t.Fatal("target/symlink.tf was not created")
	}
}
