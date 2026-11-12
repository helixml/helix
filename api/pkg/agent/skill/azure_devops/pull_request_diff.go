package azuredevops

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/helixml/helix/api/pkg/netguard"

	"github.com/helixml/helix/api/pkg/agent"
	"github.com/helixml/helix/api/pkg/git"
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

func NewPullRequestDiffSkill(organizationURL string, personalAccessToken string, outbound netguard.Policy) agent.Skill {

	return agent.Skill{
		Name:        "AzureDevOpsPullRequestDiff",
		Description: pullRequestDiffSkillDescription,
		Parameters:  pullRequestDiffParameters,
		Direct:      true,
		Tools: []agent.Tool{
			&AzureDevOpsPullRequestDiffTool{
				orgURL:   organizationURL,
				token:    personalAccessToken,
				outbound: outbound,
			},
		},
	}
}

// AzureDevOpsPullRequestDiffTool - allows the agent to get pull request diffs
type AzureDevOpsPullRequestDiffTool struct { //nolint:revive
	// client *AzureDevOpsClient
	orgURL   string
	token    string
	outbound netguard.Policy
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

func (t *AzureDevOpsPullRequestDiffTool) Execute(ctx context.Context, _ agent.Meta, _ map[string]interface{}) (string, error) {
	azureCtx, ok := types.GetAzureDevopsRepositoryContext(ctx)
	if !ok {
		return "", fmt.Errorf("azure devops repository context not found")
	}

	// Get pull request details and diffs
	diffResult, err := GetPullRequestDiff(ctx, t.orgURL, t.token, azureCtx, t.outbound)
	if err != nil {
		return "", fmt.Errorf("failed to get pull request diff: %w", err)
	}

	// Format the response
	response := fmt.Sprintln("Pull Request Details:")
	response += fmt.Sprintf("Title: %s\n", diffResult.PullRequest.Title)
	response += fmt.Sprintf("Description: %s\n", diffResult.PullRequest.Body)
	response += fmt.Sprintf("Author: %s\n", diffResult.PullRequest.Author)

	for _, change := range diffResult.Changes {
		response += fmt.Sprintf("Content: %s\n", change.Content)
	}

	return response, nil
}

// GetPullRequestDiff is a reusable function that gets pull request diffs and can be used
// by other functions for reviews, analysis, etc.
// The organization URL is user-supplied and the remote URL comes from the
// webhook payload: every connection, the SDK's and git's, is held to outbound.
func GetPullRequestDiff(ctx context.Context, orgURL, token string, azureCtx types.AzureDevopsRepositoryContext, outbound netguard.Policy) (*git.PullRequestDiffResult, error) {
	logger := log.Ctx(ctx)
	ctx = outbound.GuardContext(ctx)

	logger.Debug().
		Str("repository_id", azureCtx.RepositoryID).
		Str("project_id", azureCtx.ProjectID).
		Int("pull_request_id", azureCtx.PullRequestID).
		Msg("Getting pull request diff")

	// The remote URL comes from the webhook payload and is cloned with the
	// configured token, so it must point at the configured organization's host.
	if err := checkRemoteURLMatchesOrg(azureCtx.RemoteURL, orgURL); err != nil {
		return nil, err
	}

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

	changes, err := git.GetPullRequestChange(ctx, token, git.GetPullRequestChangeOptions{
		RemoteURL:      azureCtx.RemoteURL,
		TargetCommitID: azureCtx.LastMergeTargetCommitID,
		SourceCommitID: azureCtx.LastMergeSourceCommitID,
		Owner:          azureCtx.ProjectID,
		Repository:     azureCtx.RepositoryID,
		Outbound:       &outbound,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get pull request changes: %w", err)
	}

	return &git.PullRequestDiffResult{
		Changes:     changes,
		PullRequest: pr,
	}, nil
}

// checkRemoteURLMatchesOrg requires remoteURL to be an https URL on the same
// host and port as the configured Azure DevOps organization URL (default
// ports normalised, so https://host and https://host:443 match).
func checkRemoteURLMatchesOrg(remoteURL, orgURL string) error {
	remote, err := url.Parse(remoteURL)
	if err != nil || remote.Scheme != "https" || remote.Hostname() == "" {
		return fmt.Errorf("repository remote URL %q is not an https URL", remoteURL)
	}
	org, err := url.Parse(orgURL)
	if err != nil || org.Hostname() == "" {
		return fmt.Errorf("invalid organization URL %q", orgURL)
	}
	if !strings.EqualFold(remote.Hostname(), org.Hostname()) || effectivePort(remote) != effectivePort(org) {
		return fmt.Errorf("repository remote URL host %q does not match organization host %q", remote.Host, org.Host)
	}
	return nil
}

// effectivePort returns u's port, or the scheme's default port.
func effectivePort(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	if strings.EqualFold(u.Scheme, "http") {
		return "80"
	}
	return "443"
}
