//go:build !cgo || !linux

package desktop

import (
	"encoding/json"
	"net/http"
)

// handleRecordingStart is a stub for non-CGO builds.
func (s *Server) handleRecordingStart(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusServiceUnavailable)
	json.NewEncoder(w).Encode(map[string]string{
		"error": "recording not supported without CGO",
	})
}

// handleRecordingStop is a stub for non-CGO builds.
func (s *Server) handleRecordingStop(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusServiceUnavailable)
	json.NewEncoder(w).Encode(map[string]string{
		"error": "recording not supported without CGO",
	})
}

// handleRecordingSubtitle is a stub for non-CGO builds.
func (s *Server) handleRecordingSubtitle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusServiceUnavailable)
	json.NewEncoder(w).Encode(map[string]string{
		"error": "recording not supported without CGO",
	})
}

// handleRecordingSubtitles is a stub for non-CGO builds.
func (s *Server) handleRecordingSubtitles(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusServiceUnavailable)
	json.NewEncoder(w).Encode(map[string]string{
		"error": "recording not supported without CGO",
	})
}

// handleRecordingStatus is a stub for non-CGO builds.
func (s *Server) handleRecordingStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"recording": false,
		"error":     "recording not supported without CGO",
	})
}
