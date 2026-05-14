package oauth

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/helixml/helix/api/pkg/types"
)

// notionAPIVersion is sent on every Notion API request via the Notion-Version
// header. Notion versions its API by date string and rejects calls without
// this header. Bump centrally when we upgrade.
const notionAPIVersion = "2025-09-03"

// notionUsersMeResponse is the shape Notion returns from GET /v1/users/me when
// called with an OAuth-integration access token. The token belongs to a bot,
// so the integration's owning user info is nested under bot.owner.user.
type notionUsersMeResponse struct {
	Object    string `json:"object"`
	ID        string `json:"id"`
	Type      string `json:"type"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
	Bot       *struct {
		Owner *struct {
			Type string `json:"type"`
			User *struct {
				ID        string `json:"id"`
				Name      string `json:"name"`
				AvatarURL string `json:"avatar_url"`
				Person    *struct {
					Email string `json:"email"`
				} `json:"person"`
			} `json:"user"`
		} `json:"owner"`
		WorkspaceName string `json:"workspace_name"`
	} `json:"bot"`
}

// parseNotionUserInfo extracts a standardised OAuthUserInfo from Notion's
// /v1/users/me response. Falls back to bot-level fields if the owner shape
// isn't present (e.g. workspace-level installs).
func parseNotionUserInfo(body []byte) (*types.OAuthUserInfo, error) {
	var r notionUsersMeResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("parse notion user info: %w", err)
	}

	info := &types.OAuthUserInfo{Raw: string(body)}

	if r.Bot != nil && r.Bot.Owner != nil && r.Bot.Owner.User != nil {
		u := r.Bot.Owner.User
		info.ID = u.ID
		info.Name = u.Name
		info.DisplayName = u.Name
		info.AvatarURL = u.AvatarURL
		if u.Person != nil {
			info.Email = u.Person.Email
		}
	}

	if info.ID == "" {
		// Fall back to bot identity (workspace-level install or unusual shape).
		info.ID = r.ID
		info.Name = r.Name
		info.DisplayName = r.Name
		info.AvatarURL = r.AvatarURL
	}

	if info.ID == "" {
		return nil, errors.New("notion user info missing id")
	}
	return info, nil
}
