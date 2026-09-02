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
	return apiServer.createClaudeSubscriptionFrom(req.Context(), user, createReq)
}

// createClaudeSubscriptionFrom is the shared connect path: owner resolution,
// credential encryption, liveness probe and harness enablement. Both the REST
// create endpoint and the OAuth login completion go through it.
func (apiServer *HelixAPIServer) createClaudeSubscriptionFrom(ctx context.Context, user *types.User, createReq types.CreateClaudeSubscriptionRequest) (*types.ClaudeSubscription, *system.HTTPError) {

	// Determine owner
	ownerID := user.ID
	ownerType := types.OwnerTypeUser
	if createReq.OwnerType == types.OwnerTypeOrg {
		if createReq.OwnerID == "" {
			return nil, system.NewHTTPError400("owner_id required for org-level subscriptions")
		}
		org, err := apiServer.lookupOrg(ctx, createReq.OwnerID)
		if err != nil {
			return nil, system.NewHTTPError404("organization not found")
		}
		_, err = apiServer.authorizeOrgOwner(ctx, user, org.ID)
		if err != nil {
			return nil, system.NewHTTPError403("not authorized to manage org subscriptions: " + err.Error())
		}
		ownerID = org.ID
		ownerType = types.OwnerTypeOrg
	}
	harnessOrgID := ""
	if createReq.OrganizationID != "" {
		org, err := apiServer.lookupOrg(ctx, createReq.OrganizationID)
		if err != nil {
			return nil, system.NewHTTPError404("organization not found")
		}
		if _, err := apiServer.authorizeOrgOwner(ctx, user, org.ID); err != nil {
			return nil, system.NewHTTPError403("not authorized to enable organization harness")
		}
		harnessOrgID = org.ID
	} else if ownerType == types.OwnerTypeOrg {
		harnessOrgID = ownerID
	}

	var encrypted string
	var credentialType string
	var expiresAt time.Time
	var refreshTokenExpiresAt time.Time
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
		// The credentials file carries this, and the PKCE exchange fills it in
		// too. It is the deadline the user actually needs warning about.
		if creds.RefreshTokenExpiresAt > 0 {
			refreshTokenExpiresAt = time.UnixMilli(creds.RefreshTokenExpiresAt)
		}
	}

	// Re-authentication replaces credentials, not the owner's explicit sharing consent.
	existingSubs, _ := apiServer.Store.ListClaudeSubscriptions(ctx, ownerID)
	var delegatedOrgIDs []string
	for _, old := range existingSubs {
		if old.OwnerType == ownerType {
			delegatedOrgIDs = append(delegatedOrgIDs, old.DelegatedOrgIDs...)
			_ = apiServer.Store.DeleteClaudeSubscription(ctx, old.ID)
			log.Info().Str("old_subscription_id", old.ID).Msg("Deleted old Claude subscription on re-auth")
		}
	}

	sub := &types.ClaudeSubscription{
		OwnerID:               ownerID,
		OwnerType:             ownerType,
		Name:                  createReq.Name,
		EncryptedCredentials:  encrypted,
		CredentialType:        credentialType,
		SubscriptionType:      subscriptionType,
		RateLimitTier:         rateLimitTier,
		Scopes:                scopes,
		AccessTokenExpiresAt:  expiresAt,
		RefreshTokenExpiresAt: refreshTokenExpiresAt,
		Status:                "active",
		CreatedBy:             user.ID,
		DelegatedOrgIDs:       delegatedOrgIDs,
	}

	created, err := apiServer.Store.CreateClaudeSubscription(ctx, sub)
	if err != nil {
		return nil, system.NewHTTPError500("failed to create subscription: " + err.Error())
	}

	// Liveness-probe the freshly connected credentials so Status reflects reality
	// instead of an unconditional "active". A dead token is caught here rather
	// than at the first agent turn.
	created = apiServer.revalidateClaudeSubscription(ctx, created)
	if created.Status == "active" {
		if err := apiServer.enableSubscriptionCodeAgentHarness(ctx, harnessOrgID, user.ID, types.CodeAgentRuntimeClaudeCode); err != nil {
			if deleteErr := apiServer.Store.DeleteClaudeSubscription(context.WithoutCancel(ctx), created.ID); deleteErr != nil {
				log.Error().Err(deleteErr).Str("subscription_id", created.ID).Msg("failed to roll back Claude subscription")
			}
			return nil, system.NewHTTPError500("failed to enable Claude Code harness: " + err.Error())
		}
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
				// Guarded like the plan fields below: FetchClaudeProfile succeeds on
				// any parseable 200, so a partial body must not clear an identity
				// we already have.
				if profile.AccountEmail != "" {
					sub.AccountEmail = profile.AccountEmail
				}
				if profile.AccountDisplayName != "" {
					sub.AccountDisplayName = profile.AccountDisplayName
				}
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
	// Status and identity only. This function holds its copy of the row across a
	// probe and a profile fetch — up to ~18s — during which a container may push
	// refreshed credentials. Writing the whole row back would revert them, and
	// since Anthropic rotates the refresh token on every use, reverting a
	// credential resurrects a dead token and bricks the subscription.
	if err := apiServer.Store.UpdateClaudeSubscriptionStatus(ctx, sub); err != nil {
		log.Warn().Err(err).Str("subscription_id", sub.ID).Msg("failed to persist Claude subscription validation result")
	}
	return sub
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
	ClaudeOrganizationID string `json:"claude_organization_id,omitempty"`
	// RefreshTokenExpiresAt is when the user must sign in again. Refreshing
	// keeps the access token alive but does not move this, so it is the only
	// honest basis for an expiry warning.
	RefreshTokenExpiresAt *time.Time `json:"refresh_token_expires_at,omitempty"`
	Status                string     `json:"status,omitempty"`
	LastValidatedAt       *time.Time `json:"last_validated_at,omitempty"`
	LastError             string     `json:"last_error,omitempty"`
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
	if !sub.RefreshTokenExpiresAt.IsZero() {
		expiry := sub.RefreshTokenExpiresAt
		status.RefreshTokenExpiresAt = &expiry
	}
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
	DelegatedOrgIDs      []string `json:"delegated_org_ids"`
	SwitchToSubscription bool     `json:"switch_to_subscription,omitempty"`
}

// @Summary Set which orgs may use a Claude subscription for delegated agent runs
// @Description Grant (or revoke) permission for an organization's orchestrated agents to authenticate as the subscription owner. Sharing with an owned organization also enables Claude Code subscription mode there. Only the subscription owner may change this.
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
// @Failure 409 {object} system.HTTPError
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

	harnesses := make(map[string]*types.OrgCodeAgentHarness, len(granted))
	for _, orgID := range granted {
		if _, err := apiServer.authorizeOrgOwner(req.Context(), user, orgID); err != nil {
			return nil, system.NewHTTPError403("only an organization owner can enable Claude Code for " + orgID)
		}
		harness, err := apiServer.Store.GetOrgCodeAgentHarness(req.Context(), orgID, types.CodeAgentRuntimeClaudeCode)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return nil, system.NewHTTPError500("failed to resolve Claude Code settings: " + err.Error())
		}
		if err == nil && claudeHarnessUsesAPIProviderMode(harness) && !body.SwitchToSubscription {
			return nil, system.NewHTTPError409("organization " + orgID + " is using API-provider mode; confirm switching Claude Code to the subscription")
		}
		if err == nil && harness.SubscriptionEnabled != nil && *harness.SubscriptionEnabled {
			continue
		}
		harnesses[orgID] = harness
	}
	updated, err := apiServer.Store.UpdateClaudeSubscriptionDelegation(req.Context(), sub.ID, granted)
	if err != nil {
		var conflict *store.ClaudeSubscriptionDelegationConflictError
		if errors.As(err, &conflict) {
			org, orgErr := apiServer.lookupOrg(req.Context(), conflict.OrganizationID)
			if orgErr != nil {
				return nil, system.NewHTTPError500("failed to resolve organization name")
			}
			owner, ownerErr := apiServer.Store.GetUser(req.Context(), &store.GetUserQuery{ID: conflict.OwnerID})
			if ownerErr != nil {
				return nil, system.NewHTTPError500("failed to resolve subscription owner")
			}
			orgName := org.DisplayName
			if orgName == "" {
				orgName = org.Name
			}
			ownerName := userDisplayName(owner)
			return nil, system.NewHTTPError409(fmt.Sprintf(
				"%s already uses %s's Claude subscription. Ask %s to remove that delegation before adding yours.",
				orgName, ownerName, ownerName,
			))
		}
		return nil, system.NewHTTPError500("failed to update subscription: " + err.Error())
	}
	for orgID := range harnesses {
		if err := apiServer.enableSubscriptionCodeAgentHarness(req.Context(), orgID, user.ID, types.CodeAgentRuntimeClaudeCode); err != nil {
			return nil, system.NewHTTPError500(err.Error())
		}
	}

	log.Info().
		Str("subscription_id", sub.ID).
		Str("owner_id", sub.OwnerID).
		Strs("delegated_org_ids", granted).
		Msg("Updated Claude subscription delegation")

	return updated, nil
}

// @Summary Delete a Claude subscription
// @Description Disconnect a Claude subscription owned by the current user, or by an organization they own
// @Tags Claude
// @Produce json
// @Param id path string true "Subscription ID"
// @Success 200 {object} map[string]string
// @Failure 401 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Security BearerAuth
// @Router /api/v1/claude-subscriptions/{id} [delete]
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

	// Same guarded write the background refresher uses: whichever refresh
	// happened later wins, so a container and the refresher cannot revert each
	// other's rotated token.
	expiresAt := time.Time{}
	if creds.ExpiresAt > 0 {
		expiresAt = time.UnixMilli(creds.ExpiresAt)
	}
	refreshExpiresAt := time.Time{}
	if creds.RefreshTokenExpiresAt > 0 {
		refreshExpiresAt = time.UnixMilli(creds.RefreshTokenExpiresAt)
	}
	now := time.Now()
	stored, err := apiServer.Store.UpdateClaudeSubscriptionCredentialsIfNewer(ctx, sub.ID, encrypted, expiresAt, refreshExpiresAt, now)
	if err != nil {
		return nil, system.NewHTTPError500("failed to update subscription: " + err.Error())
	}
	if !stored {
		// Newer credentials already landed; the daemon retries with its own copy.
		return map[string]string{"status": "stale"}, nil
	}
	sub.AccessTokenExpiresAt = expiresAt

	log.Info().
		Str("session_id", sessionID).
		Str("subscription_id", sub.ID).
		Time("expires_at", sub.AccessTokenExpiresAt).
		Msg("Updated Claude subscription credentials from session")

	return map[string]string{"status": "ok"}, nil
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

// ClaudeLoginStartResponse hands the browser everything it needs to run the
// PKCE flow. The verifier goes to the browser rather than being parked
// server-side: the user's session is the OAuth client here, so keeping it in
// API memory would add no security and would break across API replicas.
type ClaudeLoginStartResponse struct {
	AuthorizeURL string `json:"authorize_url"`
	CodeVerifier string `json:"code_verifier"`
	State        string `json:"state"`
}

// @Summary Start a Claude subscription login
// @Description Build the Anthropic authorization URL and PKCE material for connecting a Claude subscription
// @Tags Claude
// @Produce json
// @Success 200 {object} ClaudeLoginStartResponse
// @Failure 401 {object} system.HTTPError
// @Security BearerAuth
// @Router /api/v1/claude-subscriptions/oauth/start [post]
func (apiServer *HelixAPIServer) startClaudeOAuthLogin(_ http.ResponseWriter, req *http.Request) (*ClaudeLoginStartResponse, *system.HTTPError) {
	if getRequestUser(req) == nil {
		return nil, system.NewHTTPError401("authentication required")
	}
	challenge, err := anthropic.StartClaudeLogin()
	if err != nil {
		return nil, system.NewHTTPError500("failed to start Claude login: " + err.Error())
	}
	return &ClaudeLoginStartResponse{
		AuthorizeURL: challenge.AuthorizeURL,
		CodeVerifier: challenge.CodeVerifier,
		State:        challenge.State,
	}, nil
}

// CompleteClaudeLoginRequest carries the code the user copied from Anthropic
// back with the PKCE material the login started with.
type CompleteClaudeLoginRequest struct {
	Code         string `json:"code"`
	CodeVerifier string `json:"code_verifier"`
	State        string `json:"state"`
	// Same ownership knobs as a direct create.
	Name           string          `json:"name,omitempty"`
	OwnerType      types.OwnerType `json:"owner_type,omitempty"`
	OwnerID        string          `json:"owner_id,omitempty"`
	OrganizationID string          `json:"organization_id,omitempty"`
}

// @Summary Complete a Claude subscription login
// @Description Exchange the pasted authorization code for tokens and connect the subscription
// @Tags Claude
// @Accept json
// @Produce json
// @Param body body CompleteClaudeLoginRequest true "Authorization code and PKCE material"
// @Success 200 {object} types.ClaudeSubscription
// @Failure 400 {object} system.HTTPError
// @Failure 401 {object} system.HTTPError
// @Security BearerAuth
// @Router /api/v1/claude-subscriptions/oauth/complete [post]
func (apiServer *HelixAPIServer) completeClaudeOAuthLogin(_ http.ResponseWriter, req *http.Request) (*types.ClaudeSubscription, *system.HTTPError) {
	user := getRequestUser(req)
	if user == nil {
		return nil, system.NewHTTPError401("authentication required")
	}
	var loginReq CompleteClaudeLoginRequest
	if err := json.NewDecoder(req.Body).Decode(&loginReq); err != nil {
		return nil, system.NewHTTPError400("invalid request body: " + err.Error())
	}

	// Anthropic's callback page hands back "<code>#<state>"; accept either form.
	code, pastedState := anthropic.SplitPastedCode(loginReq.Code)
	if code == "" {
		return nil, system.NewHTTPError400("paste the code Anthropic showed you after signing in")
	}
	state := loginReq.State
	if pastedState != "" {
		// The code carries the state it was issued against. If it disagrees with
		// the login we started, this code belongs to a different attempt.
		if state != "" && pastedState != state {
			return nil, system.NewHTTPError400("that code is from a different login attempt — start again")
		}
		state = pastedState
	}

	tokens, err := anthropic.ExchangeClaudeCode(req.Context(), code, loginReq.CodeVerifier, state)
	if err != nil {
		return nil, system.NewHTTPError400(err.Error())
	}

	createReq := types.CreateClaudeSubscriptionRequest{
		Name:           loginReq.Name,
		OwnerType:      loginReq.OwnerType,
		OwnerID:        loginReq.OwnerID,
		OrganizationID: loginReq.OrganizationID,
	}
	createReq.Credentials.ClaudeAiOauth = types.ClaudeOAuthCredentials{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		ExpiresAt:    tokens.ExpiresAt,
		// Carry the login deadline through. Dropping it left every
		// browser-connected subscription with no recorded expiry until a
		// background refresh happened to supply one.
		RefreshTokenExpiresAt: tokens.RefreshExpiresAt,
		Scopes:                tokens.Scopes,
	}
	return apiServer.createClaudeSubscriptionFrom(req.Context(), user, createReq)
}
