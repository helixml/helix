import { IAgentSkill } from '../../../types';
import { MicrosoftLogo } from '../../icons/ProviderIcons';

const schema = `openapi: 3.0.3
info:
  title: Microsoft Graph API
  description: Access Outlook, OneDrive, and Microsoft Teams with OAuth authentication
  version: "v1.0"
servers:
  - url: https://graph.microsoft.com/v1.0
paths:
  /me:
    get:
      summary: Get user profile
      operationId: getUserProfile
      security:
        - BearerAuth: []
      responses:
        '200':
          description: User profile information
          content:
            application/json:
              schema:
                type: object
                properties:
                  id:
                    type: string
                  displayName:
                    type: string
                  mail:
                    type: string
                  userPrincipalName:
                    type: string
                  jobTitle:
                    type: string
                  department:
                    type: string
                  officeLocation:
                    type: string
  /me/messages:
    get:
      summary: List Outlook emails
      operationId: listOutlookMessages
      security:
        - BearerAuth: []
      parameters:
        - name: $filter
          in: query
          schema:
            type: string
          description: Filter messages (e.g., "isRead eq false")
        - name: $search
          in: query
          schema:
            type: string
          description: Search for messages containing specific text
        - name: $orderby
          in: query
          schema:
            type: string
            default: "receivedDateTime desc"
        - name: $top
          in: query
          schema:
            type: integer
            default: 50
            maximum: 999
      responses:
        '200':
          description: List of Outlook messages
          content:
            application/json:
              schema:
                type: object
                properties:
                  value:
                    type: array
                    items:
                      type: object
                      properties:
                        id:
                          type: string
                        subject:
                          type: string
                        bodyPreview:
                          type: string
                        isRead:
                          type: boolean
                        receivedDateTime:
                          type: string
                        from:
                          type: object
                          properties:
                            emailAddress:
                              type: object
                              properties:
                                name:
                                  type: string
                                address:
                                  type: string
  /me/drive/root/children:
    get:
      summary: List OneDrive files
      operationId: listOneDriveFiles
      security:
        - BearerAuth: []
      parameters:
        - name: $top
          in: query
          schema:
            type: integer
            default: 200
            maximum: 999
      responses:
        '200':
          description: List of OneDrive files
          content:
            application/json:
              schema:
                type: object
                properties:
                  value:
                    type: array
                    items:
                      type: object
                      properties:
                        id:
                          type: string
                        name:
                          type: string
                        size:
                          type: integer
                        lastModifiedDateTime:
                          type: string
                        webUrl:
                          type: string
  /me/calendar/events:
    get:
      summary: List calendar events
      operationId: listCalendarEvents
      security:
        - BearerAuth: []
      parameters:
        - name: $top
          in: query
          schema:
            type: integer
            default: 50
            maximum: 999
      responses:
        '200':
          description: List of calendar events
          content:
            application/json:
              schema:
                type: object
                properties:
                  value:
                    type: array
                    items:
                      type: object
                      properties:
                        id:
                          type: string
                        subject:
                          type: string
                        start:
                          type: object
                          properties:
                            dateTime:
                              type: string
                            timeZone:
                              type: string
                        end:
                          type: object
                          properties:
                            dateTime:
                              type: string
                            timeZone:
                              type: string
                        organizer:
                          type: object
                          properties:
                            emailAddress:
                              type: object
                              properties:
                                name:
                                  type: string
                                address:
                                  type: string
components:
  securitySchemes:
    BearerAuth:
      type: http
      scheme: bearer
      bearerFormat: JWT`;

export const microsoftTool: IAgentSkill = {
  name: "Microsoft 365",
  icon: <MicrosoftLogo sx={{ fontSize: 24 }} />,
  description: `Access Microsoft 365 services including Outlook, OneDrive, and Teams with OAuth authentication.

This skill provides secure access to Microsoft Graph API, allowing you to:
• Read, search, and send Outlook emails
• List, search, and manage OneDrive files
• View, create, and manage calendar events
• Get user profile information

Example Queries:
- "Show me my unread emails from today"
- "List my recent OneDrive documents"
- "Show my calendar events for this week"`,
  systemPrompt: `You are a Microsoft 365 productivity assistant. Your expertise is in helping users manage their Outlook email, OneDrive files, and Calendar events through Microsoft Graph API.

Key capabilities:
- Email management: read, search, and organize Outlook messages
- File management: access and organize OneDrive documents and files
- Calendar management: view, create, and schedule meetings and events
- Microsoft 365 ecosystem integration

When users ask about Microsoft 365 services, email, or productivity:
1. Help them efficiently manage their Outlook inbox and emails
2. Assist with OneDrive file access and organization  
3. Support calendar scheduling and meeting management
4. Provide Microsoft 365 productivity workflows
5. Help integrate across Microsoft ecosystem

Always focus on Microsoft 365 productivity tasks. If asked about other platforms or non-Microsoft services, politely redirect to Microsoft-specific assistance.

Examples of what you can help with:
- "Show me unread emails from my colleagues"
- "Find the Excel file I worked on yesterday in OneDrive"
- "Schedule a meeting with the project team"
- "Search for emails about budget approval"
- "List my calendar events for next week"`,
  apiSkill: {
    schema: schema,
    url: "https://graph.microsoft.com/v1.0",
    requiredParameters: [],
    oauth_provider: "Microsoft",
    oauth_scopes: [
      "Mail.Read",
      "Files.Read", 
      "Calendars.ReadWrite",
      "User.Read"
    ]
  },
  configurable: false,
} 