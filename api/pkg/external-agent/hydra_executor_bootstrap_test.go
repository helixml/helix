package external_agent

import (
	"context"
	"slices"
	"strings"
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

func TestApplySessionBootstrapInstanceSkills(t *testing.T) {
	for _, tc := range []struct {
		name    string
		profile types.BotInstanceProfile
		want    []string
	}{
		{"default profile links no helix skills", types.DefaultBotInstanceProfile(), []string{"HELIX_WORKER_ID=b-broker", "HELIX_SKILLS=none"}},
		{"helix skills enabled", types.BotInstanceProfile{HelixSkills: true}, []string{"HELIX_WORKER_ID=b-broker"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			profile := tc.profile
			agent := &types.DesktopAgent{SessionID: "ses_instance"}
			err := applySessionBootstrap(types.SessionMetadata{
				OrgWorkerID:         "b-broker",
				RuntimeInstructions: "broker instructions",
				SessionRole:         types.SessionRoleOrgBotInstance,
				BotInstance:         &profile,
			}, agent)
			if err != nil {
				t.Fatalf("applySessionBootstrap: %v", err)
			}
			if len(agent.Env) != len(tc.want) {
				t.Fatalf("agent env = %v, want %v", agent.Env, tc.want)
			}
			for i := range tc.want {
				if agent.Env[i] != tc.want[i] {
					t.Fatalf("agent env = %v, want %v", agent.Env, tc.want)
				}
			}
			if !agent.NoContainerEngine {
				t.Fatal("an instance must run without a container engine")
			}
			if !agent.RestrictProjectSecrets || len(agent.ProjectSecretNames) != 0 {
				t.Fatalf("an instance with no grants must receive no project secrets: restrict=%t names=%v", agent.RestrictProjectSecrets, agent.ProjectSecretNames)
			}
			if agent.DiskSizeGB != types.DefaultBotInstanceDiskSizeGB || agent.PidsLimit != types.DefaultBotInstancePidsLimit {
				t.Fatalf("instance limits = disk %d GB / pids %d", agent.DiskSizeGB, agent.PidsLimit)
			}
			if !agent.NoNewPrivileges {
				t.Fatal("instance must default to no-new-privileges")
			}
		})
	}
}

func TestApplySessionBootstrapInstanceSecurityOptions(t *testing.T) {
	for _, tc := range []struct {
		name     string
		metadata types.SessionMetadata
		wantDisk int
		wantNNP  bool
	}{
		{"explicit headless options", types.SessionMetadata{SandboxRuntime: types.SandboxRuntimeHeadlessUbuntu, BotInstanceDiskSizeGB: 24, BotInstanceAllowSudo: true}, 24, false},
		{"headless default", types.SessionMetadata{SandboxRuntime: types.SandboxRuntimeHeadlessUbuntu}, types.DefaultBotInstanceDiskSizeGB, true},
		// GNOME startup configures devices with sudo.
		{"desktop always allows sudo", types.SessionMetadata{SandboxRuntime: types.SandboxRuntimeUbuntuDesktop}, types.DefaultBotInstanceDiskSizeGB, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			profile := types.DefaultBotInstanceProfile()
			metadata := tc.metadata
			metadata.OrgWorkerID = "b-broker"
			metadata.RuntimeInstructions = "instructions"
			metadata.BotInstance = &profile
			agent := &types.DesktopAgent{SessionID: "ses_instance"}
			if err := applySessionBootstrap(metadata, agent); err != nil {
				t.Fatal(err)
			}
			if agent.DiskSizeGB != tc.wantDisk || agent.NoNewPrivileges != tc.wantNNP {
				t.Fatalf("instance security options = disk %d GB / nnp %t, want %d / %t", agent.DiskSizeGB, agent.NoNewPrivileges, tc.wantDisk, tc.wantNNP)
			}
		})
	}
}

func TestApplySessionBootstrapCopiesInstanceSecretAllowlist(t *testing.T) {
	profile := types.DefaultBotInstanceProfile()
	requested := []string{"CRM_TOKEN", "SUPPORT_KEY"}
	agent := &types.DesktopAgent{SessionID: "ses_instance"}
	err := applySessionBootstrap(types.SessionMetadata{
		OrgWorkerID:         "b-broker",
		RuntimeInstructions: "broker instructions",
		BotInstance:         &profile,
		BotInstanceSecrets:  requested,
	}, agent)
	if err != nil {
		t.Fatalf("applySessionBootstrap: %v", err)
	}
	if len(agent.ProjectSecretNames) != len(requested) || agent.ProjectSecretNames[0] != requested[0] || agent.ProjectSecretNames[1] != requested[1] {
		t.Fatalf("ProjectSecretNames = %v, want %v", agent.ProjectSecretNames, requested)
	}
	requested[0] = "CHANGED"
	if agent.ProjectSecretNames[0] != "CRM_TOKEN" {
		t.Fatal("secret allowlist aliases session metadata")
	}
}

func TestInjectProjectSecrets(t *testing.T) {
	all := []string{"CRM_URL=one", "REGION=eu=west", "CRM=prefix-of-another-name"}
	for _, tc := range []struct {
		name       string
		agent      types.DesktopAgent
		wantEnv    []string
		wantCalled bool
		wantErr    string
	}{
		{"ordinary session gets every secret", types.DesktopAgent{ProjectID: "prj_1"}, all, true, ""},
		{"instance without grants skips the lookup", types.DesktopAgent{ProjectID: "prj_1", RestrictProjectSecrets: true}, nil, false, ""},
		{"instance gets only granted secrets", types.DesktopAgent{ProjectID: "prj_1", RestrictProjectSecrets: true, ProjectSecretNames: []string{"REGION", "CRM"}}, []string{"REGION=eu=west", "CRM=prefix-of-another-name"}, true, ""},
		{"missing grant fails the start", types.DesktopAgent{ProjectID: "prj_1", RestrictProjectSecrets: true, ProjectSecretNames: []string{"MISSING", "CRM_URL"}}, nil, true, "unavailable: MISSING"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			h := &HydraExecutor{getProjectSecrets: func(_ context.Context, projectID string) ([]string, error) {
				called = true
				return all, nil
			}}
			agent := tc.agent
			err := h.injectProjectSecrets(context.Background(), &agent)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if called != tc.wantCalled || !slices.Equal(agent.Env, tc.wantEnv) {
				t.Fatalf("called=%t env=%v, want called=%t env=%v", called, agent.Env, tc.wantCalled, tc.wantEnv)
			}
		})
	}
}

func TestApplySessionBootstrapKeepsContainerEngineForBotMainSession(t *testing.T) {
	agent := &types.DesktopAgent{SessionID: "ses_main"}
	err := applySessionBootstrap(types.SessionMetadata{OrgWorkerID: "b-broker", RuntimeInstructions: "broker instructions"}, agent)
	if err != nil {
		t.Fatalf("applySessionBootstrap: %v", err)
	}
	if agent.NoContainerEngine {
		t.Fatal("a bot's main session keeps its container engine")
	}
}
