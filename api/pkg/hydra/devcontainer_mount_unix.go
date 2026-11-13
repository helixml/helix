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

// prepareAgentCacheMount makes the shared agent-binary cache writable by the
// desktop user. The settings-sync daemon runs as that user and installs
// admin-pinned agent releases here; Docker would otherwise auto-create the
// source as root:root 0755 and every install would fail.
func prepareAgentCacheMount(path string) error {
	if path != filepath.Clean(path) || !filepath.IsAbs(path) || path == string(filepath.Separator) {
		return fmt.Errorf("agent cache mount source must be a canonical absolute non-root path")
	}
	fd, created, err := openOrCreateDirNoFollow(path)
	if err != nil {
		return err
	}
	defer unix.Close(fd)

	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return err
	}
	if stat.Uid == instanceUID && stat.Gid == instanceUID && stat.Mode&0o7777 == 0o755 {
		return nil
	}
	if !created {
		// Never re-own a filesystem someone mounted here: its root belongs to
		// whoever provisioned it, not to the desktop user.
		var parent unix.Stat_t
		if err := unix.Fstatat(fd, "..", &parent, 0); err != nil {
			return err
		}
		if parent.Dev != stat.Dev {
			return fmt.Errorf("%s is a mountpoint owned by %d:%d with mode %o; make it owned by %d:%d with mode 755",
				path, stat.Uid, stat.Gid, stat.Mode&0o7777, instanceUID, instanceUID)
		}
	}
	if err := unix.Fchown(fd, instanceUID, instanceUID); err != nil {
		return err
	}
	return unix.Fchmod(fd, 0o755)
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
