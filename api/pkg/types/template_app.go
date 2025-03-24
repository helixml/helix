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
						OAuthProvider: OAuthProviderTypeGitHub,
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

// GetAppTemplates returns all available app templates
func GetAppTemplates() []*TemplateAppConfig {
	return []*TemplateAppConfig{
		GetGitHubIssuesTemplate(),
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
