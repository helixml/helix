import { IAgentSkill } from '../../../types';
import CalendarTodayIcon from '@mui/icons-material/CalendarToday';

const schema = `openapi: 3.0.3
info:
  title: Google Calendar API
  description: Access Google Calendar events and calendars
  version: "v3"
servers:
  - url: https://www.googleapis.com
paths:
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
          description: Lower bound for event start time (RFC3339 timestamp)
        - name: timeMax
          in: query
          schema:
            type: string
            format: date-time
          description: Upper bound for event start time (RFC3339 timestamp)
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
        - name: orderBy
          in: query
          schema:
            type: string
            enum: [startTime, updated]
          description: Order of events returned
        - name: maxResults
          in: query
          schema:
            type: integer
            default: 250
            maximum: 2500
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
                            timeZone:
                              type: string
                        end:
                          type: object
                          properties:
                            dateTime:
                              type: string
                            date:
                              type: string
                            timeZone:
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
                        location:
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
  /calendar/v3/calendars/primary/events/{eventId}:
    get:
      summary: Get calendar event by ID
      operationId: getCalendarEvent
      security:
        - BearerAuth: []
      parameters:
        - name: eventId
          in: path
          required: true
          schema:
            type: string
      responses:
        '200':
          description: Calendar event details
          content:
            application/json:
              schema:
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
                  end:
                    type: object
                  attendees:
                    type: array
                  location:
                    type: string
    patch:
      summary: Update calendar event
      operationId: updateCalendarEvent
      security:
        - BearerAuth: []
      parameters:
        - name: eventId
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
                summary:
                  type: string
                description:
                  type: string
                start:
                  type: object
                  properties:
                    dateTime:
                      type: string
                      format: date-time
                    timeZone:
                      type: string
                end:
                  type: object
                  properties:
                    dateTime:
                      type: string
                      format: date-time
                    timeZone:
                      type: string
                location:
                  type: string
      responses:
        '200':
          description: Event updated successfully
    delete:
      summary: Delete calendar event
      operationId: deleteCalendarEvent
      security:
        - BearerAuth: []
      parameters:
        - name: eventId
          in: path
          required: true
          schema:
            type: string
      responses:
        '204':
          description: Event deleted successfully
  /calendar/v3/calendars:
    get:
      summary: List calendars
      operationId: listCalendars
      security:
        - BearerAuth: []
      responses:
        '200':
          description: List of calendars
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
                        primary:
                          type: boolean
                        accessRole:
                          type: string
components:
  securitySchemes:
    BearerAuth:
      type: http
      scheme: bearer
      bearerFormat: JWT`;

export const googleCalendarTool: IAgentSkill = {
  name: "Google Calendar",
  icon: <CalendarTodayIcon sx={{ fontSize: 24 }} />,
  description: `Access Google Calendar events and scheduling with OAuth authentication.

This skill provides secure access to Google Calendar API, allowing you to:
• View, create, and manage calendar events
• Schedule meetings and appointments
• Search calendar events
• Manage multiple calendars

Example Queries:
- "Show my calendar events for tomorrow"
- "Schedule a team meeting for next Tuesday"
- "Find my meeting with John next week"
- "Create a reminder for the project deadline"`,
  systemPrompt: `You are a Google Calendar scheduling assistant. Your expertise is in helping users manage their calendar events, schedule meetings, and organize their time efficiently.

Key capabilities:
- Calendar event viewing and management
- Meeting and appointment scheduling
- Event search and filtering
- Multi-calendar management
- Time zone handling for events

When users ask about scheduling, meetings, or calendar management:
1. Help them view and search their calendar events
2. Assist with scheduling meetings and appointments
3. Support event creation with proper time zones and attendees
4. Provide calendar organization and time management tips
5. Help with recurring event management

Always focus on Google Calendar scheduling and time management. If asked about other Google services or non-calendar topics, politely redirect to Google Calendar-specific assistance.

Examples of what you can help with:
- "What meetings do I have today?"
- "Schedule a 1-hour meeting with the team tomorrow at 2 PM"
- "Find all events related to the project launch"
- "Block out time for focused work this afternoon"
- "Show me my availability for next week"`,
  apiSkill: {
    schema: schema,
    url: "https://www.googleapis.com",
    requiredParameters: [],
    oauth_provider: "Google",
    oauth_scopes: [
      "https://www.googleapis.com/auth/calendar"
    ],
    headers: {
      "Authorization": "Bearer {token}"
    }
  },
  configurable: false,
}; 