package mcptools

// mint_credential is a deliberate, recorded exception to the
// "keep the MCP surface small" guardrail in CLAUDE.md and in
// design/2026-06-10-helix-org-application-services.md §8. That rule
// bans per-action MCP wrappers like publish_to_blog / fetch_url —
// agents do those through shell tools (bash/curl/gh/git) with the
// Role prompt describing how. A *generic credential-minting primitive*
// is a different kind of thing: it is what makes those shell tools
// usable on a long-running session whose boot-time credentials have
// expired. The exception is documented in
// design/tasks/002092_helix-org-mintcredential/design.md §2.

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/domain/credential"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// MintCredential is the MCP tool that lets a Worker obtain an
// org-scoped external-provider credential on demand. It is the single
// surface for "give the agent a gh/git/curl token": there is no
// boot-time env-var injection. The canonical flow is:
//
//	mint_credential → export TOKEN → run gh/git/curl
//	on 401/403      → mint_credential → export → retry
//
// The Description() body is load-bearing — it is the only signal the
// agent has that the token has to be minted-then-exported before its
// first authenticated command, and re-minted on any auth error.
//
// orgID is read from inv.Caller.OrganizationID() only — never from
// args — so a Worker physically cannot mint another org's credential.
// Same pattern as create_topic and the other org-scoped tools.
type MintCredential struct {
	deps      Deps
	providers map[string]credential.Provider
}

const MintCredentialName tool.Name = "mint_credential"

type mintCredentialArgs struct {
	// Provider names the external system to mint a credential for.
	// Must match a registered credential.Provider on the server
	// (e.g. "github"). The list of valid values is enumerated in
	// the error message when a caller picks an unknown provider.
	Provider string `json:"provider"`
}

var mintCredentialSchema = mustSchema[mintCredentialArgs]()

func (t *MintCredential) Name() tool.Name { return MintCredentialName }

func (t *MintCredential) Description() string {
	avail := strings.Join(providerNames(t.providers), ", ")
	if avail == "" {
		avail = "(none configured on this server)"
	}
	return "Mint a short-lived credential (~1 hour) for an external provider " +
		"scoped to your organization. Supported providers: " + avail + ".\n\n" +
		"No provider tokens are in your shell environment by default — you " +
		"**must** call mint_credential and export the returned token before " +
		"the first git, gh, or authenticated curl command " +
		"(e.g. `export GH_TOKEN=$(mint_credential provider=github | jq -r .token)`).\n\n" +
		"**If a command later fails with 401/403 or any authentication " +
		"error, your token has expired** — call mint_credential again, " +
		"re-export, and retry. Do not give up on the task; expired tokens " +
		"are expected for any work that takes more than ~1 hour.\n\n" +
		"Args: provider (string, required) — one of: " + avail + ".\n" +
		"Returns: { token, expires_at (RFC3339), usage }."
}

func (t *MintCredential) InputSchema() *jsonschema.Schema { return mintCredentialSchema }

func (t *MintCredential) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args mintCredentialArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.Provider == "" {
		return nil, fmt.Errorf("mint_credential: provider is required (one of: %s)", strings.Join(providerNames(t.providers), ", "))
	}
	p, ok := t.providers[args.Provider]
	if !ok {
		return nil, fmt.Errorf("mint_credential: unknown provider %q; available: %s", args.Provider, strings.Join(providerNames(t.providers), ", "))
	}
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return nil, fmt.Errorf("mint_credential: caller has no OrgID")
	}
	cred, err := p.Mint(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("mint %s credential: %w", args.Provider, err)
	}
	out := map[string]any{
		"token": cred.Token,
		"usage": cred.Usage,
	}
	if !cred.ExpiresAt.IsZero() {
		out["expires_at"] = cred.ExpiresAt.UTC().Format(time.RFC3339)
	}
	return json.Marshal(out)
}

// providerNames returns the registered provider names in stable
// alphabetical order so error messages and the tool description are
// deterministic across server restarts and helpful for the agent
// reading them.
func providerNames(providers map[string]credential.Provider) []string {
	names := make([]string, 0, len(providers))
	for name := range providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
