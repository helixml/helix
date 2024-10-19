package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

func (s *HelixAPIServer) listSecrets(w http.ResponseWriter, r *http.Request) ([]*types.Secret, *system.HTTPError) {
	ctx := r.Context()
	owner := r.URL.Query().Get("owner")
	ownerType := types.OwnerType(r.URL.Query().Get("owner_type"))

	if owner == "" {
		return nil, system.NewHTTPError400("owner is required")
	}

	query := &store.ListSecretsQuery{
		Owner:     owner,
		OwnerType: ownerType,
	}

	secrets, err := s.Store.ListSecrets(ctx, query)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	return secrets, nil
}

func (s *HelixAPIServer) createSecret(w http.ResponseWriter, r *http.Request) (*types.Secret, *system.HTTPError) {
	ctx := r.Context()
	var secret types.Secret
	if err := json.NewDecoder(r.Body).Decode(&secret); err != nil {
		return nil, system.NewHTTPError400(err.Error())
	}

	createdSecret, err := s.Store.CreateSecret(ctx, &secret)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	return createdSecret, nil
}

func (s *HelixAPIServer) updateSecret(w http.ResponseWriter, r *http.Request) (*types.Secret, *system.HTTPError) {
	ctx := r.Context()
	id := mux.Vars(r)["id"]

	var secret types.Secret
	if err := json.NewDecoder(r.Body).Decode(&secret); err != nil {
		return nil, system.NewHTTPError400(err.Error())
	}

	secret.ID = id

	updatedSecret, err := s.Store.UpdateSecret(ctx, &secret)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, system.NewHTTPError404("Secret not found")
		}
		return nil, system.NewHTTPError500(err.Error())
	}

	return updatedSecret, nil
}

func (s *HelixAPIServer) deleteSecret(w http.ResponseWriter, r *http.Request) (*types.Secret, *system.HTTPError) {
	ctx := r.Context()
	id := getID(r)

	err := s.Store.DeleteSecret(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, system.NewHTTPError404("Secret not found")
		}
		return nil, system.NewHTTPError500(err.Error())
	}

	return nil, nil
}
