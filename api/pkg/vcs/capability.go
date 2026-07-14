// Package vcs holds the generic, provider-agnostic capability registry used by
// the project VCS connection lozenge and the push/verify paths. Everything
// provider-specific (auth mechanism, required OAuth scopes, identity handle,
// access-verify probe shape) lives in a per-provider Capability entry keyed by
// types.ExternalRepositoryType. Adding a provider = adding an entry here; the
// shared board component and push logic stay generic.
package vcs

import (
	"fmt"

	"github.com/helixml/helix/api/pkg/types"
)

// Capability is the provider-specific metadata the generic lozenge/verify code
// needs. Keep it declarative — behavior lives in the generic layer.
type Capability struct {
	// Type is the repo external type this entry describes.
	Type types.ExternalRepositoryType
	// Provider is the matching OAuth provider type (for connection lookups).
	Provider types.OAuthProviderType
	// AuthMechanism is the git credential username used with the OAuth token
	// (e.g. "x-access-token" for GitHub, "oauth2" for GitLab).
	AuthMechanism string
	// RequiredScopes is the full scope set to REQUEST at connect time so the
	// lozenge can display identity and verify access. Broader than the minimum
	// needed to merely push — see design root cause #6.
	RequiredScopes []string
	// AccessProbePath is a printf-style template for the provider API path used
	// to verify a connection can reach a repo, given "owner/repo" split. Used by
	// the verify probe in the service layer via oauth.Provider.MakeAuthorizedRequest.
	AccessProbePath string
}

// capabilities is the registry. GitHub is fully specified; the others carry
// their scope/auth metadata so they render and verify without shared-code edits.
var capabilities = map[types.ExternalRepositoryType]Capability{
	types.ExternalRepositoryTypeGitHub: {
		Type:            types.ExternalRepositoryTypeGitHub,
		Provider:        types.OAuthProviderTypeGitHub,
		AuthMechanism:   "x-access-token",
		RequiredScopes:  []string{"repo", "read:user", "user:email", "read:org"},
		AccessProbePath: "https://api.github.com/repos/%s/%s",
	},
	types.ExternalRepositoryTypeGitLab: {
		Type:            types.ExternalRepositoryTypeGitLab,
		Provider:        types.OAuthProviderTypeGitLab,
		AuthMechanism:   "oauth2",
		RequiredScopes:  []string{"api", "read_repository", "write_repository"},
		AccessProbePath: "https://gitlab.com/api/v4/projects/%s%%2F%s",
	},
	types.ExternalRepositoryTypeADO: {
		Type:           types.ExternalRepositoryTypeADO,
		Provider:       types.OAuthProviderTypeAzureDevOps,
		AuthMechanism:  "oauth2",
		RequiredScopes: []string{"vso.code_write"},
	},
	types.ExternalRepositoryTypeBitbucket: {
		Type:           types.ExternalRepositoryTypeBitbucket,
		Provider:       types.OAuthProviderTypeUnknown,
		AuthMechanism:  "",
		RequiredScopes: []string{"repository", "repository:write"},
	},
}

// For returns the capability entry for a repo external type.
func For(t types.ExternalRepositoryType) (Capability, bool) {
	c, ok := capabilities[t]
	return c, ok
}

// RequiredScopes returns the full connect scope set for a provider, or nil if
// the provider is unknown.
func RequiredScopes(t types.ExternalRepositoryType) []string {
	if c, ok := capabilities[t]; ok {
		return c.RequiredScopes
	}
	return nil
}

// IdentityHandle returns the display handle for a connection (e.g. "@login"),
// or "" if the connection has no username.
func IdentityHandle(conn *types.OAuthConnection) string {
	if conn == nil || conn.ProviderUsername == "" {
		return ""
	}
	return "@" + conn.ProviderUsername
}

// AccessProbeURL builds the provider API URL to verify access to owner/repo,
// or "" if the provider has no probe defined.
func AccessProbeURL(t types.ExternalRepositoryType, owner, repo string) string {
	c, ok := capabilities[t]
	if !ok || c.AccessProbePath == "" {
		return ""
	}
	return fmt.Sprintf(c.AccessProbePath, owner, repo)
}
