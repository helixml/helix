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

func TestApplySessionBootstrapAppliesOrgWorkerLaunchConfig(t *testing.T) {
	agent := &types.DesktopAgent{SessionID: "ses_worker"}
	err := applySessionBootstrap(types.SessionMetadata{
		OrgWorkerID:              "b-alex",
		RuntimeInstructions:      "worker instructions",
		SandboxRuntime:           types.SandboxRuntimeHeadlessUbuntu,
		SandboxResourceOverrides: &types.SandboxResourceOverrides{VCPUs: 4, MemoryMB: 8192},
	}, agent)
	if err != nil {
		t.Fatalf("applySessionBootstrap: %v", err)
	}
	if agent.OrgWorkerID != "b-alex" {
		t.Fatalf("OrgWorkerID = %q", agent.OrgWorkerID)
	}
	if agent.DesktopType != "headless" {
		t.Fatalf("DesktopType = %q, want headless", agent.DesktopType)
	}
	if agent.VCPUs != 4 || agent.MemoryMB != 8192 {
		t.Fatalf("resources = %d/%d, want 4/8192", agent.VCPUs, agent.MemoryMB)
	}
}

func TestApplySessionBootstrapDefaultsLegacyOrgWorkerToDesktopPreset(t *testing.T) {
	// Sessions created before the launch config existed carry no runtime or
	// size. They must keep booting as a desktop, now capped at the standard
	// preset (what they were already billed as) instead of uncapped.
	agent := &types.DesktopAgent{SessionID: "ses_worker"}
	err := applySessionBootstrap(types.SessionMetadata{
		OrgWorkerID:         "b-legacy",
		RuntimeInstructions: "worker instructions",
	}, agent)
	if err != nil {
		t.Fatalf("applySessionBootstrap: %v", err)
	}
	if agent.DesktopType != "" {
		t.Fatalf("DesktopType = %q, want desktop default", agent.DesktopType)
	}
	std := types.EffectiveSpecTaskSandboxResources(nil)
	if agent.VCPUs != std.VCPUs || agent.MemoryMB != std.MemoryMB {
		t.Fatalf("resources = %d/%d, want standard preset %d/%d", agent.VCPUs, agent.MemoryMB, std.VCPUs, std.MemoryMB)
	}
}
