package controller

import (
	"path/filepath"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
)

func TestFilestoreScopedPathsRejectTraversal(t *testing.T) {
	cfg := &config.ServerConfig{}
	cfg.Controller.FilePrefixGlobal = "dev"
	c := &Controller{Options: Options{Config: cfg}}

	_, err := c.GetFilestoreUserPath(types.OwnerContext{Owner: "user-1"}, "../../users/user-2/secret.json")
	require.Error(t, err)

	_, err = c.GetFilestoreAppPath("app-1", "../../apps/app-2/secret.json")
	require.Error(t, err)
}

func TestFilestoreScopedPathsAcceptLogicalRootPaths(t *testing.T) {
	cfg := &config.ServerConfig{}
	cfg.Controller.FilePrefixGlobal = "dev"
	c := &Controller{Options: Options{Config: cfg}}

	userPath, err := c.GetFilestoreUserPath(types.OwnerContext{Owner: "user-1"}, "/documents/report.txt")
	require.NoError(t, err)
	require.Equal(t, "dev/users/user-1/documents/report.txt", filepath.ToSlash(userPath))

	sessionPath, err := c.GetFilestoreUserPath(types.OwnerContext{Owner: "user-1"}, "/sessions/ses_1/result.txt")
	require.NoError(t, err)
	require.Equal(t, "dev/users/user-1/sessions/ses_1/result.txt", filepath.ToSlash(sessionPath))

	appPath, err := c.GetFilestoreAppPath("app-1", "/knowledge/source.txt")
	require.NoError(t, err)
	require.Equal(t, "dev/apps/app-1/knowledge/source.txt", filepath.ToSlash(appPath))
}
