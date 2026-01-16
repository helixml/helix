# Touch Trackpad Mode and Voice Input

**Date:** 2026-01-16
**Status:** Draft
**Author:** Claude Code

## Overview

Two features to improve mobile and accessibility for the remote desktop:

1. **Trackpad Mode** - Touch input mode where finger movement controls cursor relatively (like a laptop trackpad), with two-finger scroll
2. **Voice Input** - Hold-to-record button that transcribes speech via local Whisper model and types text at cursor location

---

## Feature 1: Trackpad Mode

### Problem

Direct touch on a phone screen is imprecise for desktop UIs:
- Small click targets (IDE buttons, menus, text)
- Finger obscures the target
- Desktop UIs designed for mouse precision, not fat fingers

### Solution

Implement trackpad mode where:
- **One finger drag** = relative cursor movement (cursor moves by drag delta)
- **Tap** = click at current cursor position
- **Two finger drag** = scroll

### User Flow

1. User taps trackpad mode toggle in toolbar (switches from direct touch)
2. A visual cursor overlay appears on the video stream
3. User drags finger → cursor moves relatively
4. User taps → click happens at cursor position
5. User drags with two fingers → scroll at cursor position

### Implementation

#### Frontend Changes (`DesktopStreamViewer.tsx`)

```typescript
// State
const [touchMode, setTouchMode] = useState<'direct' | 'trackpad'>('direct');
const [virtualCursor, setVirtualCursor] = useState({ x: 0, y: 0, visible: false });
const lastTouchRef = useRef<{ x: number; y: number } | null>(null);
const touchStartTimeRef = useRef<number>(0);
const touchMovedRef = useRef<boolean>(false);

// Gesture detection constants
const TAP_MAX_DURATION_MS = 200;
const TAP_MAX_MOVEMENT_PX = 10;
const CURSOR_SENSITIVITY = 1.5; // Multiplier for cursor movement

// Touch handlers (trackpad mode)
const handleTrackpadTouchStart = (e: TouchEvent) => {
  e.preventDefault();
  const touch = e.touches[0];
  lastTouchRef.current = { x: touch.clientX, y: touch.clientY };
  touchStartTimeRef.current = Date.now();
  touchMovedRef.current = false;

  // Show cursor on first touch if hidden
  if (!virtualCursor.visible) {
    // Initialize cursor at center of video
    setVirtualCursor({ x: videoWidth / 2, y: videoHeight / 2, visible: true });
  }
};

const handleTrackpadTouchMove = (e: TouchEvent) => {
  e.preventDefault();
  if (!lastTouchRef.current) return;

  if (e.touches.length === 1) {
    // One finger: relative mouse movement
    const touch = e.touches[0];
    const dx = (touch.clientX - lastTouchRef.current.x) * CURSOR_SENSITIVITY;
    const dy = (touch.clientY - lastTouchRef.current.y) * CURSOR_SENSITIVITY;

    // Update virtual cursor position (clamped to video bounds)
    setVirtualCursor(prev => ({
      x: Math.max(0, Math.min(videoWidth, prev.x + dx)),
      y: Math.max(0, Math.min(videoHeight, prev.y + dy)),
      visible: true,
    }));

    // Send mouse move to server
    sendMouseMove(virtualCursor.x + dx, virtualCursor.y + dy);

    lastTouchRef.current = { x: touch.clientX, y: touch.clientY };
    touchMovedRef.current = true;

  } else if (e.touches.length === 2) {
    // Two fingers: scroll
    const avgY = (e.touches[0].clientY + e.touches[1].clientY) / 2;
    if (lastTwoFingerY !== null) {
      const deltaY = avgY - lastTwoFingerY;
      sendScroll(virtualCursor.x, virtualCursor.y, deltaY > 0 ? -1 : 1);
    }
    lastTwoFingerY = avgY;
  }
};

const handleTrackpadTouchEnd = (e: TouchEvent) => {
  const duration = Date.now() - touchStartTimeRef.current;

  if (duration < TAP_MAX_DURATION_MS && !touchMovedRef.current) {
    // Tap: click at virtual cursor position
    sendClick(virtualCursor.x, virtualCursor.y, 'left');
  }

  lastTouchRef.current = null;
};
```

#### Virtual Cursor Overlay

Render a cursor indicator on top of the video:

```tsx
{touchMode === 'trackpad' && virtualCursor.visible && (
  <div
    className="absolute pointer-events-none z-50"
    style={{
      left: virtualCursor.x,
      top: virtualCursor.y,
      transform: 'translate(-50%, -50%)',
    }}
  >
    {/* Simple cursor indicator */}
    <div className="w-4 h-4 border-2 border-white rounded-full shadow-lg" />
    <div className="w-1 h-1 bg-white rounded-full absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2" />
  </div>
)}
```

#### Toolbar Toggle

Add toggle button to existing toolbar:

```tsx
<Button
  variant="ghost"
  size="icon"
  onClick={() => setTouchMode(m => m === 'direct' ? 'trackpad' : 'direct')}
  title={touchMode === 'direct' ? 'Switch to trackpad mode' : 'Switch to direct touch'}
>
  {touchMode === 'direct' ? <Hand /> : <Pointer />}
</Button>
```

#### Persistence

Save preference to localStorage:

```typescript
useEffect(() => {
  const saved = localStorage.getItem('touchMode');
  if (saved === 'trackpad' || saved === 'direct') {
    setTouchMode(saved);
  }
}, []);

useEffect(() => {
  localStorage.setItem('touchMode', touchMode);
}, [touchMode]);
```

### No Backend Changes Required

The backend already receives absolute mouse coordinates. Trackpad mode just changes how the frontend calculates those coordinates - relatively instead of directly from touch position.

---

## Feature 2: Voice Input with Local Whisper

### Problem

Typing on mobile is slow and frustrating. Voice input would be much faster, especially for longer text.

### Solution

Hold-to-record button that:
1. Captures audio from browser microphone
2. Sends audio to desktop-bridge
3. Runs through local Whisper model (GPU-accelerated)
4. Types or pastes transcribed text at cursor position

### Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                           Browser                                    │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │                  DesktopStreamViewer                         │    │
│  │   ┌──────────────┐                                           │    │
│  │   │ 🎤 Voice     │  ← Hold to record                         │    │
│  │   │    Button    │                                           │    │
│  │   └──────────────┘                                           │    │
│  │         │                                                    │    │
│  │         │ MediaRecorder API                                  │    │
│  │         ▼                                                    │    │
│  │   Audio chunks (WebM/Opus)                                   │    │
│  └─────────────────────────────────────────────────────────────┘    │
│              │                                                       │
│              │ WebSocket: /ws/voice (new endpoint)                  │
│              │ or HTTP POST: /api/v1/external-agents/{id}/voice     │
└──────────────┼───────────────────────────────────────────────────────┘
               │
               ▼
┌──────────────────────────────────────────────────────────────────────┐
│                           API Server                                  │
│                               │                                       │
│                               │ Forward via RevDial                   │
│                               ▼                                       │
└──────────────────────────────────────────────────────────────────────┘
               │
               ▼
┌──────────────────────────────────────────────────────────────────────┐
│                        Desktop Container                              │
│  ┌────────────────────────────────────────────────────────────────┐  │
│  │                      desktop-bridge                             │  │
│  │                           │                                     │  │
│  │         ┌─────────────────┼─────────────────┐                   │  │
│  │         ▼                 ▼                 ▼                   │  │
│  │  ┌────────────┐   ┌─────────────┐   ┌────────────┐             │  │
│  │  │ Save audio │   │ Run Whisper │   │ Type text  │             │  │
│  │  │ to temp    │──►│ (GPU)       │──►│ via input  │             │  │
│  │  │ file       │   │             │   │ protocol   │             │  │
│  │  └────────────┘   └─────────────┘   └────────────┘             │  │
│  │                                            │                    │  │
│  │                                            ▼                    │  │
│  │                                     wtype (Wayland)              │  │
│  │                                     or clipboard paste           │  │
│  └────────────────────────────────────────────────────────────────┘  │
│                                                                       │
│  ┌────────────────────────────────────────────────────────────────┐  │
│  │  Whisper Model (whisper.cpp or faster-whisper)                  │  │
│  │  - Small/Medium model for speed                                 │  │
│  │  - CUDA acceleration                                            │  │
│  │  - ~1-2 second transcription for 10s audio                      │  │
│  └────────────────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────────────┘
```

### User Flow

1. User taps voice button (floating, top-right of video)
2. Button changes to "recording" state (red, pulsing)
3. User speaks while holding button
4. User releases button
5. Audio is sent to desktop-bridge
6. Whisper transcribes (show loading indicator)
7. Text is typed at cursor position
8. Button returns to idle state

### Implementation

#### Phase 1: Basic Voice Input

##### Frontend (`DesktopStreamViewer.tsx`)

```typescript
// State
const [voiceEnabled, setVoiceEnabled] = useState(true); // Toggleable in menu
const [isRecording, setIsRecording] = useState(false);
const [isTranscribing, setIsTranscribing] = useState(false);
const mediaRecorderRef = useRef<MediaRecorder | null>(null);
const audioChunksRef = useRef<Blob[]>([]);

// Start recording
const startRecording = async () => {
  try {
    const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
    const mediaRecorder = new MediaRecorder(stream, {
      mimeType: 'audio/webm;codecs=opus',
    });

    audioChunksRef.current = [];

    mediaRecorder.ondataavailable = (e) => {
      if (e.data.size > 0) {
        audioChunksRef.current.push(e.data);
      }
    };

    mediaRecorder.onstop = async () => {
      const audioBlob = new Blob(audioChunksRef.current, { type: 'audio/webm' });
      stream.getTracks().forEach(track => track.stop());
      await sendAudioForTranscription(audioBlob);
    };

    mediaRecorderRef.current = mediaRecorder;
    mediaRecorder.start(100); // Collect data every 100ms
    setIsRecording(true);
  } catch (err) {
    console.error('Failed to start recording:', err);
  }
};

// Stop recording
const stopRecording = () => {
  if (mediaRecorderRef.current && isRecording) {
    mediaRecorderRef.current.stop();
    setIsRecording(false);
  }
};

// Send audio to backend
const sendAudioForTranscription = async (audioBlob: Blob) => {
  setIsTranscribing(true);
  try {
    const formData = new FormData();
    formData.append('audio', audioBlob, 'recording.webm');

    const response = await fetch(
      `/api/v1/external-agents/${sessionId}/voice`,
      {
        method: 'POST',
        headers: { 'Authorization': `Bearer ${apiKey}` },
        body: formData,
      }
    );

    if (!response.ok) {
      throw new Error(`Voice transcription failed: ${response.status}`);
    }

    // Text is typed on the desktop side, nothing to do here
    const result = await response.json();
    console.log('Transcription complete:', result.text);

  } catch (err) {
    console.error('Transcription error:', err);
  } finally {
    setIsTranscribing(false);
  }
};
```

##### Voice Button UI

```tsx
{voiceEnabled && (
  <div className="absolute top-4 right-4 z-50">
    <button
      onMouseDown={startRecording}
      onMouseUp={stopRecording}
      onMouseLeave={stopRecording}
      onTouchStart={startRecording}
      onTouchEnd={stopRecording}
      className={cn(
        "w-14 h-14 rounded-full shadow-lg flex items-center justify-center transition-all",
        isRecording
          ? "bg-red-500 animate-pulse scale-110"
          : isTranscribing
          ? "bg-yellow-500"
          : "bg-gray-800/80 hover:bg-gray-700/80"
      )}
      disabled={isTranscribing}
    >
      {isTranscribing ? (
        <Loader2 className="w-6 h-6 text-white animate-spin" />
      ) : (
        <Mic className={cn("w-6 h-6", isRecording ? "text-white" : "text-gray-300")} />
      )}
    </button>
  </div>
)}
```

##### API Server (`external_agent_handlers.go`)

New endpoint to receive audio and forward to desktop-bridge:

```go
// POST /api/v1/external-agents/{sessionID}/voice
func (s *HelixAPIServer) handleVoiceInput(w http.ResponseWriter, r *http.Request) {
    sessionID := chi.URLParam(r, "sessionID")

    // Parse multipart form
    if err := r.ParseMultipartForm(10 << 20); err != nil { // 10MB max
        http.Error(w, "Failed to parse form", http.StatusBadRequest)
        return
    }

    file, _, err := r.FormFile("audio")
    if err != nil {
        http.Error(w, "No audio file", http.StatusBadRequest)
        return
    }
    defer file.Close()

    // Read audio data
    audioData, err := io.ReadAll(file)
    if err != nil {
        http.Error(w, "Failed to read audio", http.StatusInternalServerError)
        return
    }

    // Get runner connection
    runnerID := getRunnerIDForSession(sessionID)

    // Forward to desktop-bridge via HTTP
    conn, err := s.connman.Dial(r.Context(), runnerID)
    if err != nil {
        http.Error(w, "Desktop not connected", http.StatusServiceUnavailable)
        return
    }
    defer conn.Close()

    // Send HTTP request to desktop-bridge /voice endpoint
    req := fmt.Sprintf("POST /voice HTTP/1.1\r\nHost: localhost\r\nContent-Length: %d\r\nContent-Type: audio/webm\r\n\r\n", len(audioData))
    conn.Write([]byte(req))
    conn.Write(audioData)

    // Read response
    resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
    if err != nil {
        http.Error(w, "Transcription failed", http.StatusInternalServerError)
        return
    }
    defer resp.Body.Close()

    // Forward response to client
    body, _ := io.ReadAll(resp.Body)
    w.Header().Set("Content-Type", "application/json")
    w.Write(body)
}
```

##### Desktop Bridge (`desktop-bridge`)

New handler for voice transcription:

```go
// POST /voice
func (s *Server) handleVoice(w http.ResponseWriter, r *http.Request) {
    // Save audio to temp file
    audioData, err := io.ReadAll(r.Body)
    if err != nil {
        http.Error(w, "Failed to read audio", http.StatusBadRequest)
        return
    }

    tmpFile, err := os.CreateTemp("", "voice-*.webm")
    if err != nil {
        http.Error(w, "Failed to create temp file", http.StatusInternalServerError)
        return
    }
    defer os.Remove(tmpFile.Name())

    if _, err := tmpFile.Write(audioData); err != nil {
        http.Error(w, "Failed to write audio", http.StatusInternalServerError)
        return
    }
    tmpFile.Close()

    // Convert WebM to WAV (Whisper prefers WAV)
    wavFile := tmpFile.Name() + ".wav"
    defer os.Remove(wavFile)

    cmd := exec.Command("ffmpeg", "-i", tmpFile.Name(), "-ar", "16000", "-ac", "1", wavFile)
    if err := cmd.Run(); err != nil {
        http.Error(w, "Failed to convert audio", http.StatusInternalServerError)
        return
    }

    // Run Whisper
    text, err := s.transcribeWithWhisper(wavFile)
    if err != nil {
        http.Error(w, "Transcription failed", http.StatusInternalServerError)
        return
    }

    // Type the text at cursor position
    if err := s.typeText(text); err != nil {
        log.Printf("Failed to type text: %v, falling back to clipboard", err)
        // Fallback: paste via clipboard
        if err := s.pasteText(text); err != nil {
            http.Error(w, "Failed to input text", http.StatusInternalServerError)
            return
        }
    }

    // Return success
    json.NewEncoder(w).Encode(map[string]string{
        "text": text,
        "status": "success",
    })
}

func (s *Server) transcribeWithWhisper(wavFile string) (string, error) {
    // Using whisper.cpp CLI or faster-whisper
    // whisper.cpp is simpler to deploy, faster-whisper is more accurate

    cmd := exec.Command("whisper-cpp",
        "--model", "/models/whisper-small.bin",
        "--file", wavFile,
        "--output-txt",
        "--no-timestamps",
    )

    output, err := cmd.Output()
    if err != nil {
        return "", fmt.Errorf("whisper failed: %w", err)
    }

    return strings.TrimSpace(string(output)), nil
}

func (s *Server) typeText(text string) error {
    // Use wtype for Wayland text input
    return exec.Command("wtype", text).Run()
}

func (s *Server) pasteText(text string) error {
    // Set clipboard via wl-copy
    cmd := exec.Command("wl-copy", text)
    if err := cmd.Run(); err != nil {
        return err
    }

    // Simulate Ctrl+V via wtype
    return exec.Command("wtype", "-M", "ctrl", "v", "-m", "ctrl").Run()
}
```

##### Desktop Image Changes

Add Whisper to the desktop image:

```dockerfile
# In Dockerfile.ubuntu or Dockerfile.sway

# Install whisper.cpp with CUDA support
RUN git clone https://github.com/ggerganov/whisper.cpp.git /opt/whisper.cpp && \
    cd /opt/whisper.cpp && \
    make clean && \
    WHISPER_CUDA=1 make -j$(nproc) && \
    cp main /usr/local/bin/whisper-cpp

# Download small model (good balance of speed/accuracy)
RUN cd /opt/whisper.cpp/models && \
    ./download-ggml-model.sh small

# Or use faster-whisper (Python, more accurate)
RUN pip install faster-whisper && \
    python -c "from faster_whisper import WhisperModel; WhisperModel('small', device='cuda')"
```

#### Phase 2: LLM Post-Processing (Roadmap)

After basic transcription works, add optional LLM post-processing:

```go
func (s *Server) transcribeWithLLM(wavFile string) (string, error) {
    // Get raw transcription
    rawText, err := s.transcribeWithWhisper(wavFile)
    if err != nil {
        return "", err
    }

    // Post-process with LLM for accuracy
    prompt := fmt.Sprintf(`Fix any transcription errors in this text.
Only fix obvious errors, do not change meaning or style.
Return only the corrected text, nothing else.

Text: %s`, rawText)

    resp, err := s.llmClient.Complete(prompt)
    if err != nil {
        // Fall back to raw transcription
        return rawText, nil
    }

    return resp.Text, nil
}
```

### Configuration

Add settings for voice input:

```typescript
interface VoiceSettings {
  enabled: boolean;
  showButton: boolean;
  position: 'top-right' | 'top-left' | 'bottom-right' | 'bottom-left';
  useLLMPostProcessing: boolean;  // Phase 2
}
```

### Security Considerations

1. **Audio data** - Audio is processed on the desktop container, not sent to external services
2. **Microphone permission** - Browser prompts for permission, user must explicitly allow
3. **Audio retention** - Temp files are deleted immediately after transcription
4. **Rate limiting** - Consider limiting voice requests to prevent abuse

### Performance Considerations

1. **Whisper model size**:
   - `tiny`: ~1s for 10s audio, lower accuracy
   - `small`: ~2s for 10s audio, good accuracy (recommended)
   - `medium`: ~4s for 10s audio, better accuracy

2. **GPU memory**: Small model needs ~1GB VRAM

3. **Concurrent requests**: Process one at a time per session to avoid GPU contention

---

## Implementation Plan

### Phase 1: Trackpad Mode (1-2 days)

1. Add touch mode state and toggle to `DesktopStreamViewer.tsx`
2. Implement trackpad touch handlers (one-finger move, two-finger scroll, tap)
3. Add virtual cursor overlay
4. Add toolbar toggle button
5. Persist preference to localStorage
6. Test on mobile devices

### Phase 2: Basic Voice Input (2-3 days)

1. Add voice button UI to `DesktopStreamViewer.tsx`
2. Implement MediaRecorder capture
3. Add `/voice` endpoint to API server
4. Add `/voice` handler to desktop-bridge
5. Integrate whisper.cpp in desktop image
6. Implement text typing via wtype/xdotool
7. Add clipboard fallback
8. Test end-to-end

### Phase 3: Polish & LLM (Future)

1. Add LLM post-processing option
2. Add visual feedback for transcription progress
3. Add error handling and retry
4. Add voice activity detection (auto-stop on silence)
5. Consider streaming transcription for long recordings

---

## Open Questions

1. **Whisper model**: Use whisper.cpp (simpler, C++) or faster-whisper (Python, more accurate)?

2. **Audio format**: WebM/Opus from browser → WAV for Whisper. Need ffmpeg in container.

3. **Typing vs Pasting**:
   - Typing with wtype/xdotool is more natural but slower
   - Pasting is instant but overwrites clipboard
   - Could offer both as options

4. **Mobile keyboard conflict**: Voice button should not interfere with mobile keyboard when it's open

5. **Latency target**: What's acceptable? ~2-3 seconds for 10s of speech seems reasonable.

---

## Files to Modify/Create

### Trackpad Mode
- `frontend/src/components/external-agent/DesktopStreamViewer.tsx` - Touch handlers, cursor overlay, toggle

### Voice Input
- `frontend/src/components/external-agent/DesktopStreamViewer.tsx` - Voice button, MediaRecorder
- `api/pkg/server/external_agent_handlers.go` - Voice endpoint
- `api/cmd/desktop-bridge/main.go` - Voice handler
- `desktop/Dockerfile.ubuntu` - Whisper installation
- `desktop/Dockerfile.sway` - Whisper installation
