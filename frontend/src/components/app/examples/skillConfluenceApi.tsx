import { IAgentSkill } from '../../../types';
import atlassianLogo from '../../../../assets/img/atlassian-logo.png';

const schema = `openapi: 3.0.3
info:
  title: Confluence REST API
  description: Access Confluence spaces, pages, comments, and content management
  version: "1.0"
servers:
  - url: https://your-domain.atlassian.net/wiki/rest/api
paths:
  /space:
    get:
      summary: Get all spaces
      operationId: getAllSpaces
      security:
        - BearerAuth: []
      parameters:
        - name: limit
          in: query
          schema:
            type: integer
            default: 25
            maximum: 100
        - name: start
          in: query
          schema:
            type: integer
            default: 0
        - name: type
          in: query
          schema:
            type: string
            enum: [global, personal]
      responses:
        '200':
          description: List of spaces
          content:
            application/json:
              schema:
                type: object
                properties:
                  results:
                    type: array
                    items:
                      type: object
                      properties:
                        id:
                          type: integer
                        key:
                          type: string
                        name:
                          type: string
                        type:
                          type: string
                        description:
                          type: object
                          properties:
                            plain:
                              type: object
                              properties:
                                value:
                                  type: string
                        homepage:
                          type: object
                          properties:
                            id:
                              type: string
                            title:
                              type: string
  /space/{spaceKey}:
    get:
      summary: Get space by key
      operationId: getSpace
      security:
        - BearerAuth: []
      parameters:
        - name: spaceKey
          in: path
          required: true
          schema:
            type: string
      responses:
        '200':
          description: Space details
          content:
            application/json:
              schema:
                type: object
                properties:
                  id:
                    type: integer
                  key:
                    type: string
                  name:
                    type: string
                  type:
                    type: string
                  description:
                    type: object
  /content:
    get:
      summary: Get content (pages, blog posts)
      operationId: getContent
      security:
        - BearerAuth: []
      parameters:
        - name: spaceKey
          in: query
          schema:
            type: string
          description: The space key to filter content
        - name: title
          in: query
          schema:
            type: string
          description: The title of the content to search for
        - name: type
          in: query
          schema:
            type: string
            enum: [page, blogpost, comment, attachment]
            default: page
        - name: status
          in: query
          schema:
            type: string
            enum: [current, trashed, draft]
            default: current
        - name: limit
          in: query
          schema:
            type: integer
            default: 25
            maximum: 100
        - name: expand
          in: query
          schema:
            type: string
            default: "space,history,body.storage,version"
      responses:
        '200':
          description: List of content
          content:
            application/json:
              schema:
                type: object
                properties:
                  results:
                    type: array
                    items:
                      type: object
                      properties:
                        id:
                          type: string
                        type:
                          type: string
                        title:
                          type: string
                        space:
                          type: object
                          properties:
                            key:
                              type: string
                            name:
                              type: string
                        body:
                          type: object
                          properties:
                            storage:
                              type: object
                              properties:
                                value:
                                  type: string
                        version:
                          type: object
                          properties:
                            number:
                              type: integer
                        history:
                          type: object
                          properties:
                            createdDate:
                              type: string
                            createdBy:
                              type: object
                              properties:
                                displayName:
                                  type: string
    post:
      summary: Create new content
      operationId: createContent
      security:
        - BearerAuth: []
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [type, title, space, body]
              properties:
                type:
                  type: string
                  enum: [page, blogpost]
                title:
                  type: string
                space:
                  type: object
                  properties:
                    key:
                      type: string
                body:
                  type: object
                  properties:
                    storage:
                      type: object
                      properties:
                        value:
                          type: string
                        representation:
                          type: string
                          default: storage
                ancestors:
                  type: array
                  items:
                    type: object
                    properties:
                      id:
                        type: string
      responses:
        '200':
          description: Created content
  /content/{id}:
    get:
      summary: Get content by ID
      operationId: getContentById
      security:
        - BearerAuth: []
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: string
        - name: expand
          in: query
          schema:
            type: string
            default: "space,history,body.storage,version,ancestors"
      responses:
        '200':
          description: Content details
          content:
            application/json:
              schema:
                type: object
                properties:
                  id:
                    type: string
                  type:
                    type: string
                  title:
                    type: string
                  body:
                    type: object
                    properties:
                      storage:
                        type: object
                        properties:
                          value:
                            type: string
    put:
      summary: Update content
      operationId: updateContent
      security:
        - BearerAuth: []
      parameters:
        - name: id
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
              required: [version, title, type, body]
              properties:
                version:
                  type: object
                  properties:
                    number:
                      type: integer
                title:
                  type: string
                type:
                  type: string
                body:
                  type: object
                  properties:
                    storage:
                      type: object
                      properties:
                        value:
                          type: string
                        representation:
                          type: string
                          default: storage
      responses:
        '200':
          description: Updated content
    delete:
      summary: Delete content
      operationId: deleteContent
      security:
        - BearerAuth: []
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: string
      responses:
        '204':
          description: Content deleted successfully
  /content/{id}/child:
    get:
      summary: Get child pages
      operationId: getChildPages
      security:
        - BearerAuth: []
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: string
        - name: type
          in: query
          schema:
            type: string
            enum: [page, comment, attachment]
            default: page
        - name: expand
          in: query
          schema:
            type: string
            default: "space,history,version"
      responses:
        '200':
          description: List of child content
  /search:
    get:
      summary: Search content
      operationId: searchContent
      security:
        - BearerAuth: []
      parameters:
        - name: cql
          in: query
          required: true
          schema:
            type: string
          description: "Confluence Query Language search. Example: 'space = DEV and type = page'"
        - name: limit
          in: query
          schema:
            type: integer
            default: 20
            maximum: 100
      responses:
        '200':
          description: Search results
          content:
            application/json:
              schema:
                type: object
                properties:
                  results:
                    type: array
                    items:
                      type: object
                      properties:
                        content:
                          type: object
                          properties:
                            id:
                              type: string
                            title:
                              type: string
                            type:
                              type: string
                            space:
                              type: object
                              properties:
                                key:
                                  type: string
                                name:
                                  type: string
components:
  securitySchemes:
    BearerAuth:
      type: http
      scheme: bearer
      bearerFormat: JWT`;

export const confluenceTool: IAgentSkill = {
  name: 'Confluence',
  icon: <img src={atlassianLogo} alt="Confluence" style={{ width: '24px', height: '24px' }} />,
  description: 'Access Confluence spaces, pages, and content management. Search, create, update, and organize documentation.',
  systemPrompt: `You are a Confluence documentation specialist. Your role is to help users interact with their Confluence wiki spaces, pages, and content.

Key capabilities:
- Search and retrieve pages across spaces using CQL (Confluence Query Language)
- Create, update, and organize documentation pages
- Manage space hierarchies and page relationships
- Access page content, comments, and attachments
- Navigate space structures and find specific documentation

When users ask about documentation, wiki content, or knowledge management:
1. Use search to find relevant pages or spaces
2. For content creation, gather requirements for title, space, and content structure
3. When updating pages, always get the current version number first
4. Use CQL for complex searches (e.g., 'space = PROJ and type = page and title ~ "API"')
5. Help organize content by understanding page hierarchies and relationships

Always provide actionable responses focused on Confluence documentation tasks. If asked about non-Confluence topics, politely redirect to Confluence-related assistance.

Examples of what you can help with:
- "Find all pages about API documentation in the DEV space"
- "Create a new page about deployment process in the Operations space"
- "Search for pages mentioning 'database migration'"
- "Update the getting started guide with new information"
- "List all child pages under the Architecture page"`,
  apiSkill: {
    schema,
    url: 'https://your-domain.atlassian.net/wiki/rest/api',
    requiredParameters: [
      {
        name: 'domain',
        description: 'Your Atlassian domain (e.g., "company" for company.atlassian.net)',
        type: 'url_replace' as any,
        required: true,
      },
    ],
    query: {},
    headers: {},
    oauth_provider: 'Atlassian',
    oauth_scopes: ['read:confluence-content.all', 'write:confluence-content', 'read:confluence-space.summary', 'read:confluence-user'],
  },
  configurable: true,
}; 