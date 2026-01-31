package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// getUserDetails godoc
// @Summary Get user details
// @Description Get user by ID
// @Tags    users
// @Success 200 {object} types.User
// @Param id path string true "User ID"
// @Router /api/v1/users/{id} [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) getUserDetails(rw http.ResponseWriter, r *http.Request) {
	userID := mux.Vars(r)["id"]
	if userID == "" {
		http.Error(rw, "user ID is required", http.StatusBadRequest)
		return
	}

	user, err := apiServer.Store.GetUser(r.Context(), &store.GetUserQuery{ID: userID})
	if err != nil {
		http.Error(rw, fmt.Sprintf("error getting user '%s': %v", userID, err), http.StatusInternalServerError)
		return
	}

	user.Token = ""
	user.TokenType = ""

	writeResponse(rw, user, http.StatusOK)
}

// searchUsers godoc
// @Summary Search users
// @Description Search users by email, name, or username
// @Tags    users
// @Success 200 {object} types.UserSearchResponse
// @Param query query string true "Query"
// @Param organization_id query string false "Organization ID"
// @Param limit query int false "Limit"
// @Param offset query int false "Offset"
// @Router /api/v1/users/search [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) searchUsers(rw http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	query := &store.SearchUsersQuery{
		Query:          r.URL.Query().Get("query"),
		OrganizationID: r.URL.Query().Get("organization_id"),
	}

	if query.Query == "" {
		http.Error(rw, "query is required", http.StatusBadRequest)
		return
	}

	if query.OrganizationID != "" {
		_, err := apiServer.authorizeOrgMember(r.Context(), getRequestUser(r), query.OrganizationID)
		if err != nil {
			http.Error(rw, fmt.Sprintf("error authorizing organization member: %v", err), http.StatusForbidden)
			return
		}
		// OK
	}

	// If query is less than 2 characters, return an error
	if len(query.Query) < 2 {
		http.Error(rw, "query must be at least 2 characters", http.StatusBadRequest)
		return
	}

	// Parse pagination parameters
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil {
			http.Error(rw, fmt.Sprintf("invalid limit parameter: %v", err), http.StatusBadRequest)
			return
		}
		query.Limit = limit
	} else {
		// Default limit
		query.Limit = 20
	}

	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		offset, err := strconv.Atoi(offsetStr)
		if err != nil {
			http.Error(rw, fmt.Sprintf("invalid offset parameter: %v", err), http.StatusBadRequest)
			return
		}
		query.Offset = offset
	}

	// Execute the search
	users, total, err := apiServer.Store.SearchUsers(r.Context(), query)
	if err != nil {
		http.Error(rw, fmt.Sprintf("error searching users: %v", err), http.StatusInternalServerError)
		return
	}

	writeResponse(rw, &types.UserSearchResponse{
		Users:      users,
		TotalCount: total,
		Limit:      query.Limit,
		Offset:     query.Offset,
	}, http.StatusOK)
}

// getUserTokenUsage godoc
// @Summary Get user token usage
// @Description Get current user's monthly token usage and limits
// @Tags    users
// @Success 200 {object} types.UserTokenUsageResponse
// @Router /api/v1/users/token-usage [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) getUserTokenUsage(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	if user == nil {
		http.Error(rw, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Skip if quotas are disabled
	if !apiServer.Cfg.SubscriptionQuotas.Enabled || !apiServer.Cfg.SubscriptionQuotas.Inference.Enabled {
		writeResponse(rw, &types.UserTokenUsageResponse{
			QuotasEnabled: false,
		}, http.StatusOK)
		return
	}

	// Get current monthly usage
	monthlyTokens, err := apiServer.Store.GetUserMonthlyTokenUsage(r.Context(), user.ID, types.GlobalProviders)
	if err != nil {
		http.Error(rw, fmt.Sprintf("error getting token usage: %v", err), http.StatusInternalServerError)
		return
	}

	// Check if user is pro tier
	usermeta, err := apiServer.Store.GetUserMeta(r.Context(), user.ID)
	if err != nil && err != store.ErrNotFound {
		http.Error(rw, fmt.Sprintf("error getting user meta: %v", err), http.StatusInternalServerError)
		return
	}

	isProTier := false
	if usermeta != nil && usermeta.Config.StripeSubscriptionActive {
		isProTier = true
	}

	var limit int
	if isProTier {
		limit = apiServer.Cfg.SubscriptionQuotas.Inference.Pro.MaxMonthlyTokens
	} else {
		limit = apiServer.Cfg.SubscriptionQuotas.Inference.Free.MaxMonthlyTokens
	}

	writeResponse(rw, &types.UserTokenUsageResponse{
		QuotasEnabled:   true,
		MonthlyUsage:    monthlyTokens,
		MonthlyLimit:    limit,
		IsProTier:       isProTier,
		UsagePercentage: float64(monthlyTokens) / float64(limit) * 100,
	}, http.StatusOK)
}

// getUserGuidelines godoc
// @Summary Get user guidelines
// @Description Get the current user's personal workspace guidelines
// @Tags Users
// @Produce json
// @Success 200 {object} types.UserGuidelinesResponse
// @Failure 401 {object} system.HTTPError
// @Router /api/v1/users/me/guidelines [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) getUserGuidelines(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	if user == nil {
		http.Error(rw, "unauthorized", http.StatusUnauthorized)
		return
	}

	userMeta, err := apiServer.Store.GetUserMeta(r.Context(), user.ID)
	if err != nil {
		if err == store.ErrNotFound {
			// Return empty guidelines for users without a meta record
			writeResponse(rw, &types.UserGuidelinesResponse{
				Guidelines:        "",
				GuidelinesVersion: 0,
			}, http.StatusOK)
			return
		}
		log.Error().Err(err).Str("user_id", user.ID).Msg("failed to get user meta")
		http.Error(rw, "Failed to get user guidelines", http.StatusInternalServerError)
		return
	}

	writeResponse(rw, &types.UserGuidelinesResponse{
		Guidelines:          userMeta.Guidelines,
		GuidelinesVersion:   userMeta.GuidelinesVersion,
		GuidelinesUpdatedAt: userMeta.GuidelinesUpdatedAt,
		GuidelinesUpdatedBy: userMeta.GuidelinesUpdatedBy,
	}, http.StatusOK)
}

// updateUserGuidelines godoc
// @Summary Update user guidelines
// @Description Update the current user's personal workspace guidelines
// @Tags Users
// @Accept json
// @Produce json
// @Param request body types.UpdateUserGuidelinesRequest true "Guidelines update request"
// @Success 200 {object} types.UserGuidelinesResponse
// @Failure 400 {object} system.HTTPError
// @Failure 401 {object} system.HTTPError
// @Router /api/v1/users/me/guidelines [put]
// @Security BearerAuth
func (apiServer *HelixAPIServer) updateUserGuidelines(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	if user == nil {
		http.Error(rw, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req types.UpdateUserGuidelinesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(rw, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Get or create user meta
	userMeta, err := apiServer.Store.GetUserMeta(r.Context(), user.ID)
	if err != nil {
		if err == store.ErrNotFound {
			// Create a new user meta record
			userMeta = &types.UserMeta{
				ID: user.ID,
			}
		} else {
			log.Error().Err(err).Str("user_id", user.ID).Msg("failed to get user meta")
			http.Error(rw, "Failed to update user guidelines", http.StatusInternalServerError)
			return
		}
	}

	// Save current version to history before updating
	if userMeta.Guidelines != "" && userMeta.Guidelines != req.Guidelines {
		history := &types.GuidelinesHistory{
			ID:         system.GenerateUUID(),
			UserID:     user.ID,
			Version:    userMeta.GuidelinesVersion,
			Guidelines: userMeta.Guidelines,
			UpdatedBy:  userMeta.GuidelinesUpdatedBy,
			UpdatedAt:  userMeta.GuidelinesUpdatedAt,
		}
		if err := apiServer.Store.CreateGuidelinesHistory(r.Context(), history); err != nil {
			log.Warn().Err(err).Str("user_id", user.ID).Msg("failed to save guidelines history")
		}
	}

	// Update guidelines
	userMeta.Guidelines = req.Guidelines
	userMeta.GuidelinesVersion++
	userMeta.GuidelinesUpdatedAt = time.Now()
	userMeta.GuidelinesUpdatedBy = user.ID

	// Save the updated user meta
	updatedMeta, err := apiServer.Store.EnsureUserMeta(r.Context(), *userMeta)
	if err != nil {
		log.Error().Err(err).Str("user_id", user.ID).Msg("failed to update user meta")
		http.Error(rw, "Failed to update user guidelines", http.StatusInternalServerError)
		return
	}

	writeResponse(rw, &types.UserGuidelinesResponse{
		Guidelines:          updatedMeta.Guidelines,
		GuidelinesVersion:   updatedMeta.GuidelinesVersion,
		GuidelinesUpdatedAt: updatedMeta.GuidelinesUpdatedAt,
		GuidelinesUpdatedBy: updatedMeta.GuidelinesUpdatedBy,
	}, http.StatusOK)
}

// getUserGuidelinesHistory godoc
// @Summary Get user guidelines history
// @Description Get the version history of the current user's personal workspace guidelines
// @Tags Users
// @Produce json
// @Success 200 {array} types.GuidelinesHistory
// @Failure 401 {object} system.HTTPError
// @Router /api/v1/users/me/guidelines-history [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) getUserGuidelinesHistory(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	if user == nil {
		http.Error(rw, "unauthorized", http.StatusUnauthorized)
		return
	}

	history, err := apiServer.Store.ListGuidelinesHistory(r.Context(), "", "", user.ID)
	if err != nil {
		log.Error().Err(err).Str("user_id", user.ID).Msg("failed to get user guidelines history")
		http.Error(rw, "Failed to get guidelines history", http.StatusInternalServerError)
		return
	}

	// Populate user display names and emails
	for _, entry := range history {
		if entry.UpdatedBy != "" {
			if u, err := apiServer.Store.GetUser(r.Context(), &store.GetUserQuery{ID: entry.UpdatedBy}); err == nil && u != nil {
				entry.UpdatedByName = u.FullName
				entry.UpdatedByEmail = u.Email
			}
		}
	}

	writeResponse(rw, history, http.StatusOK)
}
