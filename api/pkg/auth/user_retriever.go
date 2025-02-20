package auth

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Nerzal/gocloak/v13"

	"github.com/helixml/helix/api/pkg/config"
)

type UserRetriever interface {
	GetUserByID(ctx context.Context, userID string) (*UserRetrieverResponse, error)
}

type UserRetrieverResponse struct {
	ID       string
	Username string
	Email    string
	FullName string
}

type KeycloakUserRetriever struct {
	cfg     *config.Keycloak
	gocloak *gocloak.GoCloak

	adminTokenMu      *sync.Mutex
	adminToken        *gocloak.JWT
	adminTokenExpires time.Time
}

func NewKeycloakUserRetriever(gck *gocloak.GoCloak, cfg *config.Keycloak) (*KeycloakUserRetriever, error) {
	return &KeycloakUserRetriever{
		cfg:          cfg,
		gocloak:      gck,
		adminTokenMu: &sync.Mutex{},
	}, nil
}

func (k *KeycloakUserRetriever) GetUserByID(ctx context.Context, userID string) (*UserRetrieverResponse, error) {
	adminToken, err := k.getAdminToken(ctx)
	if err != nil {
		return nil, err
	}

	user, err := k.gocloak.GetUserByID(ctx, adminToken.AccessToken, k.cfg.Realm, userID)
	if err != nil {
		return nil, err
	}

	return &UserRetrieverResponse{
		ID:       gocloak.PString(user.ID),
		Username: gocloak.PString(user.Username),
		Email:    gocloak.PString(user.Email),
		FullName: fmt.Sprintf("%s %s", gocloak.PString(user.FirstName), gocloak.PString(user.LastName)),
	}, nil
}

func (k *KeycloakUserRetriever) getAdminToken(ctx context.Context) (*gocloak.JWT, error) {
	k.adminTokenMu.Lock()
	defer k.adminTokenMu.Unlock()

	if k.adminToken != nil {
		if k.adminTokenExpires.After(time.Now().Add(5 * time.Second)) {
			return k.adminToken, nil
		}
	}

	token, err := k.gocloak.LoginAdmin(ctx, k.cfg.Username, k.cfg.Password, k.cfg.AdminRealm)
	if err != nil {
		return nil, err
	}

	k.adminToken = token
	k.adminTokenExpires = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)

	return token, nil
}

// Compile-time interface check:
var _ UserRetriever = (*KeycloakUserRetriever)(nil)
