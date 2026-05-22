package server

import (
	"context"
	"strings"
	"testing"

	helixorgconfig "github.com/helixml/helix/api/pkg/org/config"
	"github.com/helixml/helix/api/pkg/org/store/sqlite"
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
// registerHelixOrgConfigSpecs.
func TestRegisterHelixOrgConfigSpecs_RedactsTransportGitHubSecrets(t *testing.T) {
	t.Parallel()

	st, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	reg := helixorgconfig.New(st.Configs)
	registerHelixOrgConfigSpecs(reg)

	const raw = `{"token":"plaintext-token-leaked","webhook_secret":"plaintext-secret-leaked"}`
	if err := reg.Set(context.Background(), "transport.github", raw, ""); err != nil {
		t.Fatalf("set: %v", err)
	}

	got, err := reg.GetRedacted(context.Background(), "transport.github")
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

	st, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	reg := helixorgconfig.New(st.Configs)
	registerHelixOrgConfigSpecs(reg)

	spec, ok := reg.Spec("transport.postmark")
	if !ok {
		t.Fatalf("transport.postmark not registered")
	}
	if !stringSliceContains(spec.Secrets, "token") {
		t.Fatalf("spec.Secrets = %v, missing \"token\"", spec.Secrets)
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
