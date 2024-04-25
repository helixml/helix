package filestore

import (
	"context"
	"io"
)

type FileStoreItem struct {
	// timestamp
	Created int64 `json:"created"`
	// bytes
	Size int64 `json:"size"`
	// is this thing a folder or not?
	Directory bool `json:"directory"`
	// the filename
	Name string `json:"name"`
	// the relative path to the file from the base path of the storage instance
	Path string `json:"path"`
	// the URL that can be used to load the object directly
	URL string `json:"url"`
}

// top level filestore folders that have special meaning
type FilestoreFolder struct {
	Name     string `json:"name"`
	Readonly bool   `json:"readonly"`
}

type FilestoreConfig struct {
	// this will be the virtual path from the storage instance
	// to the users root directory
	// we use this to strip the full paths in the frontend so we can deal with only relative paths
	UserPrefix string            `json:"user_prefix"`
	Folders    []FilestoreFolder `json:"folders"`
}

type FileStore interface {
	// list the items at a certain path
	List(ctx context.Context, path string) ([]FileStoreItem, error)
	Get(ctx context.Context, path string) (FileStoreItem, error)
	SignedURL(ctx context.Context, path string) (string, error)
	CreateFolder(ctx context.Context, path string) (FileStoreItem, error)
	DownloadFile(ctx context.Context, path string) (io.Reader, error)
	UploadFile(ctx context.Context, path string, r io.Reader) (FileStoreItem, error)
	// this will return a tar file stream
	DownloadFolder(ctx context.Context, path string) (io.Reader, error)
	// upload a tar stream to a path
	UploadFolder(ctx context.Context, path string, r io.Reader) error
	Rename(ctx context.Context, path string, newPath string) (FileStoreItem, error)
	Delete(ctx context.Context, path string) error
	CopyFile(ctx context.Context, from string, to string) error
}
