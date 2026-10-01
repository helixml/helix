package fs

import (
	"context"
	"fmt"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(newWriteCmd())
}

// NewUploadCmd returns a fresh top-level compatibility shortcut. Cobra commands
// have exactly one parent, so this must not return the command already mounted
// under `filesystem` or it disappears from that command's help and routing.
func NewUploadCmd() *cobra.Command {
	cmd := newWriteCmd()
	cmd.Use = "upload <local_file_path> <remote_file_path>"
	cmd.Aliases = nil
	cmd.Short = "Upload a file to the Helix filestore"
	return cmd
}

func newWriteCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "write <local_file_path> <remote_file_path>",
		Aliases: []string{"upload"},
		Short:   "Write a local file to an exact Helix filestore path",
		Long:    `Write a local file, or a directory recursively, to the specified Helix filestore path.`,
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			apiClient, err := client.NewClientFromEnv()
			if err != nil {
				return err
			}
			return UploadFiles(cmd.Context(), apiClient, args[0], args[1])
		},
	}
}

// UploadFiles upload files to the Helix filestore. If localPath is a directory, it will upload all files recursively in the directory
// to the remote path. If localPath is a file, it will upload the file to the remote path.
func UploadFiles(ctx context.Context, apiClient client.Client, localPath string, remotePath string) error {
	if strings.TrimSpace(remotePath) == "" {
		return fmt.Errorf("remote file path is required")
	}

	fileInfo, err := os.Stat(localPath)
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}

	if !fileInfo.IsDir() {
		return UploadFile(ctx, apiClient, localPath, remotePath)
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

			remoteFilePath := pathpkg.Join(remotePath, filepath.ToSlash(relativePath))
			err = UploadFile(ctx, apiClient, path, remoteFilePath)
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

func UploadFile(ctx context.Context, apiClient client.Client, localPath string, remotePath string) error {
	if strings.TrimSpace(remotePath) == "" || strings.HasSuffix(remotePath, "/") {
		return fmt.Errorf("remote file path must include a filename")
	}
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
