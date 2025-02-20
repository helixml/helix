package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

var (
	ErrNoAPIKeyFound = errors.New("no API key found")
)

type ApiKeyAuthenticator struct {
	Store          store.Store
	UserRetriever UserRetriever

	AdminConfig *AdminConfig
}

func (d ApiKeyAuthenticator) ValidateAndReturnUser(ctx context.Context, token string) (*types.User, error) {
	if strings.HasPrefix(token, types.APIKeyPrefix) {
		// we have an API key - we should load it from the database and construct our user that way
		apiKey, err := d.Store.GetAPIKey(ctx, token)
		if err != nil {
			return nil, fmt.Errorf("error getting API key: %s", err.Error())
		}
		if apiKey == nil {
			return nil, fmt.Errorf("error getting API key: %w", ErrNoAPIKeyFound)
		}

		userResponse, err := d.UserRetriever.GetUserByID(ctx, apiKey.Owner)
		if err != nil {
			return nil, fmt.Errorf("error loading user from store: %s", err.Error())
		}

		acct := account{userID: apiKey.Owner}

		var appId string
		if apiKey.AppID != nil && apiKey.AppID.Valid {
			appId = apiKey.AppID.String
		}

		return &types.User{
			ID:        apiKey.Owner,
			Type:      apiKey.OwnerType,
			Token:     token,
			TokenType: types.TokenTypeAPIKey,
			Admin:     acct.isAdmin(d.AdminConfig),
			AppID:     appId,
			FullName:  userResponse.FullName,
			Email:     userResponse.Email,
			Username:  userResponse.Username,
		}, nil
	}
	return nil, nil
}

var _ Authenticator = (*ApiKeyAuthenticator)(nil)
