package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

func (s *HelixAPIServer) listSecrets(w http.ResponseWriter, r *http.Request) ([]*types.Secret, *system.HTTPError) {
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

	return secrets, nil
}

func (s *HelixAPIServer) createSecret(w http.ResponseWriter, r *http.Request) (*types.Secret, *system.HTTPError) {
	ctx := r.Context()
	user := getRequestUser(r)
	if user == nil {
		return nil, system.NewHTTPError401("user not found")
	}

	var secret types.Secret
	if err := json.NewDecoder(r.Body).Decode(&secret); err != nil {
		return nil, system.NewHTTPError400(err.Error())
	}

	secret.Owner = user.ID
	secret.OwnerType = types.OwnerTypeUser

	createdSecret, err := s.Store.CreateSecret(ctx, &secret)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	return createdSecret, nil
}

func (s *HelixAPIServer) updateSecret(w http.ResponseWriter, r *http.Request) (*types.Secret, *system.HTTPError) {
	ctx := r.Context()
	id := getID(r)

	user := getRequestUser(r)
	if user == nil {
		return nil, system.NewHTTPError401("user not found")
	}

	var secret types.Secret
	if err := json.NewDecoder(r.Body).Decode(&secret); err != nil {
		return nil, system.NewHTTPError400(err.Error())
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

	return updatedSecret, nil
}

func (s *HelixAPIServer) deleteSecret(w http.ResponseWriter, r *http.Request) (*types.Secret, *system.HTTPError) {
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

	return existing, nil
}
