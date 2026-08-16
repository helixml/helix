package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// Coding-agent provider settings are the organization's allow list: which
// runtimes members may pick for a task, and how each one authenticates.
//
// The list is deliberately viewer-scoped on read. Subscription-backed runtimes
// resolve to the acting user's OWN subscription at run time (see
// ResolveClaudeCredentialOwner), so whether Claude Code actually works differs
// per member. Reporting one org-wide "connected" flag would be wrong for every
// member who has not connected their own account, and would invite exactly the
// quota-borrowing the subscription consent model exists to prevent.

// listOrgCodeAgentProviders godoc
// @Summary List an organization's coding-agent providers
// @Description Returns every selectable coding-agent runtime with the organization's setting for it and whether the requesting user can currently run it.
// @Tags    organizations
// @Success 200 {array} types.OrgCodeAgentProviderStatus
// @Param   org_id path string true "Organization ID or name"
// @Router  /api/v1/orgs/{org_id}/code-agent-providers [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) listOrgCodeAgentProviders(_ http.ResponseWriter, req *http.Request) ([]*types.OrgCodeAgentProviderStatus, *system.HTTPError) {
	user := getRequestUser(req)
	if user == nil {
		return nil, system.NewHTTPError401("authentication required")
	}

	ctx := req.Context()
	org, err := apiServer.lookupOrg(ctx, mux.Vars(req)["org_id"])
	if err != nil {
		return nil, system.NewHTTPError404(err.Error())
	}
	if _, err := apiServer.authorizeOrgMember(ctx, user, org.ID); err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}

	statuses, err := apiServer.buildOrgCodeAgentProviderStatuses(ctx, org.ID, user)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}
	return statuses, nil
}

// updateOrgCodeAgentProviders godoc
// @Summary Update an organization's coding-agent providers
// @Description Enables or disables coding-agent runtimes for the organization and sets how each authenticates. Runtimes omitted from the request are left unchanged.
// @Tags    organizations
// @Success 200 {array} types.OrgCodeAgentProviderStatus
// @Param   org_id path string true "Organization ID or name"
// @Param   request body types.OrgCodeAgentProvidersUpdateRequest true "Providers to update"
// @Router  /api/v1/orgs/{org_id}/code-agent-providers [put]
// @Security BearerAuth
func (apiServer *HelixAPIServer) updateOrgCodeAgentProviders(_ http.ResponseWriter, req *http.Request) ([]*types.OrgCodeAgentProviderStatus, *system.HTTPError) {
	user := getRequestUser(req)
	if user == nil {
		return nil, system.NewHTTPError401("authentication required")
	}

	ctx := req.Context()
	org, err := apiServer.lookupOrg(ctx, mux.Vars(req)["org_id"])
	if err != nil {
		return nil, system.NewHTTPError404(err.Error())
	}
	// Writing the allow list decides what every member may spend org budget on,
	// so it is an owner-level action rather than a member one.
	if _, err := apiServer.authorizeOrgOwner(ctx, user, org.ID); err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}

	var request types.OrgCodeAgentProvidersUpdateRequest
	if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
		return nil, system.NewHTTPError400("failed to decode request body: " + err.Error())
	}

	for _, update := range request.Providers {
		if !types.IsSelectableCodeAgentRuntime(update.Runtime) {
			return nil, system.NewHTTPError400(fmt.Sprintf("unsupported code agent runtime %q", update.Runtime))
		}
		if update.CredentialType == types.CodeAgentCredentialTypeSubscription &&
			!update.Runtime.SupportsSubscriptionCredentials() {
			return nil, system.NewHTTPError400(fmt.Sprintf("runtime %q cannot authenticate with a subscription", update.Runtime))
		}
		// A pinned provider endpoint must belong to this org (or be global),
		// otherwise enabling a runtime here would point members at another
		// tenant's endpoint.
		if update.ProviderEndpointID != "" {
			if err := apiServer.assertProviderEndpointUsableByOrg(ctx, update.ProviderEndpointID, org.ID); err != nil {
				return nil, system.NewHTTPError400(err.Error())
			}
		}
	}

	if _, err := apiServer.Store.UpsertOrgCodeAgentProviders(ctx, org.ID, user.ID, request.Providers); err != nil {
		return nil, system.NewHTTPError500("failed to save providers: " + err.Error())
	}

	statuses, err := apiServer.buildOrgCodeAgentProviderStatuses(ctx, org.ID, user)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}
	return statuses, nil
}

// assertProviderEndpointUsableByOrg rejects a provider endpoint that belongs to
// a different organization. Global endpoints and the org's own are allowed.
func (apiServer *HelixAPIServer) assertProviderEndpointUsableByOrg(ctx context.Context, endpointID string, orgID string) error {
	endpoint, err := apiServer.Store.GetProviderEndpoint(ctx, &store.GetProviderEndpointsQuery{ID: endpointID})
	if err != nil {
		return fmt.Errorf("provider endpoint %q not found", endpointID)
	}
	if endpoint.EndpointType == types.ProviderEndpointTypeGlobal {
		return nil
	}
	// Org-owned endpoints are keyed by Owner/OwnerType rather than a dedicated
	// column, so an org endpoint from another tenant is caught here.
	if endpoint.OwnerType == types.OwnerTypeOrg && endpoint.Owner != orgID {
		return fmt.Errorf("provider endpoint %q belongs to another organization", endpointID)
	}
	return nil
}

// buildOrgCodeAgentProviderStatuses joins the stored allow list with the live
// facts the settings UI and the task picker both need: which runtimes exist,
// which the viewer holds a subscription for, and therefore which the viewer can
// actually run.
func (apiServer *HelixAPIServer) buildOrgCodeAgentProviderStatuses(ctx context.Context, orgID string, user *types.User) ([]*types.OrgCodeAgentProviderStatus, error) {
	stored, err := apiServer.Store.ListOrgCodeAgentProviders(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to list providers: %w", err)
	}
	byRuntime := make(map[types.CodeAgentRuntime]*types.OrgCodeAgentProvider, len(stored))
	for _, row := range stored {
		byRuntime[row.Runtime] = row
	}

	hasClaude := apiServer.viewerHasClaudeSubscription(ctx, orgID, user)
	hasCodex := apiServer.viewerHasCodexSubscription(ctx, orgID, user)

	statuses := make([]*types.OrgCodeAgentProviderStatus, 0, len(types.SelectableCodeAgentRuntimes))
	for _, runtime := range types.SelectableCodeAgentRuntimes {
		status := &types.OrgCodeAgentProviderStatus{
			Runtime:              runtime,
			SupportsSubscription: runtime.SupportsSubscriptionCredentials(),
		}
		if row, ok := byRuntime[runtime]; ok {
			status.Enabled = row.Enabled
			status.CredentialType = row.CredentialType
			status.ProviderEndpointID = row.ProviderEndpointID
			status.DefaultModel = row.DefaultModel
		}

		switch runtime {
		case types.CodeAgentRuntimeClaudeCode:
			status.ViewerHasSubscription = hasClaude
		case types.CodeAgentRuntimeCodexCLI:
			status.ViewerHasSubscription = hasCodex
		}

		status.Available, status.UnavailableReason = codeAgentAvailability(status)
		statuses = append(statuses, status)
	}

	return statuses, nil
}

// codeAgentAvailability decides whether this viewer can run this runtime now.
// Kept separate from the assembly above so the rule is testable on its own and
// the task picker and the settings UI cannot drift apart on what "available"
// means.
func codeAgentAvailability(status *types.OrgCodeAgentProviderStatus) (bool, string) {
	if !status.Enabled {
		return false, "Not enabled for this organization"
	}
	if status.CredentialType == types.CodeAgentCredentialTypeSubscription {
		if !status.ViewerHasSubscription {
			// Deliberately about the viewer, not the org: another member with
			// their own subscription can still run this runtime.
			return false, "Connect your own subscription to use this agent"
		}
		return true, ""
	}
	if status.ProviderEndpointID == "" {
		return false, "No provider configured"
	}
	return true, ""
}

// viewerHasClaudeSubscription reports whether the requesting user can
// authenticate Claude Code — either through their own subscription or one the
// organization itself owns. Subscriptions owned by other members of the org
// deliberately do not count: using one would spend that person's quota without
// their consent.
func (apiServer *HelixAPIServer) viewerHasClaudeSubscription(ctx context.Context, orgID string, user *types.User) bool {
	if subs, err := apiServer.Store.ListClaudeSubscriptions(ctx, user.ID); err == nil && len(subs) > 0 {
		return true
	}
	if subs, err := apiServer.Store.ListClaudeSubscriptions(ctx, orgID); err == nil && len(subs) > 0 {
		return true
	}
	return false
}

// viewerHasCodexSubscription is the Codex counterpart of
// viewerHasClaudeSubscription.
func (apiServer *HelixAPIServer) viewerHasCodexSubscription(ctx context.Context, orgID string, user *types.User) bool {
	if subs, err := apiServer.Store.ListCodexSubscriptions(ctx, user.ID); err == nil && len(subs) > 0 {
		return true
	}
	if subs, err := apiServer.Store.ListCodexSubscriptions(ctx, orgID); err == nil && len(subs) > 0 {
		return true
	}
	return false
}
