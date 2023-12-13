package filestore

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/lukemarsden/helix/api/pkg/system"
)

type FileSystemStorage struct {
	basePath string
	baseURL  string
}

func NewFileSystemStorage(basePath string, baseURL string) *FileSystemStorage {
	return &FileSystemStorage{
		basePath: basePath,
		baseURL:  baseURL,
	}
}

func (s *FileSystemStorage) List(ctx context.Context, prefix string) ([]FileStoreItem, error) {
	fullPath := filepath.Join(s.basePath, prefix)
	files, err := os.ReadDir(fullPath)
	if err != nil {
		return []FileStoreItem{}, nil
	}

	items := []FileStoreItem{}
	for _, f := range files {
		path := filepath.Join(prefix, f.Name())
		info, err := f.Info()
		if err != nil {
			return nil, fmt.Errorf("error fetching file info: %w", err)
		}
		items = append(items, FileStoreItem{
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

func (s *FileSystemStorage) Get(ctx context.Context, path string) (FileStoreItem, error) {
	fullPath := filepath.Join(s.basePath, path)
	info, err := os.Stat(fullPath)
	if err != nil {
		return FileStoreItem{}, fmt.Errorf("error fetching file info: %w", err)
	}
	return FileStoreItem{
		Directory: info.IsDir(),
		Name:      info.Name(),
		Path:      path,
		URL:       fmt.Sprintf("%s/%s", s.baseURL, path),
		Created:   info.ModTime().Unix(),
		Size:      info.Size(),
	}, nil
}

func (s *FileSystemStorage) UploadFile(ctx context.Context, path string, r io.Reader) (FileStoreItem, error) {
	fullPath := filepath.Join(s.basePath, path)

	// Create the directory structure if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(fullPath), os.ModePerm); err != nil {
		return FileStoreItem{}, fmt.Errorf("failed to create directory structure: %w", err)
	}

	file, err := os.Create(fullPath)
	if err != nil {
		return FileStoreItem{}, fmt.Errorf("error creating file: %w", err)
	}
	defer file.Close()

	if _, err := io.Copy(file, r); err != nil {
		return FileStoreItem{}, fmt.Errorf("failed to copy content to file: %w", err)
	}

	return s.Get(ctx, path)
}

func (s *FileSystemStorage) DownloadFile(ctx context.Context, path string) (io.Reader, error) {
	fullPath := filepath.Join(s.basePath, path)

	file, err := os.Open(fullPath)
	if err != nil {
		return nil, fmt.Errorf("error opening file: %w", err)
	}

	return file, nil
}

func (s *FileSystemStorage) DownloadFolder(ctx context.Context, path string) (io.Reader, error) {
	fullPath := filepath.Join(s.basePath, path)
	return system.GetTarStream(fullPath)
}

// UploadFolder uploads a folder from a tarball in the io.Reader to the specified path.
func (s *FileSystemStorage) UploadFolder(ctx context.Context, path string, r io.Reader) error {
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

func (s *FileSystemStorage) Rename(ctx context.Context, path string, newPath string) (FileStoreItem, error) {
	src := filepath.Join(s.basePath, path)
	dst := filepath.Join(s.basePath, newPath)

	if err := os.Rename(src, dst); err != nil {
		return FileStoreItem{}, fmt.Errorf("failed to rename file or directory: %w", err)
	}

	return s.Get(ctx, newPath)
}

func (s *FileSystemStorage) Delete(ctx context.Context, path string) error {
	fullPath := filepath.Join(s.basePath, path)

	if err := os.RemoveAll(fullPath); err != nil {
		return fmt.Errorf("failed to delete file or directory: %w", err)
	}

	return nil
}

func (s *FileSystemStorage) CreateFolder(ctx context.Context, path string) (FileStoreItem, error) {
	fullPath := filepath.Join(s.basePath, path)

	if err := os.MkdirAll(fullPath, os.ModePerm); err != nil {
		return FileStoreItem{}, fmt.Errorf("failed to create folder: %w", err)
	}

	return s.Get(ctx, path)
}

func (s *FileSystemStorage) CopyFile(ctx context.Context, fromPath string, toPath string) error {
	fullFromPath := filepath.Join(s.basePath, fromPath)
	fullToPath := filepath.Join(s.basePath, toPath)

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

	// Create the destination file
	destFile, err := os.Create(fullToPath)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer destFile.Close()

	// Copy the contents from source to destination
	if _, err := io.Copy(destFile, srcFile); err != nil {
		return fmt.Errorf("failed to copy file contents: %w", err)
	}

	return nil
}

// Compile-time interface check:
var _ FileStore = (*FileSystemStorage)(nil)
