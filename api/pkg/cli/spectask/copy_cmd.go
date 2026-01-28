package spectask

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
)

// CopyResponse represents the response from the upload endpoint
type CopyResponse struct {
	Success  bool   `json:"success,omitempty"`
	Path     string `json:"path,omitempty"`
	Filename string `json:"filename,omitempty"`
	Error    string `json:"error,omitempty"`
}

func newCopyCommand() *cobra.Command {
	var (
		destPath        string
		openFileManager bool
	)

	cmd := &cobra.Command{
		Use:   "copy <session-id> <local-file>",
		Short: "Copy a file into a session container",
		Long: `Copy a local file into a running session container.

By default, files are copied to ~/work/incoming/ in the container.
Use --dest to specify a different destination path.

Examples:
  # Copy a file to the default incoming folder
  helix spectask copy ses_01xxx ./script.py

  # Copy to a specific destination
  helix spectask copy ses_01xxx ./config.json --dest /home/retro/work/config.json

  # Copy without opening file manager
  helix spectask copy ses_01xxx ./data.txt --no-file-manager
`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			sessionID := args[0]
			localFile := args[1]

			apiURL := getAPIURL()
			token := getToken()

			if apiURL == "" || token == "" {
				return fmt.Errorf("HELIX_URL and HELIX_API_KEY environment variables must be set")
			}

			// Check if local file exists
			fileInfo, err := os.Stat(localFile)
			if err != nil {
				return fmt.Errorf("cannot access local file: %w", err)
			}
			if fileInfo.IsDir() {
				return fmt.Errorf("cannot copy directories, only files are supported")
			}

			result, err := copyFileToSession(apiURL, token, sessionID, localFile, destPath, openFileManager)
			if err != nil {
				return err
			}

			if result.Error != "" {
				return fmt.Errorf("copy failed: %s", result.Error)
			}

			fmt.Printf("✓ Copied %s to session %s\n", filepath.Base(localFile), sessionID)
			if result.Path != "" {
				fmt.Printf("  Destination: %s\n", result.Path)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&destPath, "dest", "", "Destination path in the container (default: ~/work/incoming/<filename>)")
	cmd.Flags().BoolVar(&openFileManager, "no-file-manager", false, "Don't open file manager after upload")

	return cmd
}

func copyFileToSession(apiURL, token, sessionID, localFile, destPath string, noFileManager bool) (*CopyResponse, error) {
	uploadURL := fmt.Sprintf("%s/api/v1/external-agents/%s/upload", apiURL, sessionID)

	// Add query parameters
	if !noFileManager {
		uploadURL += "?open_file_manager=true"
	} else {
		uploadURL += "?open_file_manager=false"
	}

	// Open the file
	file, err := os.Open(localFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Create multipart form
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// Add the file
	filename := filepath.Base(localFile)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}

	if _, err := io.Copy(part, file); err != nil {
		return nil, fmt.Errorf("failed to copy file content: %w", err)
	}

	// Add destination path if specified
	if destPath != "" {
		if err := writer.WriteField("dest_path", destPath); err != nil {
			return nil, fmt.Errorf("failed to write dest_path field: %w", err)
		}
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	// Create request
	req, err := http.NewRequest("POST", uploadURL, &body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 60 * time.Second}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("upload API returned %d: %s", resp.StatusCode, string(respBody))
	}

	return &CopyResponse{
		Success:  true,
		Filename: filename,
	}, nil
}
