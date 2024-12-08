package copydir

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

func CopyDir(dst, src string) error {
	startTime := time.Now()
	src, err := filepath.EvalSymlinks(src)
	if err != nil {
		return err
	}

	// Check if source and destination are on the same filesystem
	useSymlinks := sameFilesystem(src, dst)

	// Add counters for operations and timing
	stats := struct {
		copies      int
		symlinks    int
		skipped     int
		evalSymTime time.Duration
		statTime    time.Duration
		symTime     time.Duration
		copyTime    time.Duration
		walkTime    time.Duration
	}{
		evalSymTime: time.Since(startTime),
	}

	walkFn := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if path == src {
			return nil
		}

		if strings.HasPrefix(filepath.Base(path), ".") {
			// Skip any dot files
			if info.IsDir() {
				return filepath.SkipDir
			} else {
				return nil
			}
		}

		// The "path" has the src prefixed to it. We need to join our
		// destination with the path without the src on it.
		dstPath := filepath.Join(dst, path[len(src):])

		// If we have a directory, make that subdirectory, then continue
		// the walk.
		if info.IsDir() {
			if path == filepath.Join(src, dst) {
				// dst is in src; don't walk it.
				return nil
			}

			if err := os.MkdirAll(dstPath, 0755); err != nil {
				return err
			}

			return nil
		}

		// If dstPath exists and has the same size as path, don't copy it again.
		// We're mainly copying content addressed blobs here, so this is
		// probably fine.
		// Must use Lstat to get the file status here in case the file is a symlink
		statStart := time.Now()
		dstInfo, err := os.Lstat(dstPath)
		if err == nil {
			stats.statTime += time.Since(statStart)
			if dstInfo.Size() == info.Size() {
				stats.skipped++
				return nil
			}
		}

		// we don't want to try and copy the same file over itself.
		statStart = time.Now()
		if eq, err := SameFile(path, dstPath); eq {
			stats.statTime += time.Since(statStart)
			stats.skipped++
			return nil
		} else if err != nil {
			stats.statTime += time.Since(statStart)
			return err
		}

		// Try to create a symlink if we're on the same filesystem
		if useSymlinks {
			symStart := time.Now()
			err = os.Symlink(path, dstPath)
			stats.symTime += time.Since(symStart)
			if err == nil {
				stats.symlinks++
				return nil
			}
		}

		// If symlinking is disabled or fails, fall back to copying
		copyStart := time.Now()
		srcF, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcF.Close()

		dstF, err := os.Create(dstPath)
		if err != nil {
			return err
		}
		defer dstF.Close()

		if _, err := io.Copy(dstF, srcF); err != nil {
			return err
		}

		stats.copies++
		stats.copyTime += time.Since(copyStart)
		return os.Chmod(dstPath, info.Mode())
	}

	walkStart := time.Now()
	err = filepath.Walk(src, walkFn)
	stats.walkTime = time.Since(walkStart)
	if err != nil {
		return err
	}

	log.Info().
		Int("symlinks", stats.symlinks).
		Int("copies", stats.copies).
		Int("skipped", stats.skipped).
		Dur("eval_symlinks_time", stats.evalSymTime).
		Dur("stat_time", stats.statTime).
		Dur("sym_time", stats.symTime).
		Dur("copy_time", stats.copyTime).
		Dur("walk_time", stats.walkTime).
		Dur("total_time", time.Since(startTime)).
		Str("src", src).
		Str("dst", dst).
		Msg("CopyDir completed")
	return nil
}

// SameFile returns true if the two given paths refer to the same physical
// file on disk, using the unique file identifiers from the underlying
// operating system. For example, on Unix systems this checks whether the
// two files are on the same device and have the same inode.
func SameFile(a, b string) (bool, error) {
	if a == b {
		return true, nil
	}

	aInfo, err := os.Lstat(a)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	bInfo, err := os.Lstat(b)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	// If b is a symlink, check if it points to a
	if bInfo.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(b)
		if err != nil {
			return false, err
		}
		// If the symlink points to our source file, they're the same
		if target == a {
			return true, nil
		}
	}

	return os.SameFile(aInfo, bInfo), nil
}
