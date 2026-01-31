# 🤖 HyprMoon Neural TTS Server

A standalone Go HTTP server for text-to-speech using Coqui TTS neural engine. Perfect for high-quality GLaDOS-style robot voice announcements in HyprMoon streaming setup.

## ⚡ Quick Start

```bash
# Install dependencies
pip install TTS
go mod tidy

# Run server
go run main.go

# Test the neural British GLaDOS voice
curl -X POST http://localhost:8080/speak \
  -H "Content-Type: application/json" \
  -d '{"text": "Hades has connected to the test chamber"}'
```

## 🎯 Features

- **Neural TTS quality** using Coqui TTS engine
- **GLaDOS-style British voice** (p225 speaker)
- **Multiple neural voices** from VCTK dataset
- **HTTP REST API** for easy integration
- **Web interface** at http://localhost:8080
- **CORS enabled** for browser access
- **British male voices**: p225 (GLaDOS-like), p226, p227, p232 (deep)

## 🎤 API Endpoints

### POST /speak
Synthesize and play text-to-speech:

```bash
# Basic usage (uses SLT voice at 0.8 speed)
curl -X POST http://localhost:8080/speak \
  -H "Content-Type: application/json" \
  -d '{"text": "WebRTC client connected"}'

# Custom British neural voice
curl -X POST http://localhost:8080/speak \
  -H "Content-Type: application/json" \
  -d '{
    "text": "System alert: High CPU usage detected",
    "voice": "p232",
    "speed": 1.0,
    "volume": 1.0
  }'
```

### GET /voices
List available voices:

```bash
curl http://localhost:8080/voices
```

### GET /status
Server status and configuration:

```bash
curl http://localhost:8080/status
```

## 🔧 Integration with HyprMoon

The TTS server outputs audio to the default system audio device, which will automatically route to:

- **WebRTC streams** (browser clients hear robot voice)
- **Moonlight streams** (gaming clients hear robot voice)  
- **Local speakers** (system audio output)

No additional audio routing needed - it just works! 🎯

## 🎭 Voice Options

| Voice | Gender | Accent | Description |
|-------|--------|--------|-------------|
| `p225` | Male | British | Neural GLaDOS-style voice ⭐ |
| `p226` | Male | British | Alternative British neural voice |
| `p227` | Male | British | Another British neural option |
| `p232` | Male | British | Deep British neural voice |

## 🚀 Example Usage

```javascript
// From web client - announce events
async function announceEvent(message) {
  await fetch('http://localhost:8080/speak', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({
      text: message,
      voice: 'p225', // GLaDOS-style British neural voice
      speed: 1.0
    })
  });
}

// Usage examples
announceEvent("Client connected to test chamber");
announceEvent("Warning: High latency detected"); 
announceEvent("Stream quality: Excellent");
```

## 🐳 Docker Deployment

```dockerfile
FROM golang:1.21-alpine
RUN apk add --no-cache flite alsa-utils
WORKDIR /app
COPY . .
RUN go mod tidy && go build -o tts-server
EXPOSE 8080
CMD ["./tts-server"]
```

Perfect for containerized HyprMoon deployments! 🐳

## 🎯 Why This Approach?

- **Separation of concerns**: TTS server runs independently
- **Language flexibility**: Go is perfect for HTTP APIs  
- **Audio compatibility**: Outputs to system audio (works with WebRTC automatically)
- **Easy testing**: Web interface and curl commands
- **Production ready**: Proper error handling, logging, CORS

The robot voice will announce streaming events perfectly! 🤖✨