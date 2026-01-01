package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/helixml/helix/api/pkg/crypto"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// listSecrets godoc
// @Summary List secrets
// @Description List secrets for the user.
// @Tags    secrets
// @Success 200 {array} types.Secret
// @Router /api/v1/secrets [get]
// @Security BearerAuth
func (s *HelixAPIServer) listSecrets(_ http.ResponseWriter, r *http.Request) ([]*types.Secret, *system.HTTPError) {
	ctx := r.Context()
	user := getRequestUser(r)
	if user == nil {
		return nil, system.NewHTTPError401("user not found")
	}

	query := &store.ListSecretsQuery{
		Owner:     user.ID,
		OwnerType: types.OwnerTypeUser,
	}

	secrets, err := s.Store.ListSecrets(ctx, query)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	// Remove the value from the secrets
	for idx, secret := range secrets {
		secret.Value = nil
		secrets[idx] = secret
	}

	return secrets, nil
}

// createSecret godoc
// @Summary Create new secret
// @Description Create a new secret for the user.
// @Tags    secrets
// @Success 200 {object} types.Secret
// @Param request body types.Secret true "Request body with secret configuration."
// @Router /api/v1/secrets [post]
// @Security BearerAuth
func (s *HelixAPIServer) createSecret(_ http.ResponseWriter, r *http.Request) (*types.Secret, *system.HTTPError) {
	ctx := r.Context()
	user := getRequestUser(r)
	if user == nil {
		return nil, system.NewHTTPError401("user not found")
	}

	var secretReq types.CreateSecretRequest
	if err := json.NewDecoder(r.Body).Decode(&secretReq); err != nil {
		return nil, system.NewHTTPError400(err.Error())
	}

	// Encrypt the secret value before storing
	encryptionKey, err := s.getEncryptionKey()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get encryption key for secret")
		return nil, system.NewHTTPError500("Failed to encrypt secret")
	}

	encryptedValue, err := crypto.EncryptAES256GCM([]byte(secretReq.Value), encryptionKey)
	if err != nil {
		log.Error().Err(err).Msg("Failed to encrypt secret value")
		return nil, system.NewHTTPError500("Failed to encrypt secret")
	}

	secret := &types.Secret{
		Name:  secretReq.Name,
		Value: []byte(encryptedValue),
	}
	secret.Owner = user.ID
	secret.OwnerType = types.OwnerTypeUser

	createdSecret, err := s.Store.CreateSecret(ctx, secret)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	// Remove the value from the secret
	createdSecret.Value = nil

	return createdSecret, nil
}

// updateSecret godoc
// @Summary Update an existing secret
// @Description Update an existing secret for the user.
// @Tags    secrets
// @Success 200 {object} types.Secret
// @Param request body types.Secret true "Request body with updated secret configuration."
// @Param id path string true "Secret ID"
// @Router /api/v1/secrets/{id} [put]
// @Security BearerAuth
func (s *HelixAPIServer) updateSecret(_ http.ResponseWriter, r *http.Request) (*types.Secret, *system.HTTPError) {
	ctx := r.Context()
	id := getID(r)

	user := getRequestUser(r)
	if user == nil {
		return nil, system.NewHTTPError401("user not found")
	}

	// First, verify the secret exists and belongs to the user
	existing, err := s.Store.GetSecret(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, system.NewHTTPError404("Secret not found")
		}
		return nil, system.NewHTTPError500(err.Error())
	}

	// Check ownership - return 404 instead of 403 to avoid leaking existence of secret
	if existing.Owner != user.ID {
		return nil, system.NewHTTPError404("Secret not found")
	}

	var secret types.Secret
	if err := json.NewDecoder(r.Body).Decode(&secret); err != nil {
		return nil, system.NewHTTPError400(err.Error())
	}

	// Encrypt the secret value if provided
	if len(secret.Value) > 0 {
		encryptionKey, err := s.getEncryptionKey()
		if err != nil {
			log.Error().Err(err).Msg("Failed to get encryption key for secret update")
			return nil, system.NewHTTPError500("Failed to encrypt secret")
		}

		encryptedValue, err := crypto.EncryptAES256GCM(secret.Value, encryptionKey)
		if err != nil {
			log.Error().Err(err).Msg("Failed to encrypt secret value")
			return nil, system.NewHTTPError500("Failed to encrypt secret")
		}
		secret.Value = []byte(encryptedValue)
	}

	secret.ID = id
	secret.Owner = user.ID
	secret.OwnerType = types.OwnerTypeUser

	updatedSecret, err := s.Store.UpdateSecret(ctx, &secret)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, system.NewHTTPError404("Secret not found")
		}
		return nil, system.NewHTTPError500(err.Error())
	}

	// Remove the value from the secret
	updatedSecret.Value = nil

	return updatedSecret, nil
}

// deleteSecret godoc
// @Summary Delete a secret
// @Description Delete a secret for the user.
// @Tags    secrets
// @Success 200 {object} types.Secret
// @Param id path string true "Secret ID"
// @Router /api/v1/secrets/{id} [delete]
// @Security BearerAuth
func (s *HelixAPIServer) deleteSecret(_ http.ResponseWriter, r *http.Request) (*types.Secret, *system.HTTPError) {
	ctx := r.Context()
	id := getID(r)

	user := getRequestUser(r)
	if user == nil {
		return nil, system.NewHTTPError401("user not found")
	}

	existing, err := s.Store.GetSecret(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, system.NewHTTPError404("Secret not found")
		}
		return nil, system.NewHTTPError500(err.Error())
	}

	if existing.Owner != user.ID {
		return nil, system.NewHTTPError403("Secret not found")
	}

	err = s.Store.DeleteSecret(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, system.NewHTTPError404("Secret not found")
		}
		return nil, system.NewHTTPError500(err.Error())
	}

	// Remove the value from the secret
	existing.Value = nil

	return existing, nil
}
