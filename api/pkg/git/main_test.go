package git

import (
	"os"
	"testing"

	giteagit "code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/setting"
)

func TestMain(m *testing.M) {
	setting.Git.HomePath = os.TempDir()
	if err := giteagit.InitSimple(); err != nil {
		panic("failed to initialize git module: " + err.Error())
	}
	os.Exit(m.Run())
}
