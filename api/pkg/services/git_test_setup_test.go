package services

import (
	"os"
	"testing"

	giteagit "code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/setting"
)

// TestMain initializes the gitea git module before running tests
func TestMain(m *testing.M) {
	// Set Git home path to temp directory for tests
	setting.Git.HomePath = os.TempDir()

	// Tests use local bare repositories as upstreams; production restricts
	// upstream transports to http(s) unless this variable is set.
	if err := os.Setenv("GIT_ALLOW_PROTOCOL", "file:http:https"); err != nil {
		panic(err)
	}

	// Initialize gitea git module
	if err := giteagit.InitSimple(); err != nil {
		panic("failed to initialize git module: " + err.Error())
	}
	os.Exit(m.Run())
}
