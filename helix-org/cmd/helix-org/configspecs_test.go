package main

import (
	"context"
	"strings"
	"testing"

	"github.com/helixml/helix/helix-org/config"
	"github.com/helixml/helix/helix-org/store/sqlite"
)

// TestGitHubSpecRedactsBothSecrets pins down the spec registration
// for `transport.github`: both `token` and `webhook_secret` MUST be
// redacted on `helix-org config get`. Without this, a future
// refactor that drops one of the entries from the Secrets list
// would silently start leaking the secret to anyone with shell
// access who runs `config get` (logs, screenshares, terminal
// recordings, etc.).
//
// We construct a real registry, run the binary's
// registerAllConfigSpecs, set a known value, and assert the
// redacted form replaces both fields with "...".
func TestGitHubSpecRedactsBothSecrets(t *testing.T) {
	t.Parallel()

	st, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	reg := config.New(st.Configs)
	registerAllConfigSpecs(reg)

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

	// Spec must declare both field names. Asserting on the spec
	// itself catches regressions earlier than the redaction
	// behaviour test (which depends on the registry's redaction
	// logic continuing to do what it does today).
	spec, ok := reg.Spec("transport.github")
	if !ok {
		t.Fatalf("transport.github not registered")
	}
	if !contains(spec.Secrets, "token") {
		t.Fatalf("spec.Secrets = %v, missing \"token\"", spec.Secrets)
	}
	if !contains(spec.Secrets, "webhook_secret") {
		t.Fatalf("spec.Secrets = %v, missing \"webhook_secret\" — secret will leak via `config get`", spec.Secrets)
	}
}

// TestPostmarkSpecRedactsToken is the analogous regression guard for
// the email transport. Cheap to add alongside the github one; same
// failure mode (drop the entry, leak the secret).
func TestPostmarkSpecRedactsToken(t *testing.T) {
	t.Parallel()

	st, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	reg := config.New(st.Configs)
	registerAllConfigSpecs(reg)

	spec, ok := reg.Spec("transport.postmark")
	if !ok {
		t.Fatalf("transport.postmark not registered")
	}
	if !contains(spec.Secrets, "token") {
		t.Fatalf("spec.Secrets = %v, missing \"token\"", spec.Secrets)
	}
}

func contains(s []string, want string) bool {
	for _, v := range s {
		if v == want {
			return true
		}
	}
	return false
}
