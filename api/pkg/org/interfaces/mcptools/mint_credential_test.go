package mcptools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/credential"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

type fakeCredCaller struct {
	id    string
	orgID string
}

func (f fakeCredCaller) ID() string             { return f.id }
func (f fakeCredCaller) OrganizationID() string { return f.orgID }

// fakeProvider records the orgID it was called with and returns a
// pre-canned Credential or error.
type fakeProvider struct {
	name    string
	gotOrg  string
	credOut credential.Credential
	errOut  error
}

func (f *fakeProvider) Name() string { return f.name }

func (f *fakeProvider) Mint(_ context.Context, orgID string) (credential.Credential, error) {
	f.gotOrg = orgID
	return f.credOut, f.errOut
}

func newMintTool(providers ...*fakeProvider) *MintCredential {
	reg := map[string]credential.Provider{}
	for _, p := range providers {
		reg[p.name] = p
	}
	return &MintCredential{providers: reg}
}

// Happy path: a registered provider returns Credential{Token, ExpiresAt,
// Usage}; the tool surfaces all three as JSON, with expires_at in RFC3339.
func TestMintCredential_HappyPath(t *testing.T) {
	t.Parallel()
	expiry := time.Date(2026, 6, 11, 12, 30, 0, 0, time.UTC)
	gh := &fakeProvider{
		name: "github",
		credOut: credential.Credential{
			Token:     "ghs_test_token",
			ExpiresAt: expiry,
			Usage:     "export GH_TOKEN=<token>",
		},
	}
	tl := newMintTool(gh)
	raw, err := tl.Invoke(context.Background(), tool.Invocation{
		Caller: fakeCredCaller{id: "w-1", orgID: "org-1"},
		Args:   json.RawMessage(`{"provider":"github"}`),
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if out["token"] != "ghs_test_token" {
		t.Errorf("token = %v, want ghs_test_token", out["token"])
	}
	if out["usage"] != "export GH_TOKEN=<token>" {
		t.Errorf("usage = %v, want export hint", out["usage"])
	}
	if out["expires_at"] != "2026-06-11T12:30:00Z" {
		t.Errorf("expires_at = %v, want RFC3339 UTC", out["expires_at"])
	}
	if gh.gotOrg != "org-1" {
		t.Errorf("provider got orgID = %q, want org-1", gh.gotOrg)
	}
}

// Unknown provider: tool returns an error mentioning the requested name
// and the list of registered providers.
func TestMintCredential_UnknownProvider(t *testing.T) {
	t.Parallel()
	tl := newMintTool(&fakeProvider{name: "github"})
	_, err := tl.Invoke(context.Background(), tool.Invocation{
		Caller: fakeCredCaller{id: "w-1", orgID: "org-1"},
		Args:   json.RawMessage(`{"provider":"slack"}`),
	})
	if err == nil {
		t.Fatal("Invoke: want error for unknown provider, got nil")
	}
	if !strings.Contains(err.Error(), "slack") {
		t.Errorf("err %q must name the requested provider", err.Error())
	}
	if !strings.Contains(err.Error(), "github") {
		t.Errorf("err %q must list registered providers", err.Error())
	}
}

// Missing provider arg: tool errors out before consulting the registry.
func TestMintCredential_MissingProvider(t *testing.T) {
	t.Parallel()
	tl := newMintTool(&fakeProvider{name: "github"})
	_, err := tl.Invoke(context.Background(), tool.Invocation{
		Caller: fakeCredCaller{id: "w-1", orgID: "org-1"},
		Args:   json.RawMessage(`{}`),
	})
	if err == nil {
		t.Fatal("Invoke: want error for missing provider, got nil")
	}
	if !strings.Contains(err.Error(), "provider is required") {
		t.Errorf("err %q must mention provider being required", err.Error())
	}
}

// Caller has no OrgID → canonical "caller has no OrgID" error,
// mirroring create_stream. This shouldn't normally happen at runtime
// because the MCP server only routes calls from org-scoped Workers,
// but the tool defends against it for diagnosability.
func TestMintCredential_MissingOrgID(t *testing.T) {
	t.Parallel()
	gh := &fakeProvider{name: "github"}
	tl := newMintTool(gh)
	_, err := tl.Invoke(context.Background(), tool.Invocation{
		Caller: fakeCredCaller{id: "w-1", orgID: ""},
		Args:   json.RawMessage(`{"provider":"github"}`),
	})
	if err == nil {
		t.Fatal("Invoke: want error for missing OrgID, got nil")
	}
	if !strings.Contains(err.Error(), "OrgID") {
		t.Errorf("err %q must mention OrgID", err.Error())
	}
	if gh.gotOrg != "" {
		t.Errorf("provider must not be called when caller has no OrgID; got orgID = %q", gh.gotOrg)
	}
}

// A forged `org_id` field in the raw args is IGNORED. The schema does
// not declare org_id, and the implementation reads org from the caller
// — this regression test pins that contract so a future refactor can't
// silently widen the trust boundary.
func TestMintCredential_ForgedOrgIDIgnored(t *testing.T) {
	t.Parallel()
	gh := &fakeProvider{
		name:    "github",
		credOut: credential.Credential{Token: "t", Usage: "export GH_TOKEN=<token>"},
	}
	tl := newMintTool(gh)
	_, err := tl.Invoke(context.Background(), tool.Invocation{
		Caller: fakeCredCaller{id: "w-1", orgID: "org-real"},
		Args:   json.RawMessage(`{"provider":"github","org_id":"org-other"}`),
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if gh.gotOrg != "org-real" {
		t.Errorf("provider got orgID = %q, want org-real (forged org_id arg must be ignored)", gh.gotOrg)
	}
}

// Provider errors propagate, wrapped with the provider name so an
// operator reading logs can immediately see which one failed.
func TestMintCredential_ProviderErrorPropagates(t *testing.T) {
	t.Parallel()
	boom := errors.New("github API timeout")
	tl := newMintTool(&fakeProvider{name: "github", errOut: boom})
	_, err := tl.Invoke(context.Background(), tool.Invocation{
		Caller: fakeCredCaller{id: "w-1", orgID: "org-1"},
		Args:   json.RawMessage(`{"provider":"github"}`),
	})
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want wrapping %v", err, boom)
	}
	if !strings.Contains(err.Error(), "github") {
		t.Errorf("err %q must name the failing provider", err.Error())
	}
}

// Zero ExpiresAt (e.g. OAuth fallback path on the GitHub provider)
// must omit expires_at from the JSON rather than emitting "0001-01-01".
func TestMintCredential_ZeroExpiresAtOmitted(t *testing.T) {
	t.Parallel()
	gh := &fakeProvider{
		name: "github",
		credOut: credential.Credential{
			Token: "oauth-token",
			Usage: "export GH_TOKEN=<token>",
			// ExpiresAt left zero
		},
	}
	tl := newMintTool(gh)
	raw, err := tl.Invoke(context.Background(), tool.Invocation{
		Caller: fakeCredCaller{id: "w-1", orgID: "org-1"},
		Args:   json.RawMessage(`{"provider":"github"}`),
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := out["expires_at"]; ok {
		t.Errorf("expires_at must be omitted when provider returns zero time, got %v", out["expires_at"])
	}
	if out["token"] != "oauth-token" {
		t.Errorf("token = %v, want oauth-token", out["token"])
	}
}

// Description must namedrop registered providers so the agent reading
// it knows which `provider` values are valid.
func TestMintCredential_DescriptionListsProviders(t *testing.T) {
	t.Parallel()
	tl := newMintTool(&fakeProvider{name: "github"}, &fakeProvider{name: "slack"})
	desc := tl.Description()
	if !strings.Contains(desc, "github") || !strings.Contains(desc, "slack") {
		t.Errorf("description %q must mention registered providers", desc)
	}
	if !strings.Contains(strings.ToLower(desc), "401") {
		t.Errorf("description %q must mention 401/auth-error recovery", desc)
	}
}

// Empty registry: description must not crash and must surface the
// degraded state to the agent so it can fail loud instead of trying
// to guess a provider name.
func TestMintCredential_DescriptionEmptyRegistry(t *testing.T) {
	t.Parallel()
	tl := newMintTool()
	desc := tl.Description()
	if !strings.Contains(strings.ToLower(desc), "none configured") {
		t.Errorf("description %q must mention the empty registry", desc)
	}
}
