package server

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	jsoniter "github.com/json-iterator/go"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// AdminCreateServiceAccountRequest is the body for POST /admin/service-accounts.
type AdminCreateServiceAccountRequest struct {
	// OrganizationName is the URL-safe name of the organization the account
	// owns. It is the idempotency key: asking again for the same name returns
	// the same account and organization.
	OrganizationName string `json:"organization_name"`
	DisplayName      string `json:"display_name"`
}

// AdminServiceAccount is a service user and the organization it owns.
type AdminServiceAccount struct {
	User         *types.User         `json:"user"`
	Organization *types.Organization `json:"organization"`
}

// adminCreateServiceAccount godoc
// @Summary Create a service account and its organization (Admin only)
// @Description Create a non-human user that owns a new organization, so an orchestrator serving many customers can give each one its own org, projects and credentials instead of running them all as one account. Works on OIDC deployments (the account has no IdP identity; it acts only through API keys minted with POST /admin/users/{id}/api-keys). Idempotent per organization_name.
// @Tags    users
// @Accept  json
// @Produce json
// @Param   request body AdminCreateServiceAccountRequest true "Service account"
// @Success 200 {object} AdminServiceAccount
// @Router  /api/v1/admin/service-accounts [post]
// @Security BearerAuth
func (apiServer *HelixAPIServer) adminCreateServiceAccount(_ http.ResponseWriter, req *http.Request) (*AdminServiceAccount, *system.HTTPError) {
	ctx := req.Context()
	adminUser := getRequestUser(req)
	if !adminUser.Admin {
		return nil, system.NewHTTPError403("only admins can create service accounts")
	}

	var body AdminCreateServiceAccountRequest
	if err := jsoniter.NewDecoder(req.Body).Decode(&body); err != nil {
		return nil, system.NewHTTPError400("failed to decode request: " + err.Error())
	}
	if strings.TrimSpace(body.OrganizationName) == "" {
		return nil, system.NewHTTPError400("organization_name is required")
	}
	name := normalizeOrgName(body.OrganizationName)
	if body.DisplayName == "" {
		body.DisplayName = name
	}

	existing, err := apiServer.Store.GetOrganization(ctx, &store.GetOrganizationQuery{Name: name})
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, system.NewHTTPError500("failed to look up organization: " + err.Error())
	}
	if existing != nil {
		// Only hand back an org a service account owns: this must never become
		// a way to take over a person's organization by naming it.
		owner, err := apiServer.Store.GetUser(ctx, &store.GetUserQuery{ID: existing.Owner})
		if err != nil {
			return nil, system.NewHTTPError500("failed to get organization owner: " + err.Error())
		}
		if owner.AuthProvider != types.AuthProviderService {
			return nil, system.NewHTTPError409(fmt.Sprintf("organization %q exists and is not owned by a service account", name))
		}
		return &AdminServiceAccount{User: owner, Organization: existing}, nil
	}

	userID := system.GenerateUserID()
	// .invalid is reserved (RFC 2606), so this can never be someone's inbox.
	email := fmt.Sprintf("%s@service.invalid", strings.ToLower(userID))
	user, err := apiServer.Store.CreateUser(ctx, &types.User{
		ID:                  userID,
		Email:               email,
		Username:            email,
		FullName:            body.DisplayName,
		Type:                types.OwnerTypeUser,
		AuthProvider:        types.AuthProviderService,
		OnboardingCompleted: true,
		CreatedAt:           time.Now(),
	})
	if err != nil {
		return nil, system.NewHTTPError500("failed to create service user: " + err.Error())
	}

	org, err := apiServer.Store.CreateOrganization(ctx, &types.Organization{
		Name:        name,
		DisplayName: body.DisplayName,
		Owner:       user.ID,
	})
	if err != nil {
		return nil, system.NewHTTPError500("failed to create organization: " + err.Error())
	}
	if _, err := apiServer.Store.CreateOrganizationMembership(ctx, &types.OrganizationMembership{
		OrganizationID: org.ID,
		UserID:         user.ID,
		Role:           types.OrganizationRoleOwner,
	}); err != nil {
		return nil, system.NewHTTPError500("failed to create organization membership: " + err.Error())
	}
	if err := apiServer.seedOrganizationRoles(ctx, org); err != nil {
		return nil, system.NewHTTPError500("failed to seed organization roles: " + err.Error())
	}
	return &AdminServiceAccount{User: user, Organization: org}, nil
}
