package github_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	githubtransport "github.com/helixml/helix/api/pkg/org/infrastructure/transports/github"
)

func TestCredentialProvider_Name(t *testing.T) {
	t.Parallel()
	p := githubtransport.NewCredentialProvider(nil)
	if got := p.Name(); got != "github" {
		t.Errorf("Name() = %q, want github", got)
	}
}

// Happy path: resolver returns a non-empty token and an expiry; the
// provider surfaces both verbatim onto the Credential, with the usage
// hint set to the export incantation agents are prompted to run.
func TestCredentialProvider_Mint_HappyPath(t *testing.T) {
	t.Parallel()
	expiry := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	var gotOrgID string
	p := githubtransport.NewCredentialProvider(func(_ context.Context, orgID string) (githubtransport.Identity, error) {
		gotOrgID = orgID
		return githubtransport.Identity{Token: "ghs_test_token", ExpiresAt: expiry}, nil
	})
	cred, err := p.Mint(context.Background(), "org-test", "")
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if gotOrgID != "org-test" {
		t.Errorf("resolver got orgID = %q, want org-test", gotOrgID)
	}
	if cred.Token != "ghs_test_token" {
		t.Errorf("Token = %q, want ghs_test_token", cred.Token)
	}
	if !cred.ExpiresAt.Equal(expiry) {
		t.Errorf("ExpiresAt = %v, want %v", cred.ExpiresAt, expiry)
	}
	if !strings.Contains(cred.Usage, "GH_TOKEN") {
		t.Errorf("Usage = %q, want a hint mentioning GH_TOKEN", cred.Usage)
	}
}

// Empty token (no GitHub identity configured for the org) must be a
// clear error, not a silent empty credential — agents should see the
// failure and surface it to the operator, not paper over it with an
// unauthenticated `gh` call.
func TestCredentialProvider_Mint_EmptyTokenError(t *testing.T) {
	t.Parallel()
	p := githubtransport.NewCredentialProvider(func(_ context.Context, _ string) (githubtransport.Identity, error) {
		return githubtransport.Identity{}, nil
	})
	_, err := p.Mint(context.Background(), "org-empty", "")
	if err == nil {
		t.Fatal("Mint with empty identity: want error, got nil")
	}
	if !strings.Contains(err.Error(), "org-empty") {
		t.Errorf("error %q must mention orgID for diagnosability", err.Error())
	}
}

// Resolver errors propagate (wrapped) so callers see the underlying
// cause.
func TestCredentialProvider_Mint_ResolverErrorPropagates(t *testing.T) {
	t.Parallel()
	boom := errors.New("github API timeout")
	p := githubtransport.NewCredentialProvider(func(_ context.Context, _ string) (githubtransport.Identity, error) {
		return githubtransport.Identity{}, boom
	})
	_, err := p.Mint(context.Background(), "org-test", "")
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want wrapping %v", err, boom)
	}
}

// Nil resolver (mis-wiring) must error explicitly rather than panic.
func TestCredentialProvider_Mint_NilResolverError(t *testing.T) {
	t.Parallel()
	p := githubtransport.NewCredentialProvider(nil)
	_, err := p.Mint(context.Background(), "org-test", "")
	if err == nil {
		t.Fatal("Mint with nil resolver: want error, got nil")
	}
}
