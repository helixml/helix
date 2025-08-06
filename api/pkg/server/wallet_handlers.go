package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/helixml/helix/api/pkg/store"
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
	if orgID != "" {
		wallet, err := s.Store.GetWalletByOrg(ctx, orgID)
		if err == nil {
			return wallet, nil
		}

		if !errors.Is(err, store.ErrNotFound) {
			return nil, system.NewHTTPError500(fmt.Sprintf("failed to get wallet for org %s, error: %s", orgID, err))
		}

		wallet, err = s.Store.CreateWallet(ctx, &types.Wallet{
			OrgID:   orgID,
			Balance: s.Cfg.Stripe.InitialBalance,
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

	wallet, err = s.Store.CreateWallet(ctx, &types.Wallet{
		UserID:  user.ID,
		Balance: s.Cfg.Stripe.InitialBalance,
	})
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to create wallet for user %s, error: %s", user.ID, err))
	}

	return wallet, nil
}
