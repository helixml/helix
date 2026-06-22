package webservice

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"go.uber.org/mock/gomock"
)

func TestShellEscape(t *testing.T) {
	cases := map[string]string{
		"":                                "''",
		"simple":                          "'simple'",
		"with spaces":                     "'with spaces'",
		"path/to/file.txt":                "'path/to/file.txt'",
		"abc'def":                         `'abc'\''def'`,
		"; rm -rf / # injection attempt":  "'; rm -rf / # injection attempt'",
		"$(cat /etc/passwd)":              "'$(cat /etc/passwd)'",
		"`hostname`":                      "'`hostname`'",
		"https://user:p@ss@github.com/o/r": "'https://user:p@ss@github.com/o/r'",
	}
	for in, want := range cases {
		if got := shellEscape(in); got != want {
			t.Errorf("shellEscape(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestProjectSecretEnv(t *testing.T) {
	ctx := context.Background()

	// No getter wired: returns empty, never nil-panics.
	c := &Controller{}
	if got := c.projectSecretEnv(ctx, "prj_1", "sbx_1"); len(got) != 0 {
		t.Errorf("expected empty env with no getter, got %v", got)
	}

	// Empty project ID: getter must not be called.
	called := false
	c.SetProjectSecretsGetter(func(context.Context, string) ([]string, error) {
		called = true
		return []string{"X=1"}, nil
	})
	if got := c.projectSecretEnv(ctx, "", "sbx_1"); len(got) != 0 || called {
		t.Errorf("expected no getter call for empty project, got %v called=%v", got, called)
	}

	// Getter returns secrets: they are passed through verbatim.
	c.SetProjectSecretsGetter(func(_ context.Context, projectID string) ([]string, error) {
		if projectID != "prj_1" {
			t.Errorf("unexpected project id %q", projectID)
		}
		return []string{"API_KEY=prod", "DB_URL=prod"}, nil
	})
	got := c.projectSecretEnv(ctx, "prj_1", "sbx_1")
	if len(got) != 2 || got[0] != "API_KEY=prod" || got[1] != "DB_URL=prod" {
		t.Errorf("expected prod secrets passed through, got %v", got)
	}

	// Getter error: deploy is not blocked, env is empty.
	c.SetProjectSecretsGetter(func(context.Context, string) ([]string, error) {
		return nil, errors.New("boom")
	})
	if got := c.projectSecretEnv(ctx, "prj_1", "sbx_1"); len(got) != 0 {
		t.Errorf("expected empty env on getter error, got %v", got)
	}
}

// TestDeployScriptStopsBeforeStart is the single-writer guarantee: the deploy
// script must kill the previous app instance BEFORE launching the new one, so
// the durable /data dir is never opened by two processes at once.
func TestDeployScriptStopsBeforeStart(t *testing.T) {
	script := deployScript("https://github.com/o/r.git", "abc123", 3000)

	killIdx := strings.Index(script, "kill -TERM")
	launchIdx := strings.Index(script, "startup.sh")
	if killIdx < 0 {
		t.Fatalf("deploy script never stops the previous app: %s", script)
	}
	if launchIdx < 0 {
		t.Fatalf("deploy script never launches startup.sh: %s", script)
	}
	if killIdx > launchIdx {
		t.Errorf("deploy script launches the app before stopping the old one (kill@%d > launch@%d) — two writers on /data", killIdx, launchIdx)
	}
}

// TestDeployScriptExportsDataDirAndPort — the app receives the durable data
// dir and its port via env so it knows where to persist state.
func TestDeployScriptExportsDataDirAndPort(t *testing.T) {
	script := deployScript("https://github.com/o/r.git", "", 8080)
	for _, want := range []string{
		"HELIX_WEB_SERVICE_DATA_DIR=/data",
		"HELIX_WEB_SERVICE_PORT=8080",
		"/data/.helix-webservice.pid",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("deploy script missing %q:\n%s", want, script)
		}
	}
	// No SHA → no checkout line.
	if strings.Contains(script, "git checkout") {
		t.Errorf("empty SHA should not produce a checkout:\n%s", script)
	}
}

// TestDeployScriptChecksOutSHA — when a SHA is given it is checked out.
func TestDeployScriptChecksOutSHA(t *testing.T) {
	script := deployScript("https://github.com/o/r.git", "deadbeef", 8080)
	if !strings.Contains(script, "git checkout 'deadbeef'") {
		t.Errorf("expected checkout of the requested SHA:\n%s", script)
	}
}

// TestLastLiveSHA picks the most recent live/superseded deploy with a SHA,
// skipping pending/failed rows — that is the commit we roll back to.
func TestLastLiveSHA(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	st := store.NewMockStore(ctrl)
	c := &Controller{store: st}

	st.EXPECT().ListWebServiceDeploys(gomock.Any(), "prj_1", gomock.Any()).Return([]*types.WebServiceDeploy{
		{Status: types.WebServiceDeployStatusFailed, CommitSHA: "bad"},
		{Status: types.WebServiceDeployStatusSuperseded, CommitSHA: "good"},
		{Status: types.WebServiceDeployStatusLive, CommitSHA: "older"},
	}, nil)

	if got := c.lastLiveSHA(context.Background(), "prj_1"); got != "good" {
		t.Errorf("lastLiveSHA = %q, want %q", got, "good")
	}
}

// TestLastLiveSHANone — no prior good deploy means nothing to roll back to.
func TestLastLiveSHANone(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	st := store.NewMockStore(ctrl)
	c := &Controller{store: st}

	st.EXPECT().ListWebServiceDeploys(gomock.Any(), "prj_1", gomock.Any()).Return([]*types.WebServiceDeploy{
		{Status: types.WebServiceDeployStatusFailed, CommitSHA: "bad"},
	}, nil)

	if got := c.lastLiveSHA(context.Background(), "prj_1"); got != "" {
		t.Errorf("lastLiveSHA = %q, want empty", got)
	}
}
