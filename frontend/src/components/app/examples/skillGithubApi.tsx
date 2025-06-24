import { IAgentSkill } from '../../../types';
import GitHubIcon from '@mui/icons-material/GitHub';

const schema = `openapi: 3.0.3
info:
  title: GitHub API
  description: Access GitHub repositories, issues, pull requests, and user information
  version: "2022-11-28"
servers:
  - url: https://api.github.com
paths:
  /user:
    get:
      summary: Get the authenticated user
      operationId: getAuthenticatedUser
      security:
        - BearerAuth: []
      responses:
        '200':
          description: Authenticated user profile
          content:
            application/json:
              schema:
                type: object
                properties:
                  login:
                    type: string
                  name:
                    type: string
                  email:
                    type: string
                  bio:
                    type: string
                  public_repos:
                    type: integer
                  followers:
                    type: integer
                  following:
                    type: integer
  /user/repos:
    get:
      summary: List repositories for the authenticated user
      operationId: listUserRepos
      security:
        - BearerAuth: []
      parameters:
        - name: type
          in: query
          schema:
            type: string
            enum: [all, owner, public, private, member]
            default: owner
        - name: sort
          in: query
          schema:
            type: string
            enum: [created, updated, pushed, full_name]
            default: created
        - name: direction
          in: query
          schema:
            type: string
            enum: [asc, desc]
            default: desc
        - name: per_page
          in: query
          schema:
            type: integer
            default: 30
            maximum: 100
      responses:
        '200':
          description: List of repositories
          content:
            application/json:
              schema:
                type: array
                items:
                  type: object
                  properties:
                    name:
                      type: string
                    full_name:
                      type: string
                    description:
                      type: string
                    private:
                      type: boolean
                    html_url:
                      type: string
                    language:
                      type: string
                    stargazers_count:
                      type: integer
                    forks_count:
                      type: integer
                    created_at:
                      type: string
                    updated_at:
                      type: string
  /repos/{owner}/{repo}/issues:
    get:
      summary: List repository issues
      operationId: listRepoIssues
      security:
        - BearerAuth: []
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
          schema:
            type: string
            enum: [open, closed, all]
            default: open
        - name: labels
          in: query
          schema:
            type: string
          description: A list of comma separated label names
        - name: sort
          in: query
          schema:
            type: string
            enum: [created, updated, comments]
            default: created
        - name: direction
          in: query
          schema:
            type: string
            enum: [asc, desc]
            default: desc
        - name: per_page
          in: query
          schema:
            type: integer
            default: 30
            maximum: 100
      responses:
        '200':
          description: List of issues
          content:
            application/json:
              schema:
                type: array
                items:
                  type: object
                  properties:
                    number:
                      type: integer
                    title:
                      type: string
                    body:
                      type: string
                    state:
                      type: string
                    labels:
                      type: array
                      items:
                        type: object
                        properties:
                          name:
                            type: string
                          color:
                            type: string
                    assignees:
                      type: array
                      items:
                        type: object
                        properties:
                          login:
                            type: string
                    created_at:
                      type: string
                    updated_at:
                      type: string
                    html_url:
                      type: string
    post:
      summary: Create an issue
      operationId: createIssue
      security:
        - BearerAuth: []
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
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required:
                - title
              properties:
                title:
                  type: string
                  description: The title of the issue
                body:
                  type: string
                  description: The contents of the issue
                assignees:
                  type: array
                  items:
                    type: string
                  description: Logins for Users to assign to this issue
                milestone:
                  type: integer
                  description: The number of the milestone to associate this issue with
                labels:
                  type: array
                  items:
                    type: string
                  description: Labels to associate with this issue
      responses:
        '201':
          description: Issue created
          content:
            application/json:
              schema:
                type: object
                properties:
                  number:
                    type: integer
                  title:
                    type: string
                  body:
                    type: string
                  html_url:
                    type: string
  /repos/{owner}/{repo}/pulls:
    get:
      summary: List pull requests
      operationId: listPullRequests
      security:
        - BearerAuth: []
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
          schema:
            type: string
            enum: [open, closed, all]
            default: open
        - name: head
          in: query
          schema:
            type: string
          description: Filter pulls by head user or head organization and branch name
        - name: base
          in: query
          schema:
            type: string
          description: Filter pulls by base branch name
        - name: sort
          in: query
          schema:
            type: string
            enum: [created, updated, popularity, long-running]
            default: created
        - name: direction
          in: query
          schema:
            type: string
            enum: [asc, desc]
            default: desc
        - name: per_page
          in: query
          schema:
            type: integer
            default: 30
            maximum: 100
      responses:
        '200':
          description: List of pull requests
          content:
            application/json:
              schema:
                type: array
                items:
                  type: object
                  properties:
                    number:
                      type: integer
                    title:
                      type: string
                    body:
                      type: string
                    state:
                      type: string
                    head:
                      type: object
                      properties:
                        ref:
                          type: string
                        sha:
                          type: string
                    base:
                      type: object
                      properties:
                        ref:
                          type: string
                        sha:
                          type: string
                    user:
                      type: object
                      properties:
                        login:
                          type: string
                    created_at:
                      type: string
                    updated_at:
                      type: string
                    html_url:
                      type: string
  /search/repositories:
    get:
      summary: Search repositories
      operationId: searchRepositories
      security:
        - BearerAuth: []
      parameters:
        - name: q
          in: query
          required: true
          schema:
            type: string
          description: The search keywords, as well as any qualifiers
        - name: sort
          in: query
          schema:
            type: string
            enum: [stars, forks, help-wanted-issues, updated]
          description: The sort field
        - name: order
          in: query
          schema:
            type: string
            enum: [desc, asc]
            default: desc
        - name: per_page
          in: query
          schema:
            type: integer
            default: 30
            maximum: 100
      responses:
        '200':
          description: Search results
          content:
            application/json:
              schema:
                type: object
                properties:
                  total_count:
                    type: integer
                  incomplete_results:
                    type: boolean
                  items:
                    type: array
                    items:
                      type: object
                      properties:
                        name:
                          type: string
                        full_name:
                          type: string
                        description:
                          type: string
                        html_url:
                          type: string
                        language:
                          type: string
                        stargazers_count:
                          type: integer
                        forks_count:
                          type: integer
components:
  securitySchemes:
    BearerAuth:
      type: http
      scheme: bearer
      bearerFormat: token`;

export const githubTool: IAgentSkill = {
  name: "GitHub",
  icon: <GitHubIcon sx={{ fontSize: 24 }} />,
  description: `Access GitHub repositories, issues, pull requests, and user information with OAuth authentication.

This skill provides secure access to GitHub's REST API, allowing you to:
• View and manage repositories
• Create and track issues  
• Monitor pull requests and code reviews
• Search repositories and code
• Access user profiles and organizations

Example Queries:
- "Show me my latest repositories"
- "List open issues in my main project"
- "Create an issue for the bug I found"
- "Find popular Python repositories"
- "Show me recent pull requests I need to review"`,
  systemPrompt: `You are a GitHub development assistant. Your expertise is in helping users manage their GitHub repositories, issues, pull requests, and development workflows.

Key capabilities:
- Repository management and code exploration
- Issue tracking, creation, and management  
- Pull request workflows and code reviews
- User and organization information
- Repository statistics and insights

When users ask about code repositories, development tasks, or GitHub workflows:
1. Help them find and explore repositories and their contents
2. Assist with issue management - creating, updating, searching issues
3. Support pull request workflows and code collaboration
4. Provide repository insights and statistics
5. Help with user and organization management

Always focus on GitHub-related development tasks. If asked about other platforms or non-GitHub topics, politely redirect to GitHub-specific assistance.

Examples of what you can help with:
- "Show me my recent repositories"
- "Create an issue for the bug we discussed"  
- "List open pull requests for the main project"
- "Find all issues labeled 'bug' in the repository"
- "Get information about repository contributors"`,
  apiSkill: {
    schema: schema,
    url: "https://api.github.com",
    requiredParameters: [],
    oauth_provider: "GitHub",
    oauth_scopes: ["repo", "user:read"],
    headers: {
      "Accept": "application/vnd.github+json",
      "X-GitHub-Api-Version": "2022-11-28"
    }
  },
  configurable: false,
} 