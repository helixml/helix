package types

// TemplateAppType represents the type of template app
type TemplateAppType string

const (
	// TemplateAppTypeGitHub represents a GitHub template app
	TemplateAppTypeGitHub TemplateAppType = "github"
	// TemplateAppTypeJira represents a Jira template app
	TemplateAppTypeJira TemplateAppType = "jira"
	// TemplateAppTypeSlack represents a Slack template app
	TemplateAppTypeSlack TemplateAppType = "slack"
	// TemplateAppTypeGoogle represents a Google template app
	TemplateAppTypeGoogle TemplateAppType = "google"
)

// TemplateAppConfig represents a template app configuration
type TemplateAppConfig struct {
	Type        TemplateAppType        `json:"type"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Assistants  []AssistantConfig      `json:"assistants"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	APIURL      string                 `json:"api_url,omitempty"` // Base API URL for the provider
}

// TemplateKnowledge represents template knowledge configuration
type TemplateKnowledge struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// GetGitHubIssuesTemplate returns a template app configuration for GitHub issues
func GetGitHubIssuesTemplate() *TemplateAppConfig {
	return &TemplateAppConfig{
		Type:        TemplateAppTypeGitHub,
		Name:        "GitHub Repository Analyzer",
		Description: "Analyze GitHub repositories, issues, and PRs",
		APIURL:      "https://api.github.com",
		Assistants: []AssistantConfig{
			{
				Name:        "GitHub Assistant",
				Description: "AI assistant that helps manage GitHub repositories and issues",
				SystemPrompt: `You are a GitHub expert assistant.
You can access GitHub repositories, issues, and pull requests using the GitHub API.
Help the user analyze code, issues, and pull requests in their repositories.
When asked about GitHub repositories, use the API to fetch actual data.`,
				APIs: []AssistantAPI{
					{
						Name:          "GitHub API",
						Description:   "GitHub API for accessing repositories, issues, and pull requests",
						URL:           "https://api.github.com",
						OAuthProvider: string(OAuthProviderTypeGitHub),
						OAuthScopes:   []string{"repo", "read:user"},
						Schema: `openapi: 3.0.0
info:
  title: GitHub API
  description: Access GitHub repositories, issues, and pull requests
  version: "1.0"
paths:
  /user:
    get:
      summary: Get authenticated user
      description: Get the currently authenticated user
      operationId: getAuthenticatedUser
      responses:
        '200':
          description: Successful operation
  /user/repos:
    get:
      summary: List repositories for the authenticated user
      description: Lists repositories that the authenticated user has explicit permission to access
      operationId: listUserRepos
      parameters:
        - name: sort
          in: query
          description: The property to sort the results by
          schema:
            type: string
            enum: [created, updated, pushed, full_name]
            default: full_name
        - name: direction
          in: query
          description: The direction to sort the results by
          schema:
            type: string
            enum: [asc, desc]
            default: asc
      responses:
        '200':
          description: Successful operation
  /repos/{owner}/{repo}:
    get:
      summary: Get a repository
      description: Get information about a specific repository
      operationId: getRepository
      parameters:
        - name: owner
          in: path
          required: true
          schema:
            type: string
        - name: repo
          in: path
          required: true
          schema:
            type: string
      responses:
        '200':
          description: Successful operation
  /repos/{owner}/{repo}/issues:
    get:
      summary: List repository issues
      description: List issues in a repository
      operationId: listRepositoryIssues
      parameters:
        - name: owner
          in: path
          required: true
          schema:
            type: string
        - name: repo
          in: path
          required: true
          schema:
            type: string
        - name: state
          in: query
          description: Filter issues by state
          schema:
            type: string
            enum: [open, closed, all]
            default: open
        - name: sort
          in: query
          description: What to sort results by
          schema:
            type: string
            enum: [created, updated, comments]
            default: created
        - name: per_page
          in: query
          description: Results per page
          schema:
            type: integer
            default: 30
      responses:
        '200':
          description: Successful operation
        '404':
          description: Repository not found
  /issues:
    get:
      summary: List issues assigned to the authenticated user
      description: List all issues assigned to the authenticated user across all repositories
      operationId: listUserIssues
      parameters:
        - name: filter
          in: query
          description: Filter issues by state
          schema:
            type: string
            enum: [assigned, created, mentioned, subscribed, all]
            default: assigned
        - name: state
          in: query
          description: Filter issues by state
          schema:
            type: string
            enum: [open, closed, all]
            default: open
        - name: sort
          in: query
          description: What to sort results by
          schema:
            type: string
            enum: [created, updated, comments]
            default: created
      responses:
        '200':
          description: Successful operation
  /repos/{owner}/{repo}/issues/{issue_number}:
    get:
      summary: Get an issue
      description: Get information about a specific issue
      operationId: getIssue
      parameters:
        - name: owner
          in: path
          required: true
          schema:
            type: string
        - name: repo
          in: path
          required: true
          schema:
            type: string
        - name: issue_number
          in: path
          required: true
          schema:
            type: integer
      responses:
        '200':
          description: Successful operation`,
					},
				},
			},
		},
	}
}

// GetJiraProjectManagerTemplate returns a template app configuration for Jira project management
func GetJiraProjectManagerTemplate() *TemplateAppConfig {
	return &TemplateAppConfig{
		Type:        TemplateAppTypeJira,
		Name:        "Jira Project Manager",
		Description: "Manage and analyze Jira projects and issues",
		APIURL:      "https://api.atlassian.com",
		Assistants: []AssistantConfig{
			{
				Name:        "Jira Assistant",
				Description: "AI assistant that helps manage Jira projects and issues",
				SystemPrompt: `You are a Jira expert assistant.
You can access Jira projects, issues, and boards using the Jira API.
Help the user manage their projects, track issues, and analyze project data.
When asked about Jira projects, use the API to fetch actual data.`,
				APIs: []AssistantAPI{
					{
						Name:          "Jira API",
						Description:   "Jira API for managing projects and issues",
						URL:           "https://api.atlassian.com",
						OAuthProvider: "jira",
						OAuthScopes:   []string{"read:jira-work", "read:jira-user", "write:jira-work"},
						Schema: `openapi: 3.0.0
info:
  title: Jira API
  description: Access Jira projects and issues
  version: "1.0"
paths:
  /ex/jira/{cloudId}/rest/api/3/project:
    get:
      summary: Get all projects
      description: Returns all projects visible to the user
      operationId: getAllProjects
      parameters:
        - name: cloudId
          in: path
          required: true
          schema:
            type: string
      responses:
        '200':
          description: Successful operation
  /ex/jira/{cloudId}/rest/api/3/issue/{issueIdOrKey}:
    get:
      summary: Get issue
      description: Returns details about an issue
      operationId: getIssue
      parameters:
        - name: cloudId
          in: path
          required: true
          schema:
            type: string
        - name: issueIdOrKey
          in: path
          required: true
          schema:
            type: string
      responses:
        '200':
          description: Successful operation`,
					},
				},
			},
		},
	}
}

// GetSlackChannelAssistantTemplate returns a template app configuration for Slack channel assistance
func GetSlackChannelAssistantTemplate() *TemplateAppConfig {
	return &TemplateAppConfig{
		Type:        TemplateAppTypeSlack,
		Name:        "Slack Channel Assistant",
		Description: "Answer questions and perform tasks in Slack channels",
		APIURL:      "https://slack.com/api",
		Assistants: []AssistantConfig{
			{
				Name:        "Slack Assistant",
				Description: "AI assistant that helps answer questions in Slack channels",
				SystemPrompt: `You are a Slack assistant.
You can access Slack channels, messages, and users using the Slack API.
Help the user get information from their Slack workspace and interact with channels.
When asked about Slack data, use the API to fetch actual information.`,
				APIs: []AssistantAPI{
					{
						Name:          "Slack API",
						Description:   "Slack API for accessing channels and messages",
						URL:           "https://slack.com/api",
						OAuthProvider: "slack",
						OAuthScopes:   []string{"channels:read", "chat:write", "users:read"},
						Schema: `openapi: 3.0.0
info:
  title: Slack API
  description: Access Slack channels and messages
  version: "1.0"
paths:
  /conversations.list:
    get:
      summary: List conversations
      description: Lists all channels in a Slack team
      operationId: listConversations
      responses:
        '200':
          description: Successful operation
  /chat.postMessage:
    post:
      summary: Post a message
      description: Sends a message to a channel
      operationId: postMessage
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              properties:
                channel:
                  type: string
                text:
                  type: string
      responses:
        '200':
          description: Successful operation`,
					},
				},
			},
		},
	}
}

// GetGoogleDriveNavigatorTemplate returns a template app configuration for Google Drive
func GetGoogleDriveNavigatorTemplate() *TemplateAppConfig {
	return &TemplateAppConfig{
		Type:        TemplateAppTypeGoogle,
		Name:        "Google Drive Navigator",
		Description: "Search and summarize documents in Google Drive",
		APIURL:      "https://www.googleapis.com",
		Assistants: []AssistantConfig{
			{
				Name:        "Google Drive Assistant",
				Description: "AI assistant that helps search and manage Google Drive documents",
				SystemPrompt: `You are a Google Drive expert assistant.
You can access Google Drive files and folders using the Google Drive API.
Help the user find, summarize, and manage their documents.
When asked about Google Drive files, use the API to fetch actual data.`,
				APIs: []AssistantAPI{
					{
						Name:          "Google Drive API",
						Description:   "Google Drive API for accessing files and folders",
						URL:           "https://www.googleapis.com",
						OAuthProvider: "google",
						OAuthScopes:   []string{"https://www.googleapis.com/auth/drive.readonly"},
						Schema: `openapi: 3.0.0
info:
  title: Google Drive API
  description: Access Google Drive files and folders
  version: "1.0"
paths:
  /drive/v3/files:
    get:
      summary: List files
      description: Lists or searches files in Google Drive
      operationId: listFiles
      parameters:
        - name: q
          in: query
          description: Search query
          schema:
            type: string
        - name: pageSize
          in: query
          description: Maximum number of files to return
          schema:
            type: integer
            default: 10
      responses:
        '200':
          description: Successful operation
  /drive/v3/files/{fileId}:
    get:
      summary: Get a file
      description: Gets a file by ID
      operationId: getFile
      parameters:
        - name: fileId
          in: path
          required: true
          schema:
            type: string
      responses:
        '200':
          description: Successful operation`,
					},
				},
			},
		},
	}
}

// GetAppTemplates returns all available app templates
func GetAppTemplates() []*TemplateAppConfig {
	return []*TemplateAppConfig{
		GetGitHubIssuesTemplate(),
		GetJiraProjectManagerTemplate(),
		GetSlackChannelAssistantTemplate(),
		GetGoogleDriveNavigatorTemplate(),
		// Add more templates as needed
	}
}

// GetTemplateByType returns a template by its type
func GetTemplateByType(templateType TemplateAppType) *TemplateAppConfig {
	for _, template := range GetAppTemplates() {
		if template.Type == templateType {
			return template
		}
	}
	return nil
}

// GetDefaultAPIURLForProvider returns the default API URL for a given provider type
func GetDefaultAPIURLForProvider(providerType string) string {
	switch providerType {
	case "github":
		return "https://api.github.com"
	case "slack":
		return "https://slack.com/api"
	case "google":
		return "https://www.googleapis.com"
	case "jira", "atlassian":
		return "https://api.atlassian.com"
	case "microsoft":
		return "https://graph.microsoft.com/v1.0"
	default:
		return ""
	}
}

// CreateAppConfigFromTemplate creates an AppConfig from a template
func CreateAppConfigFromTemplate(template *TemplateAppConfig) *AppConfig {
	config := &AppConfig{
		Helix: AppHelixConfig{
			Name:        template.Name,
			Description: template.Description,
			Assistants:  make([]AssistantConfig, len(template.Assistants)),
		},
		Secrets: make(map[string]string),
	}

	// Copy assistants and convert APIs to Tools
	for i, assistant := range template.Assistants {
		config.Helix.Assistants[i] = AssistantConfig{
			Name:         assistant.Name,
			Description:  assistant.Description,
			SystemPrompt: assistant.SystemPrompt,
			Provider:     assistant.Provider,
			Model:        assistant.Model,
			Tools:        make([]*Tool, 0, len(assistant.APIs)),
			Knowledge:    assistant.Knowledge,
		}

		// Convert APIs to Tools
		for _, api := range assistant.APIs {
			tool := &Tool{
				Name:        api.Name,
				Description: api.Description,
				ToolType:    ToolTypeAPI,
				Config: ToolConfig{
					API: &ToolAPIConfig{
						URL:           api.URL,
						Schema:        api.Schema,
						OAuthProvider: api.OAuthProvider,
						OAuthScopes:   api.OAuthScopes,
						Headers:       api.Headers,
						Query:         api.Query,
					},
				},
			}
			config.Helix.Assistants[i].Tools = append(config.Helix.Assistants[i].Tools, tool)
		}
	}

	return config
}
