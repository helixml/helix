package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/gorilla/mux"
)

type TTSRequest struct {
	Text     string  `json:"text"`
	Voice    string  `json:"voice,omitempty"`
	Speed    float64 `json:"speed,omitempty"`
	Volume   float64 `json:"volume,omitempty"`
	Priority int     `json:"priority,omitempty"` // Higher = more important
}

type QueuedSpeech struct {
	Request   TTSRequest
	Timestamp time.Time
	ID        string
}

type TTSResponse struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type VoiceInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Language    string `json:"language"`
	Gender      string `json:"gender"`
	Description string `json:"description"`
}

type ServerConfig struct {
	Port         int     `json:"port"`
	DefaultVoice string  `json:"default_voice"`
	DefaultSpeed float64 `json:"default_speed"`
	MaxTextLen   int     `json:"max_text_length"`
	EnableCORS   bool    `json:"enable_cors"`
	MaxQueueSize int     `json:"max_queue_size"`
	QueueTimeout int     `json:"queue_timeout_seconds"`
}

var config = ServerConfig{
	Port:         8080,
	DefaultVoice: "p225", // British male neural voice (GLaDOS-like)
	DefaultSpeed: 1.0,    // Neural TTS doesn't need speed adjustment
	MaxTextLen:   1000,
	EnableCORS:   true,
	MaxQueueSize: 10,     // Max 10 announcements in queue
	QueueTimeout: 30,     // Drop announcements older than 30 seconds
}

// Global queue management
var (
	speechQueue     []QueuedSpeech
	queueMutex      sync.Mutex
	isProcessing    bool
	processingMutex sync.Mutex
	requestCounter  int
)

func main() {
	log.Println("🤖 Starting HyprMoon Neural TTS Server with Smart Queue")
	log.Printf("   Voice: %s, Speed: %.1f, Queue Size: %d", config.DefaultVoice, config.DefaultSpeed, config.MaxQueueSize)
	
	// Check if Coqui TTS is available
	if !isCoquiTTSAvailable() {
		log.Fatal("❌ Coqui TTS engine not found. Install with: pip install TTS")
	}
	
	// Start queue processor
	go startQueueProcessor()
	
	router := mux.NewRouter()
	
	// CORS middleware
	if config.EnableCORS {
		router.Use(corsMiddleware)
	}
	
	// TTS endpoints
	router.HandleFunc("/speak", handleSpeak).Methods("POST", "OPTIONS")
	router.HandleFunc("/voices", handleVoices).Methods("GET", "OPTIONS")
	router.HandleFunc("/status", handleStatus).Methods("GET", "OPTIONS")
	router.HandleFunc("/queue", handleQueue).Methods("GET", "OPTIONS")
	router.HandleFunc("/queue/clear", handleClearQueue).Methods("POST", "OPTIONS")
	router.HandleFunc("/health", handleHealth).Methods("GET")
	
	// Static endpoint for testing
	router.HandleFunc("/", handleRoot).Methods("GET")
	
	log.Printf("🚀 Neural TTS Server listening on port %d", config.Port)
	log.Printf("📡 Test with: curl -X POST http://localhost:%d/speak -H 'Content-Type: application/json' -d '{\"text\":\"Hades has connected to the test chamber\"}'", config.Port)
	
	if err := http.ListenAndServe(fmt.Sprintf(":%d", config.Port), router); err != nil {
		log.Fatal("❌ Failed to start server:", err)
	}
}

func handleSpeak(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		return // CORS preflight handled by middleware
	}
	
	var req TTSRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid JSON request")
		return
	}
	
	// Validate and set defaults
	if req.Text == "" {
		respondError(w, http.StatusBadRequest, "Text field is required")
		return
	}
	
	if len(req.Text) > config.MaxTextLen {
		respondError(w, http.StatusBadRequest, fmt.Sprintf("Text too long (max %d characters)", config.MaxTextLen))
		return
	}
	
	if req.Voice == "" {
		req.Voice = config.DefaultVoice
	}
	
	if req.Speed == 0 {
		req.Speed = config.DefaultSpeed
	}
	
	if req.Volume == 0 {
		req.Volume = 1.0
	}
	
	if req.Priority == 0 {
		req.Priority = 5 // Default priority
	}
	
	// Add to queue instead of immediate processing
	speechID := fmt.Sprintf("tts_%d_%d", time.Now().Unix(), requestCounter)
	requestCounter++
	
	queuedSpeech := QueuedSpeech{
		Request:   req,
		Timestamp: time.Now(),
		ID:        speechID,
	}
	
	queueMutex.Lock()
	
	// Check queue size and drop old items if needed
	if len(speechQueue) >= config.MaxQueueSize {
		dropped := cleanupQueue()
		if dropped > 0 {
			log.Printf("⚠️ Queue full! Dropped %d old announcements", dropped)
			// Announce that we're dropping items (high priority)
			dropAnnouncement := QueuedSpeech{
				Request: TTSRequest{
					Text:     fmt.Sprintf("Queue overload. Dropped %d announcements.", dropped),
					Voice:    config.DefaultVoice,
					Speed:    config.DefaultSpeed,
					Volume:   1.0,
					Priority: 10, // High priority
				},
				Timestamp: time.Now(),
				ID:        fmt.Sprintf("drop_announce_%d", time.Now().Unix()),
			}
			speechQueue = append(speechQueue, dropAnnouncement)
		}
	}
	
	// Add new speech to queue
	speechQueue = append(speechQueue, queuedSpeech)
	
	queueMutex.Unlock()
	
	log.Printf("🎤 Queued TTS: '%s' (voice: %s, queue size: %d)", req.Text, req.Voice, len(speechQueue))
	
	response := TTSResponse{
		Status:  "success",
		Message: fmt.Sprintf("Speech queued (ID: %s, position: %d)", speechID, len(speechQueue)),
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func handleVoices(w http.ResponseWriter, r *http.Request) {
	voices := []VoiceInfo{
		{
			ID:          "p225",
			Name:        "British Male Neural",
			Language:    "en-GB",
			Gender:      "male",
			Description: "High-quality British male neural voice, perfect for GLaDOS-style announcements",
		},
		{
			ID:          "p226",
			Name:        "British Male Neural 2",
			Language:    "en-GB", 
			Gender:      "male",
			Description: "Alternative British male neural voice",
		},
		{
			ID:          "p227",
			Name:        "British Male Neural 3",
			Language:    "en-GB",
			Gender:      "male", 
			Description: "Another British male neural voice option",
		},
		{
			ID:          "p232",
			Name:        "British Male Neural 4",
			Language:    "en-GB",
			Gender:      "male",
			Description: "Deep British male neural voice",
		},
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(voices)
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	queueMutex.Lock()
	queueSize := len(speechQueue)
	queueMutex.Unlock()
	
	processingMutex.Lock()
	processing := isProcessing
	processingMutex.Unlock()
	
	status := map[string]interface{}{
		"status":        "running",
		"engine":        "coqui-tts",
		"model":         "tts_models/en/vctk/vits",
		"default_voice": config.DefaultVoice,
		"default_speed": config.DefaultSpeed,
		"max_text_len":  config.MaxTextLen,
		"queue_size":    queueSize,
		"max_queue":     config.MaxQueueSize,
		"processing":    processing,
		"uptime":        time.Since(startTime).String(),
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	if isCoquiTTSAvailable() {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "OK")
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, "Coqui TTS not available")
	}
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	html := `<!DOCTYPE html>
<html>
<head>
    <title>🤖 HyprMoon TTS Server</title>
    <style>
        body { font-family: monospace; background: #1a1a1a; color: #00ff00; padding: 20px; }
        .container { max-width: 600px; margin: 0 auto; }
        input, textarea, button { background: #333; color: #00ff00; border: 1px solid #555; padding: 10px; margin: 5px; }
        button { cursor: pointer; }
        button:hover { background: #555; }
        .response { background: #222; padding: 10px; margin: 10px 0; border-left: 3px solid #00ff00; }
    </style>
</head>
<body>
    <div class="container">
        <h1>🤖 HyprMoon Neural TTS Server</h1>
        <p>Test the high-quality neural robot voice API:</p>
        
        <div>
            <textarea id="text" placeholder="Enter text to speak..." rows="3" cols="60">Hades has connected to the test chamber</textarea><br>
            <select id="voice">
                <option value="p225">British Male Neural (GLaDOS-style) [Default]</option>
                <option value="p226">British Male Neural 2</option>
                <option value="p227">British Male Neural 3</option>
                <option value="p232">British Male Neural 4 (Deep)</option>
            </select>
            <input type="number" id="speed" value="1.0" min="0.1" max="2.0" step="0.1" placeholder="Speed">
            <input type="number" id="priority" value="5" min="1" max="10" step="1" placeholder="Priority (1-10)">
            <button onclick="speak()">🎤 Queue Speech</button>
            <button onclick="getQueue()">📋 View Queue</button>
            <button onclick="clearQueue()">🧹 Clear Queue</button>
        </div>
        
        <div id="response" class="response" style="display:none;"></div>
        
        <h3>API Examples:</h3>
        <pre>
# Basic speech (queued)
curl -X POST http://localhost:8080/speak \
  -H "Content-Type: application/json" \
  -d '{"text": "WebRTC client connected"}'

# High priority announcement  
curl -X POST http://localhost:8080/speak \
  -H "Content-Type: application/json" \
  -d '{"text": "Critical system alert", "voice": "p232", "priority": 9}'

# Get available voices
curl http://localhost:8080/voices

# Check queue status
curl http://localhost:8080/queue

# Clear queue
curl -X POST http://localhost:8080/queue/clear

# Server status
curl http://localhost:8080/status
        </pre>
    </div>
    
    <script>
        function speak() {
            const text = document.getElementById('text').value;
            const voice = document.getElementById('voice').value;
            const speed = parseFloat(document.getElementById('speed').value);
            const priority = parseInt(document.getElementById('priority').value);
            
            fetch('/speak', {
                method: 'POST',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({text, voice, speed, priority})
            })
            .then(response => response.json())
            .then(data => {
                showResponse('Speak Response', data);
            })
            .catch(error => {
                showResponse('Error', error);
            });
        }
        
        function getQueue() {
            fetch('/queue')
            .then(response => response.json())
            .then(data => {
                showResponse('Queue Status', data);
            })
            .catch(error => {
                showResponse('Error', error);
            });
        }
        
        function clearQueue() {
            fetch('/queue/clear', {method: 'POST'})
            .then(response => response.json())
            .then(data => {
                showResponse('Clear Queue', data);
            })
            .catch(error => {
                showResponse('Error', error);
            });
        }
        
        function showResponse(title, data) {
            const responseDiv = document.getElementById('response');
            responseDiv.style.display = 'block';
            responseDiv.innerHTML = '<strong>' + title + ':</strong><br><pre>' + JSON.stringify(data, null, 2) + '</pre>';
        }
        
        // Auto-refresh status every 2 seconds
        setInterval(() => {
            fetch('/status')
            .then(response => response.json())
            .then(data => {
                document.title = 'TTS Server - Queue: ' + data.queue_size + (data.processing ? ' (Processing...)' : '');
            })
            .catch(() => {});
        }, 2000);
    </script>
</body>
</html>`
	
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, html)
}

func synthesizeSpeech(text, voice string, speed, volume float64) error {
	// Create temporary file for audio
	tempFile := filepath.Join(os.TempDir(), fmt.Sprintf("hyprmoon_neural_tts_%d.wav", time.Now().UnixNano()))
	defer os.Remove(tempFile)
	
	// Build Coqui TTS command with neural voice
	args := []string{
		"--text", text,
		"--model_name", "tts_models/en/vctk/vits",
		"--speaker_idx", voice, // voice is now speaker ID like "p225"
		"--out_path", tempFile,
	}
	
	// Execute Coqui TTS synthesis
	cmd := exec.Command("tts", args...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("coqui TTS synthesis failed: %v", err)
	}
	
	// Play the audio (this will output to default audio device, which should route to WebRTC)
	playCmd := exec.Command("aplay", tempFile)
	if err := playCmd.Run(); err != nil {
		// Try alternative players
		playCmd = exec.Command("paplay", tempFile)
		if err := playCmd.Run(); err != nil {
			return fmt.Errorf("audio playback failed: %v", err)
		}
	}
	
	return nil
}

func isCoquiTTSAvailable() bool {
	_, err := exec.LookPath("tts")
	return err == nil
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		
		next.ServeHTTP(w, r)
	})
}

func respondError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(TTSResponse{
		Status:  "error",
		Message: message,
	})
}

var startTime = time.Now()

// Queue management functions
func startQueueProcessor() {
	log.Println("🔄 Starting TTS queue processor")
	
	for {
		queueMutex.Lock()
		if len(speechQueue) == 0 {
			queueMutex.Unlock()
			time.Sleep(100 * time.Millisecond)
			continue
		}
		
		// Get next speech item (priority-based)
		nextIndex := getNextQueueIndex()
		if nextIndex == -1 {
			queueMutex.Unlock()
			time.Sleep(100 * time.Millisecond)
			continue
		}
		
		nextSpeech := speechQueue[nextIndex]
		// Remove from queue
		speechQueue = append(speechQueue[:nextIndex], speechQueue[nextIndex+1:]...)
		queueMutex.Unlock()
		
		// Mark as processing
		processingMutex.Lock()
		isProcessing = true
		processingMutex.Unlock()
		
		log.Printf("🎤 Processing TTS: '%s' (voice: %s)", nextSpeech.Request.Text, nextSpeech.Request.Voice)
		
		// Process the speech
		err := synthesizeSpeech(
			nextSpeech.Request.Text,
			nextSpeech.Request.Voice,
			nextSpeech.Request.Speed,
			nextSpeech.Request.Volume,
		)
		
		if err != nil {
			log.Printf("❌ TTS synthesis failed: %v", err)
		}
		
		// Mark as not processing
		processingMutex.Lock()
		isProcessing = false
		processingMutex.Unlock()
		
		// Small delay between announcements
		time.Sleep(200 * time.Millisecond)
	}
}

func getNextQueueIndex() int {
	if len(speechQueue) == 0 {
		return -1
	}
	
	// Find highest priority item
	maxPriority := speechQueue[0].Request.Priority
	maxIndex := 0
	
	for i, speech := range speechQueue {
		if speech.Request.Priority > maxPriority {
			maxPriority = speech.Request.Priority
			maxIndex = i
		}
	}
	
	return maxIndex
}

func cleanupQueue() int {
	now := time.Now()
	dropped := 0
	newQueue := []QueuedSpeech{}
	
	for _, speech := range speechQueue {
		age := now.Sub(speech.Timestamp).Seconds()
		if age > float64(config.QueueTimeout) || speech.Request.Priority < 5 {
			dropped++
			log.Printf("🗑️ Dropping old/low-priority announcement: '%s' (age: %.1fs, priority: %d)", 
				speech.Request.Text, age, speech.Request.Priority)
		} else {
			newQueue = append(newQueue, speech)
		}
	}
	
	speechQueue = newQueue
	return dropped
}

func handleQueue(w http.ResponseWriter, r *http.Request) {
	queueMutex.Lock()
	queueCopy := make([]QueuedSpeech, len(speechQueue))
	copy(queueCopy, speechQueue)
	queueMutex.Unlock()
	
	response := map[string]interface{}{
		"queue_size": len(queueCopy),
		"max_size":   config.MaxQueueSize,
		"items":      queueCopy,
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func handleClearQueue(w http.ResponseWriter, r *http.Request) {
	queueMutex.Lock()
	cleared := len(speechQueue)
	speechQueue = []QueuedSpeech{}
	queueMutex.Unlock()
	
	log.Printf("🧹 Queue cleared: %d items removed", cleared)
	
	response := TTSResponse{
		Status:  "success",
		Message: fmt.Sprintf("Cleared %d items from queue", cleared),
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}