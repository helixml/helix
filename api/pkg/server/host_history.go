package server

import (
	"encoding/base64"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/helixml/helix/api/pkg/hosthistory"
	"github.com/rs/zerolog/log"
)

func (s *HelixAPIServer) hostHistoryDir() string {
	node := base64.RawURLEncoding.EncodeToString([]byte(s.Cfg.HostHistoryNode))
	return filepath.Join(s.Cfg.FileStore.LocalFSPath, ".host-history", node)
}

// getHostHistory godoc
// @Summary Get API host CPU, memory and data filesystem utilization
// @Tags system
// @Param days query int false "Days of history (1 through 30)"
// @Success 200 {array} hosthistory.Sample
// @Router /api/v1/system/host-history [get]
// @Security BearerAuth
func (s *HelixAPIServer) getHostHistory(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(getRequestUser(r)) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	days := 1
	if v := r.URL.Query().Get("days"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > hosthistory.Days {
			http.Error(w, "days must be between 1 and 30", http.StatusBadRequest)
			return
		}
		days = n
	}
	now := time.Now()
	samples, err := hosthistory.Read(s.hostHistoryDir(), now.AddDate(0, 0, -days), now)
	if err != nil {
		log.Error().Err(err).Msg("reading host resource history")
		http.Error(w, "Cannot read host history", 500)
		return
	}
	writeResponse(w, samples, 200)
}
