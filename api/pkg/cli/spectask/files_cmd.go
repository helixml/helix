package spectask

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/spf13/cobra"
)

// newFilesCommand lists the files in a running session's workspace — the same
// view the web UI's "Files" panel shows. Read-only; complements `copy` (upload
// into the container) and `download` (pull a file out).
func newFilesCommand() *cobra.Command {
	var (
		workspace  string
		jsonOutput bool
	)
	cmd := &cobra.Command{
		Use:   "files <session-id>",
		Short: "List files in a session's workspace",
		Long: `List files below a running session's /home/retro/work directory.

This includes Git checkouts and sibling output directories created by the
agent. Pass --workspace to restrict the listing to one repository's tracked and
non-ignored files.

Examples:
  helix spectask files ses_01xxx
  helix spectask files ses_01xxx --workspace keel --json
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sessionID := args[0]
			apiURL, token := getAPIURL(), getToken()
			if apiURL == "" || token == "" {
				return fmt.Errorf("HELIX_URL and HELIX_API_KEY environment variables must be set")
			}
			u := fmt.Sprintf("%s/api/v1/external-agents/%s/workspace-files", apiURL, sessionID)
			if workspace != "" {
				u += "?workspace=" + url.QueryEscape(workspace)
			} else {
				u += "?root=work"
			}
			body, err := workspaceGET(u, token)
			if err != nil {
				return err
			}
			var resp types.WorkspaceFilesResponse
			if err := json.Unmarshal(body, &resp); err != nil {
				return fmt.Errorf("decode workspace files: %w", err)
			}
			if jsonOutput {
				out, _ := json.MarshalIndent(resp, "", "  ")
				fmt.Fprintln(cmd.OutOrStdout(), string(out))
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "workspace: %s\n", resp.Workspace)
			for _, e := range resp.Entries {
				marker := " "
				if e.Kind == "dir" {
					marker = "/"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "  %10d  %s%s\n", e.Size, e.Path, marker)
			}
			if resp.Truncated {
				fmt.Fprintln(cmd.OutOrStdout(), "  … (listing truncated)")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&workspace, "workspace", "", "Restrict listing to a workspace/repo name (default: full /home/retro/work tree)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Print JSON")
	return cmd
}

// newDownloadCommand pulls one file out of a running session's workspace to the
// local filesystem — the inverse of `copy`. Use it to collect a sub-agent's
// output (findings.json, a rendered report, logs) into the driving session.
func newDownloadCommand() *cobra.Command {
	var (
		workspace string
		outPath   string
	)
	cmd := &cobra.Command{
		Use:   "download <session-id> <remote-path>",
		Short: "Download a file from a session's workspace",
		Long: `Download a file from a running session container's /home/retro/work directory.

<remote-path> is relative to /home/retro/work, so files beside repository
checkouts are directly reachable. Pass --workspace to make it relative to one
repository instead. By default the file is written to the current directory
under its base name; use -o to choose a path, or -o - to stream to stdout.

Examples:
  helix spectask download ses_01xxx engagement/findings.json
  helix spectask download ses_01xxx out/report.pdf -o ./report.pdf
  helix spectask download ses_01xxx notes.md -o -           # to stdout
`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			sessionID, remotePath := args[0], args[1]
			apiURL, token := getAPIURL(), getToken()
			if apiURL == "" || token == "" {
				return fmt.Errorf("HELIX_URL and HELIX_API_KEY environment variables must be set")
			}
			q := url.Values{}
			q.Set("path", remotePath)
			if workspace != "" {
				q.Set("workspace", workspace)
			} else {
				q.Set("root", "work")
			}
			u := fmt.Sprintf("%s/api/v1/external-agents/%s/workspace-file/download?%s", apiURL, sessionID, q.Encode())
			body, err := workspaceGET(u, token)
			if err != nil {
				return err
			}
			if outPath == "-" {
				_, err := cmd.OutOrStdout().Write(body)
				return err
			}
			dest := outPath
			if dest == "" {
				dest = filepath.Base(remotePath)
			}
			if err := os.WriteFile(dest, body, 0o644); err != nil {
				return fmt.Errorf("write %s: %w", dest, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ Downloaded %s (%d bytes) from session %s\n", dest, len(body), sessionID)
			return nil
		},
	}
	cmd.Flags().StringVar(&workspace, "workspace", "", "Resolve the remote path inside this workspace/repo (default: /home/retro/work)")
	cmd.Flags().StringVarP(&outPath, "output", "o", "", "Local output path (default: base name; '-' for stdout)")
	return cmd
}

func workspaceGET(u, token string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("workspace API returned %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}
