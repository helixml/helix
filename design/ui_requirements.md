# UI Requirements for Multi-Session SpecTask Support

## Overview

This document outlines the user interface requirements for supporting the multi-session SpecTask architecture. The UI needs to provide visibility and control over complex multi-session workflows while maintaining simplicity for basic tasks.

## Current SpecTask UI Status

### Existing Components (✅ Available)
- SpecTask creation from prompts
- Specification review and approval interface
- Basic SpecTask status tracking
- Planning agent interaction
- Single-session implementation monitoring

### Missing Components (❌ Need Implementation)
- Multi-session visualization and management
- Work session spawning controls
- Inter-session coordination dashboard
- Zed instance and thread monitoring
- Real-time progress tracking across sessions

## Core UI Requirements

### 1. Multi-Session SpecTask Dashboard

#### SpecTask Overview Page
```
[SpecTask: "Implement User Authentication"]
┌─────────────────────────────────────────────────────────────┐
│ Status: Implementation  │  Progress: 3/5 Complete (60%)    │
│ Zed Instance: Active    │  Last Activity: 2 minutes ago    │
└─────────────────────────────────────────────────────────────┘

📋 Implementation Plan (5 tasks)
┌─ ✅ Database Schema (Completed)
├─ 🔄 Backend API (In Progress - 70%)
├─ ⏳ Frontend UI (Pending - Blocked by API)
├─ ⏳ Security Hardening (Pending)
└─ ⏳ Testing & Documentation (Pending)

💼 Active Work Sessions (3)
┌─ Backend API Implementation
│  ├─ Status: Active │ Zed Thread: thread_backend_123
│  ├─ Progress: 70% │ Started: 1 hour ago
│  └─ [Spawn Session] [View Details] [Open in Zed]
├─ Database Performance Optimization (Spawned)
│  ├─ Status: Active │ Zed Thread: thread_db_opt_456  
│  ├─ Progress: 30% │ Started: 15 minutes ago
│  └─ Parent: Backend API │ [View Details] [Open in Zed]
└─ Security Audit (Spawned)
   ├─ Status: Active │ Zed Thread: thread_security_789
   ├─ Progress: 10% │ Started: 5 minutes ago
   └─ Parent: Backend API │ [View Details] [Open in Zed]

🔗 Coordination Events (Recent)
┌─ 15:30 - Backend API → Frontend UI: "API endpoints ready"
├─ 15:25 - Database Optimization: "Query performance improved 3x"
└─ 15:20 - Security Audit spawned from Backend API session
```

#### Key Components Needed:
- **Progress visualization** with real-time updates
- **Session hierarchy tree** showing parent/child relationships
- **Zed integration status** with direct links to open threads
- **Coordination timeline** showing inter-session communication
- **Spawn session controls** for creating new work sessions

### 2. Work Session Detail Pages

#### Individual Work Session View
```
[Work Session: Backend API Implementation]

📊 Session Overview
├─ SpecTask: Implement User Authentication
├─ Phase: Implementation │ Status: Active
├─ Implementation Task: Authentication API endpoints (Task 2/5)
├─ Started: 1 hour ago │ Progress: 70%
└─ Zed Thread: thread_backend_123 (Active)

🎯 Current Focus
Implementation of login, logout, and profile management endpoints.
Security measures include rate limiting and JWT token management.

📝 Recent Activity
┌─ 14:45 - Created user profile endpoint
├─ 14:40 - Added JWT middleware for authentication
├─ 14:35 - Implemented login endpoint with validation
└─ 14:30 - Set up Express.js routing structure

👥 Related Sessions
├─ 🔗 Database Performance Optimization (Child)
│  └─ Spawned to optimize authentication queries
├─ 🔗 Security Audit (Child)  
│  └─ Spawned to implement additional security measures
└─ ⏳ Frontend UI (Waiting)
   └─ Blocked until API endpoints are complete

🛠️ Actions
[📱 Spawn New Session] [✅ Mark Complete] [⏸️ Pause] [🔄 View in Zed]

💬 Coordination Log
├─ Send handoff to Frontend: "API ready for integration"
├─ Request help: "Need security review of JWT implementation"
└─ Broadcast update: "Authentication endpoints 70% complete"
```

#### Required Components:
- **Session status and progress tracking**
- **Activity timeline** with real-time updates  
- **Related session navigation** (parent/child links)
- **Action buttons** for session management
- **Coordination controls** for inter-session communication

### 3. Session Spawning Interface

#### Spawn Session Modal
```
🚀 Spawn New Work Session

Parent Session: Backend API Implementation
SpecTask: Implement User Authentication

┌─────────────────────────────────────────────────────────────┐
│ Session Name: [Database Performance Optimization          ] │
│                                                             │
│ Description:                                                │
│ ┌─────────────────────────────────────────────────────────┐ │
│ │ The authentication queries are slower than expected.    │ │
│ │ Need to optimize database indexes and query structure   │ │
│ │ for better performance under load.                      │ │
│ └─────────────────────────────────────────────────────────┘ │
│                                                             │
│ Agent Configuration:                                        │
│ ├─ Agent Type: ● Zed Agent ○ Helix Agent ○ Custom         │
│ ├─ Priority: ● Normal ○ High ○ Low                         │
│ └─ Environment: [+ Add Environment Variable]               │
│                                                             │
│ Estimated Effort: ● Small ○ Medium ○ Large                 │
│ Dependencies: [Select related sessions...]                  │
│                                                             │
│ [Cancel] [Create Session]                                   │
└─────────────────────────────────────────────────────────────┘
```

#### Required Features:
- **Session name and description input**
- **Agent type selection** (Zed vs Helix)
- **Configuration options** (environment, priority)
- **Dependency management** (link to other sessions)
- **Real-time preview** of session hierarchy

### 4. Real-Time Coordination Dashboard

#### Session Coordination Center
```
🔄 Session Coordination - Implement User Authentication

🎯 Active Coordination Events
┌─ 🔴 BLOCKING: Integration Tests → Frontend UI
│  "Need UI components before tests can proceed"
│  └─ [Acknowledge] [Respond] [Escalate]
├─ 🟡 REQUEST: Security Audit → Backend API  
│  "Need review of JWT token implementation"
│  └─ [Acknowledge] [Respond] [Schedule Review]
└─ 🟢 HANDOFF: Database Schema → Backend API
   "Schema migration complete, API can proceed"
   └─ ✅ Acknowledged by Backend API

📊 Session Activity Map
Backend API ──handoff──> Frontend UI ──waits for──> Integration Tests
    │                        │                           │
    ├─spawned─> DB Optimization ├─spawned─> UI Polish     │
    └─spawned─> Security Audit  └─spawned─> Performance Tests

🔔 Notifications
├─ Database Optimization completed successfully
├─ Security Audit requires human review
└─ Frontend UI ready to begin implementation

🎮 Quick Actions
[🚀 Spawn Emergency Session] [⏸️ Pause All] [📊 Generate Report]
```

#### Required Features:
- **Real-time event stream** with WebSocket updates
- **Visual session relationship mapping**
- **Coordination action buttons** (acknowledge, respond, escalate)
- **Notification center** for important events
- **Quick action controls** for emergency situations

### 5. Zed Integration Monitoring

#### Zed Instance Dashboard
```
🖥️ Zed Instance: zed_instance_auth_system_123

📊 Instance Health
├─ Status: Active │ Uptime: 2h 15m │ CPU: 45% │ Memory: 2.1GB
├─ Project: /workspace/auth-system │ Threads: 5 active
└─ Last Activity: 30 seconds ago

🧵 Active Threads
┌─ thread_backend_api (70% complete)
│  ├─ Work Session: Backend API Implementation
│  ├─ Files: 12 modified │ Lines: +234/-45
│  ├─ Tests: 8 passing │ Last activity: 30s ago
│  └─ [Open in Zed] [View Session] [Thread Logs]
├─ thread_db_optimization (30% complete)
│  ├─ Work Session: Database Performance Optimization
│  ├─ Files: 3 modified │ Lines: +67/-12
│  ├─ Tests: 3 passing │ Last activity: 2m ago
│  └─ [Open in Zed] [View Session] [Thread Logs]
└─ [...other threads...]

🔄 Recent Activity Stream
├─ 15:45 - thread_backend_api: Added user profile endpoint
├─ 15:43 - thread_security: Implemented rate limiting
├─ 15:41 - thread_db_optimization: Optimized user lookup query
└─ 15:40 - thread_backend_api: Fixed validation bug

🛠️ Instance Controls
[🔄 Restart Instance] [📥 Export Logs] [⏹️ Shutdown] [📊 Resource Report]
```

#### Required Features:
- **Real-time resource monitoring** (CPU, memory, disk)
- **Thread activity visualization** with file change tracking
- **Direct Zed integration** with deep links
- **Activity stream** with detailed change logs
- **Instance control buttons** for management

## 2. Automatic Session Creation from Zed Threads

**No, this is not implemented yet.** Currently the flow is:
1. Helix creates work sessions → Creates Zed threads
2. But Zed thread creation → Helix session creation is **missing**

### Required Implementation