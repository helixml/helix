package slack_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	slacktransport "github.com/helixml/helix/api/pkg/org/infrastructure/transports/slack"
)

func TestCredentialProvider_Name(t *testing.T) {
	t.Parallel()
	p := slacktransport.NewCredentialProvider(nil)
	if got := p.Name(); got != "slack" {
		t.Errorf("Name() = %q, want slack", got)
	}
}

func TestCredentialProvider_Mint_HappyPath(t *testing.T) {
	t.Parallel()
	var gotOrgID string
	p := slacktransport.NewCredentialProvider(func(_ context.Context, orgID, _ string) (slacktransport.Identity, error) {
		gotOrgID = orgID
		return slacktransport.Identity{Token: "xoxb-test-token"}, nil
	})
	cred, err := p.Mint(context.Background(), "org-test", "")
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if gotOrgID != "org-test" {
		t.Errorf("resolver got orgID = %q, want org-test", gotOrgID)
	}
	if cred.Token != "xoxb-test-token" {
		t.Errorf("Token = %q, want xoxb-test-token", cred.Token)
	}
	if cred.Usage == "" {
		t.Error("Usage should carry an export hint for the agent")
	}
}

// The agent mints by passing the triggering event's team_id as resource
// so it gets the bot token for the workspace the message came from. The
// provider must forward resource through to the resolver verbatim.
func TestCredentialProvider_Mint_ForwardsResourceAsTeamID(t *testing.T) {
	t.Parallel()
	var gotResource string
	p := slacktransport.NewCredentialProvider(func(_ context.Context, _, resource string) (slacktransport.Identity, error) {
		gotResource = resource
		return slacktransport.Identity{Token: "xoxb-team-token"}, nil
	})
	if _, err := p.Mint(context.Background(), "org-test", "T123"); err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if gotResource != "T123" {
		t.Errorf("resolver got resource = %q, want the team_id T123 forwarded", gotResource)
	}
}

// No workspace connected for the org → clear error, not a silent empty
// credential, so the agent surfaces "install the Slack app first".
func TestCredentialProvider_Mint_EmptyTokenError(t *testing.T) {
	t.Parallel()
	p := slacktransport.NewCredentialProvider(func(_ context.Context, _, _ string) (slacktransport.Identity, error) {
		return slacktransport.Identity{}, nil
	})
	_, err := p.Mint(context.Background(), "org-empty", "")
	if err == nil {
		t.Fatal("Mint with empty identity: want error, got nil")
	}
	if !strings.Contains(err.Error(), "org-empty") {
		t.Errorf("error %q must mention orgID for diagnosability", err.Error())
	}
}

func TestCredentialProvider_Mint_ResolverErrorPropagates(t *testing.T) {
	t.Parallel()
	boom := errors.New("list service connections timeout")
	p := slacktransport.NewCredentialProvider(func(_ context.Context, _, _ string) (slacktransport.Identity, error) {
		return slacktransport.Identity{}, boom
	})
	_, err := p.Mint(context.Background(), "org-test", "")
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want wrapping %v", err, boom)
	}
}

func TestCredentialProvider_Mint_NilResolverError(t *testing.T) {
	t.Parallel()
	p := slacktransport.NewCredentialProvider(nil)
	_, err := p.Mint(context.Background(), "org-test", "")
	if err == nil {
		t.Fatal("Mint with nil resolver: want error, got nil")
	}
}
