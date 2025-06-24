import { IAgentSkill } from '../../../types';

const schema = `openapi: 3.0.3
info:
  title: Atlassian Jira API
  description: Access Jira issues and projects with OAuth authentication
  version: "3"
servers:
  - url: https://api.atlassian.com/ex/jira/{cloudid}/rest/api/3
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
                    description:
                      type: string
  /search:
    get:
      summary: Search for issues using JQL
      operationId: searchIssues
      security:
        - BearerAuth: []
      parameters:
        - name: jql
          in: query
          required: true
          schema:
            type: string
        - name: maxResults
          in: query
          schema:
            type: integer
            default: 50
      responses:
        '200':
          description: Search results
          content:
            application/json:
              schema:
                type: object
                properties:
                  total:
                    type: integer
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
                            status:
                              type: object
                              properties:
                                name:
                                  type: string
                            assignee:
                              type: object
                              properties:
                                displayName:
                                  type: string
components:
  securitySchemes:
    BearerAuth:
      type: http
      scheme: bearer`;

export const atlassianTool: IAgentSkill = {
  name: "Atlassian Jira",
  icon: <img src="data:image/svg+xml;base64,PHN2ZyB3aWR0aD0iMjQiIGhlaWdodD0iMjQiIHZpZXdCb3g9IjAgMCAyNCAyNCIgZmlsbD0ibm9uZSIgeG1sbnM9Imh0dHA6Ly93d3cudzMub3JnLzIwMDAvc3ZnIj4KPHBhdGggZD0iTTEyIDEyQzEyIDEyIDEyIDEyIDEyIDEyQzEyIDEyIDEyIDEyIDEyIDEyWiIgZmlsbD0iIzAwNTJDQyIvPgo8L3N2Zz4=" alt="Atlassian" style={{ width: '24px', height: '24px' }} />,
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
    url: "https://api.atlassian.com/ex/jira/{cloudid}/rest/api/3",
    requiredParameters: [{
      name: 'cloudid',
      description: 'Your Atlassian cloud site ID (replace {cloudid} in URL)',
      type: 'query',
      required: true,
    }],
    oauth_provider: "Atlassian",
    oauth_scopes: ["read:jira-user", "read:jira-work"]
  },
  configurable: false,
} 