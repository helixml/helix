package webservice

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/hydra"
	"github.com/helixml/helix/api/pkg/sandbox"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"go.uber.org/mock/gomock"
)

func TestShellEscape(t *testing.T) {
	cases := map[string]string{
		"":                                 "''",
		"simple":                           "'simple'",
		"with spaces":                      "'with spaces'",
		"path/to/file.txt":                 "'path/to/file.txt'",
		"abc'def":                          `'abc'\''def'`,
		"; rm -rf / # injection attempt":   "'; rm -rf / # injection attempt'",
		"$(cat /etc/passwd)":               "'$(cat /etc/passwd)'",
		"`hostname`":                       "'`hostname`'",
		"https://user:p@ss@github.com/o/r": "'https://user:p@ss@github.com/o/r'",
	}
	for in, want := range cases {
		if got := shellEscape(in); got != want {
			t.Errorf("shellEscape(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDeployCloneURL(t *testing.T) {
	tests := []struct {
		name string
		repo *types.GitRepository
		want string
	}{
		{
			name: "hosted repository uses sandbox API proxy",
			repo: &types.GitRepository{ID: "repo_123", CloneURL: "https://helix.example/git/repo_123"},
			want: hydra.SandboxAPIProxyURL + "/git/repo_123",
		},
		{
			name: "external mirror uses sandbox API proxy",
			repo: &types.GitRepository{
				ID:          "repo_456",
				IsExternal:  true,
				ExternalURL: "https://github.com/helixml/helix.git",
				CloneURL:    "https://helix.example/git/repo_456",
			},
			want: hydra.SandboxAPIProxyURL + "/git/repo_456",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deployCloneURL(tt.repo); got != tt.want {
				t.Fatalf("deployCloneURL() = %q, want %q", got, tt.want)
			}
		})
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

// TestDeployScriptPreparesBeforeStop keeps the current app serving if Git or
// startup-script validation fails, while preserving the single-writer rule.
func TestDeployScriptPreparesBeforeStop(t *testing.T) {
	script := deployScript("https://github.com/o/r.git", "abc123", "main", "myrepo", 3000)

	prepareIdx := strings.Index(script, `if [ ! -f "$SPECS/.helix/startup.sh" ]`)
	ownershipIdx := strings.Index(script, `chown -hR retro:retro "$CODE" "$SPECS"`)
	killIdx := strings.Index(script, "kill -TERM")
	launchIdx := strings.Index(script, `setsid "${STARTUP_USER[@]}"`)
	if prepareIdx < 0 {
		t.Fatalf("deploy script never validates the replacement startup script: %s", script)
	}
	if killIdx < 0 {
		t.Fatalf("deploy script never stops the previous app: %s", script)
	}
	if launchIdx < 0 {
		t.Fatalf("deploy script never launches startup.sh: %s", script)
	}
	if prepareIdx > killIdx {
		t.Errorf("deploy script stops the old app before preparing the replacement (prepare@%d > kill@%d)", prepareIdx, killIdx)
	}
	if ownershipIdx < 0 || ownershipIdx > killIdx {
		t.Errorf("deploy script stops the old app before preparing rootless ownership (ownership@%d > kill@%d)", ownershipIdx, killIdx)
	}
	if killIdx > launchIdx {
		t.Errorf("deploy script launches the app before stopping the old one (kill@%d > launch@%d) — two writers on /data", killIdx, launchIdx)
	}
}

func TestDeployScriptTrustsOnlyItsCheckout(t *testing.T) {
	script := deployScript("https://github.com/o/r.git", "", "main", "myrepo", 8080)
	if !strings.Contains(script, `git -c safe.directory="$CODE" -C "$CODE" fetch --all`) {
		t.Fatalf("Git commands do not trust the known checkout:\n%s", script)
	}
	if strings.Contains(script, `git -C "$CODE"`) || strings.Contains(script, "safe.directory=*") || strings.Contains(script, "config --global") {
		t.Fatalf("deploy script grants broader Git trust than its checkout:\n%s", script)
	}
}

func TestDeployScriptGitFailureLeavesCurrentAppRunning(t *testing.T) {
	tmp := t.TempDir()
	dataDir := filepath.Join(tmp, "data")
	codeDir := filepath.Join(tmp, "workspace", "myrepo")
	binDir := filepath.Join(tmp, "bin")
	for _, dir := range []string{dataDir, filepath.Join(codeDir, ".git"), binDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	sentinel := exec.Command("sleep", "30")
	if err := sentinel.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = sentinel.Process.Kill()
		_, _ = sentinel.Process.Wait()
	})
	if err := os.WriteFile(filepath.Join(dataDir, ".helix-webservice.pid"), []byte(strconv.Itoa(sentinel.Process.Pid)), 0o644); err != nil {
		t.Fatal(err)
	}

	gitArgs := filepath.Join(tmp, "git-args")
	fakeGit := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$GIT_ARGS\"\nexit 42\n"
	if err := os.WriteFile(filepath.Join(binDir, "git"), []byte(fakeGit), 0o755); err != nil {
		t.Fatal(err)
	}

	script := deployScript("https://github.com/o/r.git", "", "main", "myrepo", 8080)
	script = strings.ReplaceAll(script, "/workspace", filepath.Join(tmp, "workspace"))
	script = strings.ReplaceAll(script, "/data", dataDir)
	cmd := exec.Command("/bin/bash", "-c", script)
	cmd.Env = append(os.Environ(), "PATH="+binDir+":"+os.Getenv("PATH"), "GIT_ARGS="+gitArgs)
	if err := cmd.Run(); err == nil {
		t.Fatal("deploy unexpectedly succeeded with failing Git")
	}
	if err := sentinel.Process.Signal(syscall.Signal(0)); err != nil {
		t.Fatalf("current app was stopped before Git preparation failed: %v", err)
	}
	args, err := os.ReadFile(gitArgs)
	if err != nil {
		t.Fatal(err)
	}
	want := "-c\nsafe.directory=" + codeDir + "\n-C\n" + codeDir + "\n"
	if !strings.HasPrefix(string(args), want) {
		t.Fatalf("Git did not receive trust for the exact checkout; args:\n%s", args)
	}
}

// TestDeployScriptExportsDataDirAndPort — the app receives the durable data
// dir and its port via env so it knows where to persist state.
func TestDeployScriptExportsDataDirAndPort(t *testing.T) {
	script := deployScript("https://github.com/o/r.git", "", "main", "myrepo", 8080)
	for _, want := range []string{
		"HELIX_WEB_SERVICE_DATA_DIR=/data",
		"HELIX_WEB_SERVICE_PORT=8080",
		"/data/.helix-webservice.pid",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("deploy script missing %q:\n%s", want, script)
		}
	}
	// The startup script is sourced from the helix-specs branch, checked out as
	// a SIBLING worktree of the code (not overlaid into it).
	if !strings.Contains(script, `worktree add --force --detach "$SPECS" FETCH_HEAD`) {
		t.Errorf("helix-specs must be checked out as a sibling worktree:\n%s", script)
	}
}

func TestDeployScriptRunsRootlessStartupAsRetro(t *testing.T) {
	script := deployScript("https://github.com/o/r.git", "", "main", "myrepo", 8080)
	for _, want := range []string{
		`STARTUP_USER=()`,
		`[ "${HELIX_ROOTLESS_CONTAINER_ENGINE:-}" = "1" ]`,
		`chown -hR retro:retro "$CODE" "$SPECS"`,
		`STARTUP_USER=(gosu retro env HOME=/home/retro USER=retro LOGNAME=retro XDG_RUNTIME_DIR=/run/user/1000)`,
		`setsid "${STARTUP_USER[@]}" env HELIX_WEB_SERVICE_PORT=8080 HELIX_WEB_SERVICE_DATA_DIR=/data bash "$SPECS/.helix/startup.sh" >"$LOGFILE" 2>&1 &`,
	} {
		if !strings.Contains(script, want) {
			t.Errorf("deploy script missing %q:\n%s", want, script)
		}
	}
}

// TestDeployScriptChecksOutSHA — when a SHA is given it is checked out (in the
// code worktree).
func TestDeployScriptChecksOutSHA(t *testing.T) {
	script := deployScript("https://github.com/o/r.git", "deadbeef", "main", "myrepo", 8080)
	if !strings.Contains(script, "checkout --detach 'deadbeef'") {
		t.Errorf("expected checkout of the requested SHA:\n%s", script)
	}
}

// TestDeployScriptMirrorsSpecTaskLayout encodes the core guarantee: the deploy
// reproduces the spec-task workspace layout so a startup script behaves
// identically in both contexts. The app code is cloned into its own dir and
// helix-specs is a SIBLING worktree (never a subdir of the code); startup.sh is
// invoked with CWD = the code dir and $0 = the helix-specs worktree.
func TestDeployScriptMirrorsSpecTaskLayout(t *testing.T) {
	script := deployScript("https://github.com/o/r.git", "", "main", "myrepo", 8080)
	for _, want := range []string{
		`CODE='/workspace/myrepo'`,
		`SPECS=/workspace/helix-specs`,
		`git -c safe.directory="$CODE" -C "$CODE" worktree add --force --detach "$SPECS" FETCH_HEAD`,
		`cd "$CODE"`,
		`bash "$SPECS/.helix/startup.sh"`,
	} {
		if !strings.Contains(script, want) {
			t.Errorf("deploy script missing %q (spec-task layout parity):\n%s", want, script)
		}
	}
	// helix-specs must be a SIBLING, not nested under the code dir — otherwise
	// `dirname "$0"/..` resolves to the code dir in web-service but the
	// helix-specs worktree in spec-task, re-introducing the asymmetry.
	if strings.Contains(script, "/workspace/myrepo/helix-specs") {
		t.Errorf("helix-specs must not be nested under the code dir:\n%s", script)
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

func TestEnsureSandboxPinsReplacementToRecordedHost(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	runtimes, err := sandbox.NewRuntimeRegistry(config.Sandboxes{
		Runtimes:       "headless-ubuntu=ubuntu:22.04|sleep infinity",
		DefaultRuntime: "headless-ubuntu",
	})
	if err != nil {
		t.Fatal(err)
	}
	c := New(st, sandbox.New(st, nil, runtimes, "", ""))

	st.EXPECT().GetSandbox(gomock.Any(), "sbx_gone").Return(nil, store.ErrNotFound)
	st.EXPECT().GetSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil)
	st.EXPECT().ListSandboxes(gomock.Any(), gomock.Any()).Return(nil, nil)
	st.EXPECT().ListSandboxInstances(gomock.Any()).Return([]*types.SandboxInstance{{ID: "host-pinned", Status: "online"}}, nil)
	st.EXPECT().CreateSandbox(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, sb *types.Sandbox) (*types.Sandbox, error) {
			if sb.HostDeviceID != "host-pinned" {
				t.Fatalf("replacement host = %q, want host-pinned", sb.HostDeviceID)
			}
			return nil, errors.New("stop after placement assertion")
		},
	)

	_, err = c.ensureSandbox(context.Background(), DeployRequest{Owner: "usr_1"}, &types.Project{
		ID: "prj_1", OrganizationID: "org_1",
	}, &types.ProjectWebServiceState{
		ProjectID: "prj_1", ActiveSandboxID: "sbx_gone", HostDeviceID: "host-pinned",
	})
	if err == nil || !strings.Contains(err.Error(), "stop after placement assertion") {
		t.Fatalf("ensureSandbox error = %v", err)
	}
}

func TestEnsureSandboxRepairsStaleRunnerPin(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	runtimes, err := sandbox.NewRuntimeRegistry(config.Sandboxes{
		Runtimes:       "headless-ubuntu=ubuntu:22.04|sleep infinity",
		DefaultRuntime: "headless-ubuntu",
	})
	if err != nil {
		t.Fatal(err)
	}
	c := New(st, sandbox.New(st, nil, runtimes, "", ""))
	state := &types.ProjectWebServiceState{
		ProjectID: "prj_1", ActiveSandboxID: "sbx_live", HostDeviceID: "host-stale",
	}
	st.EXPECT().GetSandbox(gomock.Any(), "sbx_live").Return(&types.Sandbox{
		ID: "sbx_live", HostDeviceID: "host-current", Status: types.SandboxStatusRunning,
	}, nil)
	st.EXPECT().SetWebServiceHostDeviceID(gomock.Any(), "prj_1", "host-current").Return(nil)

	sb, err := c.ensureSandbox(context.Background(), DeployRequest{Owner: "usr_1"}, &types.Project{
		ID: "prj_1", OrganizationID: "org_1",
	}, state)
	if err != nil {
		t.Fatal(err)
	}
	if sb.ID != "sbx_live" || state.HostDeviceID != "host-current" {
		t.Fatalf("sandbox=%q state host=%q", sb.ID, state.HostDeviceID)
	}
}

func TestWaitForReadyProbeStopsWhenSandboxIsMissing(t *testing.T) {
	c := &Controller{readinessWait: time.Minute, readinessPoll: time.Millisecond}
	calls := 0

	err := c.waitForReadyProbe(context.Background(), "sbx_1", 8080, func(context.Context, string, int) error {
		calls++
		return hydra.ErrDevContainerNotFound
	})

	if !errors.Is(err, hydra.ErrDevContainerNotFound) {
		t.Fatalf("expected missing sandbox error, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected one probe, got %d", calls)
	}
}

func TestWaitForReadyProbeRetriesTransientFailure(t *testing.T) {
	c := &Controller{readinessWait: time.Second, readinessPoll: time.Millisecond}
	calls := 0

	err := c.waitForReadyProbe(context.Background(), "sbx_1", 8080, func(context.Context, string, int) error {
		calls++
		if calls == 1 {
			return errors.New("revdial reconnecting")
		}
		return nil
	})

	if err != nil {
		t.Fatalf("expected transient failure to recover, got %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected two probes, got %d", calls)
	}
}

func TestWaitForReadyProbeStopsWhenDeployIsSuperseded(t *testing.T) {
	c := &Controller{readinessWait: time.Minute, readinessPoll: time.Millisecond}
	calls := 0

	err := c.waitForReadyProbe(context.Background(), "sbx_1", 8080, func(context.Context, string, int) error {
		calls++
		return errDeploySuperseded
	})

	if !errors.Is(err, errDeploySuperseded) {
		t.Fatalf("expected superseded error, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected one probe, got %d", calls)
	}
}

func TestMarkFailedIfCurrentDoesNotOverwriteSupersededDeploy(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	c := &Controller{store: st}

	st.EXPECT().ListWebServiceDeploys(gomock.Any(), "prj_1", 1).Return([]*types.WebServiceDeploy{{ID: "wsd_new"}}, nil)

	c.markFailedIfCurrent(context.Background(), "prj_1", "wsd_old", "sbx_1", "old failure")
}

func TestMarkLiveIfCurrentDoesNotOverwriteNewerDeploy(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	c := &Controller{store: st}

	st.EXPECT().ListWebServiceDeploys(gomock.Any(), "prj_1", 1).Return([]*types.WebServiceDeploy{{ID: "wsd_new"}}, nil)

	c.markLiveIfCurrent(context.Background(), "prj_1", "wsd_old", "sbx_old")
}

// TestDeployScriptWithoutSHADeploysBranchTip — a redeploy with no SHA (the
// Deploy button, default-branch push auto-deploy) must check out the fetched
// tip. Fetching alone leaves the first clone's commit in the worktree, so the
// "redeploy" rebuilt old code.
func TestDeployScriptWithoutSHADeploysBranchTip(t *testing.T) {
	script := deployScript("https://github.com/o/r.git", "", "main", "myrepo", 8080)
	fetch := strings.Index(script, `git -c safe.directory="$CODE" -C "$CODE" fetch --all`)
	checkout := strings.Index(script, "checkout --detach 'origin/main'")
	if fetch < 0 || checkout < fetch {
		t.Fatalf("expected the fetched origin/main to be checked out after the fetch:\n%s", script)
	}
	if !strings.Contains(deployScript("u", "", "", "myrepo", 8080), "checkout --detach 'origin/HEAD'") {
		t.Error("no default branch recorded: expected origin/HEAD")
	}
	if !strings.Contains(script, deployedSHAMarker) {
		t.Error("the script must report the commit it checked out")
	}
}

func TestDeployedSHAFromOutput(t *testing.T) {
	out := "Cloning...\n" + deployedSHAMarker + "70e2a08abc\n"
	if got := deployedSHAFromOutput(out); got != "70e2a08abc" {
		t.Fatalf("got %q", got)
	}
	if got := deployedSHAFromOutput("no marker"); got != "" {
		t.Fatalf("got %q", got)
	}
}
