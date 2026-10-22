//go:build !windows

package hydra

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

func prepareRootlessPersistentDataMount(path string) error {
	cleanPath := filepath.Clean(path)
	if path != cleanPath || !filepath.IsAbs(path) || path == string(filepath.Separator) {
		return fmt.Errorf("data mount source must be a canonical absolute non-root path")
	}
	fd, created, err := openOrCreateDirNoFollow(path)
	if err != nil {
		return err
	}
	dir := os.NewFile(uintptr(fd), path)
	defer dir.Close()

	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return err
	}
	if !created && (stat.Uid != 0 || stat.Gid != 0 || uint32(stat.Mode) != uint32(unix.S_IFDIR|0o755)) {
		return nil
	}

	_, err = dir.Readdirnames(1)
	if err == nil {
		return nil
	}
	if !errors.Is(err, io.EOF) {
		return err
	}
	if err := unix.Fchown(fd, instanceUID, instanceUID); err != nil {
		return err
	}
	return nil
}

func openOrCreateDirNoFollow(path string) (fd int, leafCreated bool, err error) {
	fd, err = unix.Open(string(filepath.Separator), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, false, err
	}
	defer func() {
		if err != nil {
			unix.Close(fd)
		}
	}()

	parts := strings.Split(strings.TrimPrefix(path, string(filepath.Separator)), string(filepath.Separator))
	for i, part := range parts {
		created := false
		if mkdirErr := unix.Mkdirat(fd, part, 0o755); mkdirErr == nil {
			created = true
		} else if !errors.Is(mkdirErr, unix.EEXIST) {
			err = mkdirErr
			return
		}
		nextFD, openErr := unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if openErr != nil {
			err = openErr
			return
		}
		unix.Close(fd)
		fd = nextFD
		if i == len(parts)-1 {
			leafCreated = created
		}
	}
	return fd, leafCreated, nil
}
