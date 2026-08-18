package helix

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
)

// codingApp is an agent App configured the way the settings UI writes one:
// a zed_external assistant carrying the harness, provider and model the Bot
// actually runs on.
func codingApp(runtime types.CodeAgentRuntime, provider, model string) types.AppConfig {
	return types.AppConfig{
		Helix: types.AppHelixConfig{
			Assistants: []types.AssistantConfig{{
				Name:                    "main",
				AgentType:               types.AgentTypeZedExternal,
				CodeAgentRuntime:        runtime,
				CodeAgentCredentialType: types.CodeAgentCredentialTypeAPIKey,
				Provider:                provider,
				Model:                   model,
			}},
		},
	}
}

// The bug this fixes: a Worker project's task defaults were written once at
// provisioning and never refreshed, so a Bot switched to opencode kept filing
// tasks on whatever harness its project was born with. project.CodeAgentConfig
// is what a task inherits when created without an explicit one, so the two must
// converge.
func TestEnsureSyncsProjectTaskDefaultsFromBotApp(t *testing.T) {
	st, wid := newProjectTestStore(t, "# Role")
	ctx := context.Background()
	bot, err := st.Nodes.Get(ctx, "org-test", wid)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Nodes.Update(ctx, bot.WithAgentID("app-eng")); err != nil {
		t.Fatal(err)
	}
	if err := SaveProject(ctx, st, "org-test", wid, "prj_existing", "app-eng", "repo_existing"); err != nil {
		t.Fatal(err)
	}

	svc := newFakeProjectService()
	svc.appConfig = codingApp(types.CodeAgentRuntimeOpenCode, "pe_test", "qwen3.8-27b")
	// The project is stale: provisioned onto a different harness entirely.
	svc.getProjectResp = types.Project{
		ID: "prj_existing", OrganizationID: "org-test",
		DefaultHelixAppID: "app-eng", DefaultRepoID: "repo_existing",
		CodeAgentConfig: &types.CodeAgentExecutionConfig{
			Runtime:        types.CodeAgentRuntimeDeepSeekHarness,
			CredentialType: types.CodeAgentCredentialTypeAPIKey,
			ProviderRef:    "pe_test",
			Model:          "qwen3.8-27b",
		},
	}

	if _, _, _, err := newApplierGit(svc, newFakeGitForProject(), st).Ensure(ctx, "org-test", wid); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	got := svc.getProjectResp.CodeAgentConfig
	if got == nil {
		t.Fatal("project lost its code-agent config")
	}
	if got.Runtime != types.CodeAgentRuntimeOpenCode {
		t.Fatalf("project runtime = %q, want opencode (the Bot's own harness)", got.Runtime)
	}
	if got.Model != "qwen3.8-27b" || got.ProviderRef != "pe_test" {
		t.Fatalf("project model/provider = %q/%q, want qwen3.8-27b/pe_test", got.Model, got.ProviderRef)
	}
}

// Already in sync: no write. A sync that patches unconditionally would churn
// the project row on every activation and make the audit trail useless.
func TestEnsureDoesNotRewriteMatchingProjectTaskDefaults(t *testing.T) {
	st, wid := newProjectTestStore(t, "# Role")
	ctx := context.Background()
	bot, err := st.Nodes.Get(ctx, "org-test", wid)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Nodes.Update(ctx, bot.WithAgentID("app-eng")); err != nil {
		t.Fatal(err)
	}
	if err := SaveProject(ctx, st, "org-test", wid, "prj_existing", "app-eng", "repo_existing"); err != nil {
		t.Fatal(err)
	}

	svc := newFakeProjectService()
	svc.appConfig = codingApp(types.CodeAgentRuntimeOpenCode, "pe_test", "qwen3.8-27b")
	svc.getProjectResp = types.Project{
		ID: "prj_existing", OrganizationID: "org-test",
		DefaultHelixAppID: "app-eng", DefaultRepoID: "repo_existing",
		Metadata: types.ProjectMetadata{OrgMembersAccess: true},
		CodeAgentConfig: &types.CodeAgentExecutionConfig{
			Runtime:        types.CodeAgentRuntimeOpenCode,
			CredentialType: types.CodeAgentCredentialTypeAPIKey,
			ProviderRef:    "pe_test",
			Model:          "qwen3.8-27b",
		},
	}

	if _, _, _, err := newApplierGit(svc, newFakeGitForProject(), st).Ensure(ctx, "org-test", wid); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if svc.codeAgentConfigPatches != 0 {
		t.Fatalf("code-agent config patched %d times for an already-matching config; want 0", svc.codeAgentConfigPatches)
	}
}

// A Bot with no linked agent app has no configuration to sync from. Activation
// must still succeed and leave the project untouched rather than clearing it.
func TestEnsureLeavesProjectTaskDefaultsWhenBotHasNoApp(t *testing.T) {
	st, wid := newProjectTestStore(t, "# Role")
	ctx := context.Background()
	if err := SaveProject(ctx, st, "org-test", wid, "prj_existing", "", "repo_existing"); err != nil {
		t.Fatal(err)
	}

	existing := &types.CodeAgentExecutionConfig{
		Runtime:        types.CodeAgentRuntimeDeepSeekHarness,
		CredentialType: types.CodeAgentCredentialTypeAPIKey,
		ProviderRef:    "pe_test",
		Model:          "qwen3.8-27b",
	}
	svc := newFakeProjectService()
	svc.getProjectResp = types.Project{
		ID: "prj_existing", OrganizationID: "org-test",
		DefaultRepoID: "repo_existing",
		Metadata:      types.ProjectMetadata{OrgMembersAccess: true},
		// Not the Bot's own config, but it is the only one there is.
		CodeAgentConfig: existing,
	}

	if _, _, _, err := newApplierGit(svc, newFakeGitForProject(), st).Ensure(ctx, "org-test", wid); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if svc.codeAgentConfigPatches != 0 || svc.getProjectResp.CodeAgentConfig != existing {
		t.Fatal("project task defaults were modified for a Bot with no linked app")
	}
}
