package controller

import (
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
