package oauth

import (
	"encoding/json"
	"fmt"

	"github.com/helixml/helix/api/pkg/types"
)

type HubSpotUserInfo struct {
	Token             string   `json:"token"`
	User              string   `json:"user"` // Email address
	HubDomain         string   `json:"hub_domain"`
	Scopes            []string `json:"scopes"`
	SignedAccessToken struct {
		ExpiresAt                 int64  `json:"expiresAt"`
		Scopes                    string `json:"scopes"`
		HubID                     int    `json:"hubId"`
		UserID                    int    `json:"userId"`
		AppID                     int    `json:"appId"`
		Signature                 string `json:"signature"`
		ScopeToScopeGroupPks      string `json:"scopeToScopeGroupPks"`
		NewSignature              string `json:"newSignature"`
		Hublet                    string `json:"hublet"`
		TrialScopes               string `json:"trialScopes"`
		TrialScopeToScopeGroupPks string `json:"trialScopeToScopeGroupPks"`
		IsUserLevel               bool   `json:"isUserLevel"`
	} `json:"signed_access_token"`
	HubID     int    `json:"hub_id"`
	AppID     int    `json:"app_id"`
	ExpiresIn int    `json:"expires_in"`
	UserID    int    `json:"user_id"`
	TokenType string `json:"token_type"`
}

func (p *OAuth2Provider) parseHubSpotUserInfo(data []byte) (*types.OAuthUserInfo, error) {
	var userInfo HubSpotUserInfo
	if err := json.Unmarshal(data, &userInfo); err != nil {
		return nil, fmt.Errorf("failed to parse hubspot user info: %w", err)
	}

	return &types.OAuthUserInfo{
		ID:          fmt.Sprintf("%d", userInfo.UserID),
		Email:       userInfo.User,
		Name:        userInfo.User,
		DisplayName: userInfo.User,
		AvatarURL:   "",
		Raw:         string(data),
	}, nil
}
