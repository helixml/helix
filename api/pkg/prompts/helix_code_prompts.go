package prompts

import (
	"bytes"
	"errors"
	"text/template"

	"github.com/helixml/helix/api/pkg/prompts/templates"
)

func ImplementationApprovedPushInstruction(branchName string) (string, error) {
	if branchName == "" {
		return "", errors.New("branch name is required")
	}

	tmplData := struct {
		BranchName string
	}{
		BranchName: branchName,
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
