package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/pubsub"
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

// updateUserColorScheme godoc
// @Summary Update user color scheme preference
// @Description Set the user's UI color scheme. Propagates instantly to GNOME and Zed in any spec-task sessions owned by this user.
// @Tags Users
// @Accept json
// @Produce json
// @Param request body types.UpdateUserColorSchemeRequest true "Color scheme update request"
// @Success 200 {object} types.UpdateUserColorSchemeRequest
// @Failure 400 {object} system.HTTPError
// @Failure 401 {object} system.HTTPError
// @Router /api/v1/users/me/color-scheme [put]
// @Security BearerAuth
func (apiServer *HelixAPIServer) updateUserColorScheme(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	if user == nil {
		http.Error(rw, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req types.UpdateUserColorSchemeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(rw, "Invalid request body", http.StatusBadRequest)
		return
	}

	switch req.ColorScheme {
	case "", "light", "dark":
		// ok
	default:
		http.Error(rw, "color_scheme must be 'light', 'dark', or empty", http.StatusBadRequest)
		return
	}

	// EnsureUserMeta only merges a small allowlist of fields (Stripe), so we
	// fetch + mutate + Update/Create directly to make sure ColorScheme persists.
	userMeta, err := apiServer.Store.GetUserMeta(r.Context(), user.ID)
	if err != nil && err != store.ErrNotFound {
		log.Error().Err(err).Str("user_id", user.ID).Msg("failed to get user meta")
		http.Error(rw, "Failed to update color scheme", http.StatusInternalServerError)
		return
	}

	if userMeta == nil {
		userMeta = &types.UserMeta{ID: user.ID}
		userMeta.Config.ColorScheme = req.ColorScheme
		if _, err := apiServer.Store.CreateUserMeta(r.Context(), *userMeta); err != nil {
			log.Error().Err(err).Str("user_id", user.ID).Msg("failed to create user meta")
			http.Error(rw, "Failed to update color scheme", http.StatusInternalServerError)
			return
		}
	} else {
		userMeta.Config.ColorScheme = req.ColorScheme
		if _, err := apiServer.Store.UpdateUserMeta(r.Context(), *userMeta); err != nil {
			log.Error().Err(err).Str("user_id", user.ID).Msg("failed to update user meta")
			http.Error(rw, "Failed to update color scheme", http.StatusInternalServerError)
			return
		}
	}

	apiServer.publishUserColorSchemeChange(r.Context(), user.ID, req.ColorScheme)

	writeResponse(rw, &req, http.StatusOK)
}

// publishUserColorSchemeChange notifies every active spec-task session owned by
// the user so the in-desktop settings-sync-daemon can apply the new theme to
// GNOME and Zed within ~100ms (no 30s poll wait).
func (apiServer *HelixAPIServer) publishUserColorSchemeChange(ctx context.Context, userID, colorScheme string) {
	sessions, _, err := apiServer.Store.ListSessions(ctx, store.ListSessionsQuery{Owner: userID})
	if err != nil {
		log.Warn().Err(err).Str("user_id", userID).Msg("failed to list sessions for color scheme push")
		return
	}
	payload, err := json.Marshal(map[string]string{
		"type":         "config_changed",
		"field":        "color_scheme",
		"color_scheme": colorScheme,
	})
	if err != nil {
		log.Warn().Err(err).Msg("failed to marshal color scheme event")
		return
	}
	for _, s := range sessions {
		topic := pubsub.GetSessionQueue(userID, s.ID)
		if err := apiServer.pubsub.Publish(ctx, topic, payload); err != nil {
			log.Debug().Err(err).Str("topic", topic).Msg("failed to publish color scheme event")
		}
	}
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

// completeOnboarding godoc
// @Summary Complete onboarding
// @Description Mark onboarding as completed for the current user
// @Tags Users
// @Produce json
// @Success 200 {object} types.User
// @Failure 401 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Router /api/v1/users/me/onboarding [post]
// @Security BearerAuth
func (apiServer *HelixAPIServer) completeOnboarding(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	if user == nil {
		http.Error(rw, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Detach context cancellation
	ctx := context.WithoutCancel(r.Context())

	existingUser, err := apiServer.Store.GetUser(ctx, &store.GetUserQuery{ID: user.ID})
	if err != nil {
		log.Error().Err(err).Str("user_id", user.ID).Msg("failed to get user")
		http.Error(rw, "Failed to get user", http.StatusInternalServerError)
		return
	}

	existingUser.OnboardingCompleted = true
	existingUser.OnboardingCompletedAt = time.Now()

	updatedUser, err := apiServer.Store.UpdateUser(ctx, existingUser)
	if err != nil {
		log.Error().Err(err).Str("user_id", user.ID).Msg("failed to update user onboarding status")
		http.Error(rw, "Failed to update onboarding status", http.StatusInternalServerError)
		return
	}

	updatedUser.Token = ""
	updatedUser.TokenType = ""

	writeResponse(rw, updatedUser, http.StatusOK)
}

// getPinnedProjects godoc
// @Summary Get pinned project IDs
// @Description Get the list of project IDs pinned by the current user
// @Tags Users
// @Produce json
// @Success 200 {object} server.PinnedProjectsResponse
// @Failure 401 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security BearerAuth
// @Router /api/v1/users/me/pinned-projects [get]
func (s *HelixAPIServer) getPinnedProjects(_ http.ResponseWriter, r *http.Request) (*PinnedProjectsResponse, *system.HTTPError) {
	user := getRequestUser(r)

	userMeta, err := s.Store.EnsureUserMeta(r.Context(), types.UserMeta{ID: user.ID})
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to load user meta: %v", err))
	}

	ids := userMeta.Config.PinnedProjectIDs
	if ids == nil {
		ids = []string{}
	}
	return &PinnedProjectsResponse{PinnedProjectIDs: ids}, nil
}
