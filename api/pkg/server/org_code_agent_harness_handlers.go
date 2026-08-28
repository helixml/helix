package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/gorilla/mux"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// listOrgCodeAgentHarnesses godoc
// @Summary List an organization's coding-agent harnesses
// @Description Returns every selectable coding-agent harness, whether the organization enabled it, and whether the requesting user can use its subscription mode.
// @Tags    organizations
// @Success 200 {array} types.OrgCodeAgentHarnessStatus
// @Param   org_id path string true "Organization ID or name"
// @Router  /api/v1/organizations/{org_id}/code-agent-harnesses [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) listOrgCodeAgentHarnesses(_ http.ResponseWriter, req *http.Request) ([]*types.OrgCodeAgentHarnessStatus, *system.HTTPError) {
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

	statuses, err := apiServer.buildOrgCodeAgentHarnessStatuses(ctx, org.ID, user.ID)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}
	return statuses, nil
}

// updateOrgCodeAgentHarnesses godoc
// @Summary Update an organization's coding-agent harnesses
// @Description Enables or disables coding-agent harnesses and selects either subscription mode or API-provider mode for each harness. Harnesses omitted from the request are left unchanged. Models are selected per task.
// @Tags    organizations
// @Success 200 {array} types.OrgCodeAgentHarnessStatus
// @Param   org_id path string true "Organization ID or name"
// @Param   request body types.OrgCodeAgentHarnessesUpdateRequest true "Harnesses to update"
// @Router  /api/v1/organizations/{org_id}/code-agent-harnesses [put]
// @Security BearerAuth
func (apiServer *HelixAPIServer) updateOrgCodeAgentHarnesses(_ http.ResponseWriter, req *http.Request) ([]*types.OrgCodeAgentHarnessStatus, *system.HTTPError) {
	user := getRequestUser(req)
	if user == nil {
		return nil, system.NewHTTPError401("authentication required")
	}

	ctx := req.Context()
	org, err := apiServer.lookupOrg(ctx, mux.Vars(req)["org_id"])
	if err != nil {
		return nil, system.NewHTTPError404(err.Error())
	}
	if _, err := apiServer.authorizeOrgOwner(ctx, user, org.ID); err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}

	var request types.OrgCodeAgentHarnessesUpdateRequest
	if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
		return nil, system.NewHTTPError400("failed to decode request body: " + err.Error())
	}
	for idx, update := range request.Harnesses {
		if !types.IsSelectableCodeAgentRuntime(update.Runtime) {
			return nil, system.NewHTTPError400(fmt.Sprintf("unsupported code agent runtime %q", update.Runtime))
		}
		if err := normalizeHarnessCredentialSources(&update); err != nil {
			return nil, system.NewHTTPError400(err.Error())
		}
		request.Harnesses[idx] = update
	}

	if _, err := apiServer.Store.UpsertOrgCodeAgentHarnesses(ctx, org.ID, user.ID, request.Harnesses); err != nil {
		return nil, system.NewHTTPError500("failed to save harnesses: " + err.Error())
	}
	statuses, err := apiServer.buildOrgCodeAgentHarnessStatuses(ctx, org.ID, user.ID)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}
	return statuses, nil
}

func (apiServer *HelixAPIServer) buildOrgCodeAgentHarnessStatuses(ctx context.Context, orgID, userID string) ([]*types.OrgCodeAgentHarnessStatus, error) {
	stored, err := apiServer.Store.ListOrgCodeAgentHarnesses(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to list coding-agent harnesses: %w", err)
	}
	policies := make(map[types.CodeAgentRuntime]*types.OrgCodeAgentHarness, len(stored))
	for _, row := range stored {
		policies[row.Runtime] = row
	}

	hasClaude, err := apiServer.viewerHasClaudeSubscription(ctx, orgID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve Claude subscription: %w", err)
	}
	hasCodex, err := apiServer.viewerHasCodexSubscription(ctx, orgID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve Codex subscription: %w", err)
	}

	statuses := make([]*types.OrgCodeAgentHarnessStatus, 0, len(types.SelectableCodeAgentRuntimes))
	for _, runtime := range types.SelectableCodeAgentRuntimes {
		policy := policies[runtime]
		status := &types.OrgCodeAgentHarnessStatus{
			Runtime:              runtime,
			Enabled:              true,
			SupportsSubscription: runtime.SupportsSubscriptionCredentials(),
		}
		if policy != nil {
			status.Enabled = policy.Enabled
			status.SubscriptionEnabled = policy.SubscriptionEnabled
			status.ProviderRefs = policy.ProviderRefs
		}
		switch runtime {
		case types.CodeAgentRuntimeClaudeCode:
			status.ViewerHasSubscription = hasClaude
		case types.CodeAgentRuntimeCodexCLI:
			status.ViewerHasSubscription = hasCodex
		}
		statuses = append(statuses, status)
	}
	return statuses, nil
}

func normalizeHarnessProviderRefs(refs []string) ([]string, error) {
	if refs == nil {
		return nil, nil
	}
	result := make([]string, 0, len(refs))
	seen := make(map[string]struct{}, len(refs))
	for _, providerRef := range refs {
		providerRef = strings.TrimSpace(providerRef)
		if providerRef == "" {
			return nil, fmt.Errorf("provider_refs cannot contain an empty reference")
		}
		if _, exists := seen[providerRef]; exists {
			continue
		}
		seen[providerRef] = struct{}{}
		result = append(result, providerRef)
	}
	sort.Strings(result)
	return result, nil
}

func normalizeHarnessCredentialSources(update *types.OrgCodeAgentHarnessUpdate) error {
	providerRefs, err := normalizeHarnessProviderRefs(update.ProviderRefs)
	if err != nil {
		return err
	}
	update.ProviderRefs = providerRefs

	if update.SubscriptionEnabled != nil && *update.SubscriptionEnabled {
		if !update.Runtime.SupportsSubscriptionCredentials() {
			return fmt.Errorf("coding-agent harness %q does not support subscription credentials", update.Runtime)
		}
		if len(update.ProviderRefs) > 0 {
			return fmt.Errorf("subscription and API-provider modes cannot both be enabled for coding-agent harness %q", update.Runtime)
		}
		// An explicit empty list replaces legacy nil/all-provider policy.
		update.ProviderRefs = []string{}
		return nil
	}

	if len(update.ProviderRefs) > 0 {
		subscriptionEnabled := false
		update.SubscriptionEnabled = &subscriptionEnabled
	}
	return nil
}

func claudeHarnessUsesAPIProviderMode(harness *types.OrgCodeAgentHarness) bool {
	return harness != nil && !(harness.SubscriptionEnabled != nil && *harness.SubscriptionEnabled) && (harness.ProviderRefs == nil || len(harness.ProviderRefs) > 0)
}

func (apiServer *HelixAPIServer) viewerHasClaudeSubscription(ctx context.Context, orgID, userID string) (bool, error) {
	_, err := apiServer.Store.GetEffectiveClaudeSubscription(ctx, userID, orgID)
	if errors.Is(err, store.ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

func (apiServer *HelixAPIServer) viewerHasCodexSubscription(ctx context.Context, orgID, userID string) (bool, error) {
	_, err := apiServer.Store.GetEffectiveCodexSubscription(ctx, userID, orgID)
	if errors.Is(err, store.ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

func (apiServer *HelixAPIServer) enableSubscriptionCodeAgentHarness(
	ctx context.Context,
	orgID, userID string,
	runtime types.CodeAgentRuntime,
) error {
	if orgID == "" {
		return nil
	}
	subscriptionEnabled := true
	_, err := apiServer.Store.UpsertOrgCodeAgentHarnesses(ctx, orgID, userID, []types.OrgCodeAgentHarnessUpdate{{
		Runtime:             runtime,
		Enabled:             true,
		SubscriptionEnabled: &subscriptionEnabled,
		ProviderRefs:        []string{},
	}})
	if err != nil {
		return fmt.Errorf("enable subscription harness %q: %w", runtime, err)
	}
	return nil
}

// validateOrgCodeAgentHarness is the server-side policy fence shared by task
// creation, configuration changes, and task start. The picker is not an
// authorization boundary.
func (apiServer *HelixAPIServer) validateOrgCodeAgentHarness(
	ctx context.Context,
	orgID string,
	runtime types.CodeAgentRuntime,
	credentialType types.CodeAgentCredentialType,
	providerRef string,
) error {
	if orgID == "" {
		return nil
	}
	harness, err := apiServer.loadOrgCodeAgentHarnessPolicy(ctx, orgID, runtime)
	if err != nil {
		return err
	}
	if !harness.Enabled {
		return fmt.Errorf("coding-agent harness %q is not enabled for this organization", runtime)
	}
	if credentialType.IsSubscription() {
		if !harness.AllowsSubscription() {
			return fmt.Errorf("subscription credentials are not enabled for coding-agent harness %q in this organization", runtime)
		}
		return nil
	}
	if providerRef == "" {
		return nil
	}
	endpoints, err := apiServer.listEndpointsForApp(ctx, "", &types.App{OrganizationID: orgID})
	if err != nil {
		return fmt.Errorf("failed to list providers: %w", err)
	}
	if resolveProviderEndpointRef(filterProviderEndpointsForHarness(endpoints, harness, runtime), providerRef) != nil {
		return nil
	}
	return fmt.Errorf("provider %q is not enabled for coding-agent harness %q in this organization", providerRef, runtime)
}

func (apiServer *HelixAPIServer) loadOrgCodeAgentHarnessPolicy(
	ctx context.Context,
	orgID string,
	runtime types.CodeAgentRuntime,
) (*types.OrgCodeAgentHarness, error) {
	harness, err := apiServer.Store.GetOrgCodeAgentHarness(ctx, orgID, runtime)
	if errors.Is(err, store.ErrNotFound) {
		return &types.OrgCodeAgentHarness{
			OrganizationID: orgID,
			Runtime:        runtime,
			Enabled:        true,
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load coding-agent harness policy: %w", err)
	}
	if harness == nil {
		return nil, fmt.Errorf("failed to load coding-agent harness policy: store returned no policy")
	}
	return harness, nil
}
