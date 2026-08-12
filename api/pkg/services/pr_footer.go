package services

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"text/template"

	"github.com/helixml/helix/api/pkg/types"
)

const DefaultPRFooterTemplate = `---
{{- if .HelixTaskURL }}
🔗 [Open in Helix]({{.HelixTaskURL}})
{{- end }}
{{- if .SpecDocsURL }}

📋 Spec:
- [Requirements]({{.SpecDocsURL}}/requirements.md)
- [Design]({{.SpecDocsURL}}/design.md)
- [Tasks]({{.SpecDocsURL}}/tasks.md)
{{- end }}

🚀 Built with [Helix](https://helix.ml)`

type PRFooterTemplateData struct {
	HelixTaskURL string
	SpecDocsURL  string
}

func ValidatePRFooterTemplate(value string) error {
	tmpl, err := template.New("pr-footer").Option("missingkey=error").Parse(value)
	if err != nil {
		return fmt.Errorf("invalid PR footer template: %w", err)
	}
	if err := tmpl.Execute(io.Discard, PRFooterTemplateData{}); err != nil {
		return fmt.Errorf("invalid PR footer template: %w", err)
	}
	return nil
}

func RenderPRFooter(value string, repo *types.GitRepository, task *types.SpecTask, orgName, helixBaseURL string) (string, error) {
	if value == "" {
		return "", nil
	}

	tmpl, err := template.New("pr-footer").Option("missingkey=error").Parse(value)
	if err != nil {
		return "", fmt.Errorf("parse PR footer template: %w", err)
	}

	data := PRFooterTemplateData{}
	if helixBaseURL != "" && orgName != "" && task.ProjectID != "" && task.ID != "" {
		data.HelixTaskURL = fmt.Sprintf("%s/orgs/%s/projects/%s/tasks/%s",
			strings.TrimSuffix(helixBaseURL, "/"), orgName, task.ProjectID, task.ID)
	}
	if task.DesignDocPath != "" {
		data.SpecDocsURL = getSpecDocsBaseURL(repo, task.DesignDocPath)
	}

	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, data); err != nil {
		return "", fmt.Errorf("render PR footer template: %w", err)
	}
	return strings.TrimSpace(rendered.String()), nil
}

func AppendPRFooter(description, footer string) string {
	if footer == "" {
		return description
	}
	if strings.TrimSpace(description) == "" {
		return footer
	}
	return description + "\n\n" + footer
}

func UserPRFooterTemplate(user *types.User) string {
	if user == nil || user.PRFooterTemplate == nil {
		return DefaultPRFooterTemplate
	}
	return *user.PRFooterTemplate
}
