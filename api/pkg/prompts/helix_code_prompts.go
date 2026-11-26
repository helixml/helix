package prompts

import (
	"bytes"
	"errors"
	"text/template"

	"github.com/helixml/helix/api/pkg/prompts/templates"
)

// PullRequestStatusGuidance prevents task agents from polling provider APIs anonymously.
func PullRequestStatusGuidance() string {
	return "**CI and review status:** After pushing, use CI result notifications when available; do not start CI polling loops or background pollers. " +
		"For one-off status, pull request, or review checks, use authenticated information or provider tools already available in the session. " +
		"If authenticated access is unavailable, report that instead of falling back to anonymous provider API requests. Stop on every non-success HTTP response; do not treat it as a pending result. Honor `Retry-After` and `X-RateLimit-Reset`."
}

func ImplementationApprovedPushInstruction(branchName, primaryRepoName, baseBranch, taskDirName string, nonPrimaryRepoNames []string) (string, error) {
	if branchName == "" {
		return "", errors.New("branch name is required")
	}
	if taskDirName == "" {
		return "", errors.New("task directory name is required")
	}

	tmplData := struct {
		BranchName          string
		PrimaryRepoName     string
		BaseBranch          string
		TaskDirName         string
		NonPrimaryRepoNames []string
	}{
		BranchName:          branchName,
		PrimaryRepoName:     primaryRepoName,
		BaseBranch:          baseBranch,
		TaskDirName:         taskDirName,
		NonPrimaryRepoNames: nonPrimaryRepoNames,
	}
	tmpl := template.Must(template.New("ImplementationApprovedPushPrompt").Parse(templates.ImplementationApprovedPushPrompt))
	var buf bytes.Buffer
	err := tmpl.Execute(&buf, tmplData)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

// RebaseRequiredInstruction returns a prompt instructing the agent to rebase/merge
// their branch with the default branch to resolve conflicts before merge can complete.
func RebaseRequiredInstruction(branchName, defaultBranch string) (string, error) {
	if branchName == "" {
		return "", errors.New("branch name is required")
	}
	if defaultBranch == "" {
		return "", errors.New("default branch is required")
	}

	tmplData := struct {
		BranchName    string
		DefaultBranch string
	}{
		BranchName:    branchName,
		DefaultBranch: defaultBranch,
	}
	tmpl := template.Must(template.New("RebaseRequiredPrompt").Parse(templates.RebaseRequiredPrompt))
	var buf bytes.Buffer
	err := tmpl.Execute(&buf, tmplData)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}
