package artifact

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/cli"
	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "artifact",
		Aliases: []string{"artifacts"},
		Short:   "Publish and manage project static artifacts",
	}
	cmd.AddCommand(newCreateCommand(), newUpdateCommand(), newListCommand(), newGetCommand(), newDeleteCommand())
	return cmd
}

type uploadFlags struct {
	projectID     string
	name          string
	description   string
	entrypoint    string
	visibility    string
	subdomain     bool
	jsonOutput    bool
	sourceSession string
	sourceTask    string
}

func newCreateCommand() *cobra.Command {
	flags := uploadFlags{}
	cmd := &cobra.Command{
		Use:   "create <html-file-or-directory>",
		Short: "Create a static artifact from HTML or a compiled SPA directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectID := firstNonEmpty(flags.projectID, os.Getenv("HELIX_PROJECT_ID"))
			if projectID == "" {
				return errors.New("project is required: pass --project or set HELIX_PROJECT_ID")
			}
			if strings.TrimSpace(flags.name) == "" {
				return errors.New("name is required")
			}
			visibility, err := artifactVisibility(flags.visibility, types.ArtifactVisibilityProject)
			if err != nil {
				return err
			}
			content, closeContent, err := openArtifactContent(args[0])
			if err != nil {
				return err
			}
			defer closeContent()
			apiClient, err := newClient()
			if err != nil {
				return err
			}
			sourceSession, sourceTask := resolveArtifactProvenance(cmd, &flags, projectID)
			name, description, entrypoint, withSubdomain := flags.name, flags.description, flags.entrypoint, flags.subdomain
			artifact, err := apiClient.CreateArtifact(cmd.Context(), projectID, &client.ArtifactUploadRequest{
				Name: &name, Description: &description, Entrypoint: optionalString(entrypoint), Visibility: &visibility,
				WithSubdomain: &withSubdomain, SourceSessionID: sourceSession,
				SourceSpecTaskID: sourceTask, Content: content,
			})
			if err != nil {
				return err
			}
			return printArtifact(cmd, artifact, flags.jsonOutput)
		},
	}
	addUploadFlags(cmd, &flags, true)
	return cmd
}

func newUpdateCommand() *cobra.Command {
	flags := uploadFlags{}
	cmd := &cobra.Command{
		Use:   "update <artifact-id> [html-file-or-directory]",
		Short: "Update artifact metadata or publish a new content version",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			var content *client.ArtifactContent
			closeContent := func() {}
			var err error
			if len(args) == 2 {
				content, closeContent, err = openArtifactContent(args[1])
				if err != nil {
					return err
				}
				defer closeContent()
			}
			if len(args) == 1 && !anyUploadFlagChanged(cmd) {
				return errors.New("no changes specified")
			}
			apiClient, err := newClient()
			if err != nil {
				return err
			}
			input := &client.ArtifactUploadRequest{Content: content}
			if content != nil {
				current, err := apiClient.GetArtifact(cmd.Context(), args[0])
				if err != nil {
					return err
				}
				input.SourceSessionID, input.SourceSpecTaskID = resolveArtifactProvenance(cmd, &flags, current.ProjectID)
			}
			if cmd.Flags().Changed("name") {
				input.Name = &flags.name
			}
			if cmd.Flags().Changed("description") {
				input.Description = &flags.description
			}
			if cmd.Flags().Changed("entrypoint") {
				input.Entrypoint = &flags.entrypoint
			}
			if cmd.Flags().Changed("visibility") {
				visibility, err := artifactVisibility(flags.visibility, "")
				if err != nil {
					return err
				}
				input.Visibility = &visibility
			}
			if cmd.Flags().Changed("subdomain") {
				input.WithSubdomain = &flags.subdomain
			}
			artifact, err := apiClient.UpdateArtifact(cmd.Context(), args[0], input)
			if err != nil {
				return err
			}
			return printArtifact(cmd, artifact, flags.jsonOutput)
		},
	}
	addUploadFlags(cmd, &flags, false)
	return cmd
}

func newListCommand() *cobra.Command {
	var projectID string
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List artifacts in a project",
		RunE: func(cmd *cobra.Command, _ []string) error {
			projectID = firstNonEmpty(projectID, os.Getenv("HELIX_PROJECT_ID"))
			if projectID == "" {
				return errors.New("project is required: pass --project or set HELIX_PROJECT_ID")
			}
			apiClient, err := newClient()
			if err != nil {
				return err
			}
			artifacts, err := apiClient.ListArtifacts(cmd.Context(), projectID)
			if err != nil {
				return err
			}
			if jsonOutput {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(artifacts)
			}
			table := cli.NewSimpleTable(cmd.OutOrStdout(), []string{"ID", "Name", "Kind", "Visibility", "Version", "Updated", "URL"})
			for _, artifact := range artifacts {
				version := ""
				if artifact.ActiveVersion != nil {
					version = fmt.Sprintf("%d", artifact.ActiveVersion.Version)
				}
				if err := cli.AppendRow(table, []string{artifact.ID, artifact.Name, string(artifact.Kind), string(artifact.Visibility), version, artifact.UpdatedAt.Format(time.RFC3339), artifact.URL}); err != nil {
					return err
				}
			}
			return cli.RenderTable(table)
		},
	}
	cmd.Flags().StringVarP(&projectID, "project", "p", "", "Project ID (env: HELIX_PROJECT_ID)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Print JSON")
	return cmd
}

func newGetCommand() *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "get <artifact-id>",
		Short: "Show an artifact",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			apiClient, err := newClient()
			if err != nil {
				return err
			}
			artifact, err := apiClient.GetArtifact(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return printArtifact(cmd, artifact, jsonOutput)
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Print JSON")
	return cmd
}

func newDeleteCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <artifact-id>",
		Short: "Delete an artifact and all of its static files",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			apiClient, err := newClient()
			if err != nil {
				return err
			}
			if err := apiClient.DeleteArtifact(cmd.Context(), args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Deleted artifact %s\n", args[0])
			return nil
		},
	}
}

func addUploadFlags(cmd *cobra.Command, flags *uploadFlags, includeProject bool) {
	if includeProject {
		cmd.Flags().StringVarP(&flags.projectID, "project", "p", "", "Project ID (env: HELIX_PROJECT_ID)")
	}
	cmd.Flags().StringVarP(&flags.name, "name", "n", "", "Artifact name")
	cmd.Flags().StringVar(&flags.description, "description", "", "Artifact description")
	cmd.Flags().StringVar(&flags.entrypoint, "entrypoint", "", "HTML entrypoint (default: index.html)")
	cmd.Flags().StringVar(&flags.visibility, "visibility", "project", "Visibility: project or public")
	cmd.Flags().BoolVar(&flags.subdomain, "subdomain", false, "Deprecated: public artifacts automatically receive a share subdomain")
	_ = cmd.Flags().MarkDeprecated("subdomain", "public artifacts automatically receive a share subdomain")
	cmd.Flags().StringVar(&flags.sourceSession, "source-session", "", "Source session ID (defaults to HELIX_SESSION_ID)")
	cmd.Flags().StringVar(&flags.sourceTask, "source-spec-task", "", "Source spec task ID (defaults to HELIX_SPEC_TASK_ID)")
	cmd.Flags().BoolVar(&flags.jsonOutput, "json", false, "Print JSON")
	if includeProject {
		_ = cmd.MarkFlagRequired("name")
	}
}

func anyUploadFlagChanged(cmd *cobra.Command) bool {
	for _, name := range []string{"name", "description", "entrypoint", "visibility", "subdomain"} {
		if cmd.Flags().Changed(name) {
			return true
		}
	}
	return false
}

func resolveArtifactProvenance(cmd *cobra.Command, flags *uploadFlags, projectID string) (string, string) {
	sessionID := flags.sourceSession
	if !cmd.Flags().Changed("source-session") && projectID == os.Getenv("HELIX_PROJECT_ID") {
		sessionID = os.Getenv("HELIX_SESSION_ID")
	}
	taskID := flags.sourceTask
	if !cmd.Flags().Changed("source-spec-task") && projectID == os.Getenv("HELIX_PROJECT_ID") {
		taskID = os.Getenv("HELIX_SPEC_TASK_ID")
	}
	return sessionID, taskID
}

func newClient() (*client.HelixClient, error) {
	apiURL := firstNonEmpty(os.Getenv("HELIX_URL"), os.Getenv("HELIX_API_URL"), "http://localhost:8080")
	apiKey := firstNonEmpty(os.Getenv("HELIX_API_KEY"), os.Getenv("USER_API_TOKEN"))
	if apiKey == "" {
		return nil, errors.New("authentication is required: set HELIX_API_KEY or USER_API_TOKEN")
	}
	return client.NewClient(strings.TrimRight(apiURL, "/"), apiKey, false)
}

func openArtifactContent(sourcePath string) (*client.ArtifactContent, func(), error) {
	info, err := os.Lstat(sourcePath)
	if err != nil {
		return nil, func() {}, fmt.Errorf("inspect artifact source: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, func() {}, fmt.Errorf("artifact source must not be a symbolic link: %s", sourcePath)
	}
	if !info.IsDir() {
		if !info.Mode().IsRegular() {
			return nil, func() {}, fmt.Errorf("artifact source must be a regular file or directory: %s", sourcePath)
		}
		file, err := os.Open(sourcePath)
		if err != nil {
			return nil, func() {}, fmt.Errorf("open artifact file: %w", err)
		}
		return &client.ArtifactContent{Filename: filepath.Base(sourcePath), Reader: file}, func() { _ = file.Close() }, nil
	}
	temp, err := os.CreateTemp("", "helix-artifact-*.zip")
	if err != nil {
		return nil, func() {}, fmt.Errorf("create artifact archive: %w", err)
	}
	cleanup := func() {
		_ = temp.Close()
		_ = os.Remove(temp.Name())
	}
	if err := writeArtifactArchive(temp, sourcePath); err != nil {
		cleanup()
		return nil, func() {}, err
	}
	if _, err := temp.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return nil, func() {}, fmt.Errorf("rewind artifact archive: %w", err)
	}
	return &client.ArtifactContent{Filename: "artifact.zip", Reader: temp}, cleanup, nil
}

func writeArtifactArchive(destination *os.File, root string) error {
	writer := zip.NewWriter(destination)
	err := filepath.WalkDir(root, func(filename string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if filename == root || entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("artifact source contains a non-regular file: %s", filename)
		}
		relative, err := filepath.Rel(root, filename)
		if err != nil {
			return err
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(relative)
		header.Method = zip.Deflate
		part, err := writer.CreateHeader(header)
		if err != nil {
			return err
		}
		source, err := os.Open(filename)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(part, source)
		closeErr := source.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	if closeErr := writer.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("build artifact archive: %w", err)
	}
	return nil
}

func printArtifact(cmd *cobra.Command, artifact *types.Artifact, jsonOutput bool) error {
	if jsonOutput {
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(artifact)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s\nID: %s\nURL: %s\n", artifact.Name, artifact.ID, artifact.URL)
	if artifact.SubdomainURL != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Subdomain: %s\n", artifact.SubdomainURL)
	}
	return nil
}

func artifactVisibility(value string, fallback types.ArtifactVisibility) (types.ArtifactVisibility, error) {
	if strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	visibility := types.ArtifactVisibility(value)
	if visibility != types.ArtifactVisibilityProject && visibility != types.ArtifactVisibilityPublic {
		return "", errors.New("visibility must be project or public")
	}
	return visibility, nil
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
