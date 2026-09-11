package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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
// org_id is required when the user owns an organization. Other zero values use defaults.
type ActivateTrialRequest struct {
	Days    int     `json:"days"`
	Credits float64 `json:"credits"`
	OrgID   string  `json:"org_id,omitempty"`
	// Plan selects what to grant. "pro" grants a PAID plan via a PlanOverride
	// (no Stripe subscription) — for customers who paid out-of-band (bank
	// transfer). Empty or "trial" uses the Stripe trial path (Days applies).
	Plan string `json:"plan,omitempty"`
}

// ActivateTrialResponse describes the outcome of a trial activation.
//
// Status values:
//   - "stashed": user has no orgs yet; trial intent is parked on the user and
//     will be applied when they create their first org.
//   - "applied": Stripe trial subscription was created on the selected owned
//     org's wallet right now.
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

// consumeUserPlanOnFirstOrg applies any admin-stashed paid-plan intent
// (PlanOnFirstOrg) to the given org's wallet as a PlanOverride. Called after an
// org is created. Best-effort: errors are logged, never block org creation.
// Independent of the Stripe trial path — a paid out-of-band grant needs no
// subscription.
func (s *HelixAPIServer) consumeUserPlanOnFirstOrg(ctx context.Context, user *types.User, orgID string) {
	if user == nil || user.PlanOnFirstOrg == nil || *user.PlanOnFirstOrg == "" {
		return
	}
	plan := *user.PlanOnFirstOrg

	wallet, err := s.getOrCreateWallet(ctx, user, orgID)
	if err != nil {
		log.Warn().Err(err).Str("user_id", user.ID).Str("org_id", orgID).
			Msg("failed to get/create wallet for plan-override consumption")
		return
	}
	wallet.PlanOverride = plan
	if _, err := s.Store.UpdateWallet(ctx, wallet); err != nil {
		log.Warn().Err(err).Str("wallet_id", wallet.ID).
			Msg("failed to persist plan override to wallet")
		return
	}

	user.PlanOnFirstOrg = nil
	if _, err := s.Store.UpdateUser(ctx, user); err != nil {
		log.Warn().Err(err).Str("user_id", user.ID).
			Msg("failed to clear PlanOnFirstOrg after consumption")
		return
	}

	log.Info().Str("user_id", user.ID).Str("org_id", orgID).Str("plan", plan).
		Msg("admin-stashed paid plan applied to first org wallet")
}

// adminActivateTrial godoc
// @Summary Activate a trial for a user (Admin, cloud only)
// @Description Stash a trial intent when the user owns no organisations, or activate the explicitly selected owned organisation. Days defaults to 90; credits are taken verbatim from the request (0 means no admin top-up beyond what Stripe's subscription invoice contributes).
// @Tags    users
// @Accept  json
// @Produce json
// @Param id path string true "User ID"
// @Param request body ActivateTrialRequest false "Trial parameters and org_id (required iff the user owns an organisation)"
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
	wasWaitlisted := targetUser.Waitlisted

	ownedOrgs, err := apiServer.Store.ListOrganizations(ctx, &store.ListOrganizationsQuery{Owner: targetUserID})
	if err != nil {
		return nil, system.NewHTTPError500("failed to list user organizations: " + err.Error())
	}
	if len(ownedOrgs) == 0 {
		if body.OrgID != "" {
			return nil, system.NewHTTPError400("user owns no organisations; remove org_id to stash activation for their first owned org")
		}
	} else if body.OrgID == "" {
		return nil, system.NewHTTPError400(fmt.Sprintf("org_id is required: user owns %d organisation(s), pick which one to activate", len(ownedOrgs)))
	}

	var selectedOrg *types.Organization
	for _, org := range ownedOrgs {
		if org.ID == body.OrgID {
			selectedOrg = org
			break
		}
	}
	if len(ownedOrgs) > 0 && selectedOrg == nil {
		return nil, system.NewHTTPError400(fmt.Sprintf("user does not own organisation %s", body.OrgID))
	}

	// Paid plan granted out-of-band (e.g. bank transfer): set a PlanOverride,
	// no Stripe subscription. Independent of Stripe so it is never reverted by
	// a webhook. Applied to the selected owned org now, or stashed for the user's
	// first org.
	if body.Plan == types.PlanOverridePro {
		if selectedOrg == nil {
			plan := types.PlanOverridePro
			targetUser.PlanOnFirstOrg = &plan
			targetUser.Waitlisted = false
			if body.Credits > 0 {
				c := body.Credits
				targetUser.PendingAdminCreditsOnFirstOrg = &c
			}
			updated, uErr := apiServer.Store.UpdateUser(ctx, targetUser)
			if uErr != nil {
				return nil, system.NewHTTPError500("failed to stash paid-plan intent: " + uErr.Error())
			}
			log.Info().Str("admin_id", adminUser.ID).Str("target_user_id", targetUserID).
				Msg("admin stashed paid-plan intent on user (no org yet)")
			if wasWaitlisted {
				apiServer.sendActivationEmail(ctx, updated, 0, false, true)
			}
			return &ActivateTrialResponse{User: updated, Status: "stashed"}, nil
		}
		wallet, wErr := apiServer.getOrCreateWallet(ctx, targetUser, selectedOrg.ID)
		if wErr != nil {
			return nil, system.NewHTTPError500("failed to get wallet for selected org: " + wErr.Error())
		}
		wallet.PlanOverride = types.PlanOverridePro
		if _, wErr := apiServer.Store.UpdateWallet(ctx, wallet); wErr != nil {
			return nil, system.NewHTTPError500("failed to set plan override: " + wErr.Error())
		}
		if body.Credits > 0 {
			if _, bErr := apiServer.Store.UpdateWalletBalance(ctx, wallet.ID, body.Credits, types.TransactionMetadata{
				TransactionType: types.TransactionTypeSubscription,
			}); bErr != nil {
				log.Warn().Err(bErr).Str("wallet_id", wallet.ID).Msg("failed to top up wallet with credits")
			}
		}
		if targetUser.Waitlisted {
			targetUser.Waitlisted = false
			updated, uErr := apiServer.Store.UpdateUser(ctx, targetUser)
			if uErr != nil {
				return nil, system.NewHTTPError500("failed to activate user: " + uErr.Error())
			}
			targetUser = updated
		}
		log.Info().Str("admin_id", adminUser.ID).Str("target_user_id", targetUserID).Str("org_id", selectedOrg.ID).
			Msg("admin granted paid plan (PlanOverride=pro) on selected owned org")
		if wasWaitlisted {
			apiServer.sendActivationEmail(ctx, targetUser, 0, false, true)
		}
		return &ActivateTrialResponse{User: targetUser, OrgID: selectedOrg.ID, Status: "applied"}, nil
	}

	// Path A: no owned org yet. Stash intent on the user; consumeUserTrialIntent
	// will apply it when they create their first org.
	if selectedOrg == nil {
		days := body.Days
		credits := body.Credits
		targetUser.TrialDaysOnFirstOrg = &days
		targetUser.TrialCreditsOnFirstOrg = &credits
		targetUser.Waitlisted = false
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
		apiServer.sendActivationEmail(ctx, updated, days, true, wasWaitlisted)
		return &ActivateTrialResponse{User: updated, Status: "stashed"}, nil
	}

	// Path B: user already has at least one owned org. Apply directly.
	wallet, err := apiServer.getOrCreateWallet(ctx, targetUser, selectedOrg.ID)
	if err != nil {
		return nil, system.NewHTTPError500("failed to get wallet for selected org: " + err.Error())
	}
	if wallet.StripeSubscriptionID != "" && wallet.IsSubscriptionActive() {
		return nil, system.NewHTTPError422(fmt.Sprintf("org %s already has an active subscription", selectedOrg.ID))
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
	if targetUser.Waitlisted {
		targetUser.Waitlisted = false
		updated, err := apiServer.Store.UpdateUser(ctx, targetUser)
		if err != nil {
			return nil, system.NewHTTPError500("failed to activate user: " + err.Error())
		}
		targetUser = updated
	}
	log.Info().
		Str("admin_id", adminUser.ID).
		Str("target_user_id", targetUserID).
		Str("org_id", selectedOrg.ID).
		Str("subscription_id", sub.ID).
		Int("days", body.Days).
		Float64("credits", body.Credits).
		Msg("admin activated trial subscription on selected owned org")

	apiServer.sendActivationEmail(ctx, targetUser, body.Days, false, wasWaitlisted)
	return &ActivateTrialResponse{User: targetUser, OrgID: selectedOrg.ID, Status: "applied"}, nil
}

// sendActivationEmail sends one email for the activation. A waitlisted user
// gets the approval email; an already-approved user gets the trial email.
func (apiServer *HelixAPIServer) sendActivationEmail(ctx context.Context, user *types.User, days int, pending, approved bool) {
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
	event := types.EventTrialActivated
	if approved {
		event = types.EventWaitlistApproved
	}
	err := apiServer.Controller.Options.Notifier.Notify(ctx, &types.Notification{
		Event:        event,
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
// @Description Clears any stashed trial intent on the user and cancels a trialing Stripe subscription on an owned org. Paid (active) subscriptions are never cancelled.
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

	ownedOrgs, err := apiServer.Store.ListOrganizations(ctx, &store.ListOrganizationsQuery{Owner: targetUserID})
	if err != nil {
		return nil, system.NewHTTPError500("failed to list user organizations: " + err.Error())
	}

	cancelledOrgID := ""
	for _, org := range ownedOrgs {
		wallet, err := apiServer.Store.GetWalletByOrg(ctx, org.ID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return nil, system.NewHTTPError500("failed to read org wallet: " + err.Error())
		}
		// Only cancel a subscription that is currently trialing. Never touch a
		// paid subscription via this endpoint.
		if wallet != nil && wallet.StripeSubscriptionID != "" && wallet.SubscriptionStatus == stripeapi.SubscriptionStatusTrialing {
			if err := apiServer.Stripe.CancelTrialSubscription(ctx, wallet.StripeSubscriptionID); err != nil {
				return nil, system.NewHTTPError500("failed to cancel stripe subscription: " + err.Error())
			}
			cancelledOrgID = org.ID
			break
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

// enrichUserTrialDisplay sets the transient TrialStatus / TrialOrgID /
// TrialEndsAt fields on a user for the admin users list.
//   - "stashed" — admin granted a trial but the user has not yet created an org.
//   - "active"  — wallet on one of the user's owned orgs is currently trialing.
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
	ownedOrgs, err := apiServer.Store.ListOrganizations(ctx, &store.ListOrganizationsQuery{Owner: u.ID})
	if err != nil {
		log.Warn().Err(err).Str("user_id", u.ID).Msg("failed to list orgs for trial enrichment")
		return
	}
	for _, org := range ownedOrgs {
		wallet, err := apiServer.Store.GetWalletByOrg(ctx, org.ID)
		if err != nil {
			if !errors.Is(err, store.ErrNotFound) {
				log.Warn().Err(err).Str("user_id", u.ID).Str("org_id", org.ID).Msg("failed to get wallet for trial enrichment")
			}
			continue
		}
		if wallet.SubscriptionStatus == stripeapi.SubscriptionStatusTrialing {
			u.TrialStatus = "active"
			u.TrialOrgID = org.ID
			end := wallet.SubscriptionCurrentPeriodEnd
			u.TrialEndsAt = &end
			return
		}
	}
}
