package services

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Initial files named as executable are committed as 100755, the rest 100644.
func TestInitializeGitRepositoryExecutableFiles(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "r.git")
	s := &GitRepositoryService{}
	err := s.initializeGitRepository(repo, "main", "r", map[string]string{
		"selftest":  "#!/bin/bash\necho ok\n",
		"README.md": "# r\n",
	}, []string{"selftest"}, true, "Test", "test@example.com")
	require.NoError(t, err)

	out, err := exec.Command("git", "--git-dir", repo, "ls-tree", "main").CombinedOutput()
	require.NoError(t, err, string(out))
	modes := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(line)
		modes[fields[3]] = fields[0]
	}
	require.Equal(t, "100755", modes["selftest"])
	require.Equal(t, "100644", modes["README.md"])
}
