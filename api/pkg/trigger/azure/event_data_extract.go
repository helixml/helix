package azure

import (
	"bytes"
	"html/template"
	"strconv"
	"strings"
)

var pullRequestCommentedEventTemplate = `Here's the Azure DevOps Pull Request Comment Event:
- Event Type: {{.EventType}}
- What happened: {{.Message.Text}}
- User message: {{.Resource.Comment.Content}}

Reply to the user's message.
`

var pullRequestCommentedEventTmpl = template.Must(template.New("pullRequestCommentedEvent").Parse(pullRequestCommentedEventTemplate))

func renderPullRequestCommentedEvent(prc PullRequestComment) (string, error) {
	var buf bytes.Buffer
	err := pullRequestCommentedEventTmpl.Execute(&buf, prc)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

var pullRequestCreatedUpdatedEventTemplate = `Azure DevOps Pull Request {{if eq .EventType "git.pullrequest.created"}}Created{{else}}Updated{{end}} Event

EVENT DETAILS:
- Event Type: {{.EventType}}
- Event ID: {{.ID}}
- Created Date: {{.CreatedDate.Format "2006-01-02T15:04:05Z07:00"}}

PULL REQUEST DETAILS:
- PR ID: {{.Resource.PullRequestID}}
- Code Review ID: {{.Resource.CodeReviewID}}
- Title: {{.Resource.Title}}
- Description: {{.Resource.Description}}
- Status: {{.Resource.Status}}
- Is Draft: {{.Resource.IsDraft}}
- Merge Status: {{.Resource.MergeStatus}}
- Source Branch: {{.Resource.SourceRefName}}
- Target Branch: {{.Resource.TargetRefName}}
- Creation Date: {{.Resource.CreationDate.Format "2006-01-02T15:04:05Z07:00"}}

PULL REQUEST CREATOR:
- Display Name: {{.Resource.CreatedBy.DisplayName}}
- Unique Name: {{.Resource.CreatedBy.UniqueName}}
- ID: {{.Resource.CreatedBy.ID}}
- Email: {{.Resource.CreatedBy.UniqueName}}

REPOSITORY INFORMATION:
- Repository Name: {{.Resource.Repository.Name}}
- Repository ID: {{.Resource.Repository.ID}}
- Web URL: {{.Resource.Repository.WebURL}}

PROJECT INFORMATION:
- Project Name: {{.Resource.Repository.Project.Name}}
- Project ID: {{.Resource.Repository.Project.ID}}
- Project State: {{.Resource.Repository.Project.State}}
- Project Visibility: {{.Resource.Repository.Project.Visibility}}

COMMIT INFORMATION:
- Last Merge Source Commit: {{.Resource.LastMergeSourceCommit.CommitID}}
- Last Merge Target Commit: {{.Resource.LastMergeTargetCommit.CommitID}}
- Last Merge Commit: {{.Resource.LastMergeCommit.CommitID}}

LINKS:
- Pull Request URL: {{.Resource.URL}}
- Web URL: {{.Resource.Links.Web.Href}}

ORGANIZATION:
- Collection ID: {{.ResourceContainers.Collection.ID}}
- Account ID: {{.ResourceContainers.Account.ID}}
- Account Base URL: {{.ResourceContainers.Account.BaseURL}}

This event can be used to:
- Review pull request content and changes
- Analyze code quality and provide feedback
- Check for compliance with coding standards
- Suggest improvements or identify potential issues
- Automate code review processes
- Route to appropriate reviewers based on changes
`

var pullRequestCreatedUpdatedEventTmpl = template.Must(template.New("pullRequestCreatedUpdatedEvent").Parse(pullRequestCreatedUpdatedEventTemplate))

func renderPullRequestCreatedUpdatedEvent(pr PullRequest) (string, error) {
	var buf bytes.Buffer
	err := pullRequestCreatedUpdatedEventTmpl.Execute(&buf, pr)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

func getThreadID(pr PullRequestComment) int {
	// URL for example:
	// "https://dev.azure.com/helixml/_apis/git/repositories/73c763d4-bf41-49da-8481-896a4980b07c/pullRequests/1/threads/2"
	threadURL := pr.Resource.Comment.Links.Threads.Href

	// Split and get the last part
	parts := strings.Split(threadURL, "/")
	threadID := parts[len(parts)-1]

	// Convert to int
	threadIDInt, err := strconv.Atoi(threadID)
	if err != nil {
		return 0
	}

	return threadIDInt
}
