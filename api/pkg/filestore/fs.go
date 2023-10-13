package filestore

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
		return nil, fmt.Errorf("error reading directory: %w", err)
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

func (s *FileSystemStorage) Upload(ctx context.Context, path string, r io.Reader) (FileStoreItem, error) {
	fullPath := filepath.Join(s.basePath, path)
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

// Compile-time interface check:
var _ FileStore = (*FileSystemStorage)(nil)
