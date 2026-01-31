import { IAgentSkill } from '../../../types';
import { LinkedInLogo } from '../../icons/ProviderIcons';

const schema = `openapi: 3.0.3
info:
  title: LinkedIn API
  description: Access LinkedIn profile information and professional network
  version: "v2"
servers:
  - url: https://api.linkedin.com/v2
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
                  firstName:
                    type: object
                    properties:
                      localized:
                        type: object
                      preferredLocale:
                        type: object
                        properties:
                          country:
                            type: string
                          language:
                            type: string
                  lastName:
                    type: object
                    properties:
                      localized:
                        type: object
                      preferredLocale:
                        type: object
                        properties:
                          country:
                            type: string
                          language:
                            type: string
                  profilePicture:
                    type: object
                    properties:
                      displayImage:
                        type: string
  /emailAddress:
    get:
      summary: Get user email address
      operationId: getUserEmail
      security:
        - BearerAuth: []
      parameters:
        - name: q
          in: query
          required: true
          schema:
            type: string
            enum: [members]
        - name: projection
          in: query
          required: true
          schema:
            type: string
            default: "(elements*(handle~))"
      responses:
        '200':
          description: User email information
          content:
            application/json:
              schema:
                type: object
                properties:
                  elements:
                    type: array
                    items:
                      type: object
                      properties:
                        handle:
                          type: string
                        handleType:
                          type: string
  /people/(id:{person-id}):
    get:
      summary: Get person profile by ID
      operationId: getPersonProfile
      security:
        - BearerAuth: []
      parameters:
        - name: person-id
          in: path
          required: true
          schema:
            type: string
        - name: projection
          in: query
          schema:
            type: string
            default: "(id,firstName,lastName,profilePicture(displayImage~:playableStreams))"
      responses:
        '200':
          description: Person profile information
          content:
            application/json:
              schema:
                type: object
                properties:
                  id:
                    type: string
                  firstName:
                    type: object
                  lastName:
                    type: object
                  profilePicture:
                    type: object
  /connections:
    get:
      summary: Get user connections
      operationId: getUserConnections
      security:
        - BearerAuth: []
      parameters:
        - name: q
          in: query
          required: true
          schema:
            type: string
            enum: [viewer]
        - name: start
          in: query
          schema:
            type: integer
            default: 0
        - name: count
          in: query
          schema:
            type: integer
            default: 50
            maximum: 500
      responses:
        '200':
          description: User connections
          content:
            application/json:
              schema:
                type: object
                properties:
                  elements:
                    type: array
                    items:
                      type: object
                      properties:
                        to:
                          type: string
                  paging:
                    type: object
                    properties:
                      count:
                        type: integer
                      start:
                        type: integer
                      total:
                        type: integer
  /shares:
    post:
      summary: Create a share (post content)
      operationId: createShare
      security:
        - BearerAuth: []
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required:
                - author
                - lifecycleState
                - specificContent
                - visibility
              properties:
                author:
                  type: string
                  description: URN of the author (e.g., "urn:li:person:{id}")
                lifecycleState:
                  type: string
                  enum: [PUBLISHED]
                specificContent:
                  type: object
                  properties:
                    "com.linkedin.ugc.ShareContent":
                      type: object
                      properties:
                        shareCommentary:
                          type: object
                          properties:
                            text:
                              type: string
                        shareMediaCategory:
                          type: string
                          enum: [NONE, ARTICLE, IMAGE]
                visibility:
                  type: object
                  properties:
                    "com.linkedin.ugc.MemberNetworkVisibility":
                      type: string
                      enum: [PUBLIC, CONNECTIONS]
      responses:
        '201':
          description: Share created successfully
          content:
            application/json:
              schema:
                type: object
                properties:
                  id:
                    type: string
  /organizationalEntityAcls:
    get:
      summary: Get organizations user can manage
      operationId: getOrganizations
      security:
        - BearerAuth: []
      parameters:
        - name: q
          in: query
          required: true
          schema:
            type: string
            enum: [roleAssignee]
        - name: role
          in: query
          required: true
          schema:
            type: string
            enum: [ADMINISTRATOR]
        - name: projection
          in: query
          schema:
            type: string
            default: "(elements*(organizationalTarget~(id,name,logoV2(original~:playableStreams))))"
      responses:
        '200':
          description: Organizations user can manage
          content:
            application/json:
              schema:
                type: object
                properties:
                  elements:
                    type: array
                    items:
                      type: object
                      properties:
                        organizationalTarget:
                          type: object
                          properties:
                            id:
                              type: string
                            name:
                              type: object
                            logoV2:
                              type: object
components:
  securitySchemes:
    BearerAuth:
      type: http
      scheme: bearer
      bearerFormat: JWT`;

export const linkedInTool: IAgentSkill = {
  name: "LinkedIn",
  icon: <LinkedInLogo sx={{ fontSize: 24 }} />,
  description: `Access LinkedIn profile information and professional network with OAuth authentication.

This skill provides secure access to LinkedIn's API, allowing you to:
• Get user profile information and contact details
• Access professional connections and network data
• View organizations and company pages user manages
• Create and share content on LinkedIn
• Retrieve email addresses and contact information

Example Queries:
- "Get my LinkedIn profile information"
- "Show my email address from LinkedIn"
- "List my LinkedIn connections"
- "Post an update to my LinkedIn profile"
- "Show organizations I can manage on LinkedIn"`,
  systemPrompt: `You are a LinkedIn professional networking assistant. Your expertise is in helping users manage their professional presence, network connections, and career-related activities on LinkedIn.

Key capabilities:
- Professional profile management and insights
- Network connection analysis and growth
- Content creation and professional sharing
- Organization and company page management
- Professional contact information access

When users ask about professional networking, career development, or LinkedIn activities:
1. Help them access and understand their professional profile information
2. Assist with network connection insights and relationship building
3. Support content creation and professional sharing strategies
4. Provide organization management capabilities for business pages
5. Help leverage LinkedIn for career and business development

Always focus on professional networking and career development. If asked about other social platforms or non-professional topics, politely redirect to LinkedIn professional networking assistance.

Examples of what you can help with:
- "Show me my LinkedIn profile summary"
- "How many professional connections do I have?"
- "Share a professional update about my recent project"
- "List the organizations I can manage on LinkedIn"
- "Get my contact information for networking"`,
  apiSkill: {
    schema: schema,
    url: "https://api.linkedin.com/v2",
    requiredParameters: [],
    oauth_provider: "LinkedIn",
    oauth_scopes: ["r_liteprofile", "r_emailaddress", "w_member_social"],
    headers: {
      "Authorization": "Bearer {token}",
      "Content-Type": "application/json",
      "X-Restli-Protocol-Version": "2.0.0"
    }
  },
  configurable: false,
} 