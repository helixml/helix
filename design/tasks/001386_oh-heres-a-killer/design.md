# Design: Agent Screencast Recording

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────┐
│                        Desktop Container                             │
│  ┌──────────────┐    ┌─────────────────┐    ┌───────────────────┐  │
│  │   PipeWire   │───▶│ SharedVideoSource│───▶│ Live WebSocket    │  │
│  │  ScreenCast  │    │  (existing)      │    │ Clients (browser) │  │
│  └──────────────┘    └────────┬─────────┘    └───────────────────┘  │
│                               │                                      │
│                               │ frames                               │
│                               ▼                                      │
│                      ┌─────────────────┐                            │
│                      │ RecordingManager│ (NEW)                      │
│                      │  - MP4 muxer    │                            │
│                      │  - Subtitle buf │                            │
│                      └────────┬────────┘                            │
│                               │                                      │
│                               ▼                                      │
│                      ┌─────────────────┐                            │
│                      │  /tmp (staging) │                            │
│                      └────────┬────────┘                            │
│                               │ upload on stop                       │
│                               ▼                                      │
│                      ┌─────────────────┐                            │
│                      │    Filestore    │                            │
│                      │  (linkable URL) │                            │
│                      └─────────────────┘                            │
│                                                                      │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │                     MCP Server (desktop-bridge)               │  │
│  │  • start_recording(title?) → recording_id                     │  │
│  │  • stop_recording() → file_path                               │  │
│  │  • add_subtitle(text, duration_ms?)                           │  │
│  └──────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────┘
```

## Key Design Decisions

### 1. Tap Into SharedVideoSource (Not Create New Pipeline)

The existing `SharedVideoSource` already decodes H.264 frames from PipeWire and broadcasts them. The `RecordingManager` subscribes as another client—same as browser WebSocket clients do—so recording doesn't create additional GPU encoder load.

**Rationale**: One pipeline, multiple consumers. Avoids NVENC session limits and simplifies state management.

### 2. Use GStreamer MP4 Muxer

Rather than raw H.264 files, output proper MP4 containers using GStreamer's `mp4mux` element. The frames from SharedVideoSource are already H.264 NAL units, so we just need to mux them.

**Pipeline addition** (when recording):
```
sharedvideosource → queue → h264parse → mp4mux → filesink
```

### 3. Subtitles as WebVTT Sidecar + Burned-In Option

Two approaches for subtitles:
1. **Default**: Generate `.vtt` sidecar file alongside MP4 (lightweight, preserves original video)
2. **Optional**: Burn subtitles into video using `textoverlay` element (single-file output, always visible)

The agent has two options:
- `add_subtitle(text, start_ms, end_ms)` - add individual entries incrementally during recording
- `set_subtitles([{text, start_ms, end_ms}, ...])` - provide complete subtitle track at once (e.g., after recording, before stop)

Both approaches buffer subtitles until `stop_recording` writes the VTT file. The `set_subtitles` method replaces any existing entries, allowing the agent to review the recording duration and craft precise narration.

### 4. Filestore Storage with Linkable URLs

Recordings are staged locally during capture (`/tmp/helix-recordings/`), then uploaded to the filestore on `stop_recording`. The filestore provides:

- **Persistent storage** - survives container restarts
- **Linkable URLs** - can be embedded in design docs, markdown, etc. without pushing large files to git
- **API access** - `GET /api/v1/filestore/<path>` for programmatic access

The `stop_recording` response includes the filestore URL that agents can use in documentation:
```markdown
## Demo Video
Watch the feature demo: [recording.mp4](https://helix.example.com/api/v1/filestore/recordings/ses_01abc/rec_xyz/demo.mp4)
```

## Component Details

### RecordingManager (`api/pkg/desktop/recording.go`)

```go
type RecordingManager struct {
    sessionID    string
    recordings   map[string]*Recording  // active recordings by ID
    mu           sync.Mutex
}

type Recording struct {
    ID          string
    Title       string
    StartTime   time.Time
    Subtitles   []Subtitle  // buffered until stop
    outputPath  string
    pipeline    *GstPipeline  // mp4mux pipeline
}

type Subtitle struct {
    Text    string
    StartMs int64  // milliseconds from recording start
    EndMs   int64  // milliseconds from recording start
}
```

### MCP Tool Handlers

Added to existing `mcp_server.go`:

| Tool | Handler | Notes |
|------|---------|-------|
| `start_recording` | `handleStartRecording` | Creates Recording, subscribes to SharedVideoSource |
| `stop_recording` | `handleStopRecording` | Finalizes MP4, writes VTT, uploads to filestore |
| `add_subtitle` | `handleAddSubtitle` | Appends single subtitle with start_ms/end_ms |
| `set_subtitles` | `handleSetSubtitles` | Replaces entire subtitle track with array of entries |

### File Output

**During recording** (local staging):
```
/tmp/helix-recordings/ses_01abc123/rec_xyz789/
├── demo.mp4        # H.264 video (in progress)
└── demo.vtt        # WebVTT subtitles (written on stop)
```

**After stop_recording** (filestore - permanent):
```
/api/v1/filestore/recordings/<session_id>/<recording_id>/
├── demo.mp4        # Linkable video URL
└── demo.vtt        # Linkable subtitles URL
```

**stop_recording response**:
```json
{
  "recording_id": "rec_xyz789",
  "duration_ms": 45000,
  "video_url": "/api/v1/filestore/recordings/ses_01abc/rec_xyz789/demo.mp4",
  "subtitles_url": "/api/v1/filestore/recordings/ses_01abc/rec_xyz789/demo.vtt"
}
```

Agents can embed these URLs directly in design docs or other markdown outputs.

## Edge Cases

1. **Recording already active**: `start_recording` returns error if recording in progress (one recording per session)
2. **Stop without start**: `stop_recording` returns error with clear message
3. **Session ends during recording**: Desktop-bridge cleanup calls `stop_recording` automatically
4. **Disk full**: GStreamer pipeline error propagates; return actionable error to agent

## Future Extensions

- Audio recording (when audio streaming is added)
- Multiple concurrent recordings (change map to support)
- Recording presets (resolution, bitrate)
- Automatic chapter markers from subtitles