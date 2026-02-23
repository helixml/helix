# Requirements: Image Attachments for SpecTask Creation

## Overview

Users need to attach images (e.g., screenshots of UI issues) when creating a new SpecTask, so the AI agent can see visual context for the problem.

## User Stories

### US-1: Attach Images During Task Creation
**As a** user creating a new SpecTask  
**I want to** attach one or more images to my task description  
**So that** the AI agent can see visual examples of what I'm describing

### US-2: Agent Access to Attached Images
**As an** AI agent working on a task  
**I want to** access the attached images in the sandbox filesystem  
**So that** I can view them and understand the visual context

## Acceptance Criteria

### AC-1: Image Upload in NewSpecTaskForm
- [ ] User can drag & drop images onto the task creation form
- [ ] User can click to browse and select images
- [ ] Supported formats: PNG, JPEG, GIF, WebP
- [ ] Show thumbnail previews of attached images
- [ ] Allow removing attached images before submission

### AC-2: Image Storage and Transfer
- [ ] Images are uploaded to Helix filestore on attachment
- [ ] Image references are stored with the SpecTask record
- [ ] Images are copied into sandbox at `/home/retro/work/attachments/` when session starts

### AC-3: Agent Notification
- [ ] Agent receives instruction about attached images in its initial prompt
- [ ] Instruction includes file paths where images can be found
- [ ] Agent can read images using standard file tools (Zed can display images)

## Out of Scope
- Video attachments
- Real-time image annotation
- Image editing within the form