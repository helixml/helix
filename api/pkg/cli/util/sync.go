package util

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/helixml/helix/api/pkg/client"
)

// SyncLocalDirToFilestore synchronizes a local directory to the Helix filestore.
// It will only upload files that don't exist in the filestore, implementing an
// rsync-like functionality. If deleteExtraFiles is true, it will also delete files
// from the filestore that don't exist locally.
func SyncLocalDirToFilestore(ctx context.Context, apiClient client.Client, localDir, remotePath string, deleteExtraFiles bool) error {
	if localDir == "" || remotePath == "" {
		return fmt.Errorf("local directory and remote path are required")
	}

	// Check if local directory exists
	fileInfo, err := os.Stat(localDir)
	if err != nil {
		return fmt.Errorf("failed to access local directory %s: %w", localDir, err)
	}

	if !fileInfo.IsDir() {
		return fmt.Errorf("%s is not a directory", localDir)
	}

	// Get remote files
	remoteFiles, err := getRemoteFiles(ctx, apiClient, remotePath)
	if err != nil {
		return fmt.Errorf("failed to list remote files: %w", err)
	}

	// Keep track of local files for deletion purposes
	localFiles := make(map[string]bool)

	// Walk through local directory and upload files that don't exist remotely
	uploadCount := 0
	err = filepath.Walk(localDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			relPath, err := filepath.Rel(localDir, path)
			if err != nil {
				return fmt.Errorf("failed to get relative path: %w", err)
			}

			remoteFilePath := filepath.Join(remotePath, relPath)

			// Mark this file as existing locally
			localFiles[remoteFilePath] = true

			// Check if file needs to be uploaded (doesn't exist remotely)
			if !fileExistsInRemote(remoteFiles, remoteFilePath) {
				err = uploadFile(ctx, apiClient, path, remoteFilePath)
				if err != nil {
					return fmt.Errorf("failed to upload file %s: %w", path, err)
				}
				uploadCount++
			}
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("error walking local directory: %w", err)
	}

	fmt.Printf("Synced %d files to %s\n", uploadCount, remotePath)

	// Delete files that exist remotely but not locally
	if deleteExtraFiles {
		deletedCount := 0
		for remotePath := range remoteFiles {
			if !localFiles[remotePath] {
				fmt.Printf("Deleting %s (not found locally)\n", remotePath)
				err := apiClient.FilestoreDelete(ctx, remotePath)
				if err != nil {
					fmt.Printf("Warning: Failed to delete %s: %v\n", remotePath, err)
				} else {
					deletedCount++
				}
			}
		}
		fmt.Printf("Deleted %d files from %s\n", deletedCount, remotePath)
	}

	return nil
}

// uploadFile uploads a single file to the filestore
func uploadFile(ctx context.Context, apiClient client.Client, localPath, remotePath string) error {
	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open local file: %w", err)
	}
	defer file.Close()

	fmt.Printf("Uploading %s to %s\n", localPath, remotePath)
	return apiClient.FilestoreUpload(ctx, remotePath, file)
}

// UploadFile is a public version of uploadFile for external use
func UploadFile(ctx context.Context, apiClient client.Client, localPath, remotePath string) error {
	return uploadFile(ctx, apiClient, localPath, remotePath)
}

// getRemoteFiles recursively gets all files in the remote path
func getRemoteFiles(ctx context.Context, apiClient client.Client, remotePath string) (map[string]bool, error) {
	result := make(map[string]bool)

	items, err := apiClient.FilestoreList(ctx, remotePath)
	if err != nil {
		// If the directory doesn't exist yet, return an empty list
		// This is not an error, we'll create it later
		if strings.Contains(err.Error(), "not found") {
			return result, nil
		}
		return nil, err
	}

	for _, item := range items {
		if !item.Directory {
			result[item.Path] = true
		} else {
			// Recursively get files in subdirectories
			subItems, err := getRemoteFiles(ctx, apiClient, item.Path)
			if err != nil {
				return nil, err
			}

			// Add subdirectory items to result
			for path := range subItems {
				result[path] = true
			}
		}
	}

	return result, nil
}

// fileExistsInRemote checks if a file already exists in the remote filestore
func fileExistsInRemote(remoteFiles map[string]bool, filePath string) bool {
	_, exists := remoteFiles[filePath]
	return exists
}
