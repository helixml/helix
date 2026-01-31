import { IAgentSkill } from '../../../types';
import atlassianLogo from '../../../../assets/img/atlassian-logo.png';

const schema = `openapi: 3.0.3
info:
  title: Atlassian Jira API
  description: Access Jira issues and projects with OAuth authentication
  version: "3"
servers:
  - url: https://your-domain.atlassian.net/rest/api/3
paths:
  /myself:
    get:
      summary: Get current user
      operationId: getCurrentUser
      security:
        - BearerAuth: []
      responses:
        '200':
          description: Current user information
          content:
            application/json:
              schema:
                type: object
                properties:
                  accountId:
                    type: string
                  displayName:
                    type: string
                  emailAddress:
                    type: string
  /project:
    get:
      summary: Get all projects
      operationId: getAllProjects
      security:
        - BearerAuth: []
      responses:
        '200':
          description: List of projects
          content:
            application/json:
              schema:
                type: array
                items:
                  type: object
                  properties:
                    id:
                      type: string
                    key:
                      type: string
                    name:
                      type: string
                    projectTypeKey:
                      type: string
  /search:
    get:
      summary: Search for issues using JQL
      operationId: searchForIssuesUsingJql
      security:
        - BearerAuth: []
      parameters:
        - name: jql
          in: query
          schema:
            type: string
          description: JQL query string
        - name: startAt
          in: query
          schema:
            type: integer
            default: 0
        - name: maxResults
          in: query
          schema:
            type: integer
            default: 50
        - name: fields
          in: query
          schema:
            type: string
          description: Comma-separated list of fields to return
      responses:
        '200':
          description: Search results
          content:
            application/json:
              schema:
                type: object
                properties:
                  issues:
                    type: array
                    items:
                      type: object
                      properties:
                        id:
                          type: string
                        key:
                          type: string
                        fields:
                          type: object
                          properties:
                            summary:
                              type: string
                            description:
                              type: string
                            status:
                              type: object
                              properties:
                                name:
                                  type: string
                            priority:
                              type: object
                              properties:
                                name:
                                  type: string
                            assignee:
                              type: object
                              properties:
                                displayName:
                                  type: string
                            reporter:
                              type: object
                              properties:
                                displayName:
                                  type: string
                            created:
                              type: string
                            updated:
                              type: string
                  total:
                    type: integer
  /issue:
    post:
      summary: Create issue
      operationId: createIssue
      security:
        - BearerAuth: []
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required:
                - fields
              properties:
                fields:
                  type: object
                  required:
                    - project
                    - summary
                    - issuetype
                  properties:
                    project:
                      type: object
                      properties:
                        key:
                          type: string
                    summary:
                      type: string
                    description:
                      type: string
                    issuetype:
                      type: object
                      properties:
                        name:
                          type: string
                    priority:
                      type: object
                      properties:
                        name:
                          type: string
                    labels:
                      type: array
                      items:
                        type: string
      responses:
        '201':
          description: Issue created successfully
          content:
            application/json:
              schema:
                type: object
                properties:
                  id:
                    type: string
                  key:
                    type: string
  /issue/{issueIdOrKey}:
    get:
      summary: Get issue
      operationId: getIssue
      security:
        - BearerAuth: []
      parameters:
        - name: issueIdOrKey
          in: path
          required: true
          schema:
            type: string
      responses:
        '200':
          description: Issue details
          content:
            application/json:
              schema:
                type: object
                properties:
                  id:
                    type: string
                  key:
                    type: string
                  fields:
                    type: object
    put:
      summary: Update issue
      operationId: editIssue
      security:
        - BearerAuth: []
      parameters:
        - name: issueIdOrKey
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
              properties:
                fields:
                  type: object
      responses:
        '204':
          description: Issue updated successfully
components:
  securitySchemes:
    BearerAuth:
      type: http
      scheme: bearer
      bearerFormat: JWT`;

export const atlassianTool: IAgentSkill = {
  name: "Atlassian Jira",
  icon: <img src={atlassianLogo} alt="Atlassian Jira" style={{ width: '24px', height: '24px' }} />,
  description: `Access Atlassian Jira for project management and issue tracking with OAuth authentication.

This skill provides access to Jira's REST API, allowing you to:
• Search and view issues using JQL (Jira Query Language)
• Access project information
• Get user information
• Track issues and project progress

Example Queries:
- "Show me all open issues in project ABC"
- "List all issues assigned to me"
- "Get all projects I have access to"`,
  systemPrompt: `You are a Jira project management assistant. Your expertise is in helping users track issues, manage projects, and navigate their Jira workspace efficiently.

Key capabilities:
- Issue tracking and management using JQL queries
- Project overview and status monitoring  
- Team workload and assignment analysis
- User and account information access

When users ask about project management, issue tracking, or Jira workflows:
1. Help them search and filter issues using appropriate JQL queries
2. Provide project insights and status summaries
3. Assist with issue assignment and workload analysis
4. Support project planning and tracking activities
5. Help streamline Jira workflows and processes

Always focus on project management and issue tracking tasks. If asked about other tools or non-Jira topics, politely redirect to Jira-specific assistance.

Examples of what you can help with:
- "Show me all open bugs in the current sprint"
- "List issues assigned to me that are overdue"
- "Get a summary of project ABC's progress"
- "Find all high-priority issues in the backlog"
- "Show me what projects I have access to"`,
  apiSkill: {
    schema: schema,
    url: "https://your-domain.atlassian.net/rest/api/3",
    requiredParameters: [],
    oauth_provider: "Atlassian",
    oauth_scopes: ["read:jira-user", "read:jira-work"],
    headers: {
      "Accept": "application/json",
      "Content-Type": "application/json"
    }
  },
  configurable: false,
} 