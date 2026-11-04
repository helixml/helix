package types

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateGitBranchName(t *testing.T) {
	valid := []string{
		"main",
		"feature/user-auth",
		"feature/003941-tst-01-193-path",
		"release/2.11.x",
		"Fix_Login",
		"users/a.b/c",
	}
	for _, name := range valid {
		require.NoError(t, ValidateGitBranchName(name), name)
	}

	invalid := []string{
		"",
		"../../main",
		"../../../evil",
		"feature/../main",
		"-D",
		"--orphan",
		"/etc/passwd",
		"feature/",
		"feature//x",
		".hidden",
		"feature/.hidden",
		"main.lock",
		"feature/x.lock/y",
		"main.",
		"a b",
		"a\tb",
		"a\nb",
		"a~1",
		"a^",
		"a:b",
		"a?",
		"a*",
		"a[b",
		`a\b`,
		"a@{1}",
		"@",
		"a\x7f",
		strings.Repeat("a", MaxGitBranchNameLength+1),
	}
	for _, name := range invalid {
		require.Error(t, ValidateGitBranchName(name), "%q", name)
	}
}

func TestCreateTaskRequestValidateBranchFields(t *testing.T) {
	require.NoError(t, (&CreateTaskRequest{}).ValidateBranchFields())
	require.NoError(t, (&CreateTaskRequest{BaseBranch: "main", BranchPrefix: "feature/auth", WorkingBranch: "feature/x"}).ValidateBranchFields())

	err := (&CreateTaskRequest{BaseBranch: "../../main"}).ValidateBranchFields()
	require.ErrorContains(t, err, "base_branch")
	err = (&CreateTaskRequest{BranchPrefix: "../../../evil"}).ValidateBranchFields()
	require.ErrorContains(t, err, "branch_prefix")
	err = (&CreateTaskRequest{WorkingBranch: "-D"}).ValidateBranchFields()
	require.ErrorContains(t, err, "working_branch")
}
