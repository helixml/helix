package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/stripe"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// getWalletHandler godoc
// @Summary Get a wallet
// @Description Get a wallet
// @Tags    wallets
// @Success 200 {object} types.Wallet
// @Param   org_id query string false "Organization ID"
// @Router /api/v1/wallet [get]
// @Security BearerAuth
func (s *HelixAPIServer) getWalletHandler(_ http.ResponseWriter, req *http.Request) (*types.Wallet, *system.HTTPError) {
	ctx := req.Context()
	user := getRequestUser(req)

	orgID := req.URL.Query().Get("org_id")
	if orgID != "" {
		// Authorize org member
		_, err := s.authorizeOrgMember(req.Context(), user, orgID)
		if err != nil {
			log.Err(err).Msg("error authorizing org owner")
			return nil, system.NewHTTPError403("Could not authorize org owner: " + err.Error())
		}
	}

	wallet, err := s.getOrCreateWallet(ctx, user, orgID)
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to get or create wallet for user %s, error: %s", user.ID, err))
	}

	return wallet, nil
}

func (s *HelixAPIServer) getOrCreateWallet(ctx context.Context, user *types.User, orgID string) (*types.Wallet, error) {
	// Org paths
	if orgID != "" {
		wallet, err := s.Store.GetWalletByOrg(ctx, orgID)
		if err == nil {
			return wallet, nil
		}

		if !errors.Is(err, store.ErrNotFound) {
			return nil, system.NewHTTPError500(fmt.Sprintf("failed to get wallet for org %s, error: %s", orgID, err))
		}

		// Create stripe customer
		stripeCustomerID, err := s.Stripe.CreateStripeCustomer(user, orgID)
		if err != nil {
			return nil, system.NewHTTPError500(fmt.Sprintf("failed to create stripe customer for org %s, error: %s", orgID, err))
		}

		wallet, err = s.Store.CreateWallet(ctx, &types.Wallet{
			OrgID:            orgID,
			Balance:          s.Cfg.Stripe.InitialBalance,
			StripeCustomerID: stripeCustomerID,
		})
		if err != nil {
			return nil, system.NewHTTPError500(fmt.Sprintf("failed to create wallet for org %s, error: %s", orgID, err))
		}

		return wallet, nil
	}

	// User path
	wallet, err := s.Store.GetWalletByUser(ctx, user.ID)
	if err == nil {
		return wallet, nil
	}

	if !errors.Is(err, store.ErrNotFound) {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to get wallet for user %s, error: %s", user.ID, err))
	}

	// Create stripe customer
	stripeCustomerID, err := s.Stripe.CreateStripeCustomer(user, "")
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to create stripe customer for user %s, error: %s", user.ID, err))
	}

	wallet, err = s.Store.CreateWallet(ctx, &types.Wallet{
		UserID:           user.ID,
		Balance:          s.Cfg.Stripe.InitialBalance,
		StripeCustomerID: stripeCustomerID,
	})
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to create wallet for user %s, error: %s", user.ID, err))
	}

	return wallet, nil
}

type CreateTopUpRequest struct {
	Amount float64 `json:"amount"`
	OrgID  string  `json:"org_id"`
}

// createTopUp godoc
// @Summary Create a top up
// @Description Create a top up with specified amount
// @Tags    wallets
// @Param request body CreateTopUpRequest true "Request body with amount"
// @Success 200 {string} string "Top up session URL"
// @Router /api/v1/top-ups/new [post]
// @Security BearerAuth
func (s *HelixAPIServer) createTopUp(_ http.ResponseWriter, req *http.Request) (string, error) {
	user := getRequestUser(req)

	// Parse request body to get amount
	var requestBody CreateTopUpRequest
	if err := json.NewDecoder(req.Body).Decode(&requestBody); err != nil {
		return "", fmt.Errorf("failed to decode request body: %w", err)
	}

	if requestBody.OrgID != "" {
		_, err := s.authorizeOrgOwner(req.Context(), user, requestBody.OrgID)
		if err != nil {
			return "", fmt.Errorf("failed to authorize org owner: %w", err)
		}
	}

	// Validate amount
	if requestBody.Amount <= 0 {
		return "", fmt.Errorf("amount must be greater than 0")
	}

	// Get wallet
	wallet, err := s.getOrCreateWallet(req.Context(), user, requestBody.OrgID)
	if err != nil {
		return "", fmt.Errorf("failed to get or create wallet: %w", err)
	}

	params := stripe.TopUpSessionParams{
		StripeCustomerID: wallet.StripeCustomerID,
		OrgID:            requestBody.OrgID,
		UserID:           user.ID,
		Amount:           requestBody.Amount,
	}

	if requestBody.OrgID != "" {
		org, err := s.Store.GetOrganization(req.Context(), &store.GetOrganizationQuery{
			ID: requestBody.OrgID,
		})
		if err != nil {
			return "", fmt.Errorf("failed to get organization: %w", err)
		}
		params.OrgName = org.Name // Using 'slug' as this will be used in the redirect URL
	}

	return s.Stripe.GetTopUpSessionURL(params)
}

// subscriptionCreate godoc
// @Summary Create a subscription
// @Description Create a subscription
// @Tags    wallets
// @Success 200 {string} string "Subscription session URL"
// @Param   org_id query string false "Organization ID"
// @Router /api/v1/subscription/new [post]
// @Security BearerAuth
func (s *HelixAPIServer) subscriptionCreate(_ http.ResponseWriter, req *http.Request) (string, error) {
	user := getRequestUser(req)

	orgID := req.URL.Query().Get("org_id")
	if orgID != "" {
		_, err := s.authorizeOrgOwner(req.Context(), user, orgID)
		if err != nil {
			return "", fmt.Errorf("failed to authorize org owner: %w", err)
		}
	}

	wallet, err := s.getOrCreateWallet(req.Context(), user, orgID)
	if err != nil {
		return "", fmt.Errorf("failed to get or create wallet: %w", err)
	}

	return s.Stripe.GetCheckoutSessionURL(stripe.SubscriptionSessionParams{
		StripeCustomerID: wallet.StripeCustomerID,
		OrgID:            orgID,
		UserID:           user.ID,
		Amount:           s.Cfg.Stripe.InitialBalance,
	})
}

// subscriptionManage godoc
// @Summary Manage a subscription
// @Description Manage a subscription
// @Tags    wallets
// @Success 200 {string} string "Subscription session URL"
// @Param   org_id query string false "Organization ID"
// @Router /api/v1/subscription/manage [post]
// @Security BearerAuth
func (s *HelixAPIServer) subscriptionManage(_ http.ResponseWriter, req *http.Request) (string, error) {
	user := getRequestUser(req)

	orgID := req.URL.Query().Get("org_id")
	if orgID != "" {
		_, err := s.authorizeOrgOwner(req.Context(), user, orgID)
		if err != nil {
			return "", fmt.Errorf("failed to authorize org owner: %w", err)
		}
	}

	wallet, err := s.getOrCreateWallet(req.Context(), user, orgID)
	if err != nil {
		return "", fmt.Errorf("failed to get or create wallet: %w", err)
	}

	org, err := s.Store.GetOrganization(req.Context(), &store.GetOrganizationQuery{
		ID: orgID,
	})
	if err != nil {
		return "", fmt.Errorf("failed to get organization: %w", err)
	}

	return s.Stripe.GetPortalSessionURL(wallet.StripeCustomerID, org.Name)
}

func (s *HelixAPIServer) subscriptionWebhook(res http.ResponseWriter, req *http.Request) {
	s.Stripe.ProcessWebhook(res, req)
}
