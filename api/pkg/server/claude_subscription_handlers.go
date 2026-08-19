package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/anthropic"
	"github.com/helixml/helix/api/pkg/crypto"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// @Summary Create a Claude subscription
// @Description Connect a Claude subscription by providing OAuth credentials
// @Tags Claude
// @Accept json
// @Produce json
// @Param body body types.CreateClaudeSubscriptionRequest true "Claude subscription credentials"
// @Success 200 {object} types.ClaudeSubscription
// @Failure 400 {object} system.HTTPError
// @Failure 401 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security BearerAuth
// @Router /api/v1/claude-subscriptions [post]
func (apiServer *HelixAPIServer) createClaudeSubscription(_ http.ResponseWriter, req *http.Request) (*types.ClaudeSubscription, *system.HTTPError) {
	user := getRequestUser(req)
	if user == nil {
		return nil, system.NewHTTPError401("authentication required")
	}

	var createReq types.CreateClaudeSubscriptionRequest
	if err := json.NewDecoder(req.Body).Decode(&createReq); err != nil {
		return nil, system.NewHTTPError400("invalid request body: " + err.Error())
	}

	// Determine owner
	ownerID := user.ID
	ownerType := types.OwnerTypeUser
	if createReq.OwnerType == types.OwnerTypeOrg {
		if createReq.OwnerID == "" {
			return nil, system.NewHTTPError400("owner_id required for org-level subscriptions")
		}
		org, err := apiServer.lookupOrg(req.Context(), createReq.OwnerID)
		if err != nil {
			return nil, system.NewHTTPError404("organization not found")
		}
		_, err = apiServer.authorizeOrgOwner(req.Context(), user, org.ID)
		if err != nil {
			return nil, system.NewHTTPError403("not authorized to manage org subscriptions: " + err.Error())
		}
		ownerID = org.ID
		ownerType = types.OwnerTypeOrg
	}
	harnessOrgID := ""
	if createReq.OrganizationID != "" {
		org, err := apiServer.lookupOrg(req.Context(), createReq.OrganizationID)
		if err != nil {
			return nil, system.NewHTTPError404("organization not found")
		}
		if _, err := apiServer.authorizeOrgOwner(req.Context(), user, org.ID); err != nil {
			return nil, system.NewHTTPError403("not authorized to enable organization harness")
		}
		harnessOrgID = org.ID
	} else if ownerType == types.OwnerTypeOrg {
		harnessOrgID = ownerID
	}

	var encrypted string
	var credentialType string
	var expiresAt time.Time
	var subscriptionType, rateLimitTier string
	var scopes []string

	if createReq.SetupToken != "" {
		// Setup token flow: token from `claude setup-token`
		token := strings.TrimSpace(createReq.SetupToken)
		if strings.HasPrefix(token, "sk-ant-api") {
			return nil, system.NewHTTPError400(
				"This is an Anthropic API key, not a Claude Code setup token. " +
					"Run 'claude setup-token' in your terminal to generate the correct token.")
		}
		if !strings.HasPrefix(token, "sk-ant-oat") {
			return nil, system.NewHTTPError400(
				"Invalid setup token format. " +
					"Run 'claude setup-token' to generate a valid token.")
		}

		credJSON, err := json.Marshal(types.ClaudeSetupTokenCredentials{SetupToken: token})
		if err != nil {
			return nil, system.NewHTTPError500("failed to marshal credentials")
		}
		encKey, err := crypto.GetEncryptionKey()
		if err != nil {
			return nil, system.NewHTTPError500("failed to get encryption key")
		}
		encrypted, err = crypto.EncryptAES256GCM(credJSON, encKey)
		if err != nil {
			return nil, system.NewHTTPError500("failed to encrypt credentials")
		}
		credentialType = "setup_token"
		// Setup tokens carry no plan/tier and cannot be profiled. Identity comes
		// from the probe's organization header on first validation, not the user.
	} else {
		// OAuth credentials flow (from in-container browser auth)
		creds := createReq.Credentials.ClaudeAiOauth
		if creds.AccessToken == "" || creds.RefreshToken == "" {
			return nil, system.NewHTTPError400("setup_token or OAuth credentials (accessToken + refreshToken) are required")
		}
		credJSON, err := json.Marshal(creds)
		if err != nil {
			return nil, system.NewHTTPError500("failed to marshal credentials")
		}
		encKey, err := crypto.GetEncryptionKey()
		if err != nil {
			return nil, system.NewHTTPError500("failed to get encryption key")
		}
		encrypted, err = crypto.EncryptAES256GCM(credJSON, encKey)
		if err != nil {
			return nil, system.NewHTTPError500("failed to encrypt credentials")
		}
		credentialType = "oauth"
		subscriptionType = creds.SubscriptionType
		rateLimitTier = creds.RateLimitTier
		scopes = creds.Scopes
		if creds.ExpiresAt > 0 {
			expiresAt = time.UnixMilli(creds.ExpiresAt)
		}
	}

	// Delete any existing subscriptions for this owner before creating a new one.
	existingSubs, _ := apiServer.Store.ListClaudeSubscriptions(req.Context(), ownerID)
	for _, old := range existingSubs {
		if old.OwnerType == ownerType {
			_ = apiServer.Store.DeleteClaudeSubscription(req.Context(), old.ID)
			log.Info().Str("old_subscription_id", old.ID).Msg("Deleted old Claude subscription on re-auth")
		}
	}

	sub := &types.ClaudeSubscription{
		OwnerID:              ownerID,
		OwnerType:            ownerType,
		Name:                 createReq.Name,
		EncryptedCredentials: encrypted,
		CredentialType:       credentialType,
		SubscriptionType:     subscriptionType,
		RateLimitTier:        rateLimitTier,
		Scopes:               scopes,
		AccessTokenExpiresAt: expiresAt,
		Status:               "active",
		CreatedBy:            user.ID,
	}

	created, err := apiServer.Store.CreateClaudeSubscription(req.Context(), sub)
	if err != nil {
		return nil, system.NewHTTPError500("failed to create subscription: " + err.Error())
	}

	// Liveness-probe the freshly connected credentials so Status reflects reality
	// instead of an unconditional "active". A dead token is caught here rather
	// than at the first agent turn.
	created = apiServer.revalidateClaudeSubscription(req.Context(), created)
	if created.Status == "active" {
		if err := apiServer.enableSubscriptionCodeAgentHarness(req.Context(), harnessOrgID, user.ID, types.CodeAgentRuntimeClaudeCode); err != nil {
			if deleteErr := apiServer.Store.DeleteClaudeSubscription(context.WithoutCancel(req.Context()), created.ID); deleteErr != nil {
				log.Error().Err(deleteErr).Str("subscription_id", created.ID).Msg("failed to roll back Claude subscription")
			}
			return nil, system.NewHTTPError500("failed to enable Claude Code harness: " + err.Error())
		}
	}
	if cleanupErr := apiServer.cleanupSubscriptionLoginSessionsForOwner(req.Context(), user.ID, claudeLoginSessionName, claudeLoginSessionProvider); cleanupErr != nil {
		log.Warn().Err(cleanupErr).Str("user_id", user.ID).Msg("failed to clean up completed Claude login sessions")
	}

	log.Info().
		Str("subscription_id", created.ID).
		Str("owner_id", ownerID).
		Str("owner_type", string(ownerType)).
		Str("credential_type", credentialType).
		Str("status", created.Status).
		Msg("Created Claude subscription")

	return created, nil
}

// revalidateClaudeSubscription liveness-probes a subscription's stored token and
// persists the outcome (Status, LastError, LastValidatedAt). A ProbeInconclusive
// result (network error, or an oauth token that is expired-but-refreshable)
// leaves the existing Status untouched — we never downgrade a subscription to
// "error" on an ambiguous signal. Returns the updated row (or the input on
// failure to persist).
func (apiServer *HelixAPIServer) revalidateClaudeSubscription(ctx context.Context, sub *types.ClaudeSubscription) *types.ClaudeSubscription {
	if sub == nil {
		return sub
	}
	probe := anthropic.ValidateSubscription(ctx, sub)
	now := time.Now()
	// Anthropic returns the organization uuid on every probe response, whatever
	// the token's scopes — so this is the identity that works for setup tokens.
	// Only ever widen it: an inconclusive probe that never reached Anthropic
	// must not wipe a previously captured id.
	if probe.OrganizationID != "" {
		sub.ClaudeOrganizationID = probe.OrganizationID
	}
	switch probe.Result {
	case anthropic.ProbeValid:
		sub.Status = "active"
		sub.LastError = ""
		sub.LastValidatedAt = &now
		// Best-effort: fetch the Claude account the token authenticates as
		// (email, plan, rate-limit tier) from Anthropic. A profile failure
		// must never downgrade a subscription that just probed valid.
		// Setup tokens are skipped: they only ever carry inference scopes, and
		// /api/oauth/profile requires any_of(user:profile, user:office), so the
		// fetch would 403 by construction (verified against real Anthropic).
		if sub.CredentialType != "setup_token" {
			if profile, err := anthropic.FetchClaudeProfile(ctx, probe.Token); err != nil {
				log.Debug().Str("subscription_id", sub.ID).Str("detail", err.Error()).Msg("Claude profile fetch failed; identity unchanged")
			} else {
				sub.AccountEmail = profile.AccountEmail
				sub.AccountDisplayName = profile.AccountDisplayName
				if profile.Plan != "" {
					sub.SubscriptionType = profile.Plan
				}
				if profile.RateLimitTier != "" {
					sub.RateLimitTier = profile.RateLimitTier
				}
			}
		}
	case anthropic.ProbeInvalid:
		sub.Status = "error"
		sub.LastError = probe.Detail
		sub.LastValidatedAt = &now
	case anthropic.ProbeInconclusive:
		// Leave Status/LastError as-is; record that we tried.
		sub.LastValidatedAt = &now
		log.Debug().Str("subscription_id", sub.ID).Str("detail", probe.Detail).Msg("Claude subscription probe inconclusive")
	}
	updated, err := apiServer.Store.UpdateClaudeSubscription(ctx, sub)
	if err != nil {
		log.Warn().Err(err).Str("subscription_id", sub.ID).Msg("failed to persist Claude subscription validation result")
		return sub
	}
	return updated
}

// claudeSubscriptionStatusMaxAge bounds how stale a persisted validation result
// can be before the owner-status endpoint re-probes. Keeps the settings display
// honest without hammering Anthropic on every render.
const claudeSubscriptionStatusMaxAge = 5 * time.Minute

// AppClaudeSubscriptionStatus reports whether the account whose Claude
// subscription would authenticate an app's sessions (the app owner) actually has
// a working subscription connected. Backs the "whose subscription" callout and
// the cross-user warning in agent settings.
type AppClaudeSubscriptionStatus struct {
	Connected             bool   `json:"connected"`                         // owner has a subscription connected at all
	Valid                 bool   `json:"valid"`                             // that subscription passed its last liveness probe
	OwnerID               string `json:"owner_id"`                          // app owner (the likely session owner)
	OwnerName             string `json:"owner_name"`                        // human-readable owner (email / full name)
	IsCurrentUser         bool   `json:"is_current_user"`                   // true when the editor IS the owner
	SubscriptionOwnerType string `json:"subscription_owner_type,omitempty"` // "user" or "org" — where the effective sub resolved
	// Identity of the Claude subscription itself, populated when Connected.
	// SubscriptionType is the plan ("pro" / "max"), empty for setup-token
	// connections where the plan is unknown.
	// SubscriptionOwnerName is the subscription owner's email (user-owned) or
	// org name (org-owned) — i.e. WHOSE subscription authenticates the agent.
	// SubscriptionOwnerIsCurrentUser is true when that owner is the requesting
	// user's own subscription ("is it mine?" — yes).
	//
	// ClaudeAccountEmail/ClaudeAccountName identify the actual Claude account
	// the token authenticates as (fetched from Anthropic's /api/oauth/profile) —
	// the identity that gets billed. It can differ from SubscriptionOwnerName
	// (the Helix user who connected the subscription); when no valid probe has
	// enriched the row yet they are empty and consumers fall back to the owner.
	SubscriptionType               string `json:"subscription_type,omitempty"`
	SubscriptionOwnerID            string `json:"subscription_owner_id,omitempty"`
	SubscriptionOwnerName          string `json:"subscription_owner_name,omitempty"`
	SubscriptionOwnerIsCurrentUser bool   `json:"subscription_owner_is_current_user"`
	// SubscriptionRateLimitTier is the Claude org's rate-limit tier as Anthropic
	// reports it, e.g. "default_claude_max_20x"; empty when unknown.
	SubscriptionRateLimitTier string `json:"subscription_rate_limit_tier,omitempty"`
	ClaudeAccountEmail        string `json:"claude_account_email,omitempty"`
	ClaudeAccountName         string `json:"claude_account_name,omitempty"`
	// ClaudeOrganizationID is Anthropic's organization uuid for the credential.
	// Populated for setup tokens too, which cannot be profiled — it lets the UI
	// say "this is the same subscription as X" without anyone typing anything.
	ClaudeOrganizationID string     `json:"claude_organization_id,omitempty"`
	Status               string     `json:"status,omitempty"`
	LastValidatedAt      *time.Time `json:"last_validated_at,omitempty"`
	LastError            string     `json:"last_error,omitempty"`
}

// @Summary Get the Claude subscription status for an agent's owner
// @Description Reports whether the agent owner (whose subscription authenticates the agent's sessions) has a working Claude subscription
// @Tags Claude
// @Produce json
// @Param id path string true "Agent ID"
// @Success 200 {object} AppClaudeSubscriptionStatus
// @Failure 401 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Security BearerAuth
// @Router /api/v1/agents/{id}/claude-subscription-status [get]
func (apiServer *HelixAPIServer) getAppClaudeSubscriptionStatus(_ http.ResponseWriter, req *http.Request) (*AppClaudeSubscriptionStatus, *system.HTTPError) {
	user := getRequestUser(req)
	if user == nil {
		return nil, system.NewHTTPError401("authentication required")
	}

	app, err := apiServer.Store.GetApp(req.Context(), getID(req))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, system.NewHTTPError404(store.ErrNotFound.Error())
		}
		return nil, system.NewHTTPError500(err.Error())
	}
	if err := apiServer.authorizeUserToApp(req.Context(), user, app, types.ActionGet); err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}

	status := &AppClaudeSubscriptionStatus{
		OwnerID:       app.Owner,
		OwnerName:     apiServer.displayNameForUser(req.Context(), app.Owner),
		IsCurrentUser: app.Owner == user.ID,
	}

	// Resolve the subscription exactly as a session owned by the app owner would
	// (user-level first, then the app's org).
	sub, err := apiServer.Store.GetEffectiveClaudeSubscription(req.Context(), app.Owner, app.OrganizationID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return status, nil // not connected
		}
		return nil, system.NewHTTPError500("failed to resolve subscription: " + err.Error())
	}

	// Re-probe if we've never validated it or the last result is stale.
	if sub.LastValidatedAt == nil || time.Since(*sub.LastValidatedAt) > claudeSubscriptionStatusMaxAge {
		sub = apiServer.revalidateClaudeSubscription(req.Context(), sub)
	}

	status.Connected = true
	status.Valid = sub.Status == "active"
	status.SubscriptionOwnerType = string(sub.OwnerType)
	status.SubscriptionType = sub.SubscriptionType
	status.SubscriptionRateLimitTier = sub.RateLimitTier
	status.SubscriptionOwnerID = sub.OwnerID
	status.SubscriptionOwnerIsCurrentUser = sub.OwnerType == types.OwnerTypeUser && sub.OwnerID == user.ID
	status.ClaudeAccountEmail = sub.AccountEmail
	status.ClaudeAccountName = sub.AccountDisplayName
	status.ClaudeOrganizationID = sub.ClaudeOrganizationID
	// A setup token can never be profiled, but it does report its Claude org.
	// If a subscription visible in this same context (the app owner's, or the
	// app org's) has been profiled and sits on that org, it is by definition
	// the same Claude account — so name it. Scoped to already-visible rows on
	// purpose: a global lookup would leak an unrelated org's email.
	if status.ClaudeAccountEmail == "" && sub.ClaudeOrganizationID != "" {
		if email, name := apiServer.claudeIdentityForOrg(req.Context(), sub.ClaudeOrganizationID, app.Owner, app.OrganizationID); email != "" {
			status.ClaudeAccountEmail = email
			status.ClaudeAccountName = name
		}
	}
	if sub.OwnerType == types.OwnerTypeUser {
		status.SubscriptionOwnerName = apiServer.displayNameForUser(req.Context(), sub.OwnerID)
	} else if org, err := apiServer.lookupOrg(req.Context(), sub.OwnerID); err == nil && org != nil {
		status.SubscriptionOwnerName = org.Name
	}
	status.Status = sub.Status
	status.LastValidatedAt = sub.LastValidatedAt
	status.LastError = sub.LastError
	return status, nil
}

// claudeIdentityForOrg finds the account email already known for an Anthropic
// organization uuid, looking only at subscriptions the caller's context can
// already see (the app owner's own, then the app's org). Best-effort — returns
// empty strings when nothing matches.
func (apiServer *HelixAPIServer) claudeIdentityForOrg(ctx context.Context, claudeOrgID, ownerID, orgID string) (string, string) {
	if claudeOrgID == "" {
		return "", ""
	}
	scopes := []string{ownerID}
	if orgID != "" {
		scopes = append(scopes, orgID)
	}
	for _, scope := range scopes {
		subs, err := apiServer.Store.ListClaudeSubscriptions(ctx, scope)
		if err != nil {
			continue
		}
		for _, candidate := range subs {
			if candidate.ClaudeOrganizationID == claudeOrgID && candidate.AccountEmail != "" {
				return candidate.AccountEmail, candidate.AccountDisplayName
			}
		}
	}
	return "", ""
}

// displayNameForUser returns a human-readable label for a user id, preferring
// email then full name then the raw id. Best-effort — never errors.
func (apiServer *HelixAPIServer) displayNameForUser(ctx context.Context, userID string) string {
	if userID == "" {
		return ""
	}
	u, err := apiServer.Store.GetUser(ctx, &store.GetUserQuery{ID: userID})
	if err != nil || u == nil {
		return userID
	}
	if u.Email != "" {
		return u.Email
	}
	if u.FullName != "" {
		return u.FullName
	}
	return userID
}

// @Summary List Claude subscriptions
// @Description List Claude subscriptions for the current user and their org
// @Tags Claude
// @Produce json
// @Success 200 {array} types.ClaudeSubscription
// @Failure 401 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security BearerAuth
// @Router /api/v1/claude-subscriptions [get]
func (apiServer *HelixAPIServer) listClaudeSubscriptions(_ http.ResponseWriter, req *http.Request) ([]*types.ClaudeSubscription, *system.HTTPError) {
	user := getRequestUser(req)
	if user == nil {
		return nil, system.NewHTTPError401("authentication required")
	}

	// Get user's own subscriptions
	subs, err := apiServer.Store.ListClaudeSubscriptions(req.Context(), user.ID)
	if err != nil {
		return nil, system.NewHTTPError500("failed to list subscriptions: " + err.Error())
	}

	// Also get org subscriptions for any orgs the user belongs to
	memberships, err := apiServer.Store.ListOrganizationMemberships(req.Context(), &store.ListOrganizationMembershipsQuery{
		UserID: user.ID,
	})
	if err == nil {
		for _, m := range memberships {
			orgSubs, err := apiServer.Store.ListClaudeSubscriptions(req.Context(), m.OrganizationID)
			if err != nil {
				log.Warn().Err(err).Str("org_id", m.OrganizationID).Msg("Failed to list org Claude subscriptions")
				continue
			}
			subs = append(subs, orgSubs...)
		}
	}

	return subs, nil
}

// @Summary Get a Claude subscription
// @Description Get details of a specific Claude subscription (no secrets)
// @Tags Claude
// @Produce json
// @Param id path string true "Subscription ID"
// @Success 200 {object} types.ClaudeSubscription
// @Failure 401 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Security BearerAuth
// @Router /api/v1/claude-subscriptions/{id} [get]
func (apiServer *HelixAPIServer) getClaudeSubscription(_ http.ResponseWriter, req *http.Request) (*types.ClaudeSubscription, *system.HTTPError) {
	user := getRequestUser(req)
	if user == nil {
		return nil, system.NewHTTPError401("authentication required")
	}

	vars := mux.Vars(req)
	id := vars["id"]

	sub, err := apiServer.Store.GetClaudeSubscription(req.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, system.NewHTTPError404("subscription not found")
		}
		return nil, system.NewHTTPError500("failed to get subscription: " + err.Error())
	}

	// Verify ownership
	if sub.OwnerType == types.OwnerTypeUser && sub.OwnerID != user.ID {
		return nil, system.NewHTTPError403("access denied")
	}
	if sub.OwnerType == types.OwnerTypeOrg {
		if _, err := apiServer.authorizeOrgOwner(req.Context(), user, sub.OwnerID); err != nil {
			return nil, system.NewHTTPError403("access denied")
		}
	}

	return sub, nil
}

// @Summary Delete a Claude subscription
// @Description Disconnect a Claude subscription
// @Tags Claude
// @Param id path string true "Subscription ID"
// @Success 200 {object} map[string]string
// @Failure 401 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Security BearerAuth
// @Router /api/v1/claude-subscriptions/{id} [delete]
// UpdateClaudeSubscriptionDelegationRequest sets which organizations may run
// agents on this subscription for its owner.
type UpdateClaudeSubscriptionDelegationRequest struct {
	DelegatedOrgIDs []string `json:"delegated_org_ids"`
}

// @Summary Set which orgs may use a Claude subscription for delegated agent runs
// @Description Grant (or revoke) permission for an organization's orchestrated agents to authenticate as the subscription owner. Only the subscription owner may change this.
// @Tags Claude
// @Accept json
// @Produce json
// @Param id path string true "Subscription ID"
// @Param body body UpdateClaudeSubscriptionDelegationRequest true "Delegated organizations"
// @Success 200 {object} types.ClaudeSubscription
// @Failure 400 {object} system.HTTPError
// @Failure 401 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Security BearerAuth
// @Router /api/v1/claude-subscriptions/{id}/delegation [put]
func (apiServer *HelixAPIServer) updateClaudeSubscriptionDelegation(_ http.ResponseWriter, req *http.Request) (*types.ClaudeSubscription, *system.HTTPError) {
	user := getRequestUser(req)
	if user == nil {
		return nil, system.NewHTTPError401("authentication required")
	}

	sub, err := apiServer.Store.GetClaudeSubscription(req.Context(), mux.Vars(req)["id"])
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, system.NewHTTPError404("subscription not found")
		}
		return nil, system.NewHTTPError500("failed to get subscription: " + err.Error())
	}

	// Delegation lends out the owner's own Claude quota, so ONLY the owner may
	// grant it — not an org admin, not the person who would benefit.
	if sub.OwnerType != types.OwnerTypeUser || sub.OwnerID != user.ID {
		return nil, system.NewHTTPError403("only the subscription owner can change delegation")
	}

	var body UpdateClaudeSubscriptionDelegationRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		return nil, system.NewHTTPError400("invalid request body: " + err.Error())
	}

	// You can only delegate to an org you actually belong to.
	memberships, err := apiServer.Store.ListOrganizationMemberships(req.Context(), &store.ListOrganizationMembershipsQuery{UserID: user.ID})
	if err != nil {
		return nil, system.NewHTTPError500("failed to resolve org memberships: " + err.Error())
	}
	member := make(map[string]bool, len(memberships))
	for _, m := range memberships {
		member[m.OrganizationID] = true
	}
	granted := make([]string, 0, len(body.DelegatedOrgIDs))
	for _, orgID := range body.DelegatedOrgIDs {
		if !member[orgID] {
			return nil, system.NewHTTPError403("not a member of organization " + orgID)
		}
		granted = append(granted, orgID)
	}

	sub.DelegatedOrgIDs = granted
	updated, err := apiServer.Store.UpdateClaudeSubscription(req.Context(), sub)
	if err != nil {
		return nil, system.NewHTTPError500("failed to update subscription: " + err.Error())
	}

	log.Info().
		Str("subscription_id", sub.ID).
		Str("owner_id", sub.OwnerID).
		Strs("delegated_org_ids", granted).
		Msg("Updated Claude subscription delegation")

	return updated, nil
}

func (apiServer *HelixAPIServer) deleteClaudeSubscription(_ http.ResponseWriter, req *http.Request) (map[string]string, *system.HTTPError) {
	user := getRequestUser(req)
	if user == nil {
		return nil, system.NewHTTPError401("authentication required")
	}

	vars := mux.Vars(req)
	id := vars["id"]

	sub, err := apiServer.Store.GetClaudeSubscription(req.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, system.NewHTTPError404("subscription not found")
		}
		return nil, system.NewHTTPError500("failed to get subscription: " + err.Error())
	}

	// Verify ownership
	if sub.OwnerType == types.OwnerTypeUser && sub.OwnerID != user.ID {
		return nil, system.NewHTTPError403("access denied")
	}
	if sub.OwnerType == types.OwnerTypeOrg {
		if _, err := apiServer.authorizeOrgOwner(req.Context(), user, sub.OwnerID); err != nil {
			return nil, system.NewHTTPError403("access denied")
		}
	}

	if err := apiServer.Store.DeleteClaudeSubscription(req.Context(), id); err != nil {
		return nil, system.NewHTTPError500("failed to delete subscription: " + err.Error())
	}

	log.Info().
		Str("subscription_id", id).
		Str("user_id", user.ID).
		Msg("Deleted Claude subscription")

	return map[string]string{"status": "ok"}, nil
}

// ClaudeModel represents a Claude model available via Claude Code
type ClaudeModel struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// @Summary List available Claude models
// @Description List Claude models available through Claude Code subscriptions
// @Tags Claude
// @Produce json
// @Success 200 {array} ClaudeModel
// @Security BearerAuth
// @Router /api/v1/claude-subscriptions/models [get]
func (apiServer *HelixAPIServer) listClaudeModels(_ http.ResponseWriter, req *http.Request) ([]*ClaudeModel, *system.HTTPError) {
	models := []*ClaudeModel{
		{ID: "claude-opus-5", Name: "Claude Opus 5 (1M context)", Description: "Recommended Opus model with a 1M-token context window"},
		{ID: "claude-fable-5", Name: "Claude Fable 5 (1M context)", Description: "Most capable generally available Claude model"},
		{ID: "claude-opus-4-8", Name: "Claude Opus 4.8 (1M context)", Description: "Previous Opus model with a 1M-token context window"},
		{ID: "sonnet", Name: "Claude Sonnet (latest)", Description: "Best balance of speed and capability"},
		{ID: "haiku", Name: "Claude Haiku (latest)", Description: "Fastest Claude model"},
	}
	return models, nil
}

// SessionClaudeCredentialsResponse returns credentials in the appropriate format.
type SessionClaudeCredentialsResponse struct {
	CredentialType string                        `json:"credential_type"` // "oauth" or "setup_token"
	OAuthCreds     *types.ClaudeOAuthCredentials `json:"oauth_credentials,omitempty"`
	SetupToken     string                        `json:"setup_token,omitempty"`
}

// @Summary Get Claude credentials for a session
// @Description Get decrypted Claude credentials for use inside a desktop container.
// @Description Only accepts runner/session-scoped tokens.
// @Tags Claude
// @Produce json
// @Param id path string true "Session ID"
// @Success 200 {object} SessionClaudeCredentialsResponse
// @Failure 401 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security ApiKeyAuth
// @Router /api/v1/sessions/{id}/claude-credentials [get]
func (apiServer *HelixAPIServer) getSessionClaudeCredentials(_ http.ResponseWriter, req *http.Request) (*SessionClaudeCredentialsResponse, *system.HTTPError) {
	ctx := req.Context()
	vars := mux.Vars(req)
	sessionID := vars["id"]

	session, err := apiServer.Store.GetSession(ctx, sessionID)
	if err != nil {
		return nil, system.NewHTTPError404("session not found")
	}

	user := getRequestUser(req)
	if user == nil {
		return nil, system.NewHTTPError403("access denied")
	}
	if user.TokenType != types.TokenTypeRunner && session.Owner != user.ID {
		return nil, system.NewHTTPError403("access denied")
	}

	// Resolve exactly as the desktop does, so a delegated credential owner is
	// honoured here too — and so a refresh pushed back lands on the SAME
	// subscription that was handed out, not the session owner's.
	sub, err := apiServer.Store.GetSessionClaudeSubscription(ctx, session)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, system.NewHTTPError404("no Claude subscription found for session owner")
		}
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to get Claude subscription: %v", err))
	}

	encKey, err := crypto.GetEncryptionKey()
	if err != nil {
		return nil, system.NewHTTPError500("failed to get encryption key")
	}

	plaintext, err := crypto.DecryptAES256GCM(sub.EncryptedCredentials, encKey)
	if err != nil {
		return nil, system.NewHTTPError500("failed to decrypt credentials")
	}

	credType := sub.CredentialType
	if credType == "" {
		credType = "oauth" // backward compatibility
	}

	if credType == "setup_token" {
		var tokenCreds types.ClaudeSetupTokenCredentials
		if err := json.Unmarshal(plaintext, &tokenCreds); err != nil {
			return nil, system.NewHTTPError500("failed to parse credentials")
		}
		return &SessionClaudeCredentialsResponse{
			CredentialType: "setup_token",
			SetupToken:     tokenCreds.SetupToken,
		}, nil
	}

	var creds types.ClaudeOAuthCredentials
	if err := json.Unmarshal(plaintext, &creds); err != nil {
		return nil, system.NewHTTPError500("failed to parse credentials")
	}
	return &SessionClaudeCredentialsResponse{
		CredentialType: "oauth",
		OAuthCreds:     &creds,
	}, nil
}

// @Summary Update Claude credentials for a session
// @Description Push refreshed Claude OAuth credentials back to the API (e.g. after Claude Code refreshes its token).
// @Description Only accepts runner/session-scoped tokens.
// @Tags Claude
// @Accept json
// @Produce json
// @Param id path string true "Session ID"
// @Param body body types.ClaudeOAuthCredentials true "Refreshed credentials"
// @Success 200 {object} map[string]string
// @Failure 401 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security ApiKeyAuth
// @Router /api/v1/sessions/{id}/claude-credentials [put]
func (apiServer *HelixAPIServer) updateSessionClaudeCredentials(_ http.ResponseWriter, req *http.Request) (map[string]string, *system.HTTPError) {
	ctx := req.Context()
	vars := mux.Vars(req)
	sessionID := vars["id"]

	// Get session
	session, err := apiServer.Store.GetSession(ctx, sessionID)
	if err != nil {
		return nil, system.NewHTTPError404("session not found")
	}

	// Only allow runner token or session owner (same pattern as getSessionClaudeCredentials)
	user := getRequestUser(req)
	if user == nil {
		return nil, system.NewHTTPError403("access denied")
	}
	if user.TokenType != types.TokenTypeRunner && session.Owner != user.ID {
		return nil, system.NewHTTPError403("access denied")
	}

	// Parse the refreshed credentials from the request body
	var creds types.ClaudeOAuthCredentials
	if err := json.NewDecoder(req.Body).Decode(&creds); err != nil {
		return nil, system.NewHTTPError400("invalid request body: " + err.Error())
	}
	if creds.AccessToken == "" || creds.RefreshToken == "" {
		return nil, system.NewHTTPError400("accessToken and refreshToken are required")
	}

	// Look up the effective subscription for this session's owner
	// Resolve exactly as the desktop does, so a delegated credential owner is
	// honoured here too — and so a refresh pushed back lands on the SAME
	// subscription that was handed out, not the session owner's.
	sub, err := apiServer.Store.GetSessionClaudeSubscription(ctx, session)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, system.NewHTTPError404("no Claude subscription found for session owner")
		}
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to get Claude subscription: %v", err))
	}

	// Re-encrypt the updated credentials
	credJSON, err := json.Marshal(creds)
	if err != nil {
		return nil, system.NewHTTPError500("failed to marshal credentials")
	}

	encKey, err := crypto.GetEncryptionKey()
	if err != nil {
		return nil, system.NewHTTPError500("failed to get encryption key")
	}

	encrypted, err := crypto.EncryptAES256GCM(credJSON, encKey)
	if err != nil {
		return nil, system.NewHTTPError500("failed to encrypt credentials")
	}

	// Update the subscription
	sub.EncryptedCredentials = encrypted
	if creds.ExpiresAt > 0 {
		sub.AccessTokenExpiresAt = time.UnixMilli(creds.ExpiresAt)
	}
	now := time.Now()
	sub.LastRefreshedAt = &now

	if _, err := apiServer.Store.UpdateClaudeSubscription(ctx, sub); err != nil {
		return nil, system.NewHTTPError500("failed to update subscription: " + err.Error())
	}

	log.Info().
		Str("session_id", sessionID).
		Str("subscription_id", sub.ID).
		Time("expires_at", sub.AccessTokenExpiresAt).
		Msg("Updated Claude subscription credentials from session")

	return map[string]string{"status": "ok"}, nil
}

// ClaudeLoginSessionResponse is returned when starting a Claude login session
type ClaudeLoginSessionResponse struct {
	SessionID string `json:"session_id"`
}

// @Summary Start a Claude login session
// @Description Launch a temporary desktop session for interactive Claude OAuth login
// @Tags Claude
// @Produce json
// @Success 200 {object} ClaudeLoginSessionResponse
// @Failure 401 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security BearerAuth
// @Router /api/v1/claude-subscriptions/start-login [post]
func (apiServer *HelixAPIServer) startClaudeLogin(_ http.ResponseWriter, req *http.Request) (*ClaudeLoginSessionResponse, *system.HTTPError) {
	user := getRequestUser(req)
	if user == nil {
		return nil, system.NewHTTPError401("authentication required")
	}

	// Get user's org ID (use first org membership)
	orgID := ""
	memberships, err := apiServer.Store.ListOrganizationMemberships(req.Context(), &store.ListOrganizationMembershipsQuery{
		UserID: user.ID,
	})
	if err == nil && len(memberships) > 0 {
		orgID = memberships[0].OrganizationID
	}

	// Create a minimal session for the login flow
	session := &types.Session{
		ID:             system.GenerateSessionID(),
		Name:           claudeLoginSessionName,
		Created:        time.Now(),
		Updated:        time.Now(),
		Mode:           types.SessionModeInference,
		Type:           types.SessionTypeText,
		Provider:       claudeLoginSessionProvider,
		ModelName:      "external_agent",
		Owner:          user.ID,
		OwnerType:      types.OwnerTypeUser,
		OrganizationID: orgID,
		Metadata: types.SessionMetadata{
			Stream:      true,
			AgentType:   "zed_external",
			SessionRole: "exploratory",
		},
	}

	createdSession, err := apiServer.Store.CreateSession(req.Context(), *session)
	if err != nil {
		log.Error().Err(err).Str("user_id", user.ID).Msg("Failed to create Claude login session")
		return nil, system.NewHTTPError500("failed to create session")
	}

	// Create a desktop agent with minimal configuration
	// HELIX_SKIP_ZED=1 prevents the workspace setup terminal from launching
	zedAgent := &types.DesktopAgent{
		OrganizationID: orgID,
		SessionID:      createdSession.ID,
		UserID:         user.ID,
		Input:          "Claude Code login",
		ProjectPath:    "workspace",
		DisplayWidth:   1920,
		DisplayHeight:  1080,
		DesktopType:    "ubuntu",
		Env:            []string{"HELIX_SKIP_ZED=1"},
	}

	// Add user's API token inside session lock via OnBeforeCreate hook
	claudeUserID := user.ID
	zedAgent.OnBeforeCreate = func(hookCtx context.Context, a *types.DesktopAgent) error {
		return apiServer.addUserAPITokenToAgent(hookCtx, a, claudeUserID)
	}

	// Start the desktop container
	_, startErr := apiServer.externalAgentExecutor.StartDesktop(req.Context(), zedAgent)
	if startErr != nil {
		log.Error().Err(startErr).Str("session_id", createdSession.ID).Msg("Failed to start Claude login desktop")
		return nil, system.NewHTTPError500("failed to start desktop session")
	}

	// Re-fetch session to pick up ContainerName/ExternalAgentStatus set by StartDesktop
	// (StartDesktop updates the DB session internally; using the stale createdSession
	// would overwrite those fields)
	if freshSession, fetchErr := apiServer.Store.GetSession(req.Context(), createdSession.ID); fetchErr == nil {
		createdSession = freshSession
	}

	log.Info().
		Str("session_id", createdSession.ID).
		Str("user_id", user.ID).
		Msg("Started Claude login desktop session")

	return &ClaudeLoginSessionResponse{
		SessionID: createdSession.ID,
	}, nil
}

// ClaudePollLoginResponse is returned when polling for Claude credentials
type ClaudePollLoginResponse struct {
	Found       bool   `json:"found"`
	Credentials string `json:"credentials,omitempty"` // Raw credentials JSON
	URL         string `json:"url,omitempty"`         // OAuth URL for native browser
}

// @Summary Poll for Claude login credentials
// @Description Check if Claude credentials file has been written inside the desktop container
// @Tags Claude
// @Produce json
// @Param sessionId path string true "Session ID"
// @Success 200 {object} ClaudePollLoginResponse
// @Failure 401 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security BearerAuth
// @Router /api/v1/claude-subscriptions/poll-login/{sessionId} [get]
func (apiServer *HelixAPIServer) pollClaudeLogin(_ http.ResponseWriter, req *http.Request) (*ClaudePollLoginResponse, *system.HTTPError) {
	user := getRequestUser(req)
	if user == nil {
		return nil, system.NewHTTPError401("authentication required")
	}

	vars := mux.Vars(req)
	sessionID := vars["sessionId"]

	// Verify session ownership
	session, err := apiServer.Store.GetSession(req.Context(), sessionID)
	if err != nil {
		return nil, system.NewHTTPError404("session not found")
	}
	if session.Owner != user.ID {
		return nil, system.NewHTTPError403("access denied")
	}
	if !isTemporarySubscriptionLoginSession(session, claudeLoginSessionName, claudeLoginSessionProvider) {
		return nil, system.NewHTTPError404("login session not found")
	}

	runnerID := fmt.Sprintf("desktop-%s", sessionID)

	// Check for credentials first (takes priority over URL)
	credOutput, credErr := apiServer.execInContainer(req.Context(), runnerID,
		[]string{"cat", "/home/retro/.claude/.credentials.json"})
	if credErr == nil && credOutput != "" {
		var credCheck map[string]interface{}
		if err := json.Unmarshal([]byte(credOutput), &credCheck); err == nil {
			// Check for either claudeAiOauth wrapper or direct accessToken
			if _, ok := credCheck["claudeAiOauth"]; ok {
				return &ClaudePollLoginResponse{Found: true, Credentials: credOutput}, nil
			}
			if _, ok := credCheck["accessToken"]; ok {
				return &ClaudePollLoginResponse{Found: true, Credentials: credOutput}, nil
			}
		}
	}

	// No credentials yet — check for OAuth URL from claude auth login stdout.
	// The stdout contains a fallback URL with platform.claude.com/oauth/code/callback
	// redirect that works from any browser. We parse it from the "If the browser
	// didn't open, visit:" line. The wrapper script sets NO_COLOR=1 to strip ANSI codes.
	stdoutOutput, stdoutErr := apiServer.execInContainer(req.Context(), runnerID,
		[]string{"cat", "/tmp/claude-auth-stdout.txt"})
	if stdoutErr == nil && stdoutOutput != "" {
		for _, line := range strings.Split(stdoutOutput, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "If the browser didn't open, visit:") {
				url := strings.TrimSpace(strings.TrimPrefix(line, "If the browser didn't open, visit:"))
				if strings.HasPrefix(url, "https://") {
					return &ClaudePollLoginResponse{Found: false, URL: url}, nil
				}
			}
			if strings.HasPrefix(line, "https://claude.ai/oauth") {
				return &ClaudePollLoginResponse{Found: false, URL: line}, nil
			}
		}
	}

	// Fallback: read the URL captured by helix-capture-browser via BROWSER env var.
	// This URL has a localhost redirect that only works inside the container, but
	// it's better than nothing if the stdout fallback message is suppressed.
	urlOutput, urlErr := apiServer.execInContainer(req.Context(), runnerID,
		[]string{"cat", "/tmp/claude-auth-url.txt"})
	if urlErr == nil && strings.HasPrefix(strings.TrimSpace(urlOutput), "https://") {
		return &ClaudePollLoginResponse{Found: false, URL: strings.TrimSpace(urlOutput)}, nil
	}

	return &ClaudePollLoginResponse{Found: false}, nil
}

// execInContainer runs a command inside a desktop container via RevDial and
// returns the stdout output. Returns an error if the command fails or the
// container is not reachable.
func (apiServer *HelixAPIServer) execInContainer(ctx context.Context, runnerID string, command []string) (string, error) {
	revDialConn, err := apiServer.connman.Dial(ctx, runnerID)
	if err != nil {
		return "", fmt.Errorf("container not ready: %w", err)
	}
	defer revDialConn.Close()

	execReq := map[string]interface{}{
		"command": command,
		"timeout": 5,
	}
	execBody, _ := json.Marshal(execReq)

	httpReq, err := http.NewRequest("POST", "http://localhost:9876/exec", bytes.NewReader(execBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	if err := httpReq.Write(revDialConn); err != nil {
		return "", fmt.Errorf("failed to write request: %w", err)
	}

	execResp, err := http.ReadResponse(bufio.NewReader(revDialConn), httpReq)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}
	defer execResp.Body.Close()

	bodyBytes, err := io.ReadAll(execResp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read body: %w", err)
	}

	var result struct {
		Success  bool   `json:"success"`
		Output   string `json:"output"`
		ExitCode int    `json:"exit_code"`
	}
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if !result.Success || result.ExitCode != 0 {
		return "", fmt.Errorf("command failed: exit %d", result.ExitCode)
	}

	return result.Output, nil
}
