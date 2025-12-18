package auth

import (
	"github.com/helixml/helix/api/pkg/config"
)

// AdminConfig holds configuration for admin status determination
type AdminConfig struct {
	// AdminUsers can be "all" for dev mode (everyone is admin)
	AdminUsers string
}

// IsAllUsersAdmin returns true if ADMIN_USERS=all (dev mode)
func (cfg *AdminConfig) IsAllUsersAdmin() bool {
	return cfg.AdminUsers == config.AdminAllUsers
}
