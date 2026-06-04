// Tests for github.NewSecretInjector — the bridge from the
// existing TokenResolver into the spawner's generic
// SpawnSecretInjector contract.
package github_test

import (
	"context"
	"errors"
	"testing"

	githubtransport "github.com/helixml/helix/api/pkg/org/infrastructure/transports/github"
)

// TestNewSecretInjector_Label pins the transport's identity for
// the spawner's structured logs.
func TestNewSecretInjector_Label(t *testing.T) {
	t.Parallel()
	inj := githubtransport.NewSecretInjector(nil)
	if inj.Name() != "github" {
		t.Errorf("Name() = %q, want github", inj.Name())
	}
}

// TestNewSecretInjector_NilResolver pins the "no resolver wired"
// case: the constructor must not panic, and InjectSecrets must
// return an empty map so the spawner soft-skips.
func TestNewSecretInjector_NilResolver(t *testing.T) {
	t.Parallel()
	inj := githubtransport.NewSecretInjector(nil)
	got, err := inj.InjectSecrets(context.Background(), "org-test")
	if err != nil {
		t.Fatalf("InjectSecrets: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got = %v, want empty map for nil resolver", got)
	}
}

// TestNewSecretInjector_HappyPath pins the canonical resolver →
// GH_TOKEN path: a non-empty token from the resolver becomes the
// GH_TOKEN secret the spawner upserts.
func TestNewSecretInjector_HappyPath(t *testing.T) {
	t.Parallel()
	var gotOrgID string
	inj := githubtransport.NewSecretInjector(func(_ context.Context, orgID string) (string, error) {
		gotOrgID = orgID
		return "gho_test_token", nil
	})
	got, err := inj.InjectSecrets(context.Background(), "org-test")
	if err != nil {
		t.Fatalf("InjectSecrets: %v", err)
	}
	if gotOrgID != "org-test" {
		t.Errorf("resolver got orgID = %q, want org-test", gotOrgID)
	}
	if got["GH_TOKEN"] != "gho_test_token" {
		t.Errorf("GH_TOKEN = %q, want gho_test_token", got["GH_TOKEN"])
	}
}

// TestNewSecretInjector_EmptyTokenSkips pins the soft-skip path:
// a resolver returning "" without error (no GitHub OAuth wired
// yet) → empty map. The spawner won't shadow a previously-valid
// GH_TOKEN with "".
func TestNewSecretInjector_EmptyTokenSkips(t *testing.T) {
	t.Parallel()
	inj := githubtransport.NewSecretInjector(func(_ context.Context, _ string) (string, error) {
		return "", nil
	})
	got, err := inj.InjectSecrets(context.Background(), "org-test")
	if err != nil {
		t.Fatalf("InjectSecrets: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got = %v, want empty map for empty token", got)
	}
}

// TestNewSecretInjector_ResolverErrorPropagates pins that resolver
// errors flow up to the caller; the spawner's iteration loop logs
// + skips them, but the injector itself must propagate so the
// distinction is visible at the boundary.
func TestNewSecretInjector_ResolverErrorPropagates(t *testing.T) {
	t.Parallel()
	boom := errors.New("github API timeout")
	inj := githubtransport.NewSecretInjector(func(_ context.Context, _ string) (string, error) {
		return "", boom
	})
	_, err := inj.InjectSecrets(context.Background(), "org-test")
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want wrapping %v", err, boom)
	}
}
