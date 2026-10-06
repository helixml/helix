package filestore

import (
	"context"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"strings"
)

type Item struct {
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
type Folder struct {
	Name     string `json:"name"`
	Readonly bool   `json:"readonly"`
}

type Config struct {
	// this will be the virtual path from the storage instance
	// to the users root directory
	// we use this to strip the full paths in the frontend so we can deal with only relative paths
	UserPrefix string   `json:"user_prefix"`
	Folders    []Folder `json:"folders"`
}

//go:generate mockgen -source $GOFILE -destination filestore_mocks.go -package $GOPACKAGE

type FileStore interface {
	// list the items at a certain path
	List(ctx context.Context, path string) ([]Item, error)
	Get(ctx context.Context, path string) (Item, error)
	SignedURL(ctx context.Context, path string) (string, error)
	CreateFolder(ctx context.Context, path string) (Item, error)

	OpenFile(ctx context.Context, path string) (io.ReadCloser, error)
	WriteFile(ctx context.Context, path string, r io.Reader) (Item, error)
	// this will return a tar file stream
	DownloadFolder(ctx context.Context, path string) (io.Reader, error)
	// upload a tar stream to a path
	UploadFolder(ctx context.Context, path string, r io.Reader) error
	Rename(ctx context.Context, path string, newPath string) (Item, error)
	Delete(ctx context.Context, path string) error
	CopyFile(ctx context.Context, from string, to string) error
}

// CleanRelativePath normalizes a logical filestore path and rejects paths
// that would escape the caller's user or app scope when joined to its root.
// A leading separator denotes the root of that scope, not the host filesystem.
func CleanRelativePath(rawPath string) (string, error) {
	normalized := strings.ReplaceAll(rawPath, `\`, "/")
	normalized = strings.TrimLeft(normalized, "/")
	cleaned := path.Clean(normalized)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("path escapes filestore scope: %s", rawPath)
	}
	return filepath.FromSlash(cleaned), nil
}

// JoinScopedPath joins a relative path to a filestore scope root without
// allowing traversal into another user's or app's namespace.
func JoinScopedPath(scopeRoot, path string) (string, error) {
	cleaned, err := CleanRelativePath(path)
	if err != nil {
		return "", err
	}
	return filepath.Join(scopeRoot, cleaned), nil
}

func GetUserPrefix(filestorePrefix, userID string) string {
	return filepath.Join(filestorePrefix, "users", userID)
}

// GetAppPrefix returns the path for an app's filestore directory
// This creates a path structure like: {filestorePrefix}/apps/{appID}
func GetAppPrefix(filestorePrefix, appID string) string {
	return filepath.Join(filestorePrefix, "apps", appID)
}

// GetSpecTaskAttachmentsPrefix returns the filestore path for a SpecTask's attachments:
// {filestorePrefix}/spec-tasks/{taskID}/attachments
func GetSpecTaskAttachmentsPrefix(filestorePrefix, specTaskID string) string {
	return filepath.Join(filestorePrefix, "spec-tasks", specTaskID, "attachments")
}

// GetArtifactVersionPrefix returns the immutable filestore root for one
// project artifact deployment.
func GetArtifactVersionPrefix(filestorePrefix, projectID, artifactID, versionID string) string {
	return filepath.Join(filestorePrefix, "projects", projectID, "artifacts", artifactID, "versions", versionID)
}

func GetArtifactPrefix(filestorePrefix, projectID, artifactID string) string {
	return filepath.Join(filestorePrefix, "projects", projectID, "artifacts", artifactID)
}
