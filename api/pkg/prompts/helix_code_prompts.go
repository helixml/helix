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
