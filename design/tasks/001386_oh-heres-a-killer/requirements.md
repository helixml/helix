# Requirements: Agent Screencast Recording

## Overview

Enable AI agents to record screencasts of their work, creating video artifacts that demonstrate features they built or tasks they completed. This leverages the existing video streaming infrastructure.

## User Stories

### US1: Agent Records Demo Video
**As an** AI agent  
**I want to** record a screencast of my work  
**So that** users can see a visual demonstration of what I built

### US2: Agent Adds Narration
**As an** AI agent  
**I want to** add subtitle/caption text during recording  
**So that** viewers understand what's happening at each moment

### US3: User Views Recording
**As a** user  
**I want to** watch the agent's screencast  
**So that** I can see the visual results without live monitoring

## Acceptance Criteria

### Recording Controls (MCP Tools)
- [ ] `start_recording` - begins capturing the video stream
  - Optional `title` parameter for naming the recording
  - Returns recording ID on success
- [ ] `stop_recording` - ends capture and finalizes the video file
  - Returns file path/URL of the completed recording
- [ ] `add_subtitle` - adds a subtitle entry to the recording
  - Required `text` parameter (the caption to display)
  - Required `start_ms` parameter (when subtitle appears, relative to recording start)
  - Required `end_ms` parameter (when subtitle disappears)
- [ ] `set_subtitles` - sets the complete subtitle track (replaces any existing)
  - Required `subtitles` parameter: array of `{text, start_ms, end_ms}` objects
  - Allows agent to specify entire narration with precise timestamps

### Video Output
- [ ] Recordings saved as MP4 (H.264 video, AAC audio if present)
- [ ] Subtitles embedded as WebVTT or burned into video
- [ ] Recording stored in session's filestore (accessible via API)
- [ ] Metadata includes: title, duration, start/end timestamps, session ID

### Integration
- [ ] MCP tools available in desktop-bridge MCP server
- [ ] Works with existing PipeWire/GStreamer streaming pipeline
- [ ] Does not disrupt live video streaming to browser clients