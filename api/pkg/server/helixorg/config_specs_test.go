package helixorg

import (
	"context"
	"strings"
	"testing"

	helixorgconfig "github.com/helixml/helix/api/pkg/org/application/configregistry"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
)

// TestRegisterHelixOrgConfigSpecs_RedactsTransportGitHubSecrets pins
// down the spec registration for `transport.github`: both `token` and
// `webhook_secret` MUST be redacted on `config get`. Without this, a
// future refactor that drops one of the entries from the Secrets list
// would silently start leaking the secret to anyone with shell access
// who reads the configs table (logs, screenshares, terminal
// recordings, etc.).
//
// Ported from helix-org/cmd/helix-org/configspecs_test.go in H7 when
// the standalone CLI was deleted. The redaction invariant the
// original test pinned is preserved here against the embedded path's
// RegisterConfigSpecs.
func TestRegisterHelixOrgConfigSpecs_RedactsTransportGitHubSecrets(t *testing.T) {
	t.Parallel()

	st := orggorm.GetOrgTestDB(t)
	reg := helixorgconfig.New(st.Configs)
	RegisterConfigSpecs(reg)

	const raw = `{"token":"plaintext-token-leaked","webhook_secret":"plaintext-secret-leaked"}`
	if err := reg.Set(context.Background(), "org-test", "transport.github", raw); err != nil {
		t.Fatalf("set: %v", err)
	}

	got, err := reg.GetRedacted(context.Background(), "org-test", "transport.github")
	if err != nil {
		t.Fatalf("GetRedacted: %v", err)
	}
	if strings.Contains(got, "plaintext-token-leaked") {
		t.Fatalf("redacted output leaks token: %s", got)
	}
	if strings.Contains(got, "plaintext-secret-leaked") {
		t.Fatalf("redacted output leaks webhook_secret: %s", got)
	}

	// Spec must declare both field names. Asserting on the spec itself
	// catches regressions earlier than the redaction behaviour test
	// (which depends on the registry's redaction logic continuing to
	// do what it does today).
	spec, ok := reg.Spec("transport.github")
	if !ok {
		t.Fatalf("transport.github not registered")
	}
	if !stringSliceContains(spec.Secrets, "token") {
		t.Fatalf("spec.Secrets = %v, missing \"token\"", spec.Secrets)
	}
	if !stringSliceContains(spec.Secrets, "webhook_secret") {
		t.Fatalf("spec.Secrets = %v, missing \"webhook_secret\" — secret will leak via config get", spec.Secrets)
	}
}

// TestRegisterHelixOrgConfigSpecs_RedactsPostmarkToken is the
// analogous regression guard for the email transport. Cheap to keep
// alongside the github one; same failure mode (drop the entry, leak
// the secret).
func TestRegisterHelixOrgConfigSpecs_RedactsPostmarkToken(t *testing.T) {
	t.Parallel()

	st := orggorm.GetOrgTestDB(t)
	reg := helixorgconfig.New(st.Configs)
	RegisterConfigSpecs(reg)

	spec, ok := reg.Spec("transport.postmark")
	if !ok {
		t.Fatalf("transport.postmark not registered")
	}
	if !stringSliceContains(spec.Secrets, "token") {
		t.Fatalf("spec.Secrets = %v, missing \"token\"", spec.Secrets)
	}
}

func TestDefaultAgentConfigPrefersAtomicSettingAndReadsLegacy(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	reg := helixorgconfig.New(orggorm.GetOrgTestDB(t).Configs)
	RegisterConfigSpecs(reg)

	if err := reg.Set(ctx, "org-test", "worker.runtime", `"zed_agent"`); err != nil {
		t.Fatalf("set legacy runtime: %v", err)
	}
	if err := reg.Set(ctx, "org-test", "worker.provider", `"anthropic"`); err != nil {
		t.Fatalf("set legacy provider: %v", err)
	}
	legacy, err := reg.GetDefaultAgentConfig(ctx, "org-test")
	if err != nil {
		t.Fatalf("legacy config: %v", err)
	}
	if legacy.CodeAgentRuntime != "zed_agent" || legacy.Provider != "anthropic" {
		t.Fatalf("legacy config = %+v", legacy)
	}

	const raw = `{"code_agent_runtime":"codex_cli","code_agent_credential_type":"subscription","provider":"","model":"gpt-5.6"}`
	if err := reg.Set(ctx, "org-test", helixorgconfig.DefaultAgentConfigKey, raw); err != nil {
		t.Fatalf("set default agent: %v", err)
	}
	agent, err := reg.GetDefaultAgentConfig(ctx, "org-test")
	if err != nil {
		t.Fatalf("default agent config: %v", err)
	}
	if agent.CodeAgentRuntime != "codex_cli" || agent.CodeAgentCredentialType != "subscription" || agent.Model != "gpt-5.6" {
		t.Fatalf("default agent config = %+v", agent)
	}
}

func stringSliceContains(s []string, want string) bool {
	for _, v := range s {
		if v == want {
			return true
		}
	}
	return false
}
