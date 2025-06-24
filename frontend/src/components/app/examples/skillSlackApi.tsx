import { IAgentSkill } from '../../../types';
import { SlackLogo } from '../../icons/ProviderIcons';

const schema = `openapi: 3.0.3
info:
  title: Slack Web API
  description: Access Slack channels, messages, and user information
  version: "1.0"
servers:
  - url: https://slack.com/api
paths:
  /conversations.list:
    get:
      summary: List conversations (channels)
      operationId: listConversations
      security:
        - BearerAuth: []
      parameters:
        - name: types
          in: query
          schema:
            type: string
            default: "public_channel,private_channel"
        - name: limit
          in: query
          schema:
            type: integer
            default: 100
      responses:
        '200':
          description: List of conversations
          content:
            application/json:
              schema:
                type: object
                properties:
                  ok:
                    type: boolean
                  channels:
                    type: array
                    items:
                      type: object
                      properties:
                        id:
                          type: string
                        name:
                          type: string
                        is_channel:
                          type: boolean
                        is_private:
                          type: boolean
                        is_member:
                          type: boolean
                        topic:
                          type: object
                          properties:
                            value:
                              type: string
  /conversations.history:
    get:
      summary: Get conversation history
      operationId: getConversationHistory
      security:
        - BearerAuth: []
      parameters:
        - name: channel
          in: query
          required: true
          schema:
            type: string
        - name: limit
          in: query
          schema:
            type: integer
            default: 100
      responses:
        '200':
          description: Conversation history
          content:
            application/json:
              schema:
                type: object
                properties:
                  ok:
                    type: boolean
                  messages:
                    type: array
                    items:
                      type: object
                      properties:
                        type:
                          type: string
                        user:
                          type: string
                        text:
                          type: string
                        ts:
                          type: string
  /chat.postMessage:
    post:
      summary: Send a message
      operationId: postMessage
      security:
        - BearerAuth: []
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required:
                - channel
                - text
              properties:
                channel:
                  type: string
                text:
                  type: string
      responses:
        '200':
          description: Message posted
          content:
            application/json:
              schema:
                type: object
                properties:
                  ok:
                    type: boolean
                  ts:
                    type: string
components:
  securitySchemes:
    BearerAuth:
      type: http
      scheme: bearer`;

export const slackTool: IAgentSkill = {
  name: "Slack",
  icon: <SlackLogo sx={{ fontSize: 24 }} />,
  description: `Access Slack workspace channels and messages with OAuth authentication.

This skill provides access to Slack's Web API, allowing you to:
• List and browse workspace channels
• Read message history from channels  
• Send messages to channels
• Get user information

Example Queries:
- "List all channels in the workspace"
- "Show recent messages from the #general channel"
- "Send a message to the team"`,
  systemPrompt: `You are a Slack workspace communication assistant. Your expertise is in helping users navigate channels, read messages, and facilitate team communication within their Slack workspace.

Key capabilities:
- Channel exploration and discovery
- Message history retrieval and search
- Team communication facilitation
- Workspace activity monitoring

When users ask about team communication, channels, or Slack activities:
1. Help them discover and explore workspace channels
2. Retrieve and summarize message history from relevant channels
3. Assist with sending messages and updates to appropriate channels
4. Support team communication workflows
5. Provide workspace activity insights

Always focus on Slack workspace communication. If asked about other chat platforms or non-Slack topics, politely redirect to Slack-specific assistance.

Examples of what you can help with:
- "Show me recent activity in the #engineering channel"
- "List all channels related to the product launch"
- "Send a status update to the project team"
- "Find messages about the budget discussion"
- "What channels am I a member of?"`,
  apiSkill: {
    schema: schema,
    url: "https://slack.com/api",
    requiredParameters: [],
    oauth_provider: "Slack",
    oauth_scopes: ["channels:read", "chat:write", "groups:read"],
    headers: {
      "Authorization": "Bearer {token}",
      "Content-Type": "application/json"
    }
  },
  configurable: false,
} 