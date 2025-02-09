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
	if !s.isAdmin(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	license, err := s.Store.GetLicenseKey(r.Context())
	if err != nil {
		log.Error().Err(err).Msg("failed to get license key")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"license_key": license,
	})
}

func (s *HelixAPIServer) handleSetLicenseKey(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

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

	w.WriteHeader(http.StatusOK)
}
