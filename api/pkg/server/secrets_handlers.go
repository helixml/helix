package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
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
		Name:      secretReq.Name,
		Value:     []byte(encryptedValue),
		AppID:     secretReq.AppID,
		ProjectID: secretReq.ProjectID,
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

	// Check authorization: either user owns the secret OR has access to the project
	authorized := false
	if existing.Owner == user.ID {
		authorized = true
	} else if existing.ProjectID != "" {
		// For project secrets, check if user has update access to the project
		if err := s.authorizeUserToProjectByID(ctx, user, existing.ProjectID, types.ActionUpdate); err == nil {
			authorized = true
		}
	}
	if !authorized {
		// Return 404 instead of 403 to avoid leaking existence of secret
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
	// Preserve original ownership - don't let users change owner/project via update
	secret.Owner = existing.Owner
	secret.OwnerType = existing.OwnerType
	secret.ProjectID = existing.ProjectID
	secret.AppID = existing.AppID

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

	// Check authorization: either user owns the secret OR has access to the project
	authorized := false
	if existing.Owner == user.ID {
		authorized = true
	} else if existing.ProjectID != "" {
		// For project secrets, check if user has delete access to the project
		if err := s.authorizeUserToProjectByID(ctx, user, existing.ProjectID, types.ActionDelete); err == nil {
			authorized = true
		}
	}
	if !authorized {
		return nil, system.NewHTTPError403("Access denied")
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

// listProjectSecrets godoc
// @Summary List secrets for a project
// @Description List all secrets associated with a specific project.
// @Tags    secrets
// @Param   id path string true "Project ID"
// @Success 200 {array} types.Secret
// @Router /api/v1/projects/{id}/secrets [get]
// @Security BearerAuth
func (s *HelixAPIServer) listProjectSecrets(_ http.ResponseWriter, r *http.Request) ([]*types.Secret, *system.HTTPError) {
	ctx := r.Context()
	user := getRequestUser(r)
	if user == nil {
		return nil, system.NewHTTPError401("user not found")
	}

	projectID := mux.Vars(r)["id"]
	if projectID == "" {
		return nil, system.NewHTTPError400("project ID required")
	}

	// Verify user has access to the project (owner or org member)
	if err := s.authorizeUserToProjectByID(ctx, user, projectID, types.ActionGet); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, system.NewHTTPError404("Project not found")
		}
		return nil, system.NewHTTPError403("Access denied")
	}

	secrets, err := s.Store.ListProjectSecrets(ctx, projectID)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	// Remove the values from the secrets (don't expose encrypted data)
	for idx, secret := range secrets {
		secret.Value = nil
		secrets[idx] = secret
	}

	return secrets, nil
}

// createProjectSecret godoc
// @Summary Create a secret for a project
// @Description Create a new secret associated with a specific project. The secret will be injected as an environment variable in project sessions.
// @Tags    secrets
// @Param   id path string true "Project ID"
// @Param   request body types.CreateSecretRequest true "Request body with secret name and value."
// @Success 200 {object} types.Secret
// @Router /api/v1/projects/{id}/secrets [post]
// @Security BearerAuth
func (s *HelixAPIServer) createProjectSecret(_ http.ResponseWriter, r *http.Request) (*types.Secret, *system.HTTPError) {
	ctx := r.Context()
	user := getRequestUser(r)
	if user == nil {
		return nil, system.NewHTTPError401("user not found")
	}

	projectID := mux.Vars(r)["id"]
	if projectID == "" {
		return nil, system.NewHTTPError400("project ID required")
	}

	// Verify user has access to the project (owner or org member with create permission)
	if err := s.authorizeUserToProjectByID(ctx, user, projectID, types.ActionCreate); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, system.NewHTTPError404("Project not found")
		}
		return nil, system.NewHTTPError403("Access denied")
	}

	var secretReq types.CreateSecretRequest
	if err := json.NewDecoder(r.Body).Decode(&secretReq); err != nil {
		return nil, system.NewHTTPError400(err.Error())
	}

	// Encrypt the secret value before storing
	encryptionKey, err := s.getEncryptionKey()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get encryption key for project secret")
		return nil, system.NewHTTPError500("Failed to encrypt secret")
	}

	encryptedValue, err := crypto.EncryptAES256GCM([]byte(secretReq.Value), encryptionKey)
	if err != nil {
		log.Error().Err(err).Msg("Failed to encrypt project secret value")
		return nil, system.NewHTTPError500("Failed to encrypt secret")
	}

	secret := &types.Secret{
		Name:      secretReq.Name,
		Value:     []byte(encryptedValue),
		ProjectID: projectID, // Associate with project
	}
	secret.Owner = user.ID
	secret.OwnerType = types.OwnerTypeUser

	createdSecret, err := s.Store.CreateSecret(ctx, secret)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	// Remove the value from the response
	createdSecret.Value = nil

	return createdSecret, nil
}

// GetProjectSecretsAsEnvVars retrieves project secrets and returns them as environment variables
// This is used to inject secrets into desktop container environments
func (s *HelixAPIServer) GetProjectSecretsAsEnvVars(ctx context.Context, projectID string) ([]string, error) {
	if projectID == "" {
		return nil, nil
	}

	secrets, err := s.Store.ListProjectSecrets(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to list project secrets: %w", err)
	}

	if len(secrets) == 0 {
		return nil, nil
	}

	encryptionKey, err := s.getEncryptionKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get encryption key: %w", err)
	}

	var envVars []string
	for _, secret := range secrets {
		// Decrypt the secret value
		decrypted, err := crypto.DecryptAES256GCM(string(secret.Value), encryptionKey)
		if err != nil {
			log.Warn().Err(err).Str("secret_name", secret.Name).Msg("Failed to decrypt project secret, skipping")
			continue
		}

		// Use the secret name as the env var name (uppercase, replace - with _)
		envVars = append(envVars, fmt.Sprintf("%s=%s", secret.Name, string(decrypted)))
		log.Debug().Str("name", secret.Name).Str("project_id", projectID).Msg("Injecting project secret as env var")
	}

	return envVars, nil
}
