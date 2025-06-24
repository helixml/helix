import { IAgentSkill } from '../../../types';
import FolderIcon from '@mui/icons-material/Folder';

const schema = `openapi: 3.0.3
info:
  title: Google Drive API
  description: Access Google Drive files and folders
  version: "v3"
servers:
  - url: https://www.googleapis.com
paths:
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
          description: Search query (e.g., "name contains 'presentation'", "mimeType='application/pdf'")
        - name: pageSize
          in: query
          schema:
            type: integer
            default: 100
            maximum: 1000
        - name: orderBy
          in: query
          schema:
            type: string
            default: "modifiedTime desc"
        - name: fields
          in: query
          schema:
            type: string
            default: "files(id,name,mimeType,size,modifiedTime,webViewLink,parents)"
        - name: pageToken
          in: query
          schema:
            type: string
          description: Token for pagination
      responses:
        '200':
          description: List of Google Drive files
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
                        modifiedTime:
                          type: string
                        webViewLink:
                          type: string
                        parents:
                          type: array
                          items:
                            type: string
                  nextPageToken:
                    type: string
    post:
      summary: Upload file to Google Drive
      operationId: uploadDriveFile
      security:
        - BearerAuth: []
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [name]
              properties:
                name:
                  type: string
                parents:
                  type: array
                  items:
                    type: string
                description:
                  type: string
      responses:
        '200':
          description: File uploaded successfully
          content:
            application/json:
              schema:
                type: object
                properties:
                  id:
                    type: string
                  name:
                    type: string
                  webViewLink:
                    type: string
  /drive/v3/files/{fileId}:
    get:
      summary: Get Google Drive file by ID
      operationId: getDriveFile
      security:
        - BearerAuth: []
      parameters:
        - name: fileId
          in: path
          required: true
          schema:
            type: string
        - name: fields
          in: query
          schema:
            type: string
            default: "id,name,mimeType,size,modifiedTime,webViewLink,parents,description"
      responses:
        '200':
          description: Google Drive file details
          content:
            application/json:
              schema:
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
                  modifiedTime:
                    type: string
                  webViewLink:
                    type: string
                  description:
                    type: string
    patch:
      summary: Update Google Drive file
      operationId: updateDriveFile
      security:
        - BearerAuth: []
      parameters:
        - name: fileId
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
                name:
                  type: string
                description:
                  type: string
                parents:
                  type: array
                  items:
                    type: string
      responses:
        '200':
          description: File updated successfully
    delete:
      summary: Delete Google Drive file
      operationId: deleteDriveFile
      security:
        - BearerAuth: []
      parameters:
        - name: fileId
          in: path
          required: true
          schema:
            type: string
      responses:
        '204':
          description: File deleted successfully
  /drive/v3/files/{fileId}/permissions:
    get:
      summary: List file permissions
      operationId: listFilePermissions
      security:
        - BearerAuth: []
      parameters:
        - name: fileId
          in: path
          required: true
          schema:
            type: string
      responses:
        '200':
          description: List of file permissions
          content:
            application/json:
              schema:
                type: object
                properties:
                  permissions:
                    type: array
                    items:
                      type: object
                      properties:
                        id:
                          type: string
                        type:
                          type: string
                        role:
                          type: string
                        emailAddress:
                          type: string
    post:
      summary: Share Google Drive file
      operationId: shareDriveFile
      security:
        - BearerAuth: []
      parameters:
        - name: fileId
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
              required: [role, type]
              properties:
                role:
                  type: string
                  enum: [reader, writer, commenter, owner]
                type:
                  type: string
                  enum: [user, group, domain, anyone]
                emailAddress:
                  type: string
      responses:
        '200':
          description: File shared successfully
components:
  securitySchemes:
    BearerAuth:
      type: http
      scheme: bearer
      bearerFormat: JWT`;

export const googleDriveTool: IAgentSkill = {
  name: "Google Drive",
  icon: <FolderIcon sx={{ fontSize: 24 }} />,
  description: `Access Google Drive files and folders with OAuth authentication.

This skill provides secure access to Google Drive API, allowing you to:
• List, search, and manage Google Drive files
• Upload and download documents
• Share files and manage permissions
• Organize files and folders

Example Queries:
- "List my recent Google Drive documents"
- "Find the presentation file I worked on yesterday"
- "Share the project folder with the team"
- "Upload a new document to my Drive"`,
  systemPrompt: `You are a Google Drive file management assistant. Your expertise is in helping users organize, access, and manage their Google Drive files and folders efficiently.

Key capabilities:
- File and folder listing with powerful search capabilities
- Document upload, download, and organization
- File sharing and permission management
- Drive storage and organization insights

When users ask about documents, file storage, or file management:
1. Help them search and locate files using Drive's search syntax
2. Assist with file organization and folder management
3. Support file sharing and collaboration workflows
4. Provide file upload and download assistance
5. Help with Drive storage optimization

Always focus on Google Drive file management. If asked about other Google services or non-Drive topics, politely redirect to Google Drive-specific assistance.

Examples of what you can help with:
- "Find all PDF files in my Drive from last month"
- "Share the budget spreadsheet with read-only access"
- "Create a new folder for the project documents"
- "Show me files that are taking up the most storage"
- "List all files shared with me recently"`,
  apiSkill: {
    schema: schema,
    url: "https://www.googleapis.com",
    requiredParameters: [],
    oauth_provider: "Google",
    oauth_scopes: [
      "https://www.googleapis.com/auth/drive.file"
    ],
    headers: {
      "Authorization": "Bearer {token}"
    }
  },
  configurable: false,
}; 