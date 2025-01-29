package filestore

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/system"
)

// Compile-time interface check:
var _ FileStore = (*FileSystemStorage)(nil)

type FileSystemStorage struct {
	basePath string
	baseURL  string
	secret   string
}

func NewFileSystemStorage(basePath string, baseURL, secret string) *FileSystemStorage {
	return &FileSystemStorage{
		basePath: basePath,
		baseURL:  baseURL,
		secret:   secret,
	}
}

func (s *FileSystemStorage) List(_ context.Context, prefix string) ([]Item, error) {
	fullPath := filepath.Join(s.basePath, prefix)

	fullPath, err := s.getSafePath(fullPath)
	if err != nil {
		return []Item{}, fmt.Errorf("invalid path: %s", prefix)
	}

	files, err := os.ReadDir(fullPath)
	if err != nil {
		return []Item{}, nil
	}

	items := []Item{}
	for _, f := range files {
		path := filepath.Join(prefix, f.Name())
		info, err := f.Info()
		if err != nil {
			return nil, fmt.Errorf("error fetching file info: %w", err)
		}
		items = append(items, Item{
			Directory: f.IsDir(),
			Name:      f.Name(),
			Path:      path,
			URL:       fmt.Sprintf("%s/%s", s.baseURL, path),
			Created:   info.ModTime().Unix(),
			Size:      info.Size(),
		})
	}

	return items, nil
}

func (s *FileSystemStorage) Get(_ context.Context, path string) (Item, error) {
	fullPath := filepath.Join(s.basePath, path)

	fullPath, err := s.getSafePath(fullPath)
	if err != nil {
		return Item{}, fmt.Errorf("invalid path: %s", path)
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		return Item{}, fmt.Errorf("error fetching file info: %w", err)
	}
	return Item{
		Directory: info.IsDir(),
		Name:      info.Name(),
		Path:      path,
		URL:       fmt.Sprintf("%s/%s", s.baseURL, path),
		Created:   info.ModTime().Unix(),
		Size:      info.Size(),
	}, nil
}

func (s *FileSystemStorage) SignedURL(_ context.Context, path string) (string, error) {
	return PresignURL(s.baseURL, "/"+path, s.secret, 20*time.Minute), nil
}

func (s *FileSystemStorage) WriteFile(ctx context.Context, path string, r io.Reader) (Item, error) {
	fullPath := filepath.Join(s.basePath, path)

	fullPath, err := s.getSafePath(fullPath)
	if err != nil {
		return Item{}, fmt.Errorf("invalid path: %s", path)
	}

	// Create the directory structure if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(fullPath), os.ModePerm); err != nil {
		return Item{}, fmt.Errorf("failed to create directory structure: %w", err)
	}

	file, err := os.Create(fullPath)
	if err != nil {
		return Item{}, fmt.Errorf("error creating file: %w", err)
	}
	defer file.Close()

	if _, err := io.Copy(file, r); err != nil {
		return Item{}, fmt.Errorf("failed to copy content to file: %w", err)
	}

	return s.Get(ctx, path)
}

func (s *FileSystemStorage) OpenFile(_ context.Context, path string) (io.ReadCloser, error) {
	fullPath := filepath.Join(s.basePath, path)

	fullPath, err := s.getSafePath(fullPath)
	if err != nil {
		return nil, fmt.Errorf("invalid path: %s", path)
	}

	file, err := os.Open(fullPath)
	if err != nil {
		return nil, fmt.Errorf("error opening file: %w", err)
	}

	return file, nil
}

func (s *FileSystemStorage) DownloadFolder(_ context.Context, path string) (io.Reader, error) {
	fullPath := filepath.Join(s.basePath, path)
	return system.GetTarStream(fullPath)
}

// UploadFolder uploads a folder from a tarball in the io.Reader to the specified path.
func (s *FileSystemStorage) UploadFolder(_ context.Context, path string, r io.Reader) error {
	// Determine the full path to the destination folder
	fullPath := filepath.Join(s.basePath, path)

	// Create the directory structure if it doesn't exist
	if err := os.MkdirAll(fullPath, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create directory structure: %w", err)
	}

	// Read from the tarball
	tr := tar.NewReader(r)
	for {
		header, err := tr.Next()

		// If no more files are found, break out of the loop
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed reading tarball: %w", err)
		}

		if strings.Contains(header.Name, "..") {
			return fmt.Errorf("invalid tar file: %s", header.Name)
		}

		// Determine the full path for the current file
		targetPath := filepath.Join(fullPath, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			// Create the directory
			if err := os.MkdirAll(targetPath, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}
		case tar.TypeReg:
			// Create the file
			file, err := os.OpenFile(targetPath, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("failed to create file: %w", err)
			}
			defer file.Close()

			// Copy the file contents from the tarball
			if _, err := io.Copy(file, tr); err != nil {
				return fmt.Errorf("failed to write file: %w", err)
			}
		}
	}

	return nil
}

func (s *FileSystemStorage) Rename(ctx context.Context, path string, newPath string) (Item, error) {
	src := filepath.Join(s.basePath, path)
	dst := filepath.Join(s.basePath, newPath)

	src, err := s.getSafePath(src)
	if err != nil {
		return Item{}, fmt.Errorf("invalid source path: %s", src)
	}

	dst, err = s.getSafePath(dst)
	if err != nil {
		return Item{}, fmt.Errorf("invalid destination path: %s", dst)
	}

	if err := os.Rename(src, dst); err != nil {
		return Item{}, fmt.Errorf("failed to rename file or directory: %w", err)
	}

	return s.Get(ctx, newPath)
}

func (s *FileSystemStorage) Delete(_ context.Context, path string) error {
	fullPath := filepath.Join(s.basePath, path)

	fullPath, err := s.getSafePath(fullPath)
	if err != nil {
		return fmt.Errorf("invalid path: %s", path)
	}

	if err := os.RemoveAll(fullPath); err != nil {
		return fmt.Errorf("failed to delete file or directory: %w", err)
	}

	return nil
}

func (s *FileSystemStorage) CreateFolder(ctx context.Context, path string) (Item, error) {
	fullPath := filepath.Join(s.basePath, path)

	fullPath, err := s.getSafePath(fullPath)
	if err != nil {
		return Item{}, fmt.Errorf("invalid path: %s", path)
	}

	if err := os.MkdirAll(fullPath, os.ModePerm); err != nil {
		return Item{}, fmt.Errorf("failed to create folder: %w", err)
	}

	return s.Get(ctx, path)
}

func (s *FileSystemStorage) CopyFile(_ context.Context, fromPath string, toPath string) error {
	fullFromPath := filepath.Join(s.basePath, fromPath)
	fullToPath := filepath.Join(s.basePath, toPath)

	fullFromPath, err := s.getSafePath(fullFromPath)
	if err != nil {
		return fmt.Errorf("invalid from path: %s", fromPath)
	}

	fullToPath, err = s.getSafePath(fullToPath)
	if err != nil {
		return fmt.Errorf("invalid to path: %s", toPath)
	}

	srcFile, err := os.Open(fullFromPath)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer srcFile.Close()

	// Create the destination directory if it doesn't exist
	destDir := filepath.Dir(fullToPath)
	if err := os.MkdirAll(destDir, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	// Check if the destination file already exists
	if _, err := os.Stat(fullToPath); err == nil {
		// Destination file already exists, no need to create a hard link
		return nil
	}

	// Create the hard link
	if err := os.Link(fullFromPath, fullToPath); err != nil {
		return fmt.Errorf("failed to create hard link: %w", err)
	}

	return nil
}

func (s *FileSystemStorage) getSafePath(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil || !strings.HasPrefix(absPath, s.basePath) {
		return "", fmt.Errorf("invalid path: %s", path)
	}
	return absPath, nil
}
