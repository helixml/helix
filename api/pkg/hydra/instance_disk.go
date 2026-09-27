package hydra

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

const instanceDiskMarker = ".helix-instance-disk"

var instanceDisksBaseDir = "/data/instance-disks"

// prepareInstanceDisk mounts a capacity-limited filesystem and makes it the
// instance's complete home. Keeping work, tool state and caches on one
// filesystem prevents moving data between writable home paths to evade the
// workspace limit.
func prepareInstanceDisk(ctx context.Context, dockerClient *client.Client, image string, req *CreateDevContainerRequest) error {
	if req.DiskSizeGB == 0 {
		return nil
	}
	if !isResourceID(req.SessionID, "ses_") {
		return fmt.Errorf("disk quota requires a valid session id, got %q", req.SessionID)
	}

	diskDir := filepath.Join(instanceDisksBaseDir, req.SessionID)
	imagePath := filepath.Join(diskDir, "home.ext4")
	homePath := filepath.Join(diskDir, "home")
	if err := os.MkdirAll(homePath, 0o700); err != nil {
		return fmt.Errorf("create instance disk directory: %w", err)
	}

	wantBytes := int64(req.DiskSizeGB) * 1024 * 1024 * 1024
	created := false
	info, err := os.Stat(imagePath)
	switch {
	case errors.Is(err, os.ErrNotExist):
		file, createErr := os.OpenFile(imagePath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
		if createErr != nil {
			return fmt.Errorf("create instance disk image: %w", createErr)
		}
		if truncateErr := file.Truncate(wantBytes); truncateErr != nil {
			file.Close()
			return fmt.Errorf("size instance disk image: %w", truncateErr)
		}
		if closeErr := file.Close(); closeErr != nil {
			return fmt.Errorf("close instance disk image: %w", closeErr)
		}
		if output, formatErr := exec.CommandContext(ctx, "mkfs.ext4", "-F", "-m", "0", imagePath).CombinedOutput(); formatErr != nil {
			return fmt.Errorf("format instance disk: %w: %s", formatErr, output)
		}
		created = true
	case err != nil:
		return fmt.Errorf("inspect instance disk image: %w", err)
	case info.Size() != wantBytes:
		return fmt.Errorf("instance disk size is immutable: stored %d GB, requested %d GB",
			info.Size()/(1024*1024*1024), req.DiskSizeGB)
	}

	mounted, err := isMountpoint(ctx, homePath)
	if err != nil {
		return err
	}
	if !mounted {
		if output, mountErr := exec.CommandContext(ctx, "mount", "-o", "loop,nodev,nosuid", imagePath, homePath).CombinedOutput(); mountErr != nil {
			return fmt.Errorf("mount instance disk: %w: %s", mountErr, output)
		}
	}

	markerPath := filepath.Join(homePath, instanceDiskMarker)
	if _, err := os.Stat(markerPath); errors.Is(err, os.ErrNotExist) {
		entries, readErr := os.ReadDir(homePath)
		if readErr != nil {
			return fmt.Errorf("read uninitialized instance disk: %w", readErr)
		}
		for _, entry := range entries {
			if err := os.RemoveAll(filepath.Join(homePath, entry.Name())); err != nil {
				return fmt.Errorf("clear uninitialized instance disk: %w", err)
			}
		}
		if err := seedInstanceHome(ctx, dockerClient, image, req.SessionID, homePath); err != nil {
			return err
		}
		if err := os.WriteFile(markerPath, []byte(strconv.Itoa(req.DiskSizeGB)+"\n"), 0o600); err != nil {
			return fmt.Errorf("write instance disk marker: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("inspect instance disk marker: %w", err)
	} else if created {
		return errors.New("new instance disk unexpectedly contained an initialization marker")
	}

	workPath := filepath.Join(homePath, "work")
	if err := os.MkdirAll(workPath, 0o755); err != nil {
		return fmt.Errorf("create instance workspace: %w", err)
	}
	if err := os.Chown(workPath, 1000, 1000); err != nil {
		return fmt.Errorf("own instance workspace: %w", err)
	}

	for i := range req.Mounts {
		switch req.Mounts[i].Destination {
		case "/home/retro/work", "/workspace":
			req.Mounts[i].Source = workPath
		default:
			if req.Mounts[i].Destination == req.Mounts[i].Source && filepath.Base(req.Mounts[i].Source) == req.SessionID {
				req.Mounts[i].Source = workPath
			}
		}
	}
	req.Mounts = append([]MountConfig{{Source: homePath, Destination: "/home/retro"}}, req.Mounts...)
	return nil
}

func isMountpoint(ctx context.Context, path string) (bool, error) {
	err := exec.CommandContext(ctx, "mountpoint", "-q", path).Run()
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		switch exitErr.ExitCode() {
		case 1, 32:
			return false, nil
		}
	}
	return false, fmt.Errorf("check instance disk mountpoint: %w", err)
}

func seedInstanceHome(ctx context.Context, dockerClient *client.Client, image, sessionID, destination string) error {
	helperName := "helix-home-seed-" + sessionID
	if old, err := dockerClient.ContainerInspect(ctx, helperName); err == nil {
		if err := dockerClient.ContainerRemove(ctx, old.ID, container.RemoveOptions{Force: true}); err != nil {
			return fmt.Errorf("remove stale home seed container: %w", err)
		}
	}
	created, err := dockerClient.ContainerCreate(ctx, &container.Config{
		Image:      image,
		Entrypoint: []string{"/bin/true"},
	}, nil, nil, nil, helperName)
	if err != nil {
		return fmt.Errorf("create home seed container: %w", err)
	}
	defer dockerClient.ContainerRemove(context.Background(), created.ID, container.RemoveOptions{Force: true}) //nolint:errcheck

	content, _, err := dockerClient.CopyFromContainer(ctx, created.ID, "/home/retro/.")
	if err != nil {
		return fmt.Errorf("read image home: %w", err)
	}
	defer content.Close()
	if err := extractHomeArchive(content, destination); err != nil {
		return fmt.Errorf("seed instance home: %w", err)
	}
	return nil
}

func extractHomeArchive(reader io.Reader, destination string) error {
	tarReader := tar.NewReader(reader)
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		cleanName := filepath.Clean(header.Name)
		if filepath.IsAbs(cleanName) || cleanName == ".." || strings.HasPrefix(cleanName, ".."+string(filepath.Separator)) {
			return fmt.Errorf("unsafe image home path %q", header.Name)
		}
		target := destination
		if cleanName != "." {
			target = filepath.Join(destination, cleanName)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			file, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(file, tarReader)
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		case tar.TypeSymlink:
			if err := os.Symlink(header.Linkname, target); err != nil && !errors.Is(err, os.ErrExist) {
				return err
			}
		case tar.TypeLink:
			linkName := filepath.Clean(header.Linkname)
			if filepath.IsAbs(linkName) || linkName == ".." || strings.HasPrefix(linkName, ".."+string(filepath.Separator)) {
				return fmt.Errorf("unsafe image home hardlink %q", header.Linkname)
			}
			if err := os.Link(filepath.Join(destination, linkName), target); err != nil {
				return err
			}
		default:
			continue
		}
		if header.Typeflag == tar.TypeSymlink {
			if err := os.Lchown(target, header.Uid, header.Gid); err != nil {
				return err
			}
			continue
		}
		if err := os.Chown(target, header.Uid, header.Gid); err != nil {
			return err
		}
		if err := os.Chmod(target, os.FileMode(header.Mode)); err != nil {
			return err
		}
	}
}

func destroyInstanceDisk(ctx context.Context, sessionID string) error {
	if !isResourceID(sessionID, "ses_") {
		return fmt.Errorf("invalid session id %q", sessionID)
	}
	diskDir := filepath.Join(instanceDisksBaseDir, sessionID)
	homePath := filepath.Join(diskDir, "home")
	mounted, err := isMountpoint(ctx, homePath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if mounted {
		if output, unmountErr := exec.CommandContext(ctx, "umount", homePath).CombinedOutput(); unmountErr != nil {
			return fmt.Errorf("unmount instance disk: %w: %s", unmountErr, output)
		}
	}
	if err := os.RemoveAll(diskDir); err != nil {
		return fmt.Errorf("remove instance disk: %w", err)
	}
	return nil
}
