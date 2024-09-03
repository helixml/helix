package fs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(uploadCmd)
}

func NewUploadCmd() *cobra.Command {
	return uploadCmd
}

var uploadCmd = &cobra.Command{
	Use:   "upload <local_file_path> <remote_file_path>",
	Short: "Upload a file to the Helix filestore",
	Long:  `Upload a local file to the specified path in the Helix filestore.`,
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check parameters
		if len(args) != 2 {
			return fmt.Errorf("expected 2 arguments, got %d", len(args))
		}

		localPath := args[0]
		remotePath := args[1]

		if localPath == "" || remotePath == "" {
			return fmt.Errorf("local file path and remote file path are required")
		}

		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return err
		}
		ctx := cmd.Context()

		err = uploadFiles(ctx, apiClient, localPath, remotePath)
		if err != nil {
			return err
		}

		return nil
	},
}

// uploadFiles upload files to the Helix filestore. If localPath is a directory, it will upload all files recursively in the directory
// to the remote path. If localPath is a file, it will upload the file to the remote path.
func uploadFiles(ctx context.Context, apiClient client.Client, localPath string, remotePath string) error {
	fileInfo, err := os.Stat(localPath)
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}

	if !fileInfo.IsDir() {
		return uploadFile(ctx, apiClient, localPath, remotePath)
	}

	err = filepath.Walk(localPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			relativePath, err := filepath.Rel(localPath, path)
			if err != nil {
				return err
			}

			remoteFilePath := filepath.Join(remotePath, relativePath)
			err = uploadFile(ctx, apiClient, path, remoteFilePath)
			if err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk directory: %w", err)
	}

	return err
}

func uploadFile(ctx context.Context, apiClient client.Client, localPath string, remotePath string) error {
	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open local file: %w", err)
	}
	defer file.Close()

	fmt.Printf("Uploading file %s to %s\n", localPath, remotePath)
	err = apiClient.FilestoreUpload(ctx, remotePath, file)
	if err != nil {
		return fmt.Errorf("failed to upload file: %w", err)
	}

	return nil
}
