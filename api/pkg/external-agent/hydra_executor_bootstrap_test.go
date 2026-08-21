package external_agent

import (
	"testing"

	"github.com/helixml/helix/api/pkg/types"
)

func TestApplySessionBootstrapScopesInstructionsToOrgWorker(t *testing.T) {
	agent := &types.DesktopAgent{SessionID: "ses_worker"}
	err := applySessionBootstrap(types.SessionMetadata{
		OrgWorkerID:         "b-alex",
		RuntimeInstructions: "worker instructions",
	}, agent)
	if err != nil {
		t.Fatalf("applySessionBootstrap: %v", err)
	}
	if len(agent.Env) != 1 || agent.Env[0] != "HELIX_WORKER_ID=b-alex" {
		t.Fatalf("agent env = %v", agent.Env)
	}
	for _, name := range []string{"AGENTS.md", "CLAUDE.md"} {
		if got := string(agent.WorkspaceFiles[name]); got != "worker instructions" {
			t.Errorf("%s = %q", name, got)
		}
	}
}

func TestApplySessionBootstrapLeavesSpecTaskUnchanged(t *testing.T) {
	agent := &types.DesktopAgent{SessionID: "ses_spec", SpecTaskID: "spt_test"}
	if err := applySessionBootstrap(types.SessionMetadata{SpecTaskID: "spt_test"}, agent); err != nil {
		t.Fatalf("applySessionBootstrap: %v", err)
	}
	if len(agent.Env) != 0 || len(agent.WorkspaceFiles) != 0 {
		t.Fatalf("spec task inherited worker bootstrap: env=%v files=%v", agent.Env, agent.WorkspaceFiles)
	}
}

func TestAppendProjectSecretsDropsLegacyWorkerIdentity(t *testing.T) {
	got := appendProjectSecrets([]string{"BASE=1"}, []string{
		"HELIX_ORG_URL=http://helix",
		"HELIX_WORKER_ID=b-alex",
		"TOKEN=value",
	})
	want := []string{"BASE=1", "HELIX_ORG_URL=http://helix", "TOKEN=value"}
	if len(got) != len(want) {
		t.Fatalf("env = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("env = %v, want %v", got, want)
		}
	}
}

func TestOrgWorkerDesktopSkipsProjectSecretInjection(t *testing.T) {
	if !hasOrgWorkerIdentity([]string{"BASE=1", "HELIX_WORKER_ID=w-1"}) {
		t.Fatal("org Worker identity was not detected")
	}
	if hasOrgWorkerIdentity([]string{"BASE=1"}) {
		t.Fatal("ordinary project desktop detected as org Worker")
	}
}

func TestApplySessionBootstrapRejectsPartialState(t *testing.T) {
	for _, metadata := range []types.SessionMetadata{
		{OrgWorkerID: "b-alex"},
		{RuntimeInstructions: "instructions"},
	} {
		if err := applySessionBootstrap(metadata, &types.DesktopAgent{SessionID: "ses_bad"}); err == nil {
			t.Fatalf("applySessionBootstrap accepted partial state: %+v", metadata)
		}
	}
}
