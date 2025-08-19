package azuredevops

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/agent"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/util/jsonschema"

	"github.com/jfrog/froggit-go/vcsclient"
	"github.com/jfrog/froggit-go/vcsutils"

	"github.com/rs/zerolog/log"
	"github.com/sashabaranov/go-openai"
)

const pullRequestDiffSkillDescription = `Get the diff of a pull request, use it when you need to see the changes in a pull request.
DO NOT try to pass repository ID or project ID to this skill, it is set automatically to the correct values by the trigger.`

var pullRequestDiffParameters = jsonschema.Definition{
	Type:       jsonschema.Object,
	Properties: map[string]jsonschema.Definition{},
	Required:   []string{},
}

func NewPullRequestDiffSkill(organizationURL string, personalAccessToken string) agent.Skill {
	// client := NewAzureDevOpsClient(organizationURL, personalAccessToken)
	return agent.Skill{
		Name:        "AzureDevOpsPullRequestDiff",
		Description: pullRequestDiffSkillDescription,
		Parameters:  pullRequestDiffParameters,
		Tools: []agent.Tool{
			&AzureDevOpsPullRequestDiffTool{
				orgURL: organizationURL,
				token:  personalAccessToken,
			},
		},
	}
}

// AzureDevOpsPullRequestDiffTool - allows the agent to get pull request diffs
type AzureDevOpsPullRequestDiffTool struct { //nolint:revive
	// client *AzureDevOpsClient
	orgURL string
	token  string
}

func (t *AzureDevOpsPullRequestDiffTool) Name() string {
	return "PullRequestDiff"
}

func (t *AzureDevOpsPullRequestDiffTool) Description() string {
	return "Get the diff of a pull request"
}

func (t *AzureDevOpsPullRequestDiffTool) Icon() string {
	return ""
}

func (t *AzureDevOpsPullRequestDiffTool) String() string {
	return "PullRequestDiff"
}

func (t *AzureDevOpsPullRequestDiffTool) StatusMessage() string {
	return "Getting pull request diff"
}

func (t *AzureDevOpsPullRequestDiffTool) OpenAI() []openai.Tool {
	return []openai.Tool{
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "PullRequestDiff",
				Description: pullRequestDiffSkillDescription,
				Parameters:  pullRequestDiffParameters,
			},
		},
	}
}

func (t *AzureDevOpsPullRequestDiffTool) Execute(ctx context.Context, _ agent.Meta, args map[string]interface{}) (string, error) {
	azureCtx, ok := types.GetAzureDevopsRepositoryContext(ctx)
	if !ok {
		return "", fmt.Errorf("azure devops repository context not found")
	}

	// Get pull request details and diffs
	diffResult, err := GetPullRequestDiff(ctx, t.orgURL, t.token, azureCtx)
	if err != nil {
		return "", fmt.Errorf("failed to get pull request diff: %w", err)
	}

	// Format the response
	response := fmt.Sprintln("Pull Request Details:")
	response += fmt.Sprintf("Title: %s\n", diffResult.PullRequest.Title)
	response += fmt.Sprintf("Description: %s\n", diffResult.PullRequest.Body)
	response += fmt.Sprintf("Author: %s\n", diffResult.PullRequest.Author)

	for _, change := range diffResult.Changes {
		response += fmt.Sprintf("Path: %s\n", change.Path)
		response += fmt.Sprintf("Change Type: %s\n", change.ChangeType)
		response += fmt.Sprintf("Content: %s\n", change.Content)
		response += fmt.Sprintf("Content Length: %d\n", change.ContentLength)
		response += fmt.Sprintf("Content Type: %s\n", change.ContentType)
	}

	return response, nil
}

// PullRequestDiffResult contains the diff information for a pull request
type PullRequestDiffResult struct {
	Changes     []*PullRequestChange      `json:"changes"`
	PullRequest vcsclient.PullRequestInfo `json:"pull_request"`
}

type PullRequestChange struct {
	Path          string      `json:"path"`
	ChangeType    string      `json:"change_type"`
	Content       string      `json:"content"`
	ContentLength int         `json:"content_length"`
	ContentType   string      `json:"content_type"`
	Encoding      string      `json:"encoding"`
	IsBinary      bool        `json:"is_binary"`
	Hunks         []*DiffHunk `json:"hunks,omitempty"`
	LinesAdded    int         `json:"lines_added"`
	LinesDeleted  int         `json:"lines_deleted"`
}

type DiffHunk struct {
	Header   string      `json:"header"`
	OldStart int         `json:"old_start"`
	OldLines int         `json:"old_lines"`
	NewStart int         `json:"new_start"`
	NewLines int         `json:"new_lines"`
	Lines    []*DiffLine `json:"lines"`
}

type DiffLine struct {
	Type    string `json:"type"`    // "added", "deleted", "context", "unchanged"
	Content string `json:"content"` // The actual line content
	OldNum  int    `json:"old_num"` // Line number in old file (0 if added)
	NewNum  int    `json:"new_num"` // Line number in new file (0 if deleted)
}

// GetPullRequestDiff is a reusable function that gets pull request diffs and can be used
// by other functions for reviews, analysis, etc.
func GetPullRequestDiff(ctx context.Context, orgURL, token string, azureCtx types.AzureDevopsRepositoryContext) (*PullRequestDiffResult, error) {
	logger := log.Ctx(ctx)

	logger.Debug().
		Str("repository_id", azureCtx.RepositoryID).
		Str("project_id", azureCtx.ProjectID).
		Int("pull_request_id", azureCtx.PullRequestID).
		Msg("Getting pull request diff")

		// The VCS provider. Cannot be changed.
	vcsProvider := vcsutils.AzureRepos
	// API endpoint to Azure Repos. Set the organization.

	client, err := vcsclient.NewClientBuilder(vcsProvider).ApiEndpoint(orgURL).Token(token).Project(azureCtx.ProjectID).Build()
	if err != nil {
		return nil, fmt.Errorf("failed to create vcs client: %w", err)
	}

	err = client.TestConnection(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to test connection: %w", err)
	}

	pr, err := client.GetPullRequestByID(ctx, azureCtx.ProjectID, azureCtx.RepositoryID, azureCtx.PullRequestID)
	if err != nil {
		return nil, fmt.Errorf("failed to get pull request details: %w", err)
	}

	changes, err := GetPullRequestChange(ctx, token, GetPullRequestChangeOptions{
		RemoteURL:      azureCtx.RemoteURL,
		TargetCommitID: azureCtx.LastMergeTargetCommitID,
		SourceCommitID: azureCtx.LastMergeSourceCommitID,
		Owner:          azureCtx.ProjectID,
		Repository:     azureCtx.RepositoryID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get pull request changes: %w", err)
	}

	return &PullRequestDiffResult{
		Changes:     changes,
		PullRequest: pr,
	}, nil
}
