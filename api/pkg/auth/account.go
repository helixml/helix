package auth

import (
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/types"
)

type AdminConfig struct {
	AdminUserIDs []string
	AdminUserSrc config.AdminSrcType
}

type account struct {
	userInfo *UserInfo
}

func (acct *account) isAdmin(cfg *AdminConfig) bool {
	switch cfg.AdminUserSrc {
	case config.AdminSrcTypeEnv:
		return acct.isUserAdmin(cfg)
	case config.AdminSrcTypeJWT:
		return acct.isTokenAdmin()
	}
	return false
}

func (acct *account) isUserAdmin(cfg *AdminConfig) bool {
	for _, adminID := range cfg.AdminUserIDs {
		// development mode everyone is an admin
		if adminID == types.AdminAllUsers {
			return true
		}
		if adminID == acct.userInfo.Subject {
			return true
		}
	}
	return false
}

func (acct *account) isTokenAdmin() bool {
	if acct.userInfo == nil {
		return false
	}
	return acct.userInfo.Admin
}
