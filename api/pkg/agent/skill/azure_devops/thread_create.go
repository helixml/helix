package azuredevops

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/helixml/helix/api/pkg/agent"
	"github.com/helixml/helix/api/pkg/ptr"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/util/jsonschema"

	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/git"
	"github.com/sashabaranov/go-openai"
)

const createThreadSkillDescription = `Create a thread in a pull request, use it when you need to add a comment to a pull request.
DO NOT try to pass repository ID or project ID to this skill, it is set automatically to the correct values by the trigger.`

var createThreadParameters = jsonschema.Definition{
	Type: jsonschema.Object,
	Properties: map[string]jsonschema.Definition{
		"content": {
			Type:        jsonschema.String,
			Description: "The content of the thread",
		},
		"file_path": {
			Type:        jsonschema.String,
			Description: "The filepath of the file that contains the code that you want to comment on. Should be relative to the root of the repository and should have / prefix",
		},
		"line_number": {
			Type:        jsonschema.Integer,
			Description: "The line number of the file that you want to add a comment to. Starts at 1.",
		},
	},
	Required: []string{"content"},
}

// NewCreateThreadSkill - creates a skill that allows the agent to create a thread in a pull request. This is a dedicated skill
// that expects the agent to be told what comment to write
func NewCreateThreadSkill(organizationURL string, personalAccessToken string) agent.Skill {
	client := NewAzureDevOpsClient(organizationURL, personalAccessToken)
	return agent.Skill{
		Name:        "AzureDevOpsCreateThread",
		Description: createThreadSkillDescription,
		Parameters:  createThreadParameters,
		Direct:      true,
		Tools: []agent.Tool{
			&AzureDevOpsPullRequestCreateThreadTool{
				client: client,
			},
		},
	}
}

// AzureDevOpsPullRequestCommentTool - allows the agent to comment on a pull request
type AzureDevOpsPullRequestCreateThreadTool struct { //nolint:revive
	client *AzureDevOpsClient
}

func (t *AzureDevOpsPullRequestCreateThreadTool) Name() string {
	return "CreateThread"
}

func (t *AzureDevOpsPullRequestCreateThreadTool) Description() string {
	return "Create a thread in a pull request, useful for adding comments to a pull request"
}

func (t *AzureDevOpsPullRequestCreateThreadTool) Icon() string {
	return ""
}

func (t *AzureDevOpsPullRequestCreateThreadTool) String() string {
	return "CreateThread"
}

func (t *AzureDevOpsPullRequestCreateThreadTool) StatusMessage() string {
	return "Creating a thread in a pull request"
}

func (t *AzureDevOpsPullRequestCreateThreadTool) OpenAI() []openai.Tool {
	return []openai.Tool{
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "CreateThread",
				Description: createThreadSkillDescription,
				Parameters:  createThreadParameters,
			},
		},
	}
}

func (t *AzureDevOpsPullRequestCreateThreadTool) Execute(ctx context.Context, _ agent.Meta, args map[string]interface{}) (string, error) {
	content, ok := args["content"].(string)
	if !ok {
		return "", fmt.Errorf("content is required")
	}

	threadContext := git.CommentThreadContext{}
	threadContextSet := false

	filepathIntf, ok := args["file_path"]
	if ok {
		filepath := filepathIntf.(string)

		// Adding prefix, otherwise ADO doesn't understand the filepath
		if !strings.HasPrefix(filepath, "/") {
			filepath = "/" + filepath
		}

		threadContext.FilePath = &filepath
		threadContextSet = true
	}

	lineNumberIntf, ok := args["line_number"]
	if ok {
		lineNumber, err := parseInt(lineNumberIntf)
		if err != nil {
			return "", fmt.Errorf("failed to convert line_number to integer: %w", err)
		}
		if lineNumber == 0 {
			lineNumber = 1
		}
		offset := 1
		threadContext.RightFileStart = &git.CommentPosition{
			Line:   &lineNumber,
			Offset: &offset,
		}
		threadContext.RightFileEnd = &git.CommentPosition{
			Line:   &lineNumber,
			Offset: &offset,
		}
		threadContextSet = true
	}

	azureCtx, ok := types.GetAzureDevopsRepositoryContext(ctx)
	if !ok {
		return "", fmt.Errorf("azure devops repository context not found")
	}

	gitClient, err := git.NewClient(ctx, t.client.connection)
	if err != nil {
		return "", fmt.Errorf("failed to create Azure DevOps client: %w", err)
	}

	content = fmt.Sprintf("[Helix] %s", content)

	comment := []git.Comment{
		{
			Content: &content,
		},
	}

	status := git.CommentThreadStatus("active")

	createThreadArgs := git.CreateThreadArgs{
		CommentThread: &git.GitPullRequestCommentThread{
			Status:   &status,
			Comments: &comment,
			PullRequestThreadContext: &git.GitPullRequestCommentThreadContext{
				ChangeTrackingId: ptr.To(1),
			},
		},
		RepositoryId:  &azureCtx.RepositoryID,
		PullRequestId: &azureCtx.PullRequestID,
		Project:       &azureCtx.ProjectID,
	}

	if threadContextSet {
		createThreadArgs.CommentThread.ThreadContext = &threadContext
	}

	createdThread, err := gitClient.CreateThread(ctx, createThreadArgs)
	if err != nil {
		return "", fmt.Errorf("failed to create thread: %w", err)
	}

	return fmt.Sprintf("Thread created: %d", createdThread.Id), nil
}

func parseInt(value any) (int, error) {
	switch v := value.(type) {
	case int:
		return v, nil
	case float64:
		return int(v), nil
	case string:
		return strconv.Atoi(v)
	}
	return 0, fmt.Errorf("invalid integer value")
}
