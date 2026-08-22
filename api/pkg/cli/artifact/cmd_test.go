package artifact

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenArtifactContentArchivesDirectoryRoot(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, "assets"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "index.html"), []byte("<h1>Hello</h1>"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "assets", "app.js"), []byte("ok"), 0o600))

	content, cleanup, err := openArtifactContent(root)
	require.NoError(t, err)
	defer cleanup()
	archiveFile, ok := content.Reader.(*os.File)
	require.True(t, ok)
	info, err := archiveFile.Stat()
	require.NoError(t, err)
	archive, err := zip.NewReader(archiveFile, info.Size())
	require.NoError(t, err)
	require.Len(t, archive.File, 2)
	require.Equal(t, "assets/app.js", archive.File[0].Name)
	require.Equal(t, "index.html", archive.File[1].Name)
}

func TestNewClientUsesAgentEnvironment(t *testing.T) {
	t.Setenv("HELIX_URL", "")
	t.Setenv("HELIX_API_KEY", "")
	t.Setenv("HELIX_API_URL", "http://helix-api:8080")
	t.Setenv("USER_API_TOKEN", "agent-token")
	_, err := newClient()
	require.NoError(t, err)
}

func TestOpenArtifactContentRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "index.html")
	require.NoError(t, os.WriteFile(target, []byte("<h1>Hello</h1>"), 0o600))
	link := filepath.Join(root, "artifact.html")
	require.NoError(t, os.Symlink(target, link))

	_, _, err := openArtifactContent(link)
	require.ErrorContains(t, err, "must not be a symbolic link")
}

func TestResolveArtifactProvenanceUsesOnlyCurrentProject(t *testing.T) {
	t.Setenv("HELIX_PROJECT_ID", "prj_current")
	t.Setenv("HELIX_SESSION_ID", "ses_current")
	t.Setenv("HELIX_SPEC_TASK_ID", "spt_current")
	flags := uploadFlags{}
	cmd := newUpdateCommand()

	sessionID, taskID := resolveArtifactProvenance(cmd, &flags, "prj_current")
	require.Equal(t, "ses_current", sessionID)
	require.Equal(t, "spt_current", taskID)

	sessionID, taskID = resolveArtifactProvenance(cmd, &flags, "prj_other")
	require.Empty(t, sessionID)
	require.Empty(t, taskID)
}
