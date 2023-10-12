package filestore

import (
	"context"
	"io"
)

type FileStoreItem struct {
	// is this thing a folder or not?
	Directory bool `json:"directory"`
	// the filename
	Name string `json:"name"`
	// the relative path to the file from the base path of the storage instance
	Path string `json:"path"`
	// the URL that can be used to load the object directly
	URL string `json:"url"`
}

type FileStore interface {
	// list the items at a certain path
	List(ctx context.Context, path string) ([]FileStoreItem, error)
	Get(ctx context.Context, path string) (FileStoreItem, error)
	CreateFolder(ctx context.Context, path string) (FileStoreItem, error)
	Upload(ctx context.Context, path string, r io.Reader) (FileStoreItem, error)
	Rename(ctx context.Context, path string, newPath string) (FileStoreItem, error)
	Delete(ctx context.Context, path string) error
}
