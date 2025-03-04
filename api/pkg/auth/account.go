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

func (account *account) isUserAdmin(cfg *AdminConfig) bool {
	for _, adminID := range cfg.AdminUserIDs {
		// development mode everyone is an admin
		if adminID == types.AdminAllUsers {
			return true
		}
		if adminID == account.userInfo.Subject {
			return true
		}
	}
	return false
}

func (auth *account) isTokenAdmin() bool {
	if auth.userInfo == nil {
		return false
	}
	return auth.userInfo.Admin
}
