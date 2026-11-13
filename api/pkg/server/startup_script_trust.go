package server

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/types"
)

// trustStartupScriptWrite records the helix-specs commit a startup-script write
// produced as the repository's trusted startup commit (the one sandboxes run
// .helix/startup.sh from), when a person made the write. A write made with an
// agent-bound key leaves the trusted commit where it was.
func (apiServer *HelixAPIServer) trustStartupScriptWrite(ctx context.Context, user *types.User, repoID, commitSHA string) error {
	if commitSHA == "" || !user.IsHumanCredential() {
		return nil
	}
	if err := apiServer.Store.SetGitRepositoryTrustedStartupSHA(ctx, repoID, commitSHA); err != nil {
		return fmt.Errorf("record trusted startup commit: %w", err)
	}
	return nil
}

// initializeTrustedStartupScript writes a new project's initial startup script
// to helix-specs and trusts the commit when a person created the project.
func (apiServer *HelixAPIServer) initializeTrustedStartupScript(ctx context.Context, user *types.User, repoID, repoPath, projectName, script string) error {
	commitSHA, err := apiServer.projectInternalRepoService.InitializeStartupScriptInCodeRepo(repoPath, projectName, script, user.GitAuthorName(), user.GitAuthorEmail())
	if err != nil {
		return err
	}
	return apiServer.trustStartupScriptWrite(ctx, user, repoID, commitSHA)
}
