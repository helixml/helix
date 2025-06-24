import { IAgentSkill } from '../../../types';
import EmailIcon from '@mui/icons-material/Email';

const schema = `openapi: 3.0.3
info:
  title: Microsoft Outlook API
  description: Access Outlook emails and calendar via Microsoft Graph
  version: "v1.0"
servers:
  - url: https://graph.microsoft.com/v1.0
paths:
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
        - name: $select
          in: query
          schema:
            type: string
            default: "id,subject,bodyPreview,isRead,receivedDateTime,from,toRecipients"
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
                        toRecipients:
                          type: array
                          items:
                            type: object
                            properties:
                              emailAddress:
                                type: object
                                properties:
                                  name:
                                    type: string
                                  address:
                                    type: string
                  '@odata.nextLink':
                    type: string
    post:
      summary: Send Outlook email
      operationId: sendOutlookMessage
      security:
        - BearerAuth: []
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [message]
              properties:
                message:
                  type: object
                  required: [subject, body, toRecipients]
                  properties:
                    subject:
                      type: string
                    body:
                      type: object
                      properties:
                        contentType:
                          type: string
                          enum: [Text, HTML]
                          default: Text
                        content:
                          type: string
                    toRecipients:
                      type: array
                      items:
                        type: object
                        properties:
                          emailAddress:
                            type: object
                            properties:
                              address:
                                type: string
                              name:
                                type: string
                    ccRecipients:
                      type: array
                      items:
                        type: object
                        properties:
                          emailAddress:
                            type: object
                            properties:
                              address:
                                type: string
                              name:
                                type: string
                saveToSentItems:
                  type: boolean
                  default: true
      responses:
        '202':
          description: Message sent successfully
  /me/messages/{id}:
    get:
      summary: Get Outlook message by ID
      operationId: getOutlookMessage
      security:
        - BearerAuth: []
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: string
        - name: $select
          in: query
          schema:
            type: string
            default: "id,subject,body,isRead,receivedDateTime,from,toRecipients,attachments"
      responses:
        '200':
          description: Outlook message details
          content:
            application/json:
              schema:
                type: object
                properties:
                  id:
                    type: string
                  subject:
                    type: string
                  body:
                    type: object
                    properties:
                      contentType:
                        type: string
                      content:
                        type: string
                  isRead:
                    type: boolean
                  receivedDateTime:
                    type: string
                  from:
                    type: object
                  toRecipients:
                    type: array
                  attachments:
                    type: array
                    items:
                      type: object
                      properties:
                        id:
                          type: string
                        name:
                          type: string
                        contentType:
                          type: string
                        size:
                          type: integer
    patch:
      summary: Update Outlook message
      operationId: updateOutlookMessage
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
              properties:
                isRead:
                  type: boolean
                categories:
                  type: array
                  items:
                    type: string
      responses:
        '200':
          description: Message updated successfully
  /me/mailFolders:
    get:
      summary: List Outlook mail folders
      operationId: listOutlookFolders
      security:
        - BearerAuth: []
      responses:
        '200':
          description: List of mail folders
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
                        displayName:
                          type: string
                        totalItemCount:
                          type: integer
                        unreadItemCount:
                          type: integer
components:
  securitySchemes:
    BearerAuth:
      type: http
      scheme: bearer
      bearerFormat: JWT`;

export const outlookTool: IAgentSkill = {
  name: "Outlook",
  icon: <EmailIcon sx={{ fontSize: 24 }} />,
  description: `Access Outlook emails with OAuth authentication.

This skill provides secure access to Microsoft Outlook via Graph API, allowing you to:
• Read, search, and send Outlook emails
• Manage email folders and organization
• Handle email attachments
• Update email properties and categories

Example Queries:
- "Show me my unread emails from today"
- "Search for emails about the project meeting"
- "Send an email to the team about the deadline"
- "List emails in my Important folder"`,
  systemPrompt: `You are an Outlook email assistant. Your expertise is in helping users manage their Outlook inbox, read messages, and handle email communication efficiently.

Key capabilities:
- Email reading and searching with advanced filtering
- Message composition and sending
- Email folder and organization management
- Attachment handling and email categorization

When users ask about emails, inbox management, or email communication:
1. Help them search and filter emails using OData query parameters
2. Assist with reading and summarizing email content
3. Support email composition and sending to recipients
4. Provide inbox organization and folder management
5. Help with email thread management and categorization

Always focus on Outlook email management. If asked about other Microsoft services or non-email topics, politely redirect to Outlook-specific assistance.

Examples of what you can help with:
- "Show me unread emails from my manager"
- "Search for emails containing 'budget approval'"
- "Compose a follow-up email about yesterday's meeting"
- "Mark all emails from John as read"
- "List emails with attachments from this week"`,
  apiSkill: {
    schema: schema,
    url: "https://graph.microsoft.com/v1.0",
    requiredParameters: [],
    oauth_provider: "Microsoft",
    oauth_scopes: [
      "Mail.Read",
      "Mail.Send"
    ],
    headers: {
      "Authorization": "Bearer {token}"
    }
  },
  configurable: false,
}; 