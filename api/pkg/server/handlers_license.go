package server

import (
	"encoding/json"
	"net/http"

	"github.com/rs/zerolog/log"
)

type LicenseKeyRequest struct {
	LicenseKey string `json:"license_key"`
}

func (s *HelixAPIServer) handleGetLicenseKey(w http.ResponseWriter, r *http.Request) {
	license, err := s.Store.GetLicenseKey(r.Context())
	if err != nil {
		log.Error().Err(err).Msg("failed to get license key")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"license": license,
	}); err != nil {
		log.Error().Err(err).Msg("failed to encode response")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

func (s *HelixAPIServer) handleSetLicenseKey(w http.ResponseWriter, r *http.Request) {
	var req LicenseKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := s.Store.SetLicenseKey(r.Context(), req.LicenseKey); err != nil {
		log.Error().Err(err).Msg("failed to set license key")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Trigger an immediate ping to update deployment ID
	if s.pingService != nil {
		go s.pingService.SendPing(r.Context())
	}

	w.WriteHeader(http.StatusOK)
}
