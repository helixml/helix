import { IAgentSkill } from '../../../types';
import CloudIcon from '@mui/icons-material/Cloud';

const schema = `openapi: 3.0.3
info:
  title: Microsoft OneDrive API
  description: Access OneDrive files and folders via Microsoft Graph
  version: "v1.0"
servers:
  - url: https://graph.microsoft.com/v1.0
paths:
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
        - name: $orderby
          in: query
          schema:
            type: string
            default: "lastModifiedDateTime desc"
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
components:
  securitySchemes:
    BearerAuth:
      type: http
      scheme: bearer
      bearerFormat: JWT`;

export const oneDriveTool: IAgentSkill = {
  name: "OneDrive",
  icon: <CloudIcon sx={{ fontSize: 24 }} />,
  description: `Access OneDrive files and folders with OAuth authentication.

This skill provides secure access to Microsoft OneDrive via Graph API, allowing you to:
• List, search, and manage OneDrive files and folders
• Create and organize folder structures
• Share files and create sharing links
• Upload and download documents

Example Queries:
- "List my recent OneDrive documents"
- "Search for Excel files in my OneDrive"
- "Create a new folder for the project files"
- "Share the presentation with view-only access"`,
  systemPrompt: `You are a OneDrive file management assistant. Your expertise is in helping users organize, access, and manage their OneDrive files and folders efficiently.

Key capabilities:
- File and folder listing with search capabilities
- Document organization and folder management
- File sharing and link creation
- OneDrive storage insights and optimization

When users ask about documents, file storage, or file management:
1. Help them search and locate files in their OneDrive
2. Assist with file organization and folder structure
3. Support file sharing and collaboration workflows
4. Provide file management and storage optimization tips
5. Help with folder creation and document organization

Always focus on OneDrive file management. If asked about other Microsoft services or non-OneDrive topics, politely redirect to OneDrive-specific assistance.

Examples of what you can help with:
- "Find all Word documents modified this week"
- "Create a sharing link for the budget spreadsheet"
- "Organize my files into project-specific folders"
- "Show me files that are taking up the most storage"
- "List all files shared with me recently"`,
  apiSkill: {
    schema: schema,
    url: "https://graph.microsoft.com/v1.0",
    requiredParameters: [],
    oauth_provider: "Microsoft",
    oauth_scopes: [
      "Files.ReadWrite"
    ]
  },
  configurable: false,
}; 