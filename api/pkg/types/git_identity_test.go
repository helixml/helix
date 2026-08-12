package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUserGitAuthorIdentity(t *testing.T) {
	user := &User{
		FullName:       "Account Name",
		Username:       "account-user",
		Email:          "account@example.com",
		GitCommitName:  "Commit Name",
		GitCommitEmail: "commit@example.com",
	}

	assert.Equal(t, "Commit Name", user.GitAuthorName())
	assert.Equal(t, "commit@example.com", user.GitAuthorEmail())

	user.GitCommitName = ""
	user.GitCommitEmail = ""
	assert.Equal(t, "Account Name", user.GitAuthorName())
	assert.Equal(t, "account@example.com", user.GitAuthorEmail())
}

func TestUserGitAuthorNameFallbacks(t *testing.T) {
	assert.Equal(t, "username", (&User{Username: "username", Email: "user@example.com"}).GitAuthorName())
	assert.Equal(t, "user", (&User{Email: "user@example.com"}).GitAuthorName())
}
