//go:build cgo && linux

package desktop

import (
	"encoding/json"
	"net/http"
)

// handleRecordingStart starts a new recording session.
// POST /recording/start
// Body: {"title": "optional title"}
func (s *Server) handleRecordingStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Initialize recording manager if needed
	if s.recordingManager == nil {
		nodeID := s.nodeID
		if nodeID == 0 {
			nodeID = s.videoNodeID
		}
		if nodeID == 0 {
			http.Error(w, "no video source available", http.StatusServiceUnavailable)
			return
		}
		s.recordingManager = NewRecordingManager(s.config.SessionID, nodeID)
	}

	// Parse request body
	var req struct {
		Title string `json:"title"`
	}
	if r.Body != nil {
		json.NewDecoder(r.Body).Decode(&req)
	}

	// Start recording
	recording, err := s.recordingManager.StartRecording(req.Title)
	if err != nil {
		s.logger.Error("failed to start recording", "err", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Return recording info
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"recording_id": recording.ID,
		"title":        recording.Title,
		"start_time":   recording.StartTime,
	})
}

// handleRecordingStop stops the active recording and returns the result.
// POST /recording/stop
func (s *Server) handleRecordingStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.recordingManager == nil {
		http.Error(w, "no recording manager initialized", http.StatusBadRequest)
		return
	}

	result, err := s.recordingManager.StopRecording()
	if err != nil {
		s.logger.Error("failed to stop recording", "err", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// handleRecordingSubtitle adds a single subtitle to the active recording.
// POST /recording/subtitle
// Body: {"text": "subtitle text", "start_ms": 1000, "end_ms": 3000}
func (s *Server) handleRecordingSubtitle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.recordingManager == nil {
		http.Error(w, "no recording manager initialized", http.StatusBadRequest)
		return
	}

	var req struct {
		Text    string `json:"text"`
		StartMs int64  `json:"start_ms"`
		EndMs   int64  `json:"end_ms"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.Text == "" {
		http.Error(w, "text is required", http.StatusBadRequest)
		return
	}

	if err := s.recordingManager.AddSubtitle(req.Text, req.StartMs, req.EndMs); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// handleRecordingSubtitles sets the complete subtitle track.
// POST /recording/subtitles
// Body: {"subtitles": [{"text": "...", "start_ms": 0, "end_ms": 1000}, ...]}
func (s *Server) handleRecordingSubtitles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.recordingManager == nil {
		http.Error(w, "no recording manager initialized", http.StatusBadRequest)
		return
	}

	var req struct {
		Subtitles []Subtitle `json:"subtitles"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.recordingManager.SetSubtitles(req.Subtitles); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
		"count":  len(req.Subtitles),
	})
}

// handleRecordingStatus returns the current recording status.
// GET /recording/status
func (s *Server) handleRecordingStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var status map[string]interface{}
	if s.recordingManager == nil {
		status = map[string]interface{}{
			"recording": false,
		}
	} else {
		status = s.recordingManager.GetStatus()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}
