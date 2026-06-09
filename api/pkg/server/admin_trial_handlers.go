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
	stripeapi "github.com/stripe/stripe-go/v76"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

const (
	defaultTrialDays = 90
)

// ActivateTrialRequest is the body for POST /admin/users/{id}/trial-activate.
// All fields are optional; zero values use the defaults.
type ActivateTrialRequest struct {
	Days    int     `json:"days"`
	Credits float64 `json:"credits"`
}

// ActivateTrialResponse describes the outcome of a trial activation.
//
// Status values:
//   - "stashed": user has no orgs yet; trial intent is parked on the user and
//     will be applied when they create their first org.
//   - "applied": Stripe trial subscription was created on the user's oldest
//     owned org's wallet right now.
type ActivateTrialResponse struct {
	User   *types.User `json:"user"`
	OrgID  string      `json:"org_id,omitempty"`
	Status string      `json:"status"`
}

// consumeUserTrialIntent applies any admin-stashed trial intent on the user to
// the given org's wallet. Called after an org (and its owning membership) has
// been created. Best-effort: errors are logged but do not fail the caller, so
// a Stripe outage doesn't block org creation.
//
// On success: Stripe trial subscription is created on the wallet, wallet
// balance is topped up by the stashed credits, and the user's trial intent
// fields are cleared so subsequent orgs don't trigger again.
func (s *HelixAPIServer) consumeUserTrialIntent(ctx context.Context, user *types.User, orgID string) {
	if user == nil || user.TrialDaysOnFirstOrg == nil || *user.TrialDaysOnFirstOrg <= 0 {
		return
	}
	if !s.Cfg.Stripe.BillingEnabled {
		log.Warn().
			Str("user_id", user.ID).
			Str("org_id", orgID).
			Msg("user has stashed trial intent but billing is disabled; skipping")
		return
	}

	days := *user.TrialDaysOnFirstOrg
	credits := 0.0
	if user.TrialCreditsOnFirstOrg != nil {
		credits = *user.TrialCreditsOnFirstOrg
	}

	wallet, err := s.getOrCreateWallet(ctx, user, orgID)
	if err != nil {
		log.Warn().Err(err).
			Str("user_id", user.ID).
			Str("org_id", orgID).
			Msg("failed to get/create wallet for trial consumption")
		return
	}

	sub, err := s.Stripe.CreateTrialSubscription(ctx, wallet, days)
	if err != nil {
		log.Warn().Err(err).
			Str("user_id", user.ID).
			Str("org_id", orgID).
			Msg("failed to create stripe trial subscription")
		return
	}

	// Synchronously mirror the subscription state onto the wallet so the
	// frontend sees trialing immediately, without waiting for the webhook.
	wallet.StripeSubscriptionID = sub.ID
	wallet.SubscriptionStatus = sub.Status
	wallet.SubscriptionCurrentPeriodStart = sub.CurrentPeriodStart
	wallet.SubscriptionCurrentPeriodEnd = sub.CurrentPeriodEnd
	wallet.SubscriptionCreated = sub.Created
	wallet.SubscriptionCancelAtPeriodEnd = sub.CancelAtPeriodEnd
	if _, err := s.Store.UpdateWallet(ctx, wallet); err != nil {
		log.Warn().Err(err).
			Str("wallet_id", wallet.ID).
			Msg("failed to persist trial subscription state to wallet")
	}

	if credits > 0 {
		if _, err := s.Store.UpdateWalletBalance(ctx, wallet.ID, credits, types.TransactionMetadata{
			TransactionType: types.TransactionTypeSubscription,
		}); err != nil {
			log.Warn().Err(err).
				Str("wallet_id", wallet.ID).
				Float64("credits", credits).
				Msg("failed to top up wallet with trial credits")
		}
	}

	user.TrialDaysOnFirstOrg = nil
	user.TrialCreditsOnFirstOrg = nil
	if _, err := s.Store.UpdateUser(ctx, user); err != nil {
		log.Warn().Err(err).
			Str("user_id", user.ID).
			Msg("failed to clear user trial intent fields after consumption")
		return
	}

	log.Info().
		Str("user_id", user.ID).
		Str("org_id", orgID).
		Str("subscription_id", sub.ID).
		Int("days", days).
		Float64("credits", credits).
		Msg(fmt.Sprintf("admin-granted trial subscription created for %d days", days))
}

// adminActivateTrial godoc
// @Summary Activate a trial for a user (Admin, cloud only)
// @Description Stash a trial intent on the user, or immediately create a Stripe trial subscription on the user's oldest-owned org. Days defaults to 90; credits are taken verbatim from the request (0 means no admin top-up beyond what Stripe's subscription invoice contributes).
// @Tags    users
// @Accept  json
// @Produce json
// @Param id path string true "User ID"
// @Param request body ActivateTrialRequest false "Trial parameters (days, credits)"
// @Success 200 {object} ActivateTrialResponse
// @Router /api/v1/admin/users/{id}/trial-activate [post]
// @Security BearerAuth
func (apiServer *HelixAPIServer) adminActivateTrial(_ http.ResponseWriter, req *http.Request) (*ActivateTrialResponse, error) {
	ctx := req.Context()
	adminUser := getRequestUser(req)
	if !adminUser.Admin {
		return nil, system.NewHTTPError403("only admins can activate trials")
	}
	if apiServer.Cfg.Edition != "cloud" {
		return nil, system.NewHTTPError400("trials are only available on the cloud edition")
	}
	if !apiServer.Cfg.Stripe.BillingEnabled {
		return nil, system.NewHTTPError400("Stripe billing must be enabled")
	}

	targetUserID := mux.Vars(req)["id"]
	if targetUserID == "" {
		return nil, system.NewHTTPError400("user ID is required")
	}

	body := ActivateTrialRequest{}
	if req.Body != nil && req.ContentLength > 0 {
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			return nil, system.NewHTTPError400("invalid request body: " + err.Error())
		}
	}
	if body.Days <= 0 {
		body.Days = defaultTrialDays
	}
	// Credits intentionally not defaulted: the admin form value is the
	// source of truth. Silent defaulting (previously 100) compounded with
	// the Stripe trial-invoice webhook credit ($product.metadata.credits)
	// produced surprise wallet balances that didn't match what the admin
	// typed.
	if body.Credits < 0 {
		return nil, system.NewHTTPError400("credits must be zero or positive")
	}

	targetUser, err := apiServer.Store.GetUser(ctx, &store.GetUserQuery{ID: targetUserID})
	if err != nil {
		return nil, system.NewHTTPError404("user not found")
	}

	oldestOrg, err := oldestOwnedOrg(ctx, apiServer.Store, targetUserID)
	if err != nil {
		return nil, system.NewHTTPError500("failed to list user organizations: " + err.Error())
	}

	// Path A: no owned org yet. Stash intent on the user; consumeUserTrialIntent
	// will apply it when they create their first org.
	if oldestOrg == nil {
		days := body.Days
		credits := body.Credits
		targetUser.TrialDaysOnFirstOrg = &days
		targetUser.TrialCreditsOnFirstOrg = &credits
		updated, err := apiServer.Store.UpdateUser(ctx, targetUser)
		if err != nil {
			return nil, system.NewHTTPError500("failed to stash trial intent: " + err.Error())
		}
		log.Info().
			Str("admin_id", adminUser.ID).
			Str("target_user_id", targetUserID).
			Int("days", days).
			Float64("credits", credits).
			Msg("admin stashed trial intent on user (no org yet)")
		apiServer.sendTrialActivatedEmail(ctx, updated, days, true)
		return &ActivateTrialResponse{User: updated, Status: "stashed"}, nil
	}

	// Path B: user already has at least one owned org. Apply directly.
	wallet, err := apiServer.getOrCreateWallet(ctx, targetUser, oldestOrg.ID)
	if err != nil {
		return nil, system.NewHTTPError500("failed to get wallet for oldest owned org: " + err.Error())
	}
	if wallet.StripeSubscriptionID != "" && wallet.IsSubscriptionActive() {
		return nil, system.NewHTTPError422(fmt.Sprintf("org %s already has an active subscription", oldestOrg.ID))
	}

	sub, err := apiServer.Stripe.CreateTrialSubscription(ctx, wallet, body.Days)
	if err != nil {
		return nil, system.NewHTTPError500("failed to create stripe trial subscription: " + err.Error())
	}

	wallet.StripeSubscriptionID = sub.ID
	wallet.SubscriptionStatus = sub.Status
	wallet.SubscriptionCurrentPeriodStart = sub.CurrentPeriodStart
	wallet.SubscriptionCurrentPeriodEnd = sub.CurrentPeriodEnd
	wallet.SubscriptionCreated = sub.Created
	wallet.SubscriptionCancelAtPeriodEnd = sub.CancelAtPeriodEnd
	if _, err := apiServer.Store.UpdateWallet(ctx, wallet); err != nil {
		log.Warn().Err(err).Str("wallet_id", wallet.ID).Msg("failed to persist trial subscription state to wallet (webhook will retry)")
	}
	if body.Credits > 0 {
		if _, err := apiServer.Store.UpdateWalletBalance(ctx, wallet.ID, body.Credits, types.TransactionMetadata{
			TransactionType: types.TransactionTypeSubscription,
		}); err != nil {
			log.Warn().Err(err).Str("wallet_id", wallet.ID).Float64("credits", body.Credits).Msg("failed to top up wallet with trial credits")
		}
	}

	log.Info().
		Str("admin_id", adminUser.ID).
		Str("target_user_id", targetUserID).
		Str("org_id", oldestOrg.ID).
		Str("subscription_id", sub.ID).
		Int("days", body.Days).
		Float64("credits", body.Credits).
		Msg("admin activated trial subscription on oldest owned org")

	apiServer.sendTrialActivatedEmail(ctx, targetUser, body.Days, false)
	return &ActivateTrialResponse{User: targetUser, OrgID: oldestOrg.ID, Status: "applied"}, nil
}

// sendTrialActivatedEmail fires the trial_activated email. Best-effort: logs
// errors but never blocks the caller's HTTP response.
func (apiServer *HelixAPIServer) sendTrialActivatedEmail(ctx context.Context, user *types.User, days int, pending bool) {
	if user == nil || user.Email == "" {
		return
	}
	if apiServer.Controller == nil || apiServer.Controller.Options.Notifier == nil {
		return
	}
	firstName := ""
	if user.FullName != "" {
		firstName = strings.Split(user.FullName, " ")[0]
	}
	err := apiServer.Controller.Options.Notifier.Notify(ctx, &types.Notification{
		Event:        types.EventTrialActivated,
		Email:        user.Email,
		FirstName:    firstName,
		TrialDays:    days,
		TrialPending: pending,
	})
	if err != nil {
		log.Warn().Err(err).
			Str("user_id", user.ID).
			Str("email", user.Email).
			Msg("failed to send trial activated email")
	}
}

// adminRevokeTrial godoc
// @Summary Revoke an admin-granted trial (Admin, cloud only)
// @Description Clears any stashed trial intent on the user and cancels the Stripe subscription on the user's oldest owned org if it is currently in a trialing state. Paid (active) subscriptions are never cancelled.
// @Tags    users
// @Produce json
// @Param id path string true "User ID"
// @Success 200 {object} ActivateTrialResponse
// @Router /api/v1/admin/users/{id}/trial-activate [delete]
// @Security BearerAuth
func (apiServer *HelixAPIServer) adminRevokeTrial(_ http.ResponseWriter, req *http.Request) (*ActivateTrialResponse, error) {
	ctx := req.Context()
	adminUser := getRequestUser(req)
	if !adminUser.Admin {
		return nil, system.NewHTTPError403("only admins can revoke trials")
	}
	if apiServer.Cfg.Edition != "cloud" {
		return nil, system.NewHTTPError400("trials are only available on the cloud edition")
	}

	targetUserID := mux.Vars(req)["id"]
	if targetUserID == "" {
		return nil, system.NewHTTPError400("user ID is required")
	}

	targetUser, err := apiServer.Store.GetUser(ctx, &store.GetUserQuery{ID: targetUserID})
	if err != nil {
		return nil, system.NewHTTPError404("user not found")
	}

	stashedCleared := false
	if targetUser.TrialDaysOnFirstOrg != nil || targetUser.TrialCreditsOnFirstOrg != nil {
		targetUser.TrialDaysOnFirstOrg = nil
		targetUser.TrialCreditsOnFirstOrg = nil
		if _, err := apiServer.Store.UpdateUser(ctx, targetUser); err != nil {
			return nil, system.NewHTTPError500("failed to clear trial intent: " + err.Error())
		}
		stashedCleared = true
	}

	oldestOrg, err := oldestOwnedOrg(ctx, apiServer.Store, targetUserID)
	if err != nil {
		return nil, system.NewHTTPError500("failed to list user organizations: " + err.Error())
	}

	cancelledOrgID := ""
	if oldestOrg != nil {
		wallet, err := apiServer.Store.GetWalletByOrg(ctx, oldestOrg.ID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return nil, system.NewHTTPError500("failed to read org wallet: " + err.Error())
		}
		// Only cancel a subscription that is currently trialing. Never touch a
		// paid subscription via this endpoint.
		if wallet != nil && wallet.StripeSubscriptionID != "" && wallet.SubscriptionStatus == stripeapi.SubscriptionStatusTrialing {
			if err := apiServer.Stripe.CancelTrialSubscription(ctx, wallet.StripeSubscriptionID); err != nil {
				return nil, system.NewHTTPError500("failed to cancel stripe subscription: " + err.Error())
			}
			cancelledOrgID = oldestOrg.ID
		}
	}

	status := "noop"
	if stashedCleared && cancelledOrgID != "" {
		status = "cleared_and_cancelled"
	} else if stashedCleared {
		status = "cleared"
	} else if cancelledOrgID != "" {
		status = "cancelled"
	}

	log.Info().
		Str("admin_id", adminUser.ID).
		Str("target_user_id", targetUserID).
		Str("cancelled_org_id", cancelledOrgID).
		Bool("stashed_cleared", stashedCleared).
		Msg("admin revoked trial")

	return &ActivateTrialResponse{User: targetUser, OrgID: cancelledOrgID, Status: status}, nil
}

// oldestOwnedOrg returns the user's oldest owned organization (by CreatedAt),
// or nil if they own none.
func oldestOwnedOrg(ctx context.Context, st store.Store, userID string) (*types.Organization, error) {
	orgs, err := st.ListOrganizations(ctx, &store.ListOrganizationsQuery{Owner: userID})
	if err != nil {
		return nil, err
	}
	if len(orgs) == 0 {
		return nil, nil
	}
	sort.Slice(orgs, func(i, j int) bool { return orgs[i].CreatedAt.Before(orgs[j].CreatedAt) })
	return orgs[0], nil
}

// enrichUserTrialDisplay sets the transient TrialStatus / TrialOrgID /
// TrialEndsAt fields on a user for the admin users list.
//   - "stashed" — admin granted a trial but the user has not yet created an org.
//   - "active"  — wallet on the user's oldest owned org is currently trialing.
//   - ""        — neither (field is omitted from JSON via omitempty).
//
// Best-effort: errors are logged and the user is returned without enrichment.
func (apiServer *HelixAPIServer) enrichUserTrialDisplay(ctx context.Context, u *types.User) {
	if u == nil {
		return
	}
	if u.TrialDaysOnFirstOrg != nil && *u.TrialDaysOnFirstOrg > 0 {
		u.TrialStatus = "stashed"
		return
	}
	oldest, err := oldestOwnedOrg(ctx, apiServer.Store, u.ID)
	if err != nil {
		log.Warn().Err(err).Str("user_id", u.ID).Msg("failed to list orgs for trial enrichment")
		return
	}
	if oldest == nil {
		return
	}
	wallet, err := apiServer.Store.GetWalletByOrg(ctx, oldest.ID)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			log.Warn().Err(err).Str("user_id", u.ID).Str("org_id", oldest.ID).Msg("failed to get wallet for trial enrichment")
		}
		return
	}
	if wallet.SubscriptionStatus == stripeapi.SubscriptionStatusTrialing {
		u.TrialStatus = "active"
		u.TrialOrgID = oldest.ID
		end := wallet.SubscriptionCurrentPeriodEnd
		u.TrialEndsAt = &end
	}
}
