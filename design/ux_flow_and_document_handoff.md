# UX Flow and Document Handoff Strategy

## Overview

This document designs the complete user experience flow and document handoff strategy for the multi-session SpecTask architecture. The design extends existing Helix task UI components while adding sophisticated multi-session visualization and coordination capabilities.

## Document Handoff Strategy

### Git-Centric Approach (Recommended)

```
Planning Phase:
User Prompt → Planning Agent → Generated Specs → Human Approval → Git Commit

Implementation Phase:  
Git Specs → Zed Instance → Multi-Session Implementation → Session History → Git Commits

Coordination:
Database (Real-time) ↔ Git (Persistent) ↔ UI (Visualization)
```

#### 1. Spec Document Handoff Flow
```
1. SpecTask Creation
   ├── User creates SpecTask from prompt
   ├── Planning agent generates specs in Helix session
   └── Specs stored in SpecTask database record

2. Human Approval
   ├── UI displays generated specs for review
   ├── Human approves/rejects with comments
   └── Approval triggers git workflow

3. Git Commit (On Approval)
   ├── SpecDocumentService creates Kiro-style documents:
   │   ├── specs/{spec_task_id}/requirements.md
   │   ├── specs/{spec_task_id}/design.md
   │   ├── specs/{spec_task_id}/tasks.md
   │   └── specs/{spec_task_id}/spec-metadata.json
   ├── Commits to feature branch: specs/{spec_task_id}
   └── Creates pull request for spec review (optional)

4. Zed Instance Initialization
   ├── Zed instance clones/pulls repository
   ├── Reads spec documents from specs/{spec_task_id}/
   ├── Initializes project with spec context
   └── Creates initial threads with spec awareness

5. Session History Recording
   ├── Each Zed thread records activity in:
   │   ├── sessions/{spec_task_id}/{work_session_id}/conversation.md
   │   ├── sessions/{spec_task_id}/{work_session_id}/code-changes.md
   │   ├── sessions/{spec_task_id}/{work_session_id}/decisions.md
   │   └── sessions/{spec_task_id}/coordination-log.md
   ├── Periodic commits during implementation
   └── Final commit on session completion
```

#### 2. Communication Protocols

**Helix → Git → Zed (Specs)**
- Helix commits approved specs to git
- Zed reads specs on instance startup
- No direct API calls needed for spec handoff

**Zed → Git → Helix (Progress)**
- Zed commits session history to git
- Helix watches git for progress updates
- Database tracks real-time status
- UI combines database + git data

**Real-time Coordination (Database)**
- Session status and progress in database
- Coordination events via API
- WebSocket updates for UI
- Git provides persistent record

## Complete UX Design

### 1. Enhanced SpecTask Creation Flow

#### Existing: Simple Task Creation
```
[Create Task] → [Select Agent] → [Enter Prompt] → [Create]
```

#### New: SpecTask Creation with Planning Preview
```
┌─────────────────────────────────────────────────────────────┐
│ 🎯 Create SpecTask                                          │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│ Task Description                                            │
│ ┌─────────────────────────────────────────────────────────┐ │
│ │ Implement user authentication system with login,       │ │
│ │ logout, registration, and profile management           │ │
│ └─────────────────────────────────────────────────────────┘ │
│                                                             │
│ Task Type: ● Feature ○ Bug Fix ○ Refactor ○ Investigation  │
│ Priority:  ● High ○ Medium ○ Low ○ Critical                 │
│                                                             │
│ Agents:                                                     │
│ ├─ Planning Agent:    [Claude Planning v1  ▼]              │
│ └─ Implementation:    [Zed + Claude Code  ▼]               │
│                                                             │
│ Expected Complexity: ● Complex (Multi-session)             │
│                     ○ Simple (Single session)              │
│                     ○ Auto-detect                          │
│                                                             │
│ 💡 Preview: This will create a multi-session SpecTask     │
│     with planning → approval → coordinated implementation  │
│                                                             │
│ [Cancel] [Create SpecTask]                                  │
└─────────────────────────────────────────────────────────────┘
```

### 2. Specification Review and Approval Interface

#### Two-Panel Approval Interface
```
┌─────────────────────────────────────────────────────────────────────────┐
│ 📋 Specification Review: Implement User Authentication                  │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│ ┌─────────────────────────┐ ┌─────────────────────────────────────────┐ │
│ │ 📑 Specification Tabs   │ │ 📝 Content View                        │ │
│ │                         │ │                                         │ │
│ │ ● Requirements.md       │ │ # User Authentication Requirements      │ │
│ │ ○ Design.md            │ │                                         │ │
│ │ ○ Tasks.md             │ │ ## Functional Requirements              │ │
│ │ ○ Implementation Plan   │ │                                         │ │
│ │                         │ │ WHEN a user provides valid credentials  │ │
│ │ Generation Info:        │ │ THE SYSTEM SHALL authenticate the user  │ │
│ │ ├─ Agent: Claude v1     │ │ and create a secure session            │ │
│ │ ├─ Tokens: 15,420       │ │                                         │ │
│ │ ├─ Generated: 14:30     │ │ WHEN a user submits registration info   │ │
│ │ └─ Quality: ⭐⭐⭐⭐⭐    │ │ THE SYSTEM SHALL validate the data and  │ │
│ │                         │ │ create a new user account              │ │
│ │ 🎯 Implementation Plan: │ │                                         │ │
│ │ ├─ 5 tasks identified   │ │ ## Non-Functional Requirements          │ │
│ │ ├─ Est. 2-3 days        │ │ - Login response time < 200ms           │ │
│ │ ├─ Multi-session needed │ │ - Support 10,000 concurrent users       │ │
│ │ └─ Zed integration      │ │ - 99.9% uptime SLA                     │ │
│ │                         │ │                                         │ │
│ │ [💾 Download Specs]     │ │ [Continue reading...]                   │ │
│ │ [🔄 Regenerate]         │ │                                         │ │
│ └─────────────────────────┘ └─────────────────────────────────────────┘ │
│                                                                         │
│ ┌─────────────────────────────────────────────────────────────────────┐ │
│ │ 📝 Review Comments                                                  │ │
│ │ ┌─────────────────────────────────────────────────────────────────┐ │ │
│ │ │ The specifications look comprehensive and well-structured.      │ │ │
│ │ │ I particularly like the EARS notation for requirements and      │ │ │
│ │ │ the multi-session implementation approach.                      │ │ │
│ │ │                                                                 │ │ │
│ │ │ Suggestions:                                                    │ │ │
│ │ │ - Consider adding OAuth integration for Task 2                 │ │ │
│ │ │ - Add performance monitoring to Task 4                         │ │ │
│ │ └─────────────────────────────────────────────────────────────────┘ │ │
│ └─────────────────────────────────────────────────────────────────────┘ │
│                                                                         │
│ Decision: ● Approve and proceed  ○ Request changes  ○ Reject            │
│                                                                         │
│ [❌ Reject] [🔄 Request Changes] [✅ Approve & Start Implementation]      │
└─────────────────────────────────────────────────────────────────────────┘
```

### 3. Multi-Session Implementation Dashboard

#### Main SpecTask Dashboard (Extends existing TasksTable)
```
┌─────────────────────────────────────────────────────────────────────────────────┐
│ 🎯 SpecTask: Implement User Authentication                                      │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                 │
│ ┌─────────────────────┐ ┌───────────────────┐ ┌─────────────────────────────┐   │
│ │ 📊 Overall Progress │ │ 🖥️ Zed Instance   │ │ 👥 Active Work Sessions     │   │
│ │                     │ │                   │ │                             │   │
│ │    ████████░░ 78%   │ │ Status: ✅ Active │ │ 🔄 Backend API (Session 2)  │   │
│ │                     │ │ Threads: 6        │ │ 🔄 Frontend UI (Session 3)  │   │
│ │ ✅ 3 Complete       │ │ Uptime: 2h 15m    │ │ ⏸️ Testing (Session 4)      │   │
│ │ 🔄 2 In Progress    │ │ CPU: 45% Memory:  │ │ ✅ Database (Session 1)     │   │
│ │ ⏳ 1 Pending        │ │      2.1GB        │ │ ✅ Security (Session 5)     │   │
│ │                     │ │                   │ │ 🔄 Docs (Session 6)         │   │
│ │ [📊 Detailed View]  │ │ [🔍 Monitor]      │ │ [+ Spawn Session]           │   │
│ └─────────────────────┘ └───────────────────┘ └─────────────────────────────┘   │
│                                                                                 │
│ ┌─────────────────────────────────────────────────────────────────────────────┐ │
│ │ 🧵 Work Session Hierarchy                                                  │ │
│ │                                                                             │ │
│ │ SpecTask: Implement User Authentication                                     │ │
│ │ ├── 📁 Planning Phase (Complete)                                           │ │
│ │ │   └── Planning Session → Generated comprehensive specs                   │ │
│ │ └── 🏗️ Implementation Phase (In Progress)                                  │ │
│ │     ├── 🗄️ Database Schema (Session 1) ✅ Complete                         │ │
│ │     ├── 🔧 Backend API (Session 2) 🔄 70% - Last activity: 2m ago          │ │
│ │     │   ├── 🔍 Security Audit (Session 5) ✅ Complete - Spawned            │ │
│ │     │   └── 📊 Performance Opt (Session 7) 🔄 40% - Spawned               │ │
│ │     ├── 🎨 Frontend UI (Session 3) 🔄 45% - Last activity: 5m ago          │ │
│ │     │   └── 🎭 UI Polish (Session 8) ⏳ Pending - Spawned                  │ │
│ │     ├── 🧪 Testing (Session 4) ⏸️ Blocked - Waiting for frontend           │ │
│ │     └── 📚 Documentation (Session 6) 🔄 85% - Last activity: 10m ago       │ │
│ │                                                                             │ │
│ │ [🔄 Refresh] [⏸️ Pause All] [📊 Export Report] [🚀 Spawn Emergency Session] │ │
│ └─────────────────────────────────────────────────────────────────────────────┘ │
│                                                                                 │
│ ┌─────────────────────────────────────────────────────────────────────────────┐ │
│ │ 🔔 Recent Coordination Events                                               │ │
│ │                                                                             │ │
│ │ 15:45 Backend API → Frontend UI: "Authentication endpoints ready"          │ │
│ │ 15:43 Security Audit: "Rate limiting implemented, ready for testing"       │ │
│ │ 15:40 Performance Opt: "Query optimization complete, 3x speedup achieved"  │ │
│ │ 15:38 Backend API spawned Performance Optimization session                 │ │
│ │ 15:35 Database Schema → Backend API: "Schema migration complete"           │ │
│ │                                                                             │ │
│ │ [📜 View Full Log] [🔕 Manage Notifications]                               │ │
│ └─────────────────────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────────────────┘
```

## Document Handoff Implementation

### 1. Spec Document Generation and Commit

#### On Spec Approval (Helix Backend)
```typescript
// When user approves specs in UI
const approveSpecs = async (specTaskId: string, approval: SpecApprovalRequest) => {
  // 1. Update SpecTask status
  await updateSpecTaskStatus(specTaskId, 'spec_approved');
  
  // 2. Generate Kiro-style documents
  const specDocs = await generateSpecDocuments(specTaskId, {
    projectPath: `/workspace/${specTaskId}`,
    branchName: `specs/${specTaskId}`,
    createPullRequest: true,
    includeTimestamps: true,
  });
  
  // 3. Commit to git
  const gitResult = await commitSpecDocuments(specDocs);
  
  // 4. Start multi-session implementation
  const implementation = await createImplementationSessions(specTaskId, {
    projectPath: gitResult.projectPath,
    gitBranch: gitResult.branchName,
    specCommitHash: gitResult.commitHash,
  });
  
  return { specDocs, gitResult, implementation };
};
```

#### Git Repository Structure
```
project-repo/
├── src/                          # Main project code
├── specs/                        # Generated specifications
│   └── spec_task_123/
│       ├── requirements.md       # EARS notation requirements
│       ├── design.md            # Technical architecture
│       ├── tasks.md             # Implementation plan
│       └── spec-metadata.json   # Tooling metadata
├── sessions/                     # Session history (recorded during implementation)
│   └── spec_task_123/
│       ├── coordination-log.md   # Inter-session coordination
│       ├── ws_backend_api/       # Work session specific history
│       │   ├── conversation.md   # Full conversation log
│       │   ├── code-changes.md   # Code changes and reasoning
│       │   └── decisions.md      # Key decisions made
│       ├── ws_frontend_ui/
│       │   ├── conversation.md
│       │   └── code-changes.md
│       └── progress-updates.md   # Periodic progress commits
└── README.md                     # Project overview
```

### 2. Zed Instance Integration

#### Zed Startup with Spec Context
```javascript
// When Zed instance starts for SpecTask
const initializeZedInstance = async (specTaskId, projectPath) => {
  // 1. Clone/pull repository
  await git.pull(projectPath);
  
  // 2. Read spec documents
  const specs = await readSpecDocuments(`specs/${specTaskId}/`);
  
  // 3. Initialize project context
  const projectContext = {
    specTask: specs.metadata,
    requirements: specs.requirements,
    design: specs.design,
    tasks: specs.tasks,
    workspacePath: projectPath,
  };
  
  // 4. Create initial threads with spec awareness
  for (const task of specs.tasks) {
    await createThread(`thread_${task.index}`, {
      name: task.title,
      description: task.description,
      systemPrompt: generateImplementationPrompt(task, specs),
      projectContext: projectContext,
    });
  }
  
  // 5. Start session history recording
  await initializeSessionRecording(specTaskId, projectPath);
};
```

#### Session History Recording (During Implementation)
```javascript
// Continuous recording during Zed thread execution
const recordSessionActivity = async (threadId, activity) => {
  const sessionPath = `sessions/${specTaskId}/${threadId}/`;
  
  // Record different types of activity
  switch (activity.type) {
    case 'conversation':
      await appendToFile(`${sessionPath}conversation.md`, formatConversation(activity));
      break;
    case 'code_change':
      await appendToFile(`${sessionPath}code-changes.md`, formatCodeChange(activity));
      break;
    case 'decision':
      await appendToFile(`${sessionPath}decisions.md`, formatDecision(activity));
      break;
    case 'coordination':
      await appendToFile(`sessions/${specTaskId}/coordination-log.md`, formatCoordination(activity));
      break;
  }
  
  // Periodic commits (every 10 minutes or significant milestones)
  if (shouldCommit(activity)) {
    await git.commit(`Update session history: ${activity.summary}`);
  }
};
```

## UI Component Implementation

### 1. Enhanced SpecTask List (Extends TasksTable)