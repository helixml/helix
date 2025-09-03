# Testing Spec-Driven Tasks with Zed Runners

This guide walks you through testing the complete spec-driven task system using the **actual UI buttons and interactions** - no API calls needed!

## Architecture Overview

**Zed Runner**: Always-on task execution agent (replaces gptscript runner)
- Handles spec-driven tasks and code implementation
- No profile needed - starts with basic `./stack start`
- RDP access for watching agents work

**GPU Runner**: Optional LLM inference runner (unchanged)
- Handles AI model inference (spec generation)
- Started with `WITH_RUNNER=1` for GPU acceleration
- Profile-based, only needed for local LLM inference

## Prerequisites

- Docker and Docker Compose
- Go 1.21+
- OpenAI API key (for spec generation)
- RDP client (for watching Zed agents work)
- Zed source code at `../zed/` (relative to Helix directory)

## Quick Start

### 1. Start Services (Auto-builds Zed if needed)

```bash
# Start all services including Zed task runner
./stack start

# OR with GPU LLM inference (if you have GPU)
WITH_RUNNER=1 ./stack start
```

The stack script will:
- Check if `../zed/` exists (if not, tell you to get it)
- Check if Zed binary exists, build if needed
- Start all services including the Zed runner

### 2. Verify Zed Runner is Active

```bash
# Check services
docker compose -f docker-compose.dev.yaml ps

# Should see 'helix-zed-runner' container

# Test Zed runner is responding
curl http://localhost:3030/health
```

### 3. Connect via RDP to Watch Agents

```bash
# RDP connection:
# Host: localhost:5900
# User: zed
# Password: zed123

# Use any RDP client:
# - Windows: mstsc
# - Mac: Microsoft Remote Desktop
# - Linux: Remmina, rdesktop
```

## Complete UI Workflow Test

### Phase 1: Create Spec-Driven Task

1. **Open Helix UI**: Navigate to `http://localhost:3000` in your browser

2. **Login** (if needed): Use your credentials to log into Helix

3. **Go to Agent Dashboard**: 
   - Look for "Agents" or "Dashboard" in the main navigation
   - Click on the agent management section

4. **Create New Task**:
   - Click the **"Create Spec-Driven Task"** button
   - Fill in the form:
     - **Prompt**: `"Add a theme switcher with dark/light mode toggle to the header"`
     - **Project**: Select `"React Todo App"` from dropdown
     - **Priority**: Choose `"High"`
     - **Type**: Leave as `"Feature"`
   - Click **"Start Spec-Driven Development"** button

5. **Watch Task Appear**:
   - You should see your new task appear in the pending work queue
   - Status should show as "Backlog" (gray) initially
   - Then change to "Creating Specs" (amber) with spinning indicator

### Phase 2: Monitor Spec Generation

1. **Watch Status Change**:
   - Task card will show "🤖 helix-spec-agent generating specifications"
   - Status indicator changes from gray → amber
   - Progress text updates in real-time

2. **Wait for Spec Completion**:
   - Status will change to "Awaiting Approval" (blue)
   - Task card will show "Generated Specs Available"
   - You'll see: "✓ Requirements • ✓ Technical Design • ✓ Implementation Plan"

3. **View Generated Specs**:
   - Click on the task card to expand details
   - Click **"View Specifications"** button
   - Review the three generated documents:
     - Requirements Specification (user stories + acceptance criteria)
     - Technical Design (architecture, components, styling approach)
     - Implementation Plan (step-by-step tasks)

### Phase 3: Approve Specifications

1. **Review the Generated Specs**:
   - Read through each specification document
   - Check that they make sense for adding a theme switcher
   - Look for proper Material-UI integration (since it's a React app)

2. **Approve the Specs**:
   - Click the **"Approve Specifications"** button
   - Or if you want changes, click **"Request Changes"** and add comments

3. **Watch Status Update**:
   - Status changes to "✓ Specs Approved" (light green)
   - Then quickly to "Ready for Implementation" (purple)

### Phase 4: Watch Zed Runner Implementation

This is the main event - watching the Zed agent work!

1. **Monitor Status Transition**:
   - Status changes to "Coding in Progress" (amber)
   - Task card shows "💻 zed-runner-1 coding"
   - Branch name appears (e.g., "feature/task-123")

2. **Connect via RDP and Watch**:
   - Open your RDP client
   - Connect to `localhost:5900` (user: zed, password: zed123)
   - You should see a Linux desktop with Zed editor

3. **Watch the Magic Happen**:
   - Watch as the Zed runner:
     - Downloads the React todo app sample code
     - Initializes git repository
     - Creates a feature branch
     - Opens relevant files in Zed editor (`src/App.tsx`, theme files)
     - Creates theme context and components
     - Implements the dark/light mode toggle in the header
     - Tests the functionality
     - Commits the working code with proper git messages

4. **UI Status Updates**:
   - Watch the task card in the web UI update in real-time
   - Progress indicators show current activity
   - When complete, status shows "Code Review" (dark blue)
   - Finally "✓ Completed" (green)

## UI Elements to Look For

### Task Card Visual Indicators

- **Color-coded status bars**: Left border shows current phase color
- **Agent indicators**: Shows which agent is working (spec vs implementation)
- **Progress chips**: Color-coded status chips with descriptive labels
- **Context panels**: Expandable sections showing relevant info per phase

### Interactive Buttons

- **"Create Spec-Driven Task"**: Main task creation button
- **"View Specifications"**: Opens spec review modal
- **"Approve Specifications"**: Green approval button
- **"Request Changes"**: Orange revision button with comment field
- **"View Progress"**: Shows detailed phase progression
- **"View Code"**: Links to git branch when implementation complete

### Real-time Updates

- **Auto-refresh**: UI updates every 10 seconds without reload
- **Status animations**: Spinning indicators during active work
- **Toast notifications**: Success/error messages for actions
- **Live progress**: Task cards update as agents work

## Sample Projects Available in UI

When creating tasks, you'll see these options in the project dropdown:

1. **"React Todo App"** - Modern React with TypeScript and Material-UI
   - Good for UI feature testing (theme switchers, auth, etc.)

2. **"Express API Server"** - Node.js + Express + TypeScript + PostgreSQL  
   - Good for backend API testing (auth endpoints, middleware, etc.)

3. **"Flask API"** - Python + Flask + SQLAlchemy + PostgreSQL
   - Good for Python backend testing (auth, APIs, data processing)

## Business Task Examples (Non-Programming)

Spec-driven tasks aren't just for coding! Here are examples using Helix's actual business skills (web search, Gmail, Google Calendar, browser automation):

### Example 1: Competitive Intelligence Research

1. **Create Task in UI**:
   - Prompt: `"Research our top 5 competitors: [Company A, Company B, Company C, Company D, Company E]. For each one, find their latest product announcements, pricing changes, key hires, and funding news from the past 3 months"`

2. **AI-Generated Specifications Will Include**:
   
   **Requirements Spec**:
   - User story: "As a product manager, I want comprehensive competitor intelligence so I can make informed strategic decisions"
   - Acceptance criteria: Recent news, product updates, pricing info, team changes, funding rounds
   - Research scope: 3-month lookback, credible sources only, structured summary format
   
   **Technical Design**:  
   - Web search skill for each competitor
   - Browser skill to deep-dive into company websites, press releases, and news articles
   - Information extraction and categorization
   - Structured report generation with source links
   
   **Implementation Plan**:
   - Task 1: Search for each competitor's recent news and announcements
   - Task 2: Visit competitor websites to check product pages and pricing
   - Task 3: Search for hiring announcements and team changes
   - Task 4: Research funding and investment news
   - Task 5: Compile findings into structured competitor intelligence report

3. **Agent Implementation**:
   - Uses web search skill to find recent news for each competitor
   - Uses browser skill to visit company websites and extract current information
   - Compiles comprehensive research report with findings categorized by company
   - Includes source links for verification and follow-up
   
4. **Business Result**:
   - Comprehensive competitor intelligence report
   - Reduces research time from 8 hours to 30 minutes
   - Consistent format for easy comparison
   - Source links for deeper investigation

### Example 2: Event Planning and Coordination

1. **Create Task in UI**:
   - Prompt: `"Plan and coordinate our Q1 All-Hands meeting for 150 people. Find venues in downtown area, check team calendars for best dates, send save-the-dates, and create follow-up reminder sequence"`

2. **AI-Generated Specifications**:
   
   **Requirements Spec**:
   - User story: "As an executive assistant, I want the All-Hands meeting fully planned and coordinated so the team can focus on content"
   - Acceptance criteria: Venue booked, calendar invites sent, reminders scheduled, logistics confirmed
   - Constraints: Downtown location, 150+ capacity, 3-4 hour duration, AV equipment required
   
   **Implementation Plan**:
   - Task 1: Use web search to find suitable venues in downtown area with capacity and AV
   - Task 2: Use Google Calendar to check team availability and find optimal dates
   - Task 3: Use Gmail to send save-the-date emails to all attendees
   - Task 4: Schedule follow-up reminder emails at 2 weeks and 1 week before
   - Task 5: Create calendar events with venue details and agenda placeholder

3. **Agent Implementation**:
   - Searches for downtown venues with meeting specifications
   - Checks Google Calendar across the organization for date conflicts
   - Sends professional save-the-date emails via Gmail
   - Creates calendar events with venue information
   - Sets up automated reminder email sequence

4. **Business Result**:
   - Fully coordinated company event
   - Reduces planning time from 2 days to 2 hours  
   - Ensures no scheduling conflicts
   - Professional communication to all attendees

### Example 3: Customer Outreach Campaign

1. **Create Task in UI**:
   - Prompt: `"Create a customer win-back campaign for accounts that haven't purchased in 6+ months. Research what industry trends they might be interested in, draft personalized emails, and schedule follow-up meetings with our sales team"`

2. **Agent Implementation**:
   - Uses web search to research industry trends relevant to each customer segment
   - Uses browser skill to research each customer's recent company news
   - Uses Gmail to send personalized win-back emails with relevant industry insights
   - Uses Google Calendar to schedule follow-up calls with sales team
   - Creates tracking system for campaign responses

### Business Task Templates

Here are proven templates using available Helix skills:

#### **Research & Intelligence** (Web Search + Browser)
- `"Research [TOPIC/COMPETITORS/MARKET] and create comprehensive report with [SPECIFIC FOCUS AREAS]"`
- `"Monitor [COMPANIES/TRENDS/NEWS] weekly and alert me about [SPECIFIC TRIGGERS]"`
- `"Investigate [BUSINESS PROBLEM] by researching industry best practices and case studies"`

#### **Communication & Outreach** (Gmail + Calendar)
- `"Create email campaign for [AUDIENCE] about [TOPIC] with [PERSONALIZATION] and schedule follow-ups"`
- `"Coordinate [EVENT/MEETING] by checking calendars, sending invites, and managing RSVPs"`
- `"Follow up with [CLIENT LIST] about [TOPIC] and schedule meetings based on their responses"`

#### **Combined Workflows** (Multiple Skills)
- `"Plan product launch by researching competitor approaches, scheduling team meetings, and creating email announcements"`
- `"Organize customer feedback sessions by finding participants, scheduling interviews, and preparing research-based questions"`
- `"Execute partnership outreach by researching target companies, finding contacts, and coordinating introduction meetings"`

**Pro Tip**: These tasks leverage AI's ability to research, write, and coordinate - exactly what business professionals need most!

## Testing Different Features

### UI Feature Testing (React Todo App)
- Click **"Create Spec-Driven Task"**
- Prompts to try:
  - `"Add user authentication with login/logout"`
  - `"Add drag and drop todo reordering"`
  - `"Add todo categories and filtering"`
  - `"Add due dates and reminders"`

### Backend Testing (Express API)
- Select "Express API Server" from dropdown
- Prompts to try:
  - `"Add JWT authentication middleware"`
  - `"Add rate limiting and API security"`
  - `"Add file upload endpoints"`
  - `"Add email notification system"`

### Python Backend (Flask API)
- Select "Flask API" from dropdown
- Prompts to try:
  - `"Add user roles and permissions"`
  - `"Add data export functionality"`
  - `"Add webhook endpoints"`
  - `"Add background job processing"`

## Troubleshooting via UI

### Zed Runner Not Starting
- Check the agent dashboard for error indicators
- Red status badges indicate service issues
- Click on error indicators for details

### RDP Connection Issues
- Verify container health in the dashboard
- Green indicators should show for all services
- Try restarting via the UI restart button if available

### Task Stuck in One Phase
- Look for error badges or warning indicators
- Check the task details panel for error messages
- Use the "Retry" button if available

### Spec Generation Failing
- Verify OpenAI API key is configured (check settings)
- Look for API quota warnings in the UI
- Check the system status indicators

## Success Criteria (What You Should See)

A successful test shows:

1. ✅ **Task Creation**: Clean form submission, task appears in queue
2. ✅ **Status Progression**: Visual transitions through all phases
3. ✅ **Spec Generation**: Three detailed documents generated
4. ✅ **Human Approval**: Working approve/reject workflow
5. ✅ **Zed Implementation**: Visible via RDP, real code editing
6. ✅ **Real Code**: Functional implementation committed to git
7. ✅ **UI Updates**: Status flows back to dashboard in real-time
8. ✅ **End Result**: Working feature you can actually test

## Performance Expectations

- **Task Creation**: Instant UI response, appears in queue immediately
- **Spec Generation**: 30-90 seconds, progress indicators active
- **Approval Workflow**: Instant UI response when buttons clicked
- **Implementation Start**: 15-30 seconds for Zed runner pickup
- **Code Implementation**: 3-10 minutes for typical features
- **Total Time**: 5-15 minutes end-to-end for simple tasks

## Development Notes

The UI should feel responsive and provide clear feedback at every step. If you're seeing API errors, timeouts, or missing status updates, that indicates integration issues that need debugging.

The RDP connection gives you a direct view into what the Zed agent is actually doing - this is invaluable for understanding whether the system is working correctly or where it might be failing.

Focus on the **user experience** - can someone create a task with a simple description and get working code without touching APIs or command lines?