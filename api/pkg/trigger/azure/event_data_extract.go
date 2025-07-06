package azure

import (
	"bytes"
	"html/template"
)

var pullRequestCommentedEventTemplate = `Azure DevOps Pull Request Comment Event

EVENT DETAILS:
- Event Type: {{.EventType}}
- Event ID: {{.ID}}
- Created Date: {{.CreatedDate.Format "2006-01-02T15:04:05Z07:00"}}

COMMENT INFORMATION:
- Comment ID: {{.Resource.Comment.ID}}
- Parent Comment ID: {{.Resource.Comment.ParentCommentID}}
- Comment Type: {{.Resource.Comment.CommentType}}
- Content: {{.Resource.Comment.Content}}
- Published Date: {{.Resource.Comment.PublishedDate.Format "2006-01-02T15:04:05Z07:00"}}
- Last Updated: {{.Resource.Comment.LastUpdatedDate.Format "2006-01-02T15:04:05Z07:00"}}

COMMENT AUTHOR:
- Display Name: {{.Resource.Comment.Author.DisplayName}}
- Unique Name: {{.Resource.Comment.Author.UniqueName}}
- ID: {{.Resource.Comment.Author.ID}}
- Email: {{.Resource.Comment.Author.UniqueName}}

PULL REQUEST DETAILS:
- PR ID: {{.Resource.PullRequest.PullRequestID}}
- Code Review ID: {{.Resource.PullRequest.CodeReviewID}}
- Title: {{.Resource.PullRequest.Title}}
- Description: {{.Resource.PullRequest.Description}}
- Status: {{.Resource.PullRequest.Status}}
- Is Draft: {{.Resource.PullRequest.IsDraft}}
- Merge Status: {{.Resource.PullRequest.MergeStatus}}
- Source Branch: {{.Resource.PullRequest.SourceRefName}}
- Target Branch: {{.Resource.PullRequest.TargetRefName}}

PULL REQUEST CREATOR:
- Display Name: {{.Resource.PullRequest.CreatedBy.DisplayName}}
- Unique Name: {{.Resource.PullRequest.CreatedBy.UniqueName}}
- ID: {{.Resource.PullRequest.CreatedBy.ID}}
- Email: {{.Resource.PullRequest.CreatedBy.UniqueName}}

REPOSITORY INFORMATION:
- Repository Name: {{.Resource.PullRequest.Repository.Name}}
- Repository ID: {{.Resource.PullRequest.Repository.ID}}
- Repository URL: {{.Resource.PullRequest.Repository.URL}}
- Web URL: {{.Resource.PullRequest.Repository.WebURL}}
- SSH URL: {{.Resource.PullRequest.Repository.SSHURL}}
- Remote URL: {{.Resource.PullRequest.Repository.RemoteURL}}

PROJECT INFORMATION:
- Project Name: {{.Resource.PullRequest.Repository.Project.Name}}
- Project ID: {{.Resource.PullRequest.Repository.Project.ID}}
- Project URL: {{.Resource.PullRequest.Repository.Project.URL}}
- Project State: {{.Resource.PullRequest.Repository.Project.State}}
- Project Visibility: {{.Resource.PullRequest.Repository.Project.Visibility}}

COMMIT INFORMATION:
- Last Merge Source Commit: {{.Resource.PullRequest.LastMergeSourceCommit.CommitID}}
- Last Merge Target Commit: {{.Resource.PullRequest.LastMergeTargetCommit.CommitID}}
- Last Merge Commit: {{.Resource.PullRequest.LastMergeCommit.CommitID}}

LINKS:
- Comment URL: {{.Resource.Comment.Links.Self.Href}}
- Pull Request URL: {{.Resource.PullRequest.URL}}
- Repository API URL: {{.Resource.PullRequest.Repository.URL}}

ORGANIZATION:
- Collection ID: {{.ResourceContainers.Collection.ID}}
- Account ID: {{.ResourceContainers.Account.ID}}
- Account Base URL: {{.ResourceContainers.Account.BaseURL}}

This event can be used to:
- Analyze comment content for sentiment, questions, or requests
- Identify the PR context and affected files
- Determine appropriate automated responses
- Track comment activity and engagement
- Route to appropriate team members or automated workflows
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
