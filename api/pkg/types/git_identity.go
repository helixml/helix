package types

import "strings"

func (u *User) GitAuthorName() string {
	if u == nil {
		return ""
	}
	if name := strings.TrimSpace(u.GitCommitName); name != "" {
		return name
	}
	if name := strings.TrimSpace(u.FullName); name != "" {
		return name
	}
	if username := strings.TrimSpace(u.Username); username != "" {
		return username
	}
	if at := strings.IndexByte(u.Email, '@'); at > 0 {
		return u.Email[:at]
	}
	return strings.TrimSpace(u.Email)
}

func (u *User) GitAuthorEmail() string {
	if u == nil {
		return ""
	}
	if email := strings.TrimSpace(u.GitCommitEmail); email != "" {
		return email
	}
	return strings.TrimSpace(u.Email)
}
