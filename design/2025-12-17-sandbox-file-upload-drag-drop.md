# Sandbox Drag-and-Drop File Upload

**Date:** 2025-12-17
**Status:** Design

## Problem Statement

Users need to upload files (e.g., zip files of repositories, assets) directly into the sandbox work directory. Currently, there's no way to transfer files from the user's local machine into a running sandbox session.

## Requirements

1. Drag-and-drop files onto the sandbox viewer to upload them
2. Files are uploaded to `~/work/incoming/` (persistent across sessions)
3. Upload must not block user interaction with the session
4. Must work across all viewing modes (screenshot, WebSocket, SSE, low-quality)
5. Support large files (zip files of repositories can be 100MB+)
6. Show upload progress without obscuring the sandbox view
7. Show success toast notification on completion (matching "Paste successful" style)
8. Desktop-agnostic: no assumptions about file manager or desktop environment

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           User's Browser                                 │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │  ExternalAgentDesktopViewer / MoonlightStreamViewer             │    │
│  │  ┌─────────────────────────────────────────────────────────┐    │    │
│  │  │  Drop Zone Overlay (transparent, covers entire viewer)  │    │    │
│  │  │  - Shows visual feedback on drag-over                   │    │    │
│  │  │  - Triggers upload on drop                              │    │    │
│  │  └─────────────────────────────────────────────────────────┘    │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                              │                                           │
│                              │ POST /api/v1/external-agents/{id}/upload  │
│                              │ (multipart/form-data)                     │
│                              ▼                                           │
└─────────────────────────────────────────────────────────────────────────┘
                               │
                               ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                           API Server                                     │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │  POST /external-agents/{sessionID}/upload                       │    │
│  │  - Validates session ownership (RBAC)                           │    │
│  │  - Streams file via Rev Dial to sandbox                         │    │
│  │  - Returns upload status                                        │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                              │                                           │
│                              │ Rev Dial tunnel                           │
│                              ▼                                           │
└─────────────────────────────────────────────────────────────────────────┘
                               │
                               ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                     Sandbox Container                                    │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │  Screenshot Server (port 9876)                                  │    │
│  │  POST /upload                                                   │    │
│  │  - Receives multipart file                                      │    │
│  │  - Writes to /home/retro/work/incoming/ directory               │    │
│  │  - Returns success/failure                                      │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                                                          │
│  /home/retro/work/incoming/                                              │
│  └── uploaded-file.zip  ← File lands here (persistent)                  │
└─────────────────────────────────────────────────────────────────────────┘
```

## Implementation Details

### 1. Screenshot Server - Upload Endpoint

Add `POST /upload` endpoint to `api/cmd/screenshot-server/main.go`:

```go
// POST /upload
// Body: multipart/form-data with "file" field
// Response: JSON { "path": "/home/retro/work/incoming/filename", "size": 12345, "filename": "filename" }

func handleUpload(w http.ResponseWriter, r *http.Request) {
    if r.Method != "POST" {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }

    // Parse multipart form (max 500MB)
    if err := r.ParseMultipartForm(500 << 20); err != nil {
        http.Error(w, "Failed to parse form: "+err.Error(), http.StatusBadRequest)
        return
    }

    file, header, err := r.FormFile("file")
    if err != nil {
        http.Error(w, "Failed to get file: "+err.Error(), http.StatusBadRequest)
        return
    }
    defer file.Close()

    // Always upload to ~/work/incoming/
    destDir := "/home/retro/work/incoming"

    // Ensure directory exists
    if err := os.MkdirAll(destDir, 0755); err != nil {
        http.Error(w, "Failed to create directory: "+err.Error(), http.StatusInternalServerError)
        return
    }

    // Sanitize filename to prevent path traversal
    filename := filepath.Base(header.Filename)
    destPath := filepath.Join(destDir, filename)

    // Create destination file
    dst, err := os.Create(destPath)
    if err != nil {
        http.Error(w, "Failed to create file: "+err.Error(), http.StatusInternalServerError)
        return
    }
    defer dst.Close()

    // Copy file content
    written, err := io.Copy(dst, file)
    if err != nil {
        http.Error(w, "Failed to write file: "+err.Error(), http.StatusInternalServerError)
        return
    }

    // Return success with filename for toast display
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]interface{}{
        "path":     destPath,
        "size":     written,
        "filename": filename,
    })
}
```

### 2. API Server - Upload Handler

Add handler to `api/pkg/server/external_agent_handlers.go`:

```go
// @Summary Upload file to sandbox
// @Description Upload a file to the sandbox incoming folder (~/work/incoming/)
// @Tags ExternalAgents
// @Accept multipart/form-data
// @Param sessionID path string true "Session ID"
// @Param file formData file true "File to upload"
// @Success 200 {object} types.SandboxFileUploadResponse
// @Router /api/v1/external-agents/{sessionID}/upload [post]
// @Security ApiKeyAuth
func (apiServer *HelixAPIServer) uploadFileToSandbox(res http.ResponseWriter, req *http.Request) {
    vars := mux.Vars(req)
    sessionID := vars["sessionID"]

    // Get user and validate session ownership
    user := getRequestUser(req)
    session, err := apiServer.Store.GetExternalAgentSession(req.Context(), sessionID)
    if err != nil {
        http.Error(res, "Session not found", http.StatusNotFound)
        return
    }

    // RBAC check
    if err := apiServer.authorizeUserToExternalAgentSession(req.Context(), user, session); err != nil {
        http.Error(res, "Unauthorized", http.StatusForbidden)
        return
    }

    // Get rev dial connection to sandbox
    runnerID := fmt.Sprintf("sandbox-%s", sessionID)
    revDialConn, err := apiServer.connman.Dial(req.Context(), runnerID)
    if err != nil {
        http.Error(res, "Failed to connect to sandbox", http.StatusServiceUnavailable)
        return
    }
    defer revDialConn.Close()

    // Forward the multipart request to the sandbox screenshot server
    proxyReq, err := http.NewRequest("POST", "http://localhost:9876/upload", req.Body)
    if err != nil {
        http.Error(res, "Failed to create proxy request", http.StatusInternalServerError)
        return
    }
    proxyReq.Header = req.Header.Clone()

    // Write request to rev dial connection
    if err := proxyReq.Write(revDialConn); err != nil {
        http.Error(res, "Failed to send to sandbox", http.StatusInternalServerError)
        return
    }

    // Read response from sandbox
    proxyResp, err := http.ReadResponse(bufio.NewReader(revDialConn), proxyReq)
    if err != nil {
        http.Error(res, "Failed to read sandbox response", http.StatusInternalServerError)
        return
    }
    defer proxyResp.Body.Close()

    // Copy response headers and body to client
    for key, values := range proxyResp.Header {
        for _, value := range values {
            res.Header().Add(key, value)
        }
    }
    res.WriteHeader(proxyResp.StatusCode)
    io.Copy(res, proxyResp.Body)
}
```

### 3. Frontend - Drop Zone Component

Create `frontend/src/components/external-agent/SandboxDropZone.tsx`:

```typescript
import React, { useState, useCallback, useRef } from 'react'
import { Box, LinearProgress, Typography, Fade, Snackbar, Alert } from '@mui/material'
import { CloudUpload, Check } from 'lucide-react'

interface UploadState {
  progress: number  // 0-100 for progress, -1 for error, 101 for complete
  name: string
}

interface SandboxDropZoneProps {
  sessionId: string
  children: React.ReactNode
  disabled?: boolean
}

export const SandboxDropZone: React.FC<SandboxDropZoneProps> = ({
  sessionId,
  children,
  disabled = false,
}) => {
  const [isDragging, setIsDragging] = useState(false)
  const [uploads, setUploads] = useState<Map<string, UploadState>>(new Map())
  const [successToast, setSuccessToast] = useState<{ open: boolean; filename: string }>({
    open: false,
    filename: ''
  })
  const dragCounter = useRef(0)

  const handleDragEnter = useCallback((e: React.DragEvent) => {
    e.preventDefault()
    e.stopPropagation()
    if (disabled) return
    dragCounter.current++
    if (e.dataTransfer.items?.length > 0) {
      setIsDragging(true)
    }
  }, [disabled])

  const handleDragLeave = useCallback((e: React.DragEvent) => {
    e.preventDefault()
    e.stopPropagation()
    dragCounter.current--
    if (dragCounter.current === 0) {
      setIsDragging(false)
    }
  }, [])

  const handleDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault()
    e.stopPropagation()
  }, [])

  const uploadFile = async (file: File) => {
    const uploadId = `${file.name}-${Date.now()}`

    setUploads(prev => new Map(prev).set(uploadId, { progress: 0, name: file.name }))

    try {
      const formData = new FormData()
      formData.append('file', file)

      const xhr = new XMLHttpRequest()

      xhr.upload.addEventListener('progress', (event) => {
        if (event.lengthComputable) {
          const progress = Math.round((event.loaded / event.total) * 100)
          setUploads(prev => new Map(prev).set(uploadId, { progress, name: file.name }))
        }
      })

      await new Promise<void>((resolve, reject) => {
        xhr.onload = () => {
          if (xhr.status >= 200 && xhr.status < 300) {
            resolve()
          } else {
            reject(new Error(`Upload failed: ${xhr.statusText}`))
          }
        }
        xhr.onerror = () => reject(new Error('Upload failed'))

        xhr.open('POST', `/api/v1/external-agents/${sessionId}/upload`)
        xhr.send(formData)
      })

      // Mark as complete in progress indicator
      setUploads(prev => new Map(prev).set(uploadId, { progress: 101, name: file.name }))

      // Show success toast
      setSuccessToast({ open: true, filename: file.name })

      // Remove from uploads after brief delay
      setTimeout(() => {
        setUploads(prev => {
          const next = new Map(prev)
          next.delete(uploadId)
          return next
        })
      }, 2000)
    } catch (error) {
      console.error('Upload failed:', error)
      // Mark as error
      setUploads(prev => new Map(prev).set(uploadId, { progress: -1, name: file.name }))
      setTimeout(() => {
        setUploads(prev => {
          const next = new Map(prev)
          next.delete(uploadId)
          return next
        })
      }, 3000)
    }
  }

  const handleDrop = useCallback((e: React.DragEvent) => {
    e.preventDefault()
    e.stopPropagation()
    setIsDragging(false)
    dragCounter.current = 0

    if (disabled) return

    const files = Array.from(e.dataTransfer.files)
    files.forEach(file => uploadFile(file))
  }, [sessionId, disabled])

  const getProgressText = (progress: number): string => {
    if (progress === -1) return 'Upload failed'
    if (progress === 101) return 'Uploaded to ~/work/incoming'
    if (progress === 100) return 'Finishing...'
    return `Uploading to ~/work/incoming... ${progress}%`
  }

  return (
    <Box
      sx={{ position: 'relative', width: '100%', height: '100%' }}
      onDragEnter={handleDragEnter}
      onDragLeave={handleDragLeave}
      onDragOver={handleDragOver}
      onDrop={handleDrop}
    >
      {children}

      {/* Drag overlay */}
      <Fade in={isDragging}>
        <Box
          sx={{
            position: 'absolute',
            inset: 0,
            backgroundColor: 'rgba(25, 118, 210, 0.15)',
            border: '3px dashed',
            borderColor: 'primary.main',
            borderRadius: 2,
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            justifyContent: 'center',
            zIndex: 1000,
            pointerEvents: 'none',
          }}
        >
          <CloudUpload size={64} color="#1976d2" />
          <Typography variant="h6" color="primary" sx={{ mt: 2 }}>
            Drop files to upload to sandbox
          </Typography>
          <Typography variant="body2" color="text.secondary">
            Files will be saved to ~/work/incoming
          </Typography>
        </Box>
      </Fade>

      {/* Upload progress indicators */}
      {uploads.size > 0 && (
        <Box
          sx={{
            position: 'absolute',
            bottom: 16,
            right: 16,
            width: 320,
            zIndex: 1001,
            display: 'flex',
            flexDirection: 'column',
            gap: 1,
          }}
        >
          {Array.from(uploads.entries()).map(([id, { progress, name }]) => (
            <Box
              key={id}
              sx={{
                backgroundColor: 'background.paper',
                borderRadius: 1,
                p: 1.5,
                boxShadow: 3,
              }}
            >
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 0.5 }}>
                {progress === 101 && <Check size={16} color="#4caf50" />}
                <Typography variant="body2" noWrap sx={{ flex: 1 }}>
                  {name}
                </Typography>
              </Box>
              <LinearProgress
                variant={progress >= 0 && progress <= 100 ? 'determinate' : 'indeterminate'}
                value={progress >= 0 && progress <= 100 ? progress : 0}
                color={progress === -1 ? 'error' : progress === 101 ? 'success' : 'primary'}
                sx={{ mb: 0.5 }}
              />
              <Typography variant="caption" color="text.secondary">
                {getProgressText(progress)}
              </Typography>
            </Box>
          ))}
        </Box>
      )}

      {/* Success toast (matches "Paste successful" style) */}
      <Snackbar
        open={successToast.open}
        autoHideDuration={3000}
        onClose={() => setSuccessToast({ open: false, filename: '' })}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'center' }}
      >
        <Alert
          severity="success"
          variant="filled"
          onClose={() => setSuccessToast({ open: false, filename: '' })}
        >
          {successToast.filename} uploaded to ~/work/incoming
        </Alert>
      </Snackbar>
    </Box>
  )
}
```

### 4. Integration Points

#### Option A: Wrap at ExternalAgentDesktopViewer level

Wrap the entire viewer component, affecting all modes:

```typescript
// In ExternalAgentDesktopViewer.tsx
return (
  <SandboxDropZone sessionId={sessionId} disabled={!isConnected}>
    <Box sx={{ position: 'relative', width: '100%', height: '100%' }}>
      {/* existing viewer content */}
    </Box>
  </SandboxDropZone>
)
```

#### Option B: Wrap at MoonlightStreamViewer level

More precise control, only affects stream mode:

```typescript
// In MoonlightStreamViewer.tsx, wrap the main container
return (
  <SandboxDropZone sessionId={sessionId}>
    <div ref={containerRef} style={{ ... }}>
      {/* existing content */}
    </div>
  </SandboxDropZone>
)
```

**Recommendation:** Option A - wrap at `ExternalAgentDesktopViewer` level so it works in all modes including screenshot mode.

## UX Considerations

### Visual Feedback States

1. **Idle**: No visual indication (drop zone is invisible)
2. **Drag-over**: Blue tinted overlay with dashed border and upload icon, text says "Files will be saved to ~/work/incoming"
3. **Uploading**: Small progress cards in bottom-right corner showing "Uploading to ~/work/incoming... X%"
4. **Success**:
   - Progress card shows green checkmark and "Uploaded to ~/work/incoming"
   - Toast notification appears: "{filename} uploaded to ~/work/incoming" (matches "Paste successful" style)
   - Progress card fades after 2 seconds
5. **Error**: Progress card turns red, shows "Upload failed", fades after 3 seconds

### Non-blocking Design

- Progress indicators are small and positioned in the corner
- User can continue interacting with the sandbox during upload
- Multiple files can be uploaded simultaneously
- Overlay only appears during drag, not during upload
- Toast notifications stack and auto-dismiss

### Large File Handling

- Use XHR with progress events for accurate progress tracking
- No file size limit in frontend (server-side limit of 500MB)
- Streaming upload to avoid memory issues

## Security Considerations

1. **Authentication**: Upload endpoint requires valid session token
2. **Authorization**: RBAC check ensures user owns the session
3. **Path traversal**: `filepath.Base()` extracts only the filename, preventing `../` attacks
4. **Fixed destination**: Files always go to `/home/retro/work/incoming/`, no user-controllable paths
5. **File size limit**: 500MB max to prevent DoS
6. **Rev dial isolation**: Files can only be written to the user's sandbox

## API Types

Add to `api/pkg/types/`:

```go
// SandboxFileUploadResponse is returned when a file is uploaded to a sandbox
type SandboxFileUploadResponse struct {
    Path     string `json:"path"`     // Full path: /home/retro/work/incoming/filename
    Size     int64  `json:"size"`     // File size in bytes
    Filename string `json:"filename"` // Original filename
}
```

## Implementation Plan

1. Add `POST /upload` endpoint to screenshot-server
2. Add API handler with rev dial forwarding
3. Add Swagger annotations and regenerate OpenAPI client
4. Create `SandboxDropZone` component
5. Integrate into `ExternalAgentDesktopViewer`
6. Test across all viewer modes

## Future Enhancements

- Folder upload support (using webkitdirectory)
- Download files from sandbox (reverse direction)
- Drag files out of sandbox file browser
- "Show in Files" button that opens the desktop's file manager (desktop-specific)
