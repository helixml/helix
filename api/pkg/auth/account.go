package auth

import (
	"github.com/helixml/helix/api/pkg/config"
)

// AdminConfig holds configuration for admin status determination
type AdminConfig struct {
	// AdminUserIDs is a list of user IDs that should be admins.
	// Can contain "all" for dev mode (everyone is admin), or specific user IDs.
	AdminUserIDs []string
}

// IsAllUsersAdmin returns true if ADMIN_USER_IDS contains "all" (dev mode)
func (cfg *AdminConfig) IsAllUsersAdmin() bool {
	for _, id := range cfg.AdminUserIDs {
		if id == config.AdminAllUsers {
			return true
		}
	}
	return false
}

// IsUserInAdminList returns true if the given user ID is in the admin list
func (cfg *AdminConfig) IsUserInAdminList(userID string) bool {
	if userID == "" {
		return false
	}
	for _, id := range cfg.AdminUserIDs {
		if id == config.AdminAllUsers {
			return true // "all" means everyone is admin
		}
		if id == userID {
			return true
		}
	}
	return false
}
