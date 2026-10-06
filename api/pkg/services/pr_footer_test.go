package services

import (
	"testing"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderDefaultPRFooter(t *testing.T) {
	repo := &types.GitRepository{
		ExternalURL:  "https://github.com/acme/widgets.git",
		ExternalType: types.ExternalRepositoryTypeGitHub,
	}
	task := &types.SpecTask{
		ID:            "task_123",
		ProjectID:     "project_123",
		DesignDocPath: "123-widget",
	}

	footer, err := RenderPRFooter(DefaultPRFooterTemplate, repo, task, "acme", "https://app.helix.ml/")
	require.NoError(t, err)
	assert.Equal(t, `---
🔗 [Open in Helix](https://app.helix.ml/orgs/acme/projects/project_123/tasks/task_123)

📋 Spec:
- [Requirements](https://github.com/acme/widgets/blob/helix-specs/design/tasks/123-widget/requirements.md)
- [Design](https://github.com/acme/widgets/blob/helix-specs/design/tasks/123-widget/design.md)
- [Tasks](https://github.com/acme/widgets/blob/helix-specs/design/tasks/123-widget/tasks.md)

🚀 Built with [Helix](https://helix.ml)`, footer)
}

func TestRenderDefaultPRFooterJustDoItOmitsSpecLinks(t *testing.T) {
	repo := &types.GitRepository{
		ExternalURL:  "https://github.com/acme/widgets.git",
		ExternalType: types.ExternalRepositoryTypeGitHub,
	}
	task := &types.SpecTask{
		ID:            "task_123",
		ProjectID:     "project_123",
		DesignDocPath: "123-widget",
		JustDoItMode:  true,
	}

	footer, err := RenderPRFooter(DefaultPRFooterTemplate, repo, task, "acme", "https://app.helix.ml/")
	require.NoError(t, err)
	assert.Equal(t, `---
🔗 [Open in Helix](https://app.helix.ml/orgs/acme/projects/project_123/tasks/task_123)

🚀 Built with [Helix](https://helix.ml)`, footer)
}

func TestRenderCustomAndEmptyPRFooter(t *testing.T) {
	task := &types.SpecTask{ID: "task_123", ProjectID: "project_123"}

	footer, err := RenderPRFooter("Task: {{.HelixTaskURL}}", &types.GitRepository{}, task, "acme", "https://app.helix.ml")
	require.NoError(t, err)
	assert.Equal(t, "Task: https://app.helix.ml/orgs/acme/projects/project_123/tasks/task_123", footer)

	footer, err = RenderPRFooter("", &types.GitRepository{}, task, "acme", "https://app.helix.ml")
	require.NoError(t, err)
	assert.Empty(t, footer)
	assert.Equal(t, "description", AppendPRFooter("description", footer))
}

func TestValidatePRFooterTemplateRejectsUnknownFields(t *testing.T) {
	err := ValidatePRFooterTemplate("{{.Unknown}}")
	require.Error(t, err)
}
