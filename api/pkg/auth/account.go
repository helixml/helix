package auth

import (
	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/types"
)

type AdminConfig struct {
	AdminUserIDs []string
	AdminUserSrc config.AdminSrcType
}

type account struct {
	userID string
	token  *tokenAcct
}

type tokenAcct struct {
	claims jwt.MapClaims
	userID string
}

type accountType string

const (
	accountTypeUser    accountType = "user"
	accountTypeToken   accountType = "token"
	accountTypeInvalid accountType = "invalid"
)

func (a *account) Type() accountType {
	switch {
	case a.userID != "":
		return accountTypeUser
	case a.token != nil:
		return accountTypeToken
	}
	return accountTypeInvalid
}

func (acct *account) isAdmin(cfg *AdminConfig) bool {
	if acct.Type() == accountTypeInvalid {
		return false
	}

	switch cfg.AdminUserSrc {
	case config.AdminSrcTypeEnv:
		if acct.Type() == accountTypeUser {
			return acct.isUserAdmin(acct.userID, cfg)
		}
		return acct.isUserAdmin(acct.token.userID, cfg)
	case config.AdminSrcTypeJWT:
		if acct.Type() != accountTypeToken {
			return false
		}
		return acct.isTokenAdmin(acct.token.claims)
	}
	return false
}

func (account *account) isUserAdmin(userID string, cfg *AdminConfig) bool {
	if userID == "" {
		return false
	}

	for _, adminID := range cfg.AdminUserIDs {
		// development mode everyone is an admin
		if adminID == types.AdminAllUsers {
			return true
		}
		if adminID == userID {
			return true
		}
	}
	return false
}

func (auth *account) isTokenAdmin(claims jwt.MapClaims) bool {
	if claims == nil {
		return false
	}
	isAdmin := claims["admin"].(bool)
	return isAdmin
}
