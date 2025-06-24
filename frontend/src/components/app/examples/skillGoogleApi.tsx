import { IAgentSkill } from '../../../types';
import { Google as GoogleIcon } from '@mui/icons-material';

const schema = `openapi: 3.0.3
info:
  title: Google APIs
  description: Access Gmail, Google Drive, and Google Calendar with OAuth authentication
  version: "v1"
servers:
  - url: https://www.googleapis.com
paths:
  /gmail/v1/users/me/profile:
    get:
      summary: Get Gmail profile
      operationId: getGmailProfile
      security:
        - BearerAuth: []
      responses:
        '200':
          description: Gmail profile information
          content:
            application/json:
              schema:
                type: object
                properties:
                  emailAddress:
                    type: string
                  messagesTotal:
                    type: integer
                  threadsTotal:
                    type: integer
                  historyId:
                    type: string
  /gmail/v1/users/me/messages:
    get:
      summary: List Gmail messages
      operationId: listGmailMessages
      security:
        - BearerAuth: []
      parameters:
        - name: q
          in: query
          schema:
            type: string
          description: Query string for searching messages (e.g., "is:unread", "from:user@example.com")
        - name: labelIds
          in: query
          schema:
            type: array
            items:
              type: string
          description: Only return messages with labels that match all of the specified label IDs
        - name: maxResults
          in: query
          schema:
            type: integer
            default: 100
            maximum: 500
          description: Maximum number of messages to return
        - name: pageToken
          in: query
          schema:
            type: string
          description: Page token to retrieve a specific page of results
      responses:
        '200':
          description: List of Gmail messages
          content:
            application/json:
              schema:
                type: object
                properties:
                  messages:
                    type: array
                    items:
                      type: object
                      properties:
                        id:
                          type: string
                        threadId:
                          type: string
                  nextPageToken:
                    type: string
                  resultSizeEstimate:
                    type: integer
  /gmail/v1/users/me/messages/{id}:
    get:
      summary: Get a specific Gmail message
      operationId: getGmailMessage
      security:
        - BearerAuth: []
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: string
        - name: format
          in: query
          schema:
            type: string
            enum: [minimal, full, raw, metadata]
            default: full
      responses:
        '200':
          description: Gmail message details
          content:
            application/json:
              schema:
                type: object
                properties:
                  id:
                    type: string
                  threadId:
                    type: string
                  labelIds:
                    type: array
                    items:
                      type: string
                  snippet:
                    type: string
                  payload:
                    type: object
                    properties:
                      headers:
                        type: array
                        items:
                          type: object
                          properties:
                            name:
                              type: string
                            value:
                              type: string
                      body:
                        type: object
                        properties:
                          data:
                            type: string
                          size:
                            type: integer
  /drive/v3/files:
    get:
      summary: List Google Drive files
      operationId: listDriveFiles
      security:
        - BearerAuth: []
      parameters:
        - name: q
          in: query
          schema:
            type: string
          description: Query string for searching files (e.g., "name contains 'test'", "mimeType='application/pdf'")
        - name: orderBy
          in: query
          schema:
            type: string
          description: Sort order (e.g., "name", "modifiedTime desc", "createdTime")
        - name: pageSize
          in: query
          schema:
            type: integer
            default: 100
            maximum: 1000
        - name: fields
          in: query
          schema:
            type: string
            default: "files(id,name,mimeType,size,createdTime,modifiedTime,webViewLink)"
      responses:
        '200':
          description: List of Drive files
          content:
            application/json:
              schema:
                type: object
                properties:
                  files:
                    type: array
                    items:
                      type: object
                      properties:
                        id:
                          type: string
                        name:
                          type: string
                        mimeType:
                          type: string
                        size:
                          type: string
                        createdTime:
                          type: string
                        modifiedTime:
                          type: string
                        webViewLink:
                          type: string
                  nextPageToken:
                    type: string
  /calendar/v3/calendars/primary/events:
    get:
      summary: List calendar events
      operationId: listCalendarEvents
      security:
        - BearerAuth: []
      parameters:
        - name: timeMin
          in: query
          schema:
            type: string
            format: date-time
          description: Lower bound (exclusive) for an event's end time to filter by (RFC3339 timestamp)
        - name: timeMax
          in: query
          schema:
            type: string
            format: date-time
          description: Upper bound (exclusive) for an event's start time to filter by (RFC3339 timestamp)
        - name: maxResults
          in: query
          schema:
            type: integer
            default: 250
            maximum: 2500
        - name: orderBy
          in: query
          schema:
            type: string
            enum: [startTime, updated]
        - name: q
          in: query
          schema:
            type: string
          description: Free text search terms to find events
        - name: singleEvents
          in: query
          schema:
            type: boolean
            default: false
          description: Whether to expand recurring events into instances
      responses:
        '200':
          description: List of calendar events
          content:
            application/json:
              schema:
                type: object
                properties:
                  items:
                    type: array
                    items:
                      type: object
                      properties:
                        id:
                          type: string
                        summary:
                          type: string
                        description:
                          type: string
                        start:
                          type: object
                          properties:
                            dateTime:
                              type: string
                            date:
                              type: string
                        end:
                          type: object
                          properties:
                            dateTime:
                              type: string
                            date:
                              type: string
                        attendees:
                          type: array
                          items:
                            type: object
                            properties:
                              email:
                                type: string
                              displayName:
                                type: string
                              responseStatus:
                                type: string
                        htmlLink:
                          type: string
                  nextPageToken:
                    type: string
    post:
      summary: Create a calendar event
      operationId: createCalendarEvent
      security:
        - BearerAuth: []
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required:
                - summary
                - start
                - end
              properties:
                summary:
                  type: string
                  description: Title of the event
                description:
                  type: string
                  description: Description of the event
                start:
                  type: object
                  required:
                    - dateTime
                  properties:
                    dateTime:
                      type: string
                      format: date-time
                    timeZone:
                      type: string
                end:
                  type: object
                  required:
                    - dateTime
                  properties:
                    dateTime:
                      type: string
                      format: date-time
                    timeZone:
                      type: string
                attendees:
                  type: array
                  items:
                    type: object
                    properties:
                      email:
                        type: string
                location:
                  type: string
                reminders:
                  type: object
                  properties:
                    useDefault:
                      type: boolean
                    overrides:
                      type: array
                      items:
                        type: object
                        properties:
                          method:
                            type: string
                            enum: [email, popup]
                          minutes:
                            type: integer
      responses:
        '200':
          description: Event created successfully
          content:
            application/json:
              schema:
                type: object
                properties:
                  id:
                    type: string
                  summary:
                    type: string
                  htmlLink:
                    type: string
                  start:
                    type: object
                    properties:
                      dateTime:
                        type: string
                  end:
                    type: object
                    properties:
                      dateTime:
                        type: string
components:
  securitySchemes:
    BearerAuth:
      type: http
      scheme: bearer
      bearerFormat: JWT`;

export const googleTool: IAgentSkill = {
  name: "Google Services",
  icon: <GoogleIcon sx={{ fontSize: 24 }} />,
  description: `Access Gmail, Google Drive, and Google Calendar with OAuth authentication.

This skill provides secure access to popular Google services, allowing you to:
• Read, search, and send Gmail messages
• List, upload, and manage Google Drive files  
• View, create, and manage Google Calendar events
• Access user profile information across services

Example Queries:
- "Show me my unread emails from today"
- "Search for emails from john@company.com"
- "List my recent Google Drive documents"
- "Show my calendar events for tomorrow"
- "Create a meeting for next week"`,
  systemPrompt: `You are a Google Workspace productivity assistant. Your expertise is in helping users manage their Gmail, Google Drive files, and Google Calendar events efficiently.

Key capabilities:
- Email management: read, search, and organize Gmail messages
- File management: access, organize, and share Google Drive documents  
- Calendar management: view, create, and schedule events and meetings
- Cross-service productivity workflows

When users ask about email, documents, or scheduling:
1. Help them search and organize their Gmail efficiently
2. Assist with Google Drive file management and sharing
3. Support calendar scheduling and event management
4. Provide productivity insights across Google services
5. Help automate common workspace tasks

Always focus on Google Workspace productivity. If asked about other platforms or non-Google services, politely redirect to Google-specific assistance.

Examples of what you can help with:
- "Show me unread emails from my manager"
- "Find the presentation file I worked on yesterday"
- "Schedule a team meeting for next Tuesday"
- "Search for emails about the project deadline"
- "List my calendar events for this week"`,
  apiSkill: {
    schema: schema,
    url: "https://www.googleapis.com",
    requiredParameters: [],
    oauth_provider: "Google",
    oauth_scopes: [
      "https://www.googleapis.com/auth/gmail.readonly",
      "https://www.googleapis.com/auth/drive.file",
      "https://www.googleapis.com/auth/calendar"
    ]
  },
  configurable: false,
} 